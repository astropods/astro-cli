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

// TrackedAlert is one persisted firing-state row carrying its condition, for the
// cross-deployment admin listing (unlike Tracked, which is scoped to a single
// condition query).
type TrackedAlert struct {
	DeploymentID string
	Workload     string
	Condition    string
	ActiveSince  time.Time
	Notified     bool
}

// Mute is an active admin mute for a (deployment, condition).
type Mute struct {
	DeploymentID string
	Condition    string
	MutedUntil   time.Time
}

// NotifyRecord is the last time a (deployment, condition) notified, from the
// daily-cap ledger.
type NotifyRecord struct {
	DeploymentID   string
	Condition      string
	LastNotifiedAt time.Time
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

// ListAll returns every currently-tracked breach across all deployments,
// workloads, and conditions — the source for the admin cross-deployment view.
func (s *Store) ListAll(ctx context.Context) ([]TrackedAlert, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT deployment_id, workload, condition, active_since, notified FROM deployment_alert_state ORDER BY active_since`)
	if err != nil {
		return nil, fmt.Errorf("observation: list all state: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []TrackedAlert
	for rows.Next() {
		var t TrackedAlert
		if err := rows.Scan(&t.DeploymentID, &t.Workload, &t.Condition, &t.ActiveSince, &t.Notified); err != nil {
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
		 WHERE deployment_alert_notifications.last_notified_at IS NULL
		    OR deployment_alert_notifications.last_notified_at < $4
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

// Mute silences notifications for a (deployment, condition) until `until`. The
// evaluator keeps detecting and tracking the breach; it just suppresses the
// send while the mute is active. Idempotent: re-muting updates the expiry. Uses
// the notification-control row (upsert leaves any daily-cap ledger intact).
func (s *Store) Mute(ctx context.Context, deploymentID, condition string, until time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO deployment_alert_notifications (deployment_id, condition, muted_until)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (deployment_id, condition) DO UPDATE SET muted_until = $3`,
		deploymentID, condition, until)
	if err != nil {
		return fmt.Errorf("observation: mute: %w", err)
	}
	return nil
}

// Unmute clears a (deployment, condition) mute. It nulls muted_until rather than
// deleting the row so the daily-cap ledger (last_notified_at) survives.
func (s *Store) Unmute(ctx context.Context, deploymentID, condition string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE deployment_alert_notifications SET muted_until = NULL WHERE deployment_id = $1 AND condition = $2`,
		deploymentID, condition)
	if err != nil {
		return fmt.Errorf("observation: unmute: %w", err)
	}
	return nil
}

// IsMuted reports whether a (deployment, condition) has an active mute at `now`.
func (s *Store) IsMuted(ctx context.Context, deploymentID, condition string, now time.Time) (bool, error) {
	var muted bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM deployment_alert_notifications
		 WHERE deployment_id = $1 AND condition = $2 AND muted_until > $3)`,
		deploymentID, condition, now).Scan(&muted)
	if err != nil {
		return false, fmt.Errorf("observation: is muted: %w", err)
	}
	return muted, nil
}

// ListMutes returns all currently-active mutes (muted_until in the future).
func (s *Store) ListMutes(ctx context.Context, now time.Time) ([]Mute, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT deployment_id, condition, muted_until FROM deployment_alert_notifications WHERE muted_until > $1`, now)
	if err != nil {
		return nil, fmt.Errorf("observation: list mutes: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []Mute
	for rows.Next() {
		var m Mute
		if err := rows.Scan(&m.DeploymentID, &m.Condition, &m.MutedUntil); err != nil {
			return nil, fmt.Errorf("observation: scan mute: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListNotified returns the last-notified timestamp for every (deployment,
// condition) that has notified at least once — the daily-cap ledger, used to
// show when an alert last paged.
func (s *Store) ListNotified(ctx context.Context) ([]NotifyRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT deployment_id, condition, last_notified_at FROM deployment_alert_notifications WHERE last_notified_at IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("observation: list notified: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []NotifyRecord
	for rows.Next() {
		var r NotifyRecord
		if err := rows.Scan(&r.DeploymentID, &r.Condition, &r.LastNotifiedAt); err != nil {
			return nil, fmt.Errorf("observation: scan notified: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
