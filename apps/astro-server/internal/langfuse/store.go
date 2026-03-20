package langfuse

import (
	"database/sql"
	"fmt"
	"time"
)

// AccountLangfuse holds the Langfuse project credentials for an Astro account.
type AccountLangfuse struct {
	AccountID         string
	LangfuseProjectID string
	PublicKey         string
	SecretKey         string // plaintext after decryption; ciphertext (base64) when stored
	EncryptedDataKey  []byte
	Nonce             []byte
	CreatedAt         time.Time
}

// Store manages the account_langfuse table in astro-server's database.
type Store struct {
	db *sql.DB
}

// NewStore creates a new Langfuse store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Get returns the Langfuse credentials for an account, or nil if not provisioned.
func (s *Store) Get(accountID string) (*AccountLangfuse, error) {
	row := s.db.QueryRow(`
		SELECT account_id, langfuse_project_id, langfuse_public_key, langfuse_secret_key,
		       encrypted_data_key, nonce, created_at
		FROM account_langfuse
		WHERE account_id = $1
	`, accountID)

	var al AccountLangfuse
	err := row.Scan(
		&al.AccountID, &al.LangfuseProjectID, &al.PublicKey, &al.SecretKey,
		&al.EncryptedDataKey, &al.Nonce, &al.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("langfuse store get: %w", err)
	}
	return &al, nil
}

// Save inserts Langfuse credentials for an account.
func (s *Store) Save(al *AccountLangfuse) error {
	_, err := s.db.Exec(`
		INSERT INTO account_langfuse (account_id, langfuse_project_id, langfuse_public_key, langfuse_secret_key, encrypted_data_key, nonce)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, al.AccountID, al.LangfuseProjectID, al.PublicKey, al.SecretKey, al.EncryptedDataKey, al.Nonce)
	if err != nil {
		return fmt.Errorf("langfuse store save: %w", err)
	}
	return nil
}
