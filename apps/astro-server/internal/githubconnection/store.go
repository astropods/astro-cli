// Package githubconnection manages GitHub repo connections for agents.
package githubconnection

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Connection represents a linked GitHub repository for an agent.
type Connection struct {
	ID            string
	AccountID     string
	AgentName     string
	WorkOSUserID  string
	RepoFullName  string
	Branch        string
	WebhookID     int64
	WebhookSecret string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Build represents a single auto-triggered build from a GitHub push.
type Build struct {
	ID           string
	ConnectionID string
	AccountID    string
	AgentName    string
	BuildID      string
	CommitSHA    string
	Branch       string
	Status       string // pending | building | registered | failed
	Error        string
	EnqueuedAt   time.Time
	CompletedAt  *time.Time
}

// Store provides CRUD operations for github_connections and github_builds.
type Store struct {
	db *sql.DB
}

// New returns a Store backed by db.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Upsert creates or replaces the GitHub connection for an agent.
// An existing connection (same account + agent) is overwritten.
func (s *Store) Upsert(ctx context.Context, c *Connection) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_connections
			(account_id, agent_name, workos_user_id, repo_full_name, branch, webhook_id, webhook_secret, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (account_id, agent_name)
		DO UPDATE SET
			workos_user_id  = EXCLUDED.workos_user_id,
			repo_full_name  = EXCLUDED.repo_full_name,
			branch          = EXCLUDED.branch,
			webhook_id      = EXCLUDED.webhook_id,
			webhook_secret  = EXCLUDED.webhook_secret,
			updated_at      = now()
	`, c.AccountID, c.AgentName, c.WorkOSUserID, c.RepoFullName, c.Branch, c.WebhookID, c.WebhookSecret)
	return err
}

// Get returns the connection for an account+agent, or sql.ErrNoRows if none.
func (s *Store) Get(ctx context.Context, accountID, agentName string) (*Connection, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, agent_name, workos_user_id, repo_full_name, branch,
		       webhook_id, webhook_secret, created_at, updated_at
		FROM github_connections
		WHERE account_id = $1 AND agent_name = $2
	`, accountID, agentName)

	var c Connection
	if err := row.Scan(
		&c.ID, &c.AccountID, &c.AgentName, &c.WorkOSUserID,
		&c.RepoFullName, &c.Branch, &c.WebhookID, &c.WebhookSecret,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetByID returns the connection by primary key.
func (s *Store) GetByID(ctx context.Context, id string) (*Connection, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, agent_name, workos_user_id, repo_full_name, branch,
		       webhook_id, webhook_secret, created_at, updated_at
		FROM github_connections
		WHERE id = $1
	`, id)

	var c Connection
	if err := row.Scan(
		&c.ID, &c.AccountID, &c.AgentName, &c.WorkOSUserID,
		&c.RepoFullName, &c.Branch, &c.WebhookID, &c.WebhookSecret,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetByRepo returns the connection for a given repo full name (used by webhook handler).
func (s *Store) GetByRepo(ctx context.Context, repoFullName string) (*Connection, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, agent_name, workos_user_id, repo_full_name, branch,
		       webhook_id, webhook_secret, created_at, updated_at
		FROM github_connections
		WHERE repo_full_name = $1
		LIMIT 1
	`, repoFullName)

	var c Connection
	if err := row.Scan(
		&c.ID, &c.AccountID, &c.AgentName, &c.WorkOSUserID,
		&c.RepoFullName, &c.Branch, &c.WebhookID, &c.WebhookSecret,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// Delete removes a connection. Caller is responsible for removing the GitHub webhook first.
func (s *Store) Delete(ctx context.Context, accountID, agentName string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM github_connections WHERE account_id = $1 AND agent_name = $2
	`, accountID, agentName)
	return err
}

// CreateBuild inserts a new build record and returns its ID.
func (s *Store) CreateBuild(ctx context.Context, b *Build) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO github_builds
			(connection_id, account_id, agent_name, build_id, commit_sha, branch, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, b.ConnectionID, b.AccountID, b.AgentName, b.BuildID, b.CommitSHA, b.Branch, b.Status).Scan(&id)
	return id, err
}

// UpdateBuildStatus updates the status (and optionally error/completed_at) for a build.
func (s *Store) UpdateBuildStatus(ctx context.Context, id, status, buildErr string) error {
	if buildErr != "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE github_builds
			SET status = $1, error = $2, completed_at = now()
			WHERE id = $3
		`, status, buildErr, id)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_builds
		SET status = $1, completed_at = CASE WHEN $1 IN ('registered','failed') THEN now() ELSE completed_at END
		WHERE id = $2
	`, status, id)
	return err
}

// ListBuilds returns up to 10 recent builds for an agent.
func (s *Store) ListBuilds(ctx context.Context, accountID, agentName string, limit int) ([]Build, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, connection_id, account_id, agent_name, build_id, commit_sha, branch,
		       status, COALESCE(error,''), enqueued_at, completed_at
		FROM github_builds
		WHERE account_id = $1 AND agent_name = $2
		ORDER BY enqueued_at DESC
		LIMIT $3
	`, accountID, agentName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var builds []Build
	for rows.Next() {
		var b Build
		if err := rows.Scan(
			&b.ID, &b.ConnectionID, &b.AccountID, &b.AgentName,
			&b.BuildID, &b.CommitSHA, &b.Branch,
			&b.Status, &b.Error, &b.EnqueuedAt, &b.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("githubconnection: scan build: %w", err)
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}
