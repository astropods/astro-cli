package observation

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Store persists per-(deployment, workload, condition) firing state in Postgres
// so the `for` sustained window, edge-only firing, and dedup survive restarts.
// State is tracked per workload so the UI can show which workload is affected;
// notifications are still deduped to one per (deployment, condition) episode by
// the evaluator.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// State is one tracked breach: when it was first seen and whether the firing
// notification has been emitted.
type State struct {
	ActiveSince time.Time
	Notified    bool
}

// Tracked is one persisted firing-state row (a breaching workload+condition).
type Tracked struct {
	DeploymentID string
	Workload     string
	ActiveSince  time.Time
	Notified     bool
}

// ForCondition returns every currently-tracked breach for a condition, across
// all deployments and workloads.
func (s *Store) ForCondition(ctx context.Context, condition string) ([]Tracked, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT deployment_id, workload, active_since, notified FROM deployment_alert_state WHERE condition = $1`, condition)
	if err != nil {
		return nil, fmt.Errorf("observation: load state: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []Tracked
	for rows.Next() {
		var t Tracked
		if err := rows.Scan(&t.DeploymentID, &t.Workload, &t.ActiveSince, &t.Notified); err != nil {
			return nil, fmt.Errorf("observation: scan state: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ForDeploymentWorkload returns the currently-tracked breaches for one workload
// of a deployment, keyed by condition name. A condition is present only while it
// is actively breaching, so absence means "ok".
func (s *Store) ForDeploymentWorkload(ctx context.Context, deploymentID, workload string) (map[string]State, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT condition, active_since, notified FROM deployment_alert_state WHERE deployment_id = $1 AND workload = $2`,
		deploymentID, workload)
	if err != nil {
		return nil, fmt.Errorf("observation: load workload state: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	out := map[string]State{}
	for rows.Next() {
		var cond string
		var st State
		if err := rows.Scan(&cond, &st.ActiveSince, &st.Notified); err != nil {
			return nil, fmt.Errorf("observation: scan state: %w", err)
		}
		out[cond] = st
	}
	return out, rows.Err()
}

// StartTracking records a newly-seen breach. If a row already exists (a breach
// already being tracked) it is left untouched so active_since is preserved.
func (s *Store) StartTracking(ctx context.Context, deploymentID, workload, condition string, since time.Time, notified bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO deployment_alert_state (deployment_id, workload, condition, active_since, notified, updated_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 ON CONFLICT (deployment_id, workload, condition) DO NOTHING`,
		deploymentID, workload, condition, since, notified)
	if err != nil {
		return fmt.Errorf("observation: start tracking: %w", err)
	}
	return nil
}

// MarkNotified flips a tracked breach to notified (its firing edge was handled).
func (s *Store) MarkNotified(ctx context.Context, deploymentID, workload, condition string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE deployment_alert_state SET notified = true, updated_at = now()
		 WHERE deployment_id = $1 AND workload = $2 AND condition = $3`, deploymentID, workload, condition)
	if err != nil {
		return fmt.Errorf("observation: mark notified: %w", err)
	}
	return nil
}

// ClaimDailyNotify atomically records that a (deployment, condition) alert is
// being sent at `at`, but only if no alert for it went out at or after `cutoff`.
// It returns true when the caller won the claim (should send) and false when an
// alert already fired inside the window (throttled). This backstops the
// per-episode dedup: a flapping deployment that resolves and re-breaches many
// times a day would otherwise emit once per episode; the ledger row survives
// resolves, so sends are capped to one per (deployment, condition) per window.
// The upsert + conditional update makes the check-and-set race-safe.
func (s *Store) ClaimDailyNotify(ctx context.Context, deploymentID, condition string, at, cutoff time.Time) (bool, error) {
	var got string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO deployment_alert_notifications (deployment_id, condition, last_notified_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (deployment_id, condition)
		 DO UPDATE SET last_notified_at = $3
		 WHERE deployment_alert_notifications.last_notified_at < $4
		 RETURNING deployment_id`,
		deploymentID, condition, at, cutoff).Scan(&got)
	if err == sql.ErrNoRows {
		return false, nil // a send is already recorded inside the window
	}
	if err != nil {
		return false, fmt.Errorf("observation: claim daily notify: %w", err)
	}
	return true, nil
}

// Clear removes a tracked breach (the condition resolved for that workload).
func (s *Store) Clear(ctx context.Context, deploymentID, workload, condition string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM deployment_alert_state WHERE deployment_id = $1 AND workload = $2 AND condition = $3`,
		deploymentID, workload, condition)
	if err != nil {
		return fmt.Errorf("observation: clear: %w", err)
	}
	return nil
}
