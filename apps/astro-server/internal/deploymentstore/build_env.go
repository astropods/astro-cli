package deploymentstore

import (
	"database/sql"
	"fmt"
)

// BuildEnvRow is one row in deployment_build_env. Value is ciphertext;
// the caller decrypts using the deployment's data key (envelope.Decryptor).
type BuildEnvRow struct {
	DeploymentID   string
	Role           string
	EnvName        string
	ValueEncrypted []byte
	Nonce          []byte
	IsSecret       bool
	Source         string

	// Provenance for source='user_var' rows. Empty / false on
	// platform-emitted rows.
	UserVarName   string
	AccountVarRef string
	Optional      bool
}

// BuildEnvEncryptor is the subset of envelope.Encryptor used by this Store.
type BuildEnvEncryptor interface {
	Encrypt(plaintext []byte) (ciphertext, nonce []byte, err error)
}

// BuildEnvWrite is the input shape for SaveBuildEnv: plaintext value,
// the Store handles encryption.
type BuildEnvWrite struct {
	Role          string
	EnvName       string
	Value         string
	IsSecret      bool
	Source        string
	UserVarName   string
	AccountVarRef string
	Optional      bool
}

// SaveBuildEnv replaces all rows for a deployment with the given set, in
// one transaction. Existing rows are deleted first; passing an empty
// slice clears all rows for the deployment.
//
// Plaintext values in writes are encrypted using enc.
func (s *Store) SaveBuildEnv(deploymentID string, writes []BuildEnvWrite, enc BuildEnvEncryptor) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`DELETE FROM deployment_build_env WHERE deployment_id = $1`,
		deploymentID,
	); err != nil {
		return fmt.Errorf("delete existing: %w", err)
	}

	for _, w := range writes {
		ct, nonce, err := enc.Encrypt([]byte(w.Value))
		if err != nil {
			return fmt.Errorf("encrypt %s/%s: %w", w.Role, w.EnvName, err)
		}
		var userVarName, accountVarRef sql.NullString
		var optional sql.NullBool
		if w.Source == "user_var" {
			userVarName = sql.NullString{String: w.UserVarName, Valid: w.UserVarName != ""}
			accountVarRef = sql.NullString{String: w.AccountVarRef, Valid: w.AccountVarRef != ""}
			optional = sql.NullBool{Bool: w.Optional, Valid: true}
		}
		if _, err := tx.Exec(`
			INSERT INTO deployment_build_env
				(deployment_id, role, env_name, value_encrypted, nonce,
				 is_secret, source, user_var_name, account_var_ref, optional)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		`, deploymentID, w.Role, w.EnvName, ct, nonce,
			w.IsSecret, w.Source, userVarName, accountVarRef, optional); err != nil {
			return fmt.Errorf("insert %s/%s: %w", w.Role, w.EnvName, err)
		}
	}

	return tx.Commit()
}

// GetBuildEnv returns all rows for a deployment, ordered by role+env_name.
// Values are returned as ciphertext+nonce; the caller decrypts.
func (s *Store) GetBuildEnv(deploymentID string) ([]BuildEnvRow, error) {
	rows, err := s.db.Query(`
		SELECT deployment_id, role, env_name, value_encrypted, nonce,
		       is_secret, source, user_var_name, account_var_ref, optional
		FROM deployment_build_env
		WHERE deployment_id = $1
		ORDER BY role, env_name
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var result []BuildEnvRow
	for rows.Next() {
		var r BuildEnvRow
		var userVarName, accountVarRef sql.NullString
		var optional sql.NullBool
		if err := rows.Scan(&r.DeploymentID, &r.Role, &r.EnvName,
			&r.ValueEncrypted, &r.Nonce,
			&r.IsSecret, &r.Source,
			&userVarName, &accountVarRef, &optional); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		r.UserVarName = userVarName.String
		r.AccountVarRef = accountVarRef.String
		r.Optional = optional.Bool
		result = append(result, r)
	}
	return result, rows.Err()
}

// HasBuildEnv reports whether any rows exist for the deployment.
// Used by the applier to detect first-apply-after-cutover and trigger
// the lazy resolve.
func (s *Store) HasBuildEnv(deploymentID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM deployment_build_env WHERE deployment_id = $1)`,
		deploymentID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists: %w", err)
	}
	return exists, nil
}
