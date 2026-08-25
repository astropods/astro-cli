package evalrunstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	ErrorMessage  string
}

type Result struct {
	EvaluatorKey string
	Status       Status
	Value        any
	Confidence   float64
	Explanation  string
	ErrorMessage string
}

type StatusCounts struct {
	Queued     int
	InProgress int
	Completed  int
	Failed     int
}

type RunTrace struct {
	TraceID        string
	TraceTimestamp time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// StartRun claims the run the request recorded, returning nil when the trace
// holds no active one. A retry re-claims the attempt it left in progress.
func (s *Store) StartRun(
	ctx context.Context,
	evalDatasetID, traceID, evaluationRef string,
) (*Run, error) {
	run := Run{
		EvalDatasetID: evalDatasetID,
		TraceID:       traceID,
		EvaluationRef: evaluationRef,
		Status:        StatusInProgress,
	}
	err := s.db.QueryRowContext(ctx, `
		UPDATE eval_dataset_evaluation_runs
		SET status = 'in_progress', updated_at = now()
		WHERE eval_dataset_id = $1
		  AND trace_id = $2
		  AND evaluation_ref = $3
		  AND status IN ('queued', 'in_progress')
		RETURNING id
	`, evalDatasetID, traceID, evaluationRef).Scan(&run.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evalrunstore start run: %w", err)
	}
	return &run, nil
}

// CreateQueuedRuns records one run per trace before any worker picks them up,
// returning the trace IDs that now have an active run.
func (s *Store) CreateQueuedRuns(
	ctx context.Context,
	evalDatasetID, evaluationRef string,
	traces []RunTrace,
) ([]string, error) {
	if len(traces) == 0 {
		return nil, nil
	}
	traceIDs := make([]string, 0, len(traces))
	timestamps := make([]time.Time, 0, len(traces))
	for _, trace := range traces {
		traceIDs = append(traceIDs, trace.TraceID)
		timestamps = append(timestamps, trace.TraceTimestamp)
	}
	rows, err := s.db.QueryContext(ctx, `
		INSERT INTO eval_dataset_evaluation_runs (
			eval_dataset_id, trace_id, trace_timestamp, evaluation_ref, status
		)
		SELECT $1, queued.trace_id, queued.trace_timestamp, $2, 'queued'
		FROM unnest($3::text[], $4::timestamptz[]) AS queued(trace_id, trace_timestamp)
		ON CONFLICT (eval_dataset_id, trace_id, evaluation_ref)
			WHERE status IN ('queued', 'in_progress')
		DO UPDATE SET updated_at = now()
		RETURNING trace_id
	`, evalDatasetID, evaluationRef, pq.Array(traceIDs), pq.Array(timestamps))
	if err != nil {
		return nil, fmt.Errorf("evalrunstore create queued runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]string, 0, len(traces))
	for rows.Next() {
		var traceID string
		if err := rows.Scan(&traceID); err != nil {
			return nil, fmt.Errorf("evalrunstore create queued runs scan: %w", err)
		}
		out = append(out, traceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evalrunstore create queued runs iter: %w", err)
	}
	return out, nil
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

// FailQueuedRuns finalizes runs whose job never reached the queue. It touches
// only rows still queued, so a worker that already picked one up keeps it.
func (s *Store) FailQueuedRuns(
	ctx context.Context,
	evalDatasetID, evaluationRef string,
	traceIDs []string,
	message string,
) error {
	if len(traceIDs) == 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE eval_dataset_evaluation_runs
		SET status = 'failed', error_message = $4, updated_at = now()
		WHERE eval_dataset_id = $1
		  AND evaluation_ref = $2
		  AND trace_id = ANY($3)
		  AND status = 'queued'
	`, evalDatasetID, evaluationRef, pq.Array(traceIDs), message); err != nil {
		return fmt.Errorf("evalrunstore fail queued runs: %w", err)
	}
	return nil
}

// GetRun returns the run, or nil when no run carries the id.
func (s *Store) GetRun(ctx context.Context, runID string) (*Run, error) {
	var run Run
	var message sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, eval_dataset_id, trace_id, evaluation_ref, status, error_message
		FROM eval_dataset_evaluation_runs
		WHERE id = $1
	`, runID).Scan(&run.ID, &run.EvalDatasetID, &run.TraceID, &run.EvaluationRef, &run.Status, &message)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evalrunstore get run: %w", err)
	}
	run.ErrorMessage = message.String
	return &run, nil
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

func (s *Store) StatusCounts(ctx context.Context, evalDatasetID string) (StatusCounts, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT status, COUNT(*)
		FROM (
			SELECT DISTINCT ON (trace_id) status
			FROM eval_dataset_evaluation_runs
			WHERE eval_dataset_id = $1
			ORDER BY trace_id, created_at DESC
		) latest
		GROUP BY status
	`, evalDatasetID)
	if err != nil {
		return StatusCounts{}, fmt.Errorf("evalrunstore status counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var counts StatusCounts
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return StatusCounts{}, fmt.Errorf("evalrunstore status counts scan: %w", err)
		}
		switch Status(status) {
		case StatusQueued:
			counts.Queued = count
		case StatusInProgress:
			counts.InProgress = count
		case StatusCompleted:
			counts.Completed = count
		case StatusFailed:
			counts.Failed = count
		}
	}
	if err := rows.Err(); err != nil {
		return StatusCounts{}, fmt.Errorf("evalrunstore status counts iter: %w", err)
	}
	return counts, nil
}

func (s *Store) LatestRuns(ctx context.Context, evalDatasetID string, traceIDs []string) (map[string]Run, error) {
	if len(traceIDs) == 0 {
		return map[string]Run{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ON (trace_id) trace_id, id, evaluation_ref, status, error_message
		FROM eval_dataset_evaluation_runs
		WHERE eval_dataset_id = $1 AND trace_id = ANY($2)
		ORDER BY trace_id, created_at DESC
	`, evalDatasetID, pq.Array(traceIDs))
	if err != nil {
		return nil, fmt.Errorf("evalrunstore latest runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]Run{}
	for rows.Next() {
		var run Run
		var message sql.NullString
		if err := rows.Scan(&run.TraceID, &run.ID, &run.EvaluationRef, &run.Status, &message); err != nil {
			return nil, fmt.Errorf("evalrunstore latest runs scan: %w", err)
		}
		run.EvalDatasetID = evalDatasetID
		run.ErrorMessage = message.String
		out[run.TraceID] = run
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evalrunstore latest runs iter: %w", err)
	}
	return out, nil
}

func (s *Store) TracesWithCompletedRuns(
	ctx context.Context,
	evalDatasetID string,
	startTime, endTime time.Time,
	before *RunTrace,
	limit int,
) ([]RunTrace, error) {
	var beforeTime any
	var beforeTrace any
	if before != nil {
		beforeTime = before.TraceTimestamp
		beforeTrace = before.TraceID
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT trace_id, trace_timestamp
		FROM (
			SELECT DISTINCT ON (trace_id) trace_id, trace_timestamp, status
			FROM eval_dataset_evaluation_runs
			WHERE eval_dataset_id = $1
			ORDER BY trace_id, created_at DESC
		) latest
		WHERE status = 'completed'
		  AND trace_timestamp >= $2
		  AND trace_timestamp <= $3
		  AND ($4::timestamptz IS NULL OR (trace_timestamp, trace_id) < ($4, $5))
		ORDER BY trace_timestamp DESC, trace_id DESC
		LIMIT $6
	`, evalDatasetID, startTime, endTime, beforeTime, beforeTrace, limit)
	if err != nil {
		return nil, fmt.Errorf("evalrunstore traces with completed runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]RunTrace, 0, limit)
	for rows.Next() {
		var trace RunTrace
		if err := rows.Scan(&trace.TraceID, &trace.TraceTimestamp); err != nil {
			return nil, fmt.Errorf("evalrunstore traces with completed runs scan: %w", err)
		}
		out = append(out, trace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evalrunstore traces with completed runs iter: %w", err)
	}
	return out, nil
}

func (s *Store) EvaluatorResults(ctx context.Context, runID string) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT evaluator_key, status, value_json, confidence, explanation, error_message
		FROM eval_dataset_evaluator_results
		WHERE evaluation_run_id = $1
		ORDER BY evaluator_key
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("evalrunstore evaluator results: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Result{}
	for rows.Next() {
		var result Result
		var value []byte
		var confidence sql.NullFloat64
		var explanation, message sql.NullString
		if err := rows.Scan(&result.EvaluatorKey, &result.Status, &value, &confidence, &explanation, &message); err != nil {
			return nil, fmt.Errorf("evalrunstore evaluator results scan: %w", err)
		}
		if len(value) > 0 {
			if err := json.Unmarshal(value, &result.Value); err != nil {
				return nil, fmt.Errorf("evalrunstore evaluator results unmarshal %q: %w", result.EvaluatorKey, err)
			}
		}
		result.Confidence = confidence.Float64
		result.Explanation = explanation.String
		result.ErrorMessage = message.String
		out = append(out, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evalrunstore evaluator results iter: %w", err)
	}
	return out, nil
}
