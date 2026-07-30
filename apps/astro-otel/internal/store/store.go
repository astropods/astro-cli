// Package store reads astro-server's Postgres for the two things astro-otel
// needs at request time: resolving an ingest key to an account
// (otel_ingest_tokens) and resolving an account to its Langfuse project
// credentials (account_langfuse + KMS). Both are TTL-cached so the hot path
// avoids a DB round-trip (and a KMS call) per OTLP batch.
package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-otel/internal/envelope"
	"github.com/lib/pq"
)

// Store resolves ingest keys and Langfuse credentials, with caching.
type Store struct {
	db  *sql.DB
	kms envelope.KMSClient
	ttl time.Duration

	mu       sync.Mutex
	accounts map[string]accountEntry // hex(hash) -> account
	creds    map[string]credEntry    // accountID -> Langfuse basic auth
}

type accountEntry struct {
	accountID string
	found     bool
	// excluded is the set of lowercased user emails whose full-text content is
	// dropped at ingest. nil when the key excludes no one.
	excluded map[string]struct{}
	exp      time.Time
}

type credEntry struct {
	basicAuth string
	exp       time.Time
}

// New creates a Store. kms may be nil in dev (only plaintext-stored Langfuse
// secret keys can be resolved without it).
func New(db *sql.DB, kms envelope.KMSClient, ttl time.Duration) *Store {
	return &Store{
		db:       db,
		kms:      kms,
		ttl:      ttl,
		accounts: make(map[string]accountEntry),
		creds:    make(map[string]credEntry),
	}
}

// ResolveAccount returns the account id for an ingest-key hash, the key's
// exclusion set (lowercased emails, nil when empty), and whether an active
// (non-revoked) key was found. Both hits and misses are cached, so a flood of
// invalid keys can't hammer the DB, and an edited exclusion list propagates
// within the cache TTL — the same lag as a revoke.
func (s *Store) ResolveAccount(ctx context.Context, hash []byte) (accountID string, excluded map[string]struct{}, found bool, err error) {
	key := hex.EncodeToString(hash)

	s.mu.Lock()
	if e, ok := s.accounts[key]; ok && time.Now().Before(e.exp) {
		s.mu.Unlock()
		return e.accountID, e.excluded, e.found, nil
	}
	s.mu.Unlock()

	var (
		id     string
		emails []string
	)
	row := s.db.QueryRowContext(ctx,
		`SELECT account_id::text, excluded_emails FROM otel_ingest_tokens WHERE token_hash = $1 AND revoked_at IS NULL`,
		hash,
	)
	switch err := row.Scan(&id, pq.Array(&emails)); {
	case errors.Is(err, sql.ErrNoRows):
		s.put(key, accountEntry{found: false, exp: time.Now().Add(s.ttl)})
		return "", nil, false, nil
	case err != nil:
		return "", nil, false, fmt.Errorf("resolve account: %w", err)
	}

	set := toEmailSet(emails)
	s.put(key, accountEntry{accountID: id, found: true, excluded: set, exp: time.Now().Add(s.ttl)})
	return id, set, true, nil
}

// toEmailSet lowercases and trims emails into a lookup set, or nil when empty.
func toEmailSet(emails []string) map[string]struct{} {
	if len(emails) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(emails))
	for _, e := range emails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			set[e] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func (s *Store) put(key string, e accountEntry) {
	s.mu.Lock()
	s.accounts[key] = e
	s.mu.Unlock()
}

// TouchLastUsed stamps last_used_at for a key. Best-effort and fire-and-forget:
// callers should invoke it in a goroutine; errors are for logging only.
func (s *Store) TouchLastUsed(hash []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`UPDATE otel_ingest_tokens SET last_used_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`,
		hash,
	)
	return err
}

// LangfuseBasicAuth returns base64("pk:sk") for the account's Langfuse project,
// or "" if the account has no project provisioned. The secret key is decrypted
// via KMS (or read as plaintext in dev, matching astro-server's storage).
func (s *Store) LangfuseBasicAuth(ctx context.Context, accountID string) (string, error) {
	s.mu.Lock()
	if e, ok := s.creds[accountID]; ok && time.Now().Before(e.exp) {
		s.mu.Unlock()
		return e.basicAuth, nil
	}
	s.mu.Unlock()

	var (
		pk         string
		skStored   string
		encDataKey []byte
		nonce      []byte
	)
	row := s.db.QueryRowContext(ctx, `
		SELECT langfuse_public_key, langfuse_secret_key, encrypted_data_key, nonce
		FROM account_langfuse WHERE account_id = $1`, accountID)
	switch err := row.Scan(&pk, &skStored, &encDataKey, &nonce); {
	case errors.Is(err, sql.ErrNoRows):
		s.putCred(accountID, "")
		return "", nil
	case err != nil:
		return "", fmt.Errorf("read langfuse creds: %w", err)
	}

	sk, err := s.decryptSecretKey(ctx, skStored, encDataKey, nonce)
	if err != nil {
		return "", fmt.Errorf("decrypt langfuse secret: %w", err)
	}

	basic := base64.StdEncoding.EncodeToString([]byte(pk + ":" + sk))
	s.putCred(accountID, basic)
	return basic, nil
}

func (s *Store) putCred(accountID, basic string) {
	s.mu.Lock()
	s.creds[accountID] = credEntry{basicAuth: basic, exp: time.Now().Add(s.ttl)}
	s.mu.Unlock()
}

// decryptSecretKey mirrors astro-server's storage: when there is no encrypted
// data key / nonce the stored value is plaintext; otherwise it is a
// base64-encoded AES-GCM ciphertext under the KMS-wrapped data key.
func (s *Store) decryptSecretKey(ctx context.Context, stored string, encDataKey, nonce []byte) (string, error) {
	if len(encDataKey) == 0 && len(nonce) == 0 {
		return stored, nil // plaintext (dev)
	}
	if s.kms == nil {
		return "", fmt.Errorf("KMS client required to decrypt Langfuse secret key")
	}
	dec, err := envelope.NewDecryptor(ctx, s.kms, encDataKey)
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	plaintext, err := dec.Decrypt(ciphertext, nonce)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
