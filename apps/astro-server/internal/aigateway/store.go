package aigateway

import (
	"database/sql"
	"fmt"
	"time"
)

// DeploymentAIGateway holds the LiteLLM virtual key state for a single
// Astro deployment. One row per deployment. Plaintext keys never live in
// this table — only KMS-envelope-encrypted ciphertext.
type DeploymentAIGateway struct {
	DeploymentID     string
	AccountID        string
	KeyID            string
	EncryptedAPIKey  string // base64 ciphertext; plaintext after Decrypt
	EncryptedDataKey []byte
	Nonce            []byte
	IssuedAt         time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Store manages the deployment_ai_gateway table.
type Store struct {
	db *sql.DB
}

// NewStore creates a new AI Gateway store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Get returns the AI Gateway state for a deployment, or nil if not provisioned.
func (s *Store) Get(deploymentID string) (*DeploymentAIGateway, error) {
	row := s.db.QueryRow(`
		SELECT deployment_id, account_id, key_id, encrypted_api_key, encrypted_data_key, nonce,
		       issued_at, created_at, updated_at
		FROM deployment_ai_gateway
		WHERE deployment_id = $1
	`, deploymentID)

	var a DeploymentAIGateway
	err := row.Scan(
		&a.DeploymentID, &a.AccountID, &a.KeyID, &a.EncryptedAPIKey, &a.EncryptedDataKey, &a.Nonce,
		&a.IssuedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ai gateway store get: %w", err)
	}
	return &a, nil
}

// Save inserts a fresh deployment_ai_gateway row.
func (s *Store) Save(a *DeploymentAIGateway) error {
	_, err := s.db.Exec(`
		INSERT INTO deployment_ai_gateway
		  (deployment_id, account_id, key_id, encrypted_api_key, encrypted_data_key, nonce, issued_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, a.DeploymentID, a.AccountID, a.KeyID, a.EncryptedAPIKey, a.EncryptedDataKey, a.Nonce, a.IssuedAt)
	if err != nil {
		return fmt.Errorf("ai gateway store save: %w", err)
	}
	return nil
}

// Delete removes the row entirely. Called by undeploy and account purge.
func (s *Store) Delete(deploymentID string) error {
	_, err := s.db.Exec(`DELETE FROM deployment_ai_gateway WHERE deployment_id = $1`, deploymentID)
	if err != nil {
		return fmt.Errorf("ai gateway store delete: %w", err)
	}
	return nil
}

// ListByAccount returns every deployment_id with an AI Gateway row under the
// account. Used by account purge to revoke each deployment's key upstream
// before the account row is hard-deleted.
func (s *Store) ListByAccount(accountID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT deployment_id FROM deployment_ai_gateway
		WHERE account_id = $1
		ORDER BY deployment_id
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("ai gateway store list by account: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]string, 0, 4)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("ai gateway store list scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
