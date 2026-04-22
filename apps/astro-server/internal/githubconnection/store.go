// Package githubconnection manages GitHub repo connections for agents.
package githubconnection

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Connection represents a linked GitHub repository for an agent.
type Connection struct {
	ID                   string
	AccountID            string
	AccountName          string
	AgentName            string
	WorkOSUserID         string
	WorkOSOrganizationID string
	RepoFullName         string
	Branch               string
	WebhookID            int64
	WebhookSecret        string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Build represents a single auto-triggered build from a GitHub push.
type Build struct {
	ID            string           `json:"id"`
	ConnectionID  string           `json:"connection_id"`
	AccountID     string           `json:"account_id"`
	AgentName     string           `json:"agent_name"`
	BuildID       string           `json:"build_id"`
	CommitSHA     string           `json:"commit_sha"`
	Branch        string           `json:"branch"`
	Status        string           `json:"status"`         // pending | building | registering | registered | failed | cancelled
	Step          string           `json:"step,omitempty"` // fetching-spec | building | registering
	CommitMessage string           `json:"commit_message,omitempty"`
	CommitAuthor  string           `json:"commit_author,omitempty"`
	Error         string           `json:"error,omitempty"`
	EnqueuedAt    time.Time        `json:"enqueued_at"`
	CompletedAt   *time.Time       `json:"completed_at,omitempty"`
	Components    []BuildComponent `json:"components,omitempty"`
}

// BuildComponent represents a single component within a GitHub build.
type BuildComponent struct {
	ID            int64      `json:"id"`
	BuildID       string     `json:"build_id"`
	ComponentName string     `json:"component_name"`
	Status        string     `json:"status"`
	K8sJobName    string     `json:"k8s_job_name,omitempty"`
	Logs          string     `json:"logs,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
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
			(account_id, account_name, agent_name, workos_user_id, workos_org_id, repo_full_name, branch, webhook_id, webhook_secret, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (account_id, agent_name)
		DO UPDATE SET
			account_name    = EXCLUDED.account_name,
			workos_user_id  = EXCLUDED.workos_user_id,
			workos_org_id   = EXCLUDED.workos_org_id,
			repo_full_name  = EXCLUDED.repo_full_name,
			branch          = EXCLUDED.branch,
			webhook_id      = EXCLUDED.webhook_id,
			webhook_secret  = EXCLUDED.webhook_secret,
			updated_at      = now()
	`, c.AccountID, c.AccountName, c.AgentName, c.WorkOSUserID, c.WorkOSOrganizationID, c.RepoFullName, c.Branch, c.WebhookID, c.WebhookSecret)
	return err
}

