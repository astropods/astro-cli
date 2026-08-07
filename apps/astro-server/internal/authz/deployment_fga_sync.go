package authz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DeploymentFGAState is the desired or last-applied WorkOS resource state.
type DeploymentFGAState string

const (
	DeploymentFGARegistered DeploymentFGAState = "registered"
	DeploymentFGADeleted    DeploymentFGAState = "deleted"
)

// DeploymentFGAWork is one pending WorkOS resource reconciliation.
type DeploymentFGAWork struct {
	DeploymentID             string
	DesiredState             DeploymentFGAState
	DesiredVersion           int64
	SyncedState              DeploymentFGAState
	SyncedVersion            int64
	AttemptCount             int
	CreatorIsMember          bool
	CreatorAssignmentPending bool
	Name                     string
	WorkOSOrgID              string
	MembershipID             string
}

// DeploymentFGASyncStore persists desired WorkOS state alongside deployment
// lifecycle writes. It stores reconciliation state, never authorization grants.
type DeploymentFGASyncStore struct {
	db      *sql.DB
	enabled bool
}

// NewDeploymentFGASyncStore creates the deployment reconciliation store.
func NewDeploymentFGASyncStore(db *sql.DB, enabled bool) *DeploymentFGASyncStore {
	return &DeploymentFGASyncStore{db: db, enabled: enabled && db != nil}
}

// Enabled reports whether deployment FGA synchronization is configured.
func (s *DeploymentFGASyncStore) Enabled() bool {
	return s != nil && s.enabled
}

// RecordRegistrationTx queues registration only for organization deployments
// and reports whether durable work was recorded.
func (s *DeploymentFGASyncStore) RecordRegistrationTx(ctx context.Context, tx *sql.Tx, deploymentID string) (bool, error) {
	return s.recordStateTx(ctx, tx, deploymentID, DeploymentFGARegistered)
}

// RecordNameUpdateTx versions only existing registered intent. Personal,
// not-yet-backfilled, and deleting deployments no-op.
func (s *DeploymentFGASyncStore) RecordNameUpdateTx(ctx context.Context, tx *sql.Tx, deploymentID string) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	if tx == nil {
		return false, errors.New("deployment FGA sync transaction is required")
	}
	if deploymentID == "" {
		return false, errors.New("deployment id is required")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE deployment_fga_sync
		SET desired_version = desired_version + 1,
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW(),
		    updated_at = NOW()
		WHERE deployment_id = $1 AND desired_state = 'registered'
	`, deploymentID)
	if err != nil {
		return false, fmt.Errorf("record deployment FGA name update: %w", err)
	}
	return changed(result, "record deployment FGA name update")
}

// RecordDeletionTx queues deletion only for organization deployments and
// reports whether durable work was recorded.
func (s *DeploymentFGASyncStore) RecordDeletionTx(ctx context.Context, tx *sql.Tx, deploymentID string) (bool, error) {
	return s.recordStateTx(ctx, tx, deploymentID, DeploymentFGADeleted)
}

func (s *DeploymentFGASyncStore) recordStateTx(ctx context.Context, tx *sql.Tx, deploymentID string, state DeploymentFGAState) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	if tx == nil {
		return false, errors.New("deployment FGA sync transaction is required")
	}
	if deploymentID == "" {
		return false, errors.New("deployment id is required")
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO deployment_fga_sync (deployment_id, desired_state, desired_version, next_attempt_at, updated_at)
		SELECT d.id, $2, 1, NOW(), NOW()
		FROM deployments d
		JOIN accounts a ON a.id = d.account_id
		WHERE d.id = $1 AND a.type = 'organization'
		ON CONFLICT (deployment_id) DO UPDATE
		SET desired_state = EXCLUDED.desired_state,
		    desired_version = deployment_fga_sync.desired_version + 1,
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW(),
		    updated_at = NOW()
	`, deploymentID, state)
	if err != nil {
		return false, fmt.Errorf("record deployment FGA %s state: %w", state, err)
	}
	return changed(result, fmt.Sprintf("record deployment FGA %s state", state))
}

func changed(result sql.Result, operation string) (bool, error) {
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("%s rows affected: %w", operation, err)
	}
	return rows > 0, nil
}

