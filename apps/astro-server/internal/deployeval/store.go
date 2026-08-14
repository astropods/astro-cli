package deployeval

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Status is the outcome of the most recent Check (or Fix attempt) for a
// (deployment, evaluator) pair.
type Status string

const (
	StatusOK        Status = "ok"
	StatusDrifted   Status = "drifted"
	StatusFixFailed Status = "fix_failed"
)

// Store persists the last check result per (deployment, evaluator) in
// Postgres, so astro-queen can list what's currently drifted without
// re-running every evaluator against the whole fleet on page load.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Row is one persisted (deployment, evaluator) check result.
type Row struct {
	DeploymentID string
	EvaluatorID  string
	Status       Status
	Detail       string
	CheckedAt    time.Time
	FixedAt      *time.Time
}

// Summary is the aggregate state of one evaluator across every deployment
// it's been run against.
type Summary struct {
	OKCount        int
	DriftedCount   int
	FixFailedCount int
	LastCheckedAt  *time.Time
}

// Upsert records a Check result. Used by the sweep path — never touches
// fixed_at.
func (s *Store) Upsert(ctx context.Context, deploymentID, evaluatorID string, status Status, detail string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deployment_evaluator_state (deployment_id, evaluator_id, status, detail, checked_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (deployment_id, evaluator_id) DO UPDATE
		SET status = EXCLUDED.status, detail = EXCLUDED.detail, checked_at = EXCLUDED.checked_at
	`, deploymentID, evaluatorID, status, detail)
	if err != nil {
		return fmt.Errorf("deployeval: upsert: %w", err)
	}
	return nil
}

// UpsertAfterFix records the Check result taken immediately after a Fix
// attempt, stamping fixed_at so the UI can show when a fix was last tried —
// regardless of whether the resulting status is "ok" (fix worked) or
// "fix_failed"/"drifted" (it didn't fully resolve).
func (s *Store) UpsertAfterFix(ctx context.Context, deploymentID, evaluatorID string, status Status, detail string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deployment_evaluator_state (deployment_id, evaluator_id, status, detail, checked_at, fixed_at)
		VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (deployment_id, evaluator_id) DO UPDATE
		SET status = EXCLUDED.status, detail = EXCLUDED.detail, checked_at = EXCLUDED.checked_at, fixed_at = now()
	`, deploymentID, evaluatorID, status, detail)
	if err != nil {
		return fmt.Errorf("deployeval: upsert after fix: %w", err)
	}
	return nil
}

// ListDrifted returns every deployment currently drifted or fix_failed for
// one evaluator, most recently checked first.
func (s *Store) ListDrifted(ctx context.Context, evaluatorID string) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT deployment_id, evaluator_id, status, detail, checked_at, fixed_at
		FROM deployment_evaluator_state
		WHERE evaluator_id = $1 AND status <> 'ok'
		ORDER BY checked_at DESC
	`, evaluatorID)
	if err != nil {
		return nil, fmt.Errorf("deployeval: list drifted: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []Row
	for rows.Next() {
		var r Row
		var fixedAt sql.NullTime
		if err := rows.Scan(&r.DeploymentID, &r.EvaluatorID, &r.Status, &r.Detail, &r.CheckedAt, &fixedAt); err != nil {
			return nil, fmt.Errorf("deployeval: scan drifted row: %w", err)
		}
		if fixedAt.Valid {
			r.FixedAt = &fixedAt.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Summarize aggregates one evaluator's state across every deployment it's
// been run against.
func (s *Store) Summarize(ctx context.Context, evaluatorID string) (Summary, error) {
	var sum Summary
	var lastChecked sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'ok'),
			COUNT(*) FILTER (WHERE status = 'drifted'),
			COUNT(*) FILTER (WHERE status = 'fix_failed'),
			MAX(checked_at)
		FROM deployment_evaluator_state
		WHERE evaluator_id = $1
	`, evaluatorID).Scan(&sum.OKCount, &sum.DriftedCount, &sum.FixFailedCount, &lastChecked)
	if err != nil {
		return Summary{}, fmt.Errorf("deployeval: summarize: %w", err)
	}
	if lastChecked.Valid {
		sum.LastCheckedAt = &lastChecked.Time
	}
	return sum, nil
}