// Get returns the connection for an account+agent, or sql.ErrNoRows if none.
func (s *Store) Get(ctx context.Context, accountID, agentName string) (*Connection, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, account_name, agent_name, workos_user_id, workos_org_id, repo_full_name, branch,
		       webhook_id, webhook_secret, created_at, updated_at
		FROM github_connections
		WHERE account_id = $1 AND agent_name = $2
	`, accountID, agentName)

	var c Connection
	if err := row.Scan(
		&c.ID, &c.AccountID, &c.AccountName, &c.AgentName, &c.WorkOSUserID, &c.WorkOSOrganizationID,
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
		SELECT id, account_id, account_name, agent_name, workos_user_id, workos_org_id, repo_full_name, branch,
		       webhook_id, webhook_secret, created_at, updated_at
		FROM github_connections
		WHERE id = $1
	`, id)

	var c Connection
	if err := row.Scan(
		&c.ID, &c.AccountID, &c.AccountName, &c.AgentName, &c.WorkOSUserID, &c.WorkOSOrganizationID,
		&c.RepoFullName, &c.Branch, &c.WebhookID, &c.WebhookSecret,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetByRepoForAccount returns the connection for a given account + repo, or sql.ErrNoRows if none.
// Used to prevent the same repo being linked to multiple agents within an account.
func (s *Store) GetByRepoForAccount(ctx context.Context, accountID, repoFullName string) (*Connection, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, account_name, agent_name, workos_user_id, workos_org_id, repo_full_name, branch,
		       webhook_id, webhook_secret, created_at, updated_at
		FROM github_connections
		WHERE account_id = $1 AND repo_full_name = $2
	`, accountID, repoFullName)

	var c Connection
	if err := row.Scan(
		&c.ID, &c.AccountID, &c.AccountName, &c.AgentName, &c.WorkOSUserID, &c.WorkOSOrganizationID,
		&c.RepoFullName, &c.Branch, &c.WebhookID, &c.WebhookSecret,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// RepoBase returns the first two slash-separated segments of repoFullName (owner/repo).
func RepoBase(repoFullName string) string {
	parts := strings.SplitN(repoFullName, "/", 3)
	if len(parts) < 2 {
		return repoFullName
	}
	return parts[0] + "/" + parts[1]
}

// RepoSubPath returns everything after the second slash, or "" for root connections.
func RepoSubPath(repoFullName string) string {
	parts := strings.SplitN(repoFullName, "/", 3)
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

// GetByRepoBase returns any connection whose repo_full_name equals repoBase or starts
// with repoBase+"/". Used to retrieve the shared webhook secret for HMAC verification.
func (s *Store) GetByRepoBase(ctx context.Context, repoBase string) (*Connection, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, account_id, account_name, agent_name, workos_user_id, workos_org_id, repo_full_name, branch,
		       webhook_id, webhook_secret, created_at, updated_at
		FROM github_connections
		WHERE repo_full_name = $1 OR repo_full_name LIKE replace($1, '_', '\_') || '/%' ESCAPE '\'
		LIMIT 1
	`, repoBase)

	var c Connection
	if err := row.Scan(
		&c.ID, &c.AccountID, &c.AccountName, &c.AgentName, &c.WorkOSUserID, &c.WorkOSOrganizationID,
		&c.RepoFullName, &c.Branch, &c.WebhookID, &c.WebhookSecret,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// CountByRepoBase counts all connections whose repo_full_name equals repoBase or starts
// with repoBase+"/". Used to decide whether to delete the shared webhook on disconnect.
// Counts across all accounts — the webhook is shared per base repo regardless of account.
func (s *Store) CountByRepoBase(ctx context.Context, repoBase string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM github_connections
		WHERE repo_full_name = $1 OR repo_full_name LIKE replace($1, '_', '\_') || '/%' ESCAPE '\'
	`, repoBase).Scan(&n)
	return n, err
}

// ListByRepoAndBranch returns all connections whose repo_full_name equals repoFullName
// or starts with repoFullName+"/", filtered by branch. Used for webhook fan-out.
func (s *Store) ListByRepoAndBranch(ctx context.Context, repoFullName, branch string) ([]*Connection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, account_name, agent_name, workos_user_id, workos_org_id, repo_full_name, branch,
		       webhook_id, webhook_secret, created_at, updated_at
		FROM github_connections
		WHERE (repo_full_name = $1 OR repo_full_name LIKE replace($1, '_', '\_') || '/%' ESCAPE '\') AND branch = $2
	`, repoFullName, branch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var conns []*Connection
	for rows.Next() {
		var c Connection
		if err := rows.Scan(
			&c.ID, &c.AccountID, &c.AccountName, &c.AgentName, &c.WorkOSUserID, &c.WorkOSOrganizationID,
			&c.RepoFullName, &c.Branch, &c.WebhookID, &c.WebhookSecret,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("githubconnection: scan connection: %w", err)
		}
		conns = append(conns, &c)
	}
	return conns, rows.Err()
}

// ListByAccount returns all connections for an account (agent_name + repo_full_name only).
func (s *Store) ListByAccount(ctx context.Context, accountID string) ([]*Connection, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT agent_name, repo_full_name, webhook_id, created_at
		FROM github_connections
		WHERE account_id = $1
		ORDER BY agent_name
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var conns []*Connection
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.AgentName, &c.RepoFullName, &c.WebhookID, &c.CreatedAt); err != nil {
			return nil, err
		}
		conns = append(conns, &c)
	}
	return conns, rows.Err()
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
			(connection_id, account_id, agent_name, build_id, commit_sha, branch, status, step, commit_message, commit_author)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`, b.ConnectionID, b.AccountID, b.AgentName, b.BuildID, b.CommitSHA, b.Branch, b.Status, b.Step, b.CommitMessage, b.CommitAuthor).Scan(&id)
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
		SET status = $1::text, completed_at = CASE WHEN $1::text IN ('registered','failed') THEN now() ELSE completed_at END
		WHERE id = $2
	`, status, id)
	return err
}

// UpdateBuildStep records the current sub-phase of a running build.
func (s *Store) UpdateBuildStep(ctx context.Context, id, step string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE github_builds SET step = $1 WHERE id = $2`, step, id)
	return err
}

// CancelOlderBuilds marks all non-terminal builds for connectionID except keepID as cancelled.
// Called when a new push arrives so the UI reflects that older builds were superseded.
func (s *Store) CancelOlderBuilds(ctx context.Context, connectionID, keepID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_builds
		SET status = 'cancelled', completed_at = now()
		WHERE connection_id = $1
		  AND id != $2
		  AND status NOT IN ('registered', 'failed', 'cancelled')
	`, connectionID, keepID)
	return err
}

