// Package githubwebhook manages the global webhook registry for GitHub repos.
package githubwebhook

import (
	"context"
	"database/sql"
	"time"
)

// Webhook represents a GitHub webhook registered for a base repo.
type Webhook struct {
	RepoBase      string
	WebhookID     int64
	WebhookSecret string
	CreatedAt     time.Time
}

// Store provides access to the github_webhooks table.
type Store struct {
	db *sql.DB
}

// New returns a Store backed by db.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Get returns the webhook for repoBase, or sql.ErrNoRows if none exists.
func (s *Store) Get(ctx context.Context, repoBase string) (*Webhook, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT repo_base, webhook_id, webhook_secret, created_at
		FROM github_webhooks
		WHERE repo_base = $1
	`, repoBase)
	var w Webhook
	if err := row.Scan(&w.RepoBase, &w.WebhookID, &w.WebhookSecret, &w.CreatedAt); err != nil {
		return nil, err
	}
	return &w, nil
}

// Insert adds a new webhook row. Returns inserted=true if the row was created, or
// inserted=false if the repo already has a webhook (PRIMARY KEY conflict resolved via
// ON CONFLICT DO NOTHING). This lets callers distinguish a race loss from a real error.
func (s *Store) Insert(ctx context.Context, repoBase string, webhookID int64, secret string) (inserted bool, err error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO github_webhooks (repo_base, webhook_id, webhook_secret)
		VALUES ($1, $2, $3)
		ON CONFLICT (repo_base) DO NOTHING
	`, repoBase, webhookID, secret)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteIfNoConnections atomically deletes the webhook row for repoBase only when no
// github_connections rows reference that repo (exact match or subpath). Returns the
// webhookID and deleted=true if the row was removed; deleted=false if connections still
// exist or no row was found.
func (s *Store) DeleteIfNoConnections(ctx context.Context, repoBase string) (webhookID int64, deleted bool, err error) {
	row := s.db.QueryRowContext(ctx, `
		DELETE FROM github_webhooks
		WHERE repo_base = $1
		  AND NOT EXISTS (
		      SELECT 1 FROM github_connections
		      WHERE repo_full_name = $1
		         OR repo_full_name LIKE replace($1, '_', '\_') || '/%' ESCAPE '\'
		  )
		RETURNING webhook_id
	`, repoBase)
	if scanErr := row.Scan(&webhookID); scanErr == sql.ErrNoRows {
		return 0, false, nil
	} else if scanErr != nil {
		return 0, false, scanErr
	}
	return webhookID, true, nil
}
