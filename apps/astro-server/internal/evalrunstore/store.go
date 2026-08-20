package evalrunstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type Status string

const (
	StatusQueued     Status = "queued"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

type Run struct {
	ID            string
	EvalDatasetID string
	TraceID       string
	EvaluationRef string
	Status        Status
}

type Result struct {
	EvaluatorKey string
	Status       Status
	Value        any
	Confidence   float64
	Explanation  string
	ErrorMessage string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// EnsureRun starts an attempt, or returns the attempt already in flight.
func (s *Store) EnsureRun(
	ctx context.Context,
	evalDatasetID, traceID, evaluationRef string,
	traceTimestamp time.Time,
) (*Run, error) {
	run := &Run{
		EvalDatasetID: evalDatasetID,
		TraceID:       traceID,
		EvaluationRef: evaluationRef,
	}

	// DO NOTHING would return no row on conflict, leaving the caller without an id.
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO eval_dataset_evaluation_runs (
			eval_dataset_id, trace_id, trace_timestamp, evaluation_ref, status
		)
		VALUES ($1, $2, $3, $4, 'in_progress')
		ON CONFLICT (eval_dataset_id, trace_id, evaluation_ref)
			WHERE status IN ('queued', 'in_progress')
		DO UPDATE SET updated_at = now()
		RETURNING id, status
	`, evalDatasetID, traceID, traceTimestamp, evaluationRef).Scan(&run.ID, &run.Status); err != nil {
		return nil, fmt.Errorf("evalrunstore ensure run: %w", err)
	}
	return run, nil
}

func (s *Store) CreateResults(ctx context.Context, runID string, evaluatorKeys []string) error {
	if len(evaluatorKeys) == 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO eval_dataset_evaluator_results (evaluation_run_id, evaluator_key)
		SELECT $1, unnest($2::text[])
		ON CONFLICT (evaluation_run_id, evaluator_key) DO NOTHING
	`, runID, pq.Array(evaluatorKeys)); err != nil {
		return fmt.Errorf("evalrunstore create results: %w", err)
	}
	return nil
}

func (s *Store) MarkResultInProgress(ctx context.Context, runID, evaluatorKey string) error {
	return s.updateResult(ctx, runID, evaluatorKey, `
		UPDATE eval_dataset_evaluator_results
		SET status = 'in_progress', error_message = NULL
		WHERE evaluation_run_id = $1 AND evaluator_key = $2
	`)
}

func (s *Store) CompleteResult(ctx context.Context, runID string, result Result) error {
	value, err := json.Marshal(result.Value)
	if err != nil {
		return fmt.Errorf("evalrunstore complete result: marshal value: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE eval_dataset_evaluator_results
		SET status = 'completed', value_json = $3, confidence = $4, explanation = $5, error_message = NULL
		WHERE evaluation_run_id = $1 AND evaluator_key = $2
	`, runID, result.EvaluatorKey, value, result.Confidence, result.Explanation)
	if err != nil {
		return fmt.Errorf("evalrunstore complete result: %w", err)
	}
	return affectedOne(res, "complete result", result.EvaluatorKey)
}

func (s *Store) FailResult(ctx context.Context, runID, evaluatorKey, message string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE eval_dataset_evaluator_results
		SET status = 'failed', error_message = $3
		WHERE evaluation_run_id = $1 AND evaluator_key = $2
	`, runID, evaluatorKey, message)
	if err != nil {
		return fmt.Errorf("evalrunstore fail result: %w", err)
	}
	return affectedOne(res, "fail result", evaluatorKey)
}

// FailPendingResults marks every result that has not reached a terminal state,
// so a failed run leaves no row that still reads as pending.
func (s *Store) FailPendingResults(ctx context.Context, runID, message string) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE eval_dataset_evaluator_results
		SET status = 'failed', error_message = $2
		WHERE evaluation_run_id = $1 AND status IN ('queued', 'in_progress')
	`, runID, message); err != nil {
		return fmt.Errorf("evalrunstore fail pending results: %w", err)
	}
	return nil
}

func (s *Store) CompletedResultKeys(ctx context.Context, runID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT evaluator_key
		FROM eval_dataset_evaluator_results
		WHERE evaluation_run_id = $1 AND status = 'completed'
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("evalrunstore completed result keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]bool{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("evalrunstore completed result keys scan: %w", err)
		}
		out[key] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evalrunstore completed result keys iter: %w", err)
	}
	return out, nil
}

// FinalizeRun writes the run's terminal state, rejecting a non-terminal one.
func (s *Store) FinalizeRun(ctx context.Context, runID string, status Status, errorMessage *string) error {
	if status != StatusCompleted && status != StatusFailed {
		return fmt.Errorf("evalrunstore finalize run: %q is not terminal", status)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE eval_dataset_evaluation_runs
		SET status = $2, error_message = $3, updated_at = now()
		WHERE id = $1
	`, runID, string(status), errorMessage)
	if err != nil {
		return fmt.Errorf("evalrunstore finalize run: %w", err)
	}
	return affectedOne(res, "finalize run", runID)
}

func (s *Store) updateResult(ctx context.Context, runID, evaluatorKey, query string) error {
	res, err := s.db.ExecContext(ctx, query, runID, evaluatorKey)
	if err != nil {
		return fmt.Errorf("evalrunstore update result: %w", err)
	}
	return affectedOne(res, "update result", evaluatorKey)
}

func affectedOne(res sql.Result, action, subject string) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("evalrunstore %s rows affected: %w", action, err)
	}
	if affected == 0 {
		return fmt.Errorf("evalrunstore %s: %q not found", action, subject)
	}
	return nil
}
