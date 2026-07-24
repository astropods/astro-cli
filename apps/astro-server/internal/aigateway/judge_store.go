package aigateway

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// JudgeKey is the long-lived account-scoped Bifrost key used by Astro's
// internal eval-dataset judge.
type JudgeKey struct {
	AccountID        string
	KeyID            string
	EncryptedAPIKey  string
	EncryptedDataKey []byte
	Nonce            []byte
	IssuedAt         time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// JudgeStore manages account_llm_judge_keys.
type JudgeStore struct {
	db *sql.DB
}

func NewJudgeStore(db *sql.DB) *JudgeStore {
	return &JudgeStore{db: db}
}

// Get returns the judge key for an account, or nil when none exists.
func (s *JudgeStore) Get(ctx context.Context, accountID string) (*JudgeKey, error) {
	var key JudgeKey
	err := s.db.QueryRowContext(ctx, `
		SELECT account_id, key_id, encrypted_api_key, encrypted_data_key, nonce,
		       issued_at, created_at, updated_at
		FROM account_llm_judge_keys
		WHERE account_id = $1
	`, accountID).Scan(
		&key.AccountID,
		&key.KeyID,
		&key.EncryptedAPIKey,
		&key.EncryptedDataKey,
		&key.Nonce,
		&key.IssuedAt,
		&key.CreatedAt,
		&key.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ai gateway judge store get: %w", err)
	}
	return &key, nil
}

// Save inserts a newly minted judge key. There is intentionally no upsert:
// callers reuse the one long-lived row instead of rotating it implicitly.
func (s *JudgeStore) Save(ctx context.Context, key *JudgeKey) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO account_llm_judge_keys
		  (account_id, key_id, encrypted_api_key, encrypted_data_key, nonce, issued_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, key.AccountID, key.KeyID, key.EncryptedAPIKey, key.EncryptedDataKey, key.Nonce, key.IssuedAt)
	if err != nil {
		return fmt.Errorf("ai gateway judge store save: %w", err)
	}
	return nil
}

// Delete removes the account's judge-key row after upstream revocation.
func (s *JudgeStore) Delete(ctx context.Context, accountID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM account_llm_judge_keys WHERE account_id = $1`, accountID); err != nil {
		return fmt.Errorf("ai gateway judge store delete: %w", err)
	}
	return nil
}

// ListKeyIDsByAccount returns the upstream key IDs owned by the account. The
// schema permits one row, but a slice keeps the purge sweep consistent with
// the other account-scoped key stores.
func (s *JudgeStore) ListKeyIDsByAccount(ctx context.Context, accountID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT key_id FROM account_llm_judge_keys
		WHERE account_id = $1
		ORDER BY key_id
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("ai gateway judge store list by account: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []string
	for rows.Next() {
		var keyID string
		if err := rows.Scan(&keyID); err != nil {
			return nil, fmt.Errorf("ai gateway judge store list scan: %w", err)
		}
		out = append(out, keyID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai gateway judge store list iter: %w", err)
	}
	return out, nil
}
