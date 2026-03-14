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
