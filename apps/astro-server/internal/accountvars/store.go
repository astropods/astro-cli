package accountvars

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// EncryptionKey holds the per-account KMS-wrapped data key used to
// encrypt/decrypt secret account variables with per-value nonces.
type EncryptionKey struct {
	AccountID        string
	EncryptedDataKey []byte
	KMSKeyARN        string
	CreatedAt        time.Time
}

// AccountVariable is a single variable belonging to an account.
// If Secret is true, Value is base64-encoded ciphertext and Nonce is set.
type AccountVariable struct {
	AccountID   string
	Name        string
	Value       string // base64-encoded ciphertext when secret; plaintext when not
	Secret      bool
	Nonce       []byte // 12-byte AES-GCM nonce (only set for secrets)
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// VariableMetadata is the non-sensitive view returned by List.
// Value is populated for non-secret variables; it is omitted for secrets.
type VariableMetadata struct {
	Name        string    `json:"name"`
	Secret      bool      `json:"secret"`
	Description string    `json:"description"`
	Value       *string   `json:"value,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Store manages account_encryption_keys and account_variables tables.
type Store struct {
	db *sql.DB
}

// NewStore creates a new account variables store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// GetEncryptionKey returns the account's data key, or nil if none exists.
func (s *Store) GetEncryptionKey(accountID string) (*EncryptionKey, error) {
	row := s.db.QueryRow(`
		SELECT account_id, encrypted_data_key, kms_key_arn, created_at
		FROM account_encryption_keys
		WHERE account_id = $1
	`, accountID)

	var ek EncryptionKey
	err := row.Scan(&ek.AccountID, &ek.EncryptedDataKey, &ek.KMSKeyARN, &ek.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("accountvars get encryption key: %w", err)
	}
	return &ek, nil
}

// SaveEncryptionKey inserts or updates the account's encryption key.
func (s *Store) SaveEncryptionKey(accountID string, encryptedDataKey []byte, kmsKeyARN string) error {
	_, err := s.db.Exec(`
		INSERT INTO account_encryption_keys (account_id, encrypted_data_key, kms_key_arn)
		VALUES ($1, $2, $3)
		ON CONFLICT (account_id) DO UPDATE SET
			encrypted_data_key = EXCLUDED.encrypted_data_key,
			kms_key_arn = EXCLUDED.kms_key_arn
	`, accountID, encryptedDataKey, kmsKeyARN)
	if err != nil {
		return fmt.Errorf("accountvars save encryption key: %w", err)
	}
	return nil
}

// List returns metadata for all variables in an account.
// Plaintext values are included for non-secret variables; secrets omit the value.
func (s *Store) List(accountID string) ([]VariableMetadata, error) {
	rows, err := s.db.Query(`
		SELECT name, secret, description, value, created_at, updated_at
		FROM account_variables
		WHERE account_id = $1
		ORDER BY name
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("accountvars list: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	var vars []VariableMetadata
	for rows.Next() {
		var m VariableMetadata
		var rawValue string
		if err := rows.Scan(&m.Name, &m.Secret, &m.Description, &rawValue, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("accountvars list scan: %w", err)
		}
		if !m.Secret {
			m.Value = &rawValue
		}
		vars = append(vars, m)
	}
	return vars, rows.Err()
}

// Get returns a single variable, or nil if not found.
func (s *Store) Get(accountID, name string) (*AccountVariable, error) {
	row := s.db.QueryRow(`
		SELECT account_id, name, value, secret, nonce, description, created_at, updated_at
		FROM account_variables
		WHERE account_id = $1 AND name = $2
	`, accountID, name)

	var v AccountVariable
	err := row.Scan(&v.AccountID, &v.Name, &v.Value, &v.Secret, &v.Nonce,
		&v.Description, &v.CreatedAt, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("accountvars get: %w", err)
	}
	return &v, nil
}

// GetByNames fetches multiple variables by name for a given account (batch fetch for deploy).
func (s *Store) GetByNames(accountID string, names []string) ([]AccountVariable, error) {
	if len(names) == 0 {
		return nil, nil
	}

	query := `
		SELECT account_id, name, value, secret, nonce, description, created_at, updated_at
		FROM account_variables
		WHERE account_id = $1 AND name = ANY($2)
	`
	rows, err := s.db.Query(query, accountID, pq.Array(names))
	if err != nil {
		return nil, fmt.Errorf("accountvars get by names: %w", err)
	}
	defer rows.Close() //nolint:errcheck // best-effort cleanup

	var vars []AccountVariable
	for rows.Next() {
		var v AccountVariable
		if err := rows.Scan(&v.AccountID, &v.Name, &v.Value, &v.Secret, &v.Nonce,
			&v.Description, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("accountvars get by names scan: %w", err)
		}
		vars = append(vars, v)
	}
	return vars, rows.Err()
}

// Save inserts or updates a variable (upsert).
func (s *Store) Save(v *AccountVariable) error {
	_, err := s.db.Exec(`
		INSERT INTO account_variables (account_id, name, value, secret, nonce, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (account_id, name) DO UPDATE SET
			value = EXCLUDED.value,
			secret = EXCLUDED.secret,
			nonce = EXCLUDED.nonce,
			description = EXCLUDED.description,
			updated_at = now()
	`, v.AccountID, v.Name, v.Value, v.Secret, v.Nonce, v.Description)
	if err != nil {
		return fmt.Errorf("accountvars save: %w", err)
	}
	return nil
}

// Delete removes a variable.
func (s *Store) Delete(accountID, name string) error {
	result, err := s.db.Exec(`
		DELETE FROM account_variables
		WHERE account_id = $1 AND name = $2
	`, accountID, name)
	if err != nil {
		return fmt.Errorf("accountvars delete: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("variable %q not found", name)
	}
	return nil
}
