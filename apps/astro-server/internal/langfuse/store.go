package langfuse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/envelope"
)

// ErrCredentialsDecrypt marks stored Langfuse credentials that cannot be
// decrypted with the worker's current KMS configuration.
var ErrCredentialsDecrypt = errors.New("decrypt Langfuse credentials")

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
	return s.get(context.Background(), accountID)
}

func (s *Store) get(ctx context.Context, accountID string) (*AccountLangfuse, error) {
	row := s.db.QueryRowContext(ctx, `
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("langfuse store get: %w", err)
	}
	return &al, nil
}

// GetDecrypted returns account credentials with a plaintext secret key. It
// does not provision a missing Langfuse project.
func (s *Store) GetDecrypted(ctx context.Context, kmsClient envelope.KMSClient, accountID string) (*AccountLangfuse, error) {
	credentials, err := s.get(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("langfuse store get decrypted: %w", err)
	}
	if credentials == nil {
		return nil, nil
	}

	secretKey, err := decryptSecretKey(ctx, kmsClient, credentials)
	if err != nil {
		return nil, errors.Join(ErrCredentialsDecrypt, fmt.Errorf("langfuse store get decrypted: %w", err))
	}
	credentials.SecretKey = secretKey
	return credentials, nil
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

// ListAccountIDs returns every live account that has Langfuse provisioned. Used
// by the Insights refresh worker as the canonical "accounts to refresh"
// set: enumerating through this table (vs through active deployments)
// means an account that *used to* have deployments still gets its cache
// surfaced/cleared once the inner compute returns a zero response.
//
// Soft-deleted accounts are excluded. Their credential row outlives the delete
// (only a hard delete cascades), so without the join every tick fans out work
// for accounts every downstream lookup then refuses to load.
func (s *Store) ListAccountIDs() ([]string, error) {
	rows, err := s.db.Query(`
		SELECT al.account_id
		FROM account_langfuse al
		JOIN accounts a ON a.id = al.account_id AND a.deleted_at IS NULL
		ORDER BY al.account_id`)
	if err != nil {
		return nil, fmt.Errorf("langfuse store list account ids: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]string, 0, 16)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("langfuse store list scan: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("langfuse store list rows: %w", err)
	}
	return out, nil
}