// Pending returns current unsynchronized work for one deployment.
func (s *DeploymentFGASyncStore) Pending(ctx context.Context, deploymentID string) (*DeploymentFGAWork, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("deployment FGA sync store is not configured")
	}
	var work DeploymentFGAWork
	err := s.db.QueryRowContext(ctx, `
		SELECT s.deployment_id,
		       s.desired_state,
		       s.desired_version,
		       COALESCE(s.synced_state, ''),
		       COALESCE(s.synced_version, 0),
		       s.attempt_count,
		       EXISTS (
		           SELECT 1 FROM account_members am
		           WHERE am.account_id = d.account_id AND am.user_id = d.deployed_by
		       ),
		       s.creator_assignment_pending,
		       COALESCE(NULLIF(d.display_name, ''), d.agent_name),
		       COALESCE(ao.workos_org_id, ''),
		       COALESCE(amw.workos_membership_id, '')
		FROM deployment_fga_sync s
		JOIN deployments d ON d.id = s.deployment_id
		LEFT JOIN account_organizations ao ON ao.account_id = d.account_id
		LEFT JOIN account_member_workos amw
		  ON amw.account_id = d.account_id AND amw.user_id = d.deployed_by
		WHERE s.deployment_id = $1
		  AND (s.synced_state IS DISTINCT FROM s.desired_state
		       OR s.synced_version IS DISTINCT FROM s.desired_version
		       OR s.creator_assignment_pending)
	`, deploymentID).Scan(
		&work.DeploymentID,
		&work.DesiredState,
		&work.DesiredVersion,
		&work.SyncedState,
		&work.SyncedVersion,
		&work.AttemptCount,
		&work.CreatorIsMember,
		&work.CreatorAssignmentPending,
		&work.Name,
		&work.WorkOSOrgID,
		&work.MembershipID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load pending deployment FGA work: %w", err)
	}
	return &work, nil
}

// DueDeploymentIDs lists lifecycle or creator-assignment work whose retry
// delay has elapsed.
func (s *DeploymentFGASyncStore) DueDeploymentIDs(ctx context.Context, limit int) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("deployment FGA sync store is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT deployment_id
		FROM deployment_fga_sync
		WHERE (synced_state IS DISTINCT FROM desired_state
		       OR synced_version IS DISTINCT FROM desired_version
		       OR creator_assignment_pending)
		  AND next_attempt_at <= NOW()
		ORDER BY next_attempt_at, updated_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list due deployment FGA work: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan due deployment FGA work: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due deployment FGA work: %w", err)
	}
	return ids, nil
}

// MarkSynced records success only if the desired state has not changed since
// the worker loaded it.
func (s *DeploymentFGASyncStore) MarkSynced(ctx context.Context, deploymentID string, state DeploymentFGAState, version int64) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("deployment FGA sync store is not configured")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE deployment_fga_sync
		SET synced_state = $2,
		    synced_version = $3,
		    creator_assignment_pending = FALSE,
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW(),
		    synced_at = NOW(),
		    updated_at = NOW()
		WHERE deployment_id = $1
		  AND desired_state = $2
		  AND desired_version = $3
	`, deploymentID, state, version)
	if err != nil {
		return false, fmt.Errorf("mark deployment FGA state synced: %w", err)
	}
	return changed(result, "mark deployment FGA state synced")
}

// DeferCreatorAssignment marks resource lifecycle work synchronized while
// retaining a low-frequency creator-role retry that does not block purge.
func (s *DeploymentFGASyncStore) DeferCreatorAssignment(ctx context.Context, deploymentID string, state DeploymentFGAState, version int64) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("deployment FGA sync store is not configured")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE deployment_fga_sync
		SET synced_state = $2,
		    synced_version = $3,
		    creator_assignment_pending = TRUE,
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW() + INTERVAL '1 hour',
		    synced_at = NOW(),
		    updated_at = NOW()
		WHERE deployment_id = $1
		  AND desired_state = $2
		  AND desired_version = $3
	`, deploymentID, state, version)
	if err != nil {
		return false, fmt.Errorf("defer deployment FGA creator assignment: %w", err)
	}
	return changed(result, "defer deployment FGA creator assignment")
}

// RecordFailure retains the desired state and applies bounded exponential
// backoff. River retries immediate jobs; the periodic sweep covers missed jobs.
func (s *DeploymentFGASyncStore) RecordFailure(ctx context.Context, deploymentID string, state DeploymentFGAState, version int64, cause error) error {
	if s == nil || s.db == nil {
		return errors.New("deployment FGA sync store is not configured")
	}
	message := "unknown reconciliation failure"
	if cause != nil {
		message = cause.Error()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE deployment_fga_sync
		SET attempt_count = attempt_count + 1,
		    last_error = $3,
		    next_attempt_at = NOW() + POWER(2, LEAST(attempt_count + 1, 8)) * INTERVAL '1 second',
		    updated_at = NOW()
		WHERE deployment_id = $1
		  AND desired_state = $2
		  AND desired_version = $4
	`, deploymentID, state, message, version)
	if err != nil {
		return fmt.Errorf("record deployment FGA failure: %w", err)
	}
	return nil
}

// HasPendingForAccount prevents account purge from discarding deletion work
// before WorkOS resource cleanup has converged. Assignment-only retries do not
// block purge because deleting the resource also removes its assignments.
func (s *DeploymentFGASyncStore) HasPendingForAccount(ctx context.Context, accountID string) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	var pending bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM deployment_fga_sync s
			JOIN deployments d ON d.id = s.deployment_id
			WHERE d.account_id = $1
			  AND (s.synced_state IS DISTINCT FROM s.desired_state
			       OR s.synced_version IS DISTINCT FROM s.desired_version)
		)
	`, accountID).Scan(&pending)
	if err != nil {
		return false, fmt.Errorf("check pending deployment FGA work: %w", err)
	}
	return pending, nil
}