// CancelBuild marks a single build as cancelled.
func (s *Store) CancelBuild(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_builds SET status = 'cancelled', completed_at = now() WHERE id = $1
	`, id)
	return err
}

// StartBuildIfPending atomically transitions a build from pending→building.
// Returns false if the build was already cancelled before the worker picked it up.
func (s *Store) StartBuildIfPending(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE github_builds SET status = 'building' WHERE id = $1 AND status = 'pending'
	`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListBuilds returns up to 10 recent builds for an agent.
func (s *Store) ListBuilds(ctx context.Context, accountID, agentName string, limit int) ([]Build, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, connection_id, account_id, agent_name, build_id, commit_sha, branch,
		       status, step, commit_message, commit_author, COALESCE(error,''), enqueued_at, completed_at
		FROM github_builds
		WHERE account_id = $1 AND agent_name = $2
		ORDER BY enqueued_at DESC
		LIMIT $3
	`, accountID, agentName, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var builds []Build
	for rows.Next() {
		var b Build
		if err := rows.Scan(
			&b.ID, &b.ConnectionID, &b.AccountID, &b.AgentName,
			&b.BuildID, &b.CommitSHA, &b.Branch,
			&b.Status, &b.Step, &b.CommitMessage, &b.CommitAuthor, &b.Error, &b.EnqueuedAt, &b.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("githubconnection: scan build: %w", err)
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

// GetBuildByBuildID looks up a single build by its short build ID for a given account/agent.
func (s *Store) GetBuildByBuildID(ctx context.Context, accountID, agentName, buildID string) (*Build, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, connection_id, account_id, agent_name, build_id, commit_sha, branch,
		       status, step, commit_message, commit_author, COALESCE(error,''), enqueued_at, completed_at
		FROM github_builds
		WHERE account_id = $1 AND agent_name = $2 AND build_id = $3
	`, accountID, agentName, buildID)
	var b Build
	if err := row.Scan(
		&b.ID, &b.ConnectionID, &b.AccountID, &b.AgentName,
		&b.BuildID, &b.CommitSHA, &b.Branch,
		&b.Status, &b.Step, &b.CommitMessage, &b.CommitAuthor, &b.Error, &b.EnqueuedAt, &b.CompletedAt,
	); err != nil {
		return nil, err
	}
	return &b, nil
}

// CreateBuildComponent inserts a new component record for a build. Returns the row ID.
func (s *Store) CreateBuildComponent(ctx context.Context, buildID, componentName, k8sJobName string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO github_build_components (build_id, component_name, k8s_job_name)
		VALUES ($1, $2, $3)
		RETURNING id
	`, buildID, componentName, k8sJobName).Scan(&id)
	return id, err
}

// UpdateBuildComponentStatus sets the status of a component, with automatic timestamp management.
func (s *Store) UpdateBuildComponentStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_build_components
		SET status = $1,
		    started_at = CASE WHEN $1 = 'building' AND started_at IS NULL THEN now() ELSE started_at END,
		    completed_at = CASE WHEN $1 IN ('succeeded','failed') THEN now() ELSE completed_at END
		WHERE id = $2
	`, status, id)
	return err
}

// SaveBuildComponentLogs persists logs for a component build.
func (s *Store) SaveBuildComponentLogs(ctx context.Context, id int64, logs string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_build_components SET logs = $1 WHERE id = $2
	`, logs, id)
	return err
}

// ListBuildComponents returns all components for a build, ordered by creation.
func (s *Store) ListBuildComponents(ctx context.Context, buildID string) ([]BuildComponent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, build_id, component_name, status, k8s_job_name, logs, started_at, completed_at
		FROM github_build_components
		WHERE build_id = $1
		ORDER BY id
	`, buildID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var components []BuildComponent
	for rows.Next() {
		var c BuildComponent
		if err := rows.Scan(&c.ID, &c.BuildID, &c.ComponentName, &c.Status, &c.K8sJobName, &c.Logs, &c.StartedAt, &c.CompletedAt); err != nil {
			return nil, fmt.Errorf("githubconnection: scan build component: %w", err)
		}
		components = append(components, c)
	}
	return components, rows.Err()
}

// FailPendingBuildComponents marks all non-terminal components for a build as failed.
func (s *Store) FailPendingBuildComponents(ctx context.Context, buildID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_build_components
		SET status = 'failed', completed_at = now()
		WHERE build_id = $1 AND status NOT IN ('succeeded', 'failed')
	`, buildID)
	return err
}
