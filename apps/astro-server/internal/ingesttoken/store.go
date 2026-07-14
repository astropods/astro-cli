// Package ingesttoken manages account-scoped OTel ingest keys: the credential
// developer machines present to the telemetry ingest endpoint. Keys are
// ingest-only, revocable, and stored only as a sha256 hash — the plaintext is
// returned once at creation and never persisted.
package ingesttoken

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"fmt"
	"strings"
	"time"
)

// keyPrefix is the human-readable prefix on every plaintext ingest key.
const keyPrefix = "astotel_"

// displayPrefixLen is how many leading plaintext chars we retain for display
// in the management UI (keyPrefix plus a few random chars, enough to tell keys
// apart without revealing the secret).
const displayPrefixLen = 16

// Token is a stored ingest key. It never carries the plaintext secret.
type Token struct {
	ID          string
	AccountID   string
	Name        string
	TokenPrefix string
	CreatedAt   time.Time
	CreatedBy   *string
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
}

// Store manages the otel_ingest_tokens table.
type Store struct {
	db *sql.DB
}

// NewStore creates a new ingest-token store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Generate mints a new plaintext ingest key and returns it alongside the
// sha256 hash to persist and the display prefix. The plaintext is the only
// copy — the caller must return it to the user immediately and then discard it.
func Generate() (plaintext string, hash []byte, prefix string, err error) {
	b := make([]byte, 20)
	if _, err = rand.Read(b); err != nil {
		return "", nil, "", fmt.Errorf("ingesttoken: read random: %w", err)
	}
	body := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	plaintext = keyPrefix + body
	sum := sha256.Sum256([]byte(plaintext))
	return plaintext, sum[:], plaintext[:displayPrefixLen], nil
}

// Hash returns the sha256 hash of a plaintext key, matching what Generate
// stored. The ingest endpoint uses this to look a presented key up by hash.
func Hash(plaintext string) []byte {
	sum := sha256.Sum256([]byte(plaintext))
	return sum[:]
}

// Create inserts a new ingest key for an account and returns the stored row.
// createdBy may be empty (stored as NULL).
func (s *Store) Create(accountID, name string, tokenHash []byte, tokenPrefix, createdBy string) (*Token, error) {
	var by *string
	if createdBy != "" {
		by = &createdBy
	}
	row := s.db.QueryRow(`
		INSERT INTO otel_ingest_tokens (account_id, name, token_hash, token_prefix, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, account_id, name, token_prefix, created_at, created_by, last_used_at, revoked_at
	`, accountID, name, tokenHash, tokenPrefix, by)

	return scanToken(row)
}

// ListByAccount returns the account's active (non-revoked) keys, newest first.
func (s *Store) ListByAccount(accountID string) ([]*Token, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, name, token_prefix, created_at, created_by, last_used_at, revoked_at
		FROM otel_ingest_tokens
		WHERE account_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("ingesttoken list: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]*Token, 0, 8)
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Revoke marks a key revoked. Scoped to the account so one account cannot
// revoke another's key. Returns sql.ErrNoRows if no active key matches.
func (s *Store) Revoke(accountID, id string) error {
	res, err := s.db.Exec(`
		UPDATE otel_ingest_tokens
		SET revoked_at = now()
		WHERE id = $1 AND account_id = $2 AND revoked_at IS NULL
	`, id, accountID)
	if err != nil {
		return fmt.Errorf("ingesttoken revoke: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("ingesttoken revoke rows: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanToken(s scanner) (*Token, error) {
	var t Token
	if err := s.Scan(
		&t.ID, &t.AccountID, &t.Name, &t.TokenPrefix,
		&t.CreatedAt, &t.CreatedBy, &t.LastUsedAt, &t.RevokedAt,
	); err != nil {
		return nil, fmt.Errorf("ingesttoken scan: %w", err)
	}
	return &t, nil
}
