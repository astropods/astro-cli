package judgmentstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type PredictionRequestStatus string

const (
	PredictionRequestQueued     PredictionRequestStatus = "queued"
	PredictionRequestInProgress PredictionRequestStatus = "in_progress"
	PredictionRequestCompleted  PredictionRequestStatus = "completed"
	PredictionRequestFailed     PredictionRequestStatus = "failed"
)

func (s PredictionRequestStatus) Valid() bool {
	switch s {
	case PredictionRequestQueued, PredictionRequestInProgress, PredictionRequestCompleted, PredictionRequestFailed:
		return true
	default:
		return false
	}
}

// PredictionRequest is the current asynchronous generation state for one
// dataset trace. It intentionally does not retain attempt history.
type PredictionRequest struct {
	TraceID      string
	Status       PredictionRequestStatus
	ErrorMessage *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// GetPredictionRequests returns request state keyed by trace ID.
func (s *Store) GetPredictionRequests(ctx context.Context, evalDatasetID string, traceIDs []string) (map[string]PredictionRequest, error) {
	out := make(map[string]PredictionRequest, len(traceIDs))
	if len(traceIDs) == 0 {
		return out, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT trace_id, status, error_message, created_at, updated_at
		FROM eval_dataset_prediction_requests
		WHERE eval_dataset_id = $1 AND trace_id = ANY($2)
	`, evalDatasetID, pq.Array(traceIDs))
	if err != nil {
		return nil, fmt.Errorf("judgmentstore get prediction requests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			request      PredictionRequest
			errorMessage sql.NullString
		)
		if err := rows.Scan(
			&request.TraceID,
			&request.Status,
			&errorMessage,
			&request.CreatedAt,
			&request.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("judgmentstore get prediction requests scan: %w", err)
		}
		if errorMessage.Valid {
			request.ErrorMessage = &errorMessage.String
		}
		out[request.TraceID] = request
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("judgmentstore get prediction requests iter: %w", err)
	}
	return out, nil
}

// QueuePredictionRequests inserts or requeues current state for a batch and
// returns the trace IDs whose rows were inserted or reset. Active queued and
// in-progress rows are preserved and omitted from the result.
func (s *Store) QueuePredictionRequests(ctx context.Context, evalDatasetID string, traceIDs []string) ([]string, error) {
	if len(traceIDs) == 0 {
		return []string{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		INSERT INTO eval_dataset_prediction_requests (eval_dataset_id, trace_id)
		SELECT $1, unnest($2::text[])
		ON CONFLICT (eval_dataset_id, trace_id) DO UPDATE SET
			status = 'queued',
			error_message = NULL,
			updated_at = now()
		WHERE eval_dataset_prediction_requests.status IN ('completed', 'failed')
		RETURNING trace_id
	`, evalDatasetID, pq.Array(traceIDs))
	if err != nil {
		return nil, fmt.Errorf("judgmentstore queue prediction requests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	queuedTraceIDs := make([]string, 0, len(traceIDs))
	for rows.Next() {
		var traceID string
		if err := rows.Scan(&traceID); err != nil {
			return nil, fmt.Errorf("judgmentstore queue prediction requests scan: %w", err)
		}
		queuedTraceIDs = append(queuedTraceIDs, traceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("judgmentstore queue prediction requests iter: %w", err)
	}
	return queuedTraceIDs, nil
}

// UpdatePredictionRequest replaces the current lifecycle state and optional
// server-controlled failure message.
func (s *Store) UpdatePredictionRequest(
	ctx context.Context,
	evalDatasetID, traceID string,
	status PredictionRequestStatus,
	errorMessage *string,
) error {
	if !status.Valid() {
		return fmt.Errorf("judgmentstore update prediction request: invalid status %q", status)
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE eval_dataset_prediction_requests
		SET status = $3, error_message = $4, updated_at = now()
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID, string(status), errorMessage)
	if err != nil {
		return fmt.Errorf("judgmentstore update prediction request: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("judgmentstore update prediction request rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("judgmentstore update prediction request: request not found")
	}
	return nil
}

// UpdatePredictionRequests replaces lifecycle state for a batch.
func (s *Store) UpdatePredictionRequests(
	ctx context.Context,
	evalDatasetID string,
	traceIDs []string,
	status PredictionRequestStatus,
	errorMessage *string,
) error {
	if len(traceIDs) == 0 {
		return nil
	}
	if !status.Valid() {
		return fmt.Errorf("judgmentstore update prediction requests: invalid status %q", status)
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE eval_dataset_prediction_requests
		SET status = $3, error_message = $4, updated_at = now()
		WHERE eval_dataset_id = $1 AND trace_id = ANY($2)
	`, evalDatasetID, pq.Array(traceIDs), string(status), errorMessage)
	if err != nil {
		return fmt.Errorf("judgmentstore update prediction requests: %w", err)
	}
	return nil
}

// DeletePredictionRequest idempotently removes obsolete current state.
func (s *Store) DeletePredictionRequest(ctx context.Context, evalDatasetID, traceID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM eval_dataset_prediction_requests
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID)
	if err != nil {
		return fmt.Errorf("judgmentstore delete prediction request: %w", err)
	}
	return nil
}
