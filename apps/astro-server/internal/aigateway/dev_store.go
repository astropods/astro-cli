package aigateway

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DevKeySafetyMargin is how much remaining lifetime a dev key needs to be
// reused rather than replaced. A key with less than this much remaining is
// treated as effectively expired — keeps an `astro dev` session from
// inheriting a key that expires 5 minutes in.
const DevKeySafetyMargin = 30 * time.Minute

// DevKey is the per-(account, user) ephemeral key minted for local
// `astro dev` sessions. Lifecycle is independent from AccountAIGateway —
// dev sessions can run before an account has ever deployed.
type DevKey struct {
	AccountID        string
	UserID           string
	KeyID            string
	EncryptedAPIKey  string
	EncryptedDataKey []byte
	Nonce            []byte
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// IsUsable reports whether the key has enough remaining lifetime to be
// worth handing back to the CLI vs. minting fresh.
func (k *DevKey) IsUsable() bool {
	return k != nil && time.Until(k.ExpiresAt) > DevKeySafetyMargin
}

// DevStore manages the account_ai_gateway_dev_keys table.
type DevStore struct {
	db *sql.DB
}

// NewDevStore creates a new dev-key store.
func NewDevStore(db *sql.DB) *DevStore {
	return &DevStore{db: db}
}

// Get returns the dev key for (accountID, userID), or nil if none exists.
// Existence does NOT imply usability — call DevKey.IsUsable to gate on
// remaining lifetime.
func (s *DevStore) Get(accountID, userID string) (*DevKey, error) {
	row := s.db.QueryRow(`
		SELECT account_id, user_id, key_id, encrypted_api_key,
		       encrypted_data_key, nonce, expires_at, created_at, updated_at
		FROM account_ai_gateway_dev_keys
		WHERE account_id = $1 AND user_id = $2
	`, accountID, userID)

	var k DevKey
	if err := row.Scan(
		&k.AccountID, &k.UserID, &k.KeyID, &k.EncryptedAPIKey,
		&k.EncryptedDataKey, &k.Nonce, &k.ExpiresAt, &k.CreatedAt, &k.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("ai gateway dev store get: %w", err)
	}
	return &k, nil
}

// Upsert writes (or replaces) the dev key for (accountID, userID). Returns
// the previous key_id when one was present so the caller can revoke it
// upstream — keeps LiteLLM's DB tidy when a user mints repeatedly.
func (s *DevStore) Upsert(k *DevKey) (previousKeyID string, err error) {
	// The CTE captures the pre-statement key_id; INSERT…ON CONFLICT
	// then overwrites. Without the CTE the RETURNING clause would see
	// the post-update value.
	row := s.db.QueryRow(`
		WITH existing AS (
		  SELECT key_id FROM account_ai_gateway_dev_keys
		  WHERE account_id = $1 AND user_id = $2
		),
		upserted AS (
		  INSERT INTO account_ai_gateway_dev_keys
		    (account_id, user_id, key_id, encrypted_api_key,
		     encrypted_data_key, nonce, expires_at)
		  VALUES ($1, $2, $3, $4, $5, $6, $7)
		  ON CONFLICT (account_id, user_id) DO UPDATE SET
		    key_id             = EXCLUDED.key_id,
		    encrypted_api_key  = EXCLUDED.encrypted_api_key,
		    encrypted_data_key = EXCLUDED.encrypted_data_key,
		    nonce              = EXCLUDED.nonce,
		    expires_at         = EXCLUDED.expires_at,
		    updated_at         = now()
		  RETURNING 1
		)
		SELECT (SELECT key_id FROM existing) FROM upserted
	`, k.AccountID, k.UserID, k.KeyID, k.EncryptedAPIKey,
		k.EncryptedDataKey, k.Nonce, k.ExpiresAt)

	var prev sql.NullString
	if err := row.Scan(&prev); err != nil {
		return "", fmt.Errorf("ai gateway dev store upsert: %w", err)
	}
	if prev.Valid && prev.String != "" && prev.String != k.KeyID {
		return prev.String, nil
	}
	return "", nil
}

// ListKeyIDsByAccount returns the LiteLLM key_id of every dev key row under
// the account. Used by account purge to revoke each key upstream before the
// FK cascade removes the rows (LiteLLM has no FK back to us, so the
// upstream key would otherwise linger until its TTL).
func (s *DevStore) ListKeyIDsByAccount(accountID string) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT key_id FROM account_ai_gateway_dev_keys
		WHERE account_id = $1
		ORDER BY user_id
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("ai gateway dev store list by account: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]string, 0, 4)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("ai gateway dev store list scan: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeleteByAccount removes every dev key row under the account. Called by
// account purge after the upstream revokes; the FK cascade on account
// hard-delete would clear these anyway, but doing it explicitly keeps the
// sweep idempotent on retry.
func (s *DevStore) DeleteByAccount(accountID string) error {
	_, err := s.db.Exec(`DELETE FROM account_ai_gateway_dev_keys WHERE account_id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("ai gateway dev store delete by account: %w", err)
	}
	return nil
}
