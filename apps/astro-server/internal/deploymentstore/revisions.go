package deploymentstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// DeploymentRevision represents a versioned snapshot of a deployment spec.
type DeploymentRevision struct {
	ID            int64           `json:"id"`
	DeploymentID  string          `json:"deployment_id"`
	Revision      int             `json:"revision"`
	BuildID       string          `json:"build_id"`
	SpecJSON      json.RawMessage `json:"spec_json"`
	KMSCiphertext []byte          `json:"-"`
	KMSKeyID      *string         `json:"-"`
	CreatedAt     time.Time       `json:"created_at"`
}

// GetCurrentRevision returns the revision pointed to by the deployment's current_revision.
func (s *Store) GetCurrentRevision(deploymentID string) (*DeploymentRevision, error) {
	var r DeploymentRevision
	err := s.db.QueryRow(`
		SELECT r.id, r.deployment_id, r.revision, r.build_id, r.spec_json,
		       r.kms_ciphertext, r.kms_key_id, r.created_at
		FROM deployment_revisions r
		JOIN deployments d ON d.id = r.deployment_id AND d.current_revision = r.revision
		WHERE r.deployment_id = $1
	`, deploymentID).Scan(
		&r.ID, &r.DeploymentID, &r.Revision, &r.BuildID, &r.SpecJSON,
		&r.KMSCiphertext, &r.KMSKeyID, &r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get current revision: %w", err)
	}
	return &r, nil
}

// GetRevisions returns all revisions for a deployment, newest first.
func (s *Store) GetRevisions(deploymentID string) ([]DeploymentRevision, error) {
	rows, err := s.db.Query(`
		SELECT id, deployment_id, revision, build_id, spec_json,
		       kms_ciphertext, kms_key_id, created_at
		FROM deployment_revisions
		WHERE deployment_id = $1
		ORDER BY revision DESC
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query revisions: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var revisions []DeploymentRevision
	for rows.Next() {
		var r DeploymentRevision
		if err := rows.Scan(
			&r.ID, &r.DeploymentID, &r.Revision, &r.BuildID, &r.SpecJSON,
			&r.KMSCiphertext, &r.KMSKeyID, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan revision: %w", err)
		}
		revisions = append(revisions, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating revisions: %w", err)
	}
	return revisions, nil
}

// RevisionHistoryRecord is a flattened view of one deployment_revisions row joined with
// its parent deployment, used for the history API endpoint.
type RevisionHistoryRecord struct {
	DeploymentID string
	AgentName    string
	Revision     int
	BuildID      string
	Namespace    string
	DisplayName  string
	IsCurrent    bool
	Status       string
	DeployedAt   time.Time
	Source        string // "github" or "direct"
	CommitSHA     string // populated when Source == "github"
	Branch        string // populated when Source == "github"
	CommitMessage string // populated when Source == "github"
	RepoFullName  string // populated when Source == "github", e.g. "owner/repo"
}

// GetDeploymentHistoryByRevisions returns one record per revision across all deployment
// instances for the given agent, ordered newest-first. Only genuine deploys/redepolys
// create revisions — restarts, pauses, and resumes do not — so the result naturally
// excludes those lifecycle events.
func (s *Store) GetDeploymentHistoryByRevisions(accountID, agentName string) ([]RevisionHistoryRecord, error) {
	rows, err := s.db.Query(`
		SELECT
			d.id,
			d.agent_name,
			dr.revision,
			dr.build_id,
			d.namespace,
			COALESCE(d.display_name, ''),
			(dr.revision = d.current_revision) AS is_current,
			CASE WHEN dr.revision = d.current_revision THEN d.status ELSE 'undeployed' END AS status,
			dr.created_at,
			CASE WHEN gb.id IS NOT NULL THEN 'github' ELSE 'direct' END AS source,
			COALESCE(gb.commit_sha, ''),
			COALESCE(gb.branch, ''),
			COALESCE(gb.commit_message, ''),
			COALESCE(gc.repo_full_name, '')
		FROM deployment_revisions dr
		JOIN deployments d ON dr.deployment_id = d.id
		LEFT JOIN github_builds gb
			ON gb.account_id = d.account_id
			AND gb.agent_name = d.agent_name
			AND gb.build_id = dr.build_id
		LEFT JOIN github_connections gc
			ON gc.id = gb.connection_id
		WHERE d.account_id = $1 AND d.agent_name = $2
		ORDER BY dr.created_at DESC
	`, accountID, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to query revision history: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var records []RevisionHistoryRecord
	for rows.Next() {
		var r RevisionHistoryRecord
		if err := rows.Scan(
			&r.DeploymentID, &r.AgentName, &r.Revision, &r.BuildID,
			&r.Namespace, &r.DisplayName, &r.IsCurrent, &r.Status, &r.DeployedAt,
			&r.Source, &r.CommitSHA, &r.Branch, &r.CommitMessage, &r.RepoFullName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan revision history row: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating revision history: %w", err)
	}
	return records, nil
}

// GetRevisionByNumber returns a specific revision for a deployment.
func (s *Store) GetRevisionByNumber(deploymentID string, revision int) (*DeploymentRevision, error) {
	var r DeploymentRevision
	err := s.db.QueryRow(`
		SELECT id, deployment_id, revision, build_id, spec_json,
		       kms_ciphertext, kms_key_id, created_at
		FROM deployment_revisions
		WHERE deployment_id = $1 AND revision = $2
	`, deploymentID, revision).Scan(
		&r.ID, &r.DeploymentID, &r.Revision, &r.BuildID, &r.SpecJSON,
		&r.KMSCiphertext, &r.KMSKeyID, &r.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get revision: %w", err)
	}
	return &r, nil
}

// SetCurrentRevision atomically sets a deployment's current_revision to a previous revision,
// sets status to pending, and records an event. The txFn callback allows the caller to
// enqueue a River job in the same transaction.
func (s *Store) SetCurrentRevision(deploymentID string, revision int, txFn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Verify revision exists
	var exists bool
	err = tx.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM deployment_revisions WHERE deployment_id = $1 AND revision = $2)
	`, deploymentID, revision).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check revision: %w", err)
	}
	if !exists {
		return fmt.Errorf("revision %d not found for deployment %s", revision, deploymentID)
	}

	// Set current_revision
	_, err = tx.Exec(`UPDATE deployments SET current_revision = $2 WHERE id = $1`, deploymentID, revision)
	if err != nil {
		return fmt.Errorf("failed to set current revision: %w", err)
	}

	// Update status + record event
	if err := updateStatusTx(tx, deploymentID, StatusPending, fmt.Sprintf("Rollback to revision %d", revision), nil); err != nil {
		return err
	}

	if txFn != nil {
		if err := txFn(tx); err != nil {
			return fmt.Errorf("failed to run tx callback: %w", err)
		}
	}

	return tx.Commit()
}
