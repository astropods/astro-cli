package evalitemstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	ErrAlreadyAdded = errors.New("trace already in the dataset")
	ErrItemNotFound = errors.New("trace not in the dataset")
)

type Item struct {
	EvalDatasetID         string
	TraceID               string
	EvaluationRef         string
	SourceEvaluationRunID *string
	VerifiedByUserID      string
}

type Output struct {
	EvaluatorKey string          `json:"key"`
	Value        json.RawMessage `json:"value"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Add records dataset membership and its evaluator outputs together, so a trace
// is never a dataset item without the values it was admitted on.
func (s *Store) Add(ctx context.Context, item Item, outputs []Output) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("evalitemstore add: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO eval_dataset_items (
			eval_dataset_id, trace_id, evaluation_ref, source_evaluation_run_id, verified_by_user_id
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (eval_dataset_id, trace_id) DO NOTHING
	`, item.EvalDatasetID, item.TraceID, item.EvaluationRef, item.SourceEvaluationRunID, item.VerifiedByUserID)
	if err != nil {
		return fmt.Errorf("evalitemstore add item: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("evalitemstore add item rows affected: %w", err)
	}
	if affected == 0 {
		return ErrAlreadyAdded
	}

	if err := insertOutputs(ctx, tx, item.EvalDatasetID, item.TraceID, outputs); err != nil {
		return fmt.Errorf("evalitemstore add outputs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("evalitemstore add: commit: %w", err)
	}
	return nil
}

// Get returns nil when the trace is not in the dataset.
func (s *Store) Get(ctx context.Context, evalDatasetID, traceID string) (*Item, error) {
	item := Item{EvalDatasetID: evalDatasetID, TraceID: traceID}
	var runID, verifiedBy sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT evaluation_ref, source_evaluation_run_id, verified_by_user_id
		FROM eval_dataset_items
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID).Scan(&item.EvaluationRef, &runID, &verifiedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("evalitemstore get: %w", err)
	}
	if runID.Valid {
		item.SourceEvaluationRunID = &runID.String
	}
	item.VerifiedByUserID = verifiedBy.String
	return &item, nil
}

func (s *Store) ReplaceOutputs(ctx context.Context, evalDatasetID, traceID, verifiedByUserID string, outputs []Output) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("evalitemstore replace outputs: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Also locks the item so a concurrent remove cannot orphan the new rows.
	res, err := tx.ExecContext(ctx, `
		UPDATE eval_dataset_items
		SET verified_by_user_id = $3, updated_at = now()
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID, verifiedByUserID)
	if err != nil {
		return fmt.Errorf("evalitemstore replace outputs stamp item: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("evalitemstore replace outputs rows affected: %w", err)
	}
	if affected == 0 {
		return ErrItemNotFound
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM eval_dataset_item_evaluator_outputs
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID); err != nil {
		return fmt.Errorf("evalitemstore replace outputs clear: %w", err)
	}

	if err := insertOutputs(ctx, tx, evalDatasetID, traceID, outputs); err != nil {
		return fmt.Errorf("evalitemstore replace outputs insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("evalitemstore replace outputs: commit: %w", err)
	}
	return nil
}

func insertOutputs(ctx context.Context, tx *sql.Tx, evalDatasetID, traceID string, outputs []Output) error {
	if len(outputs) == 0 {
		return nil
	}
	payload, err := json.Marshal(outputs)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO eval_dataset_item_evaluator_outputs (
			eval_dataset_id, trace_id, evaluator_key, value_json
		)
		SELECT $1, $2, output->>'key', output->'value'
		FROM jsonb_array_elements($3::jsonb) AS output
	`, evalDatasetID, traceID, string(payload))
	return err
}

// Remove deletes dataset membership, returning the item and the outputs the
// delete cascaded away so a caller whose upstream write fails can restore both.
func (s *Store) Remove(ctx context.Context, evalDatasetID, traceID string) (*Item, []Output, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("evalitemstore remove: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT evaluator_key, value_json
		FROM eval_dataset_item_evaluator_outputs
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID)
	if err != nil {
		return nil, nil, fmt.Errorf("evalitemstore remove read outputs: %w", err)
	}
	outputs, err := scanOutputs(rows)
	if err != nil {
		return nil, nil, fmt.Errorf("evalitemstore remove read outputs: %w", err)
	}

	item := Item{EvalDatasetID: evalDatasetID, TraceID: traceID}
	var runID, verifiedBy sql.NullString
	err = tx.QueryRowContext(ctx, `
		DELETE FROM eval_dataset_items
		WHERE eval_dataset_id = $1 AND trace_id = $2
		RETURNING evaluation_ref, source_evaluation_run_id, verified_by_user_id
	`, evalDatasetID, traceID).Scan(&item.EvaluationRef, &runID, &verifiedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrItemNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("evalitemstore remove item: %w", err)
	}
	if runID.Valid {
		item.SourceEvaluationRunID = &runID.String
	}
	item.VerifiedByUserID = verifiedBy.String

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("evalitemstore remove: commit: %w", err)
	}
	return &item, outputs, nil
}

func scanOutputs(rows *sql.Rows) ([]Output, error) {
	defer rows.Close() //nolint:errcheck

	var outputs []Output
	for rows.Next() {
		var output Output
		if err := rows.Scan(&output.EvaluatorKey, &output.Value); err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	return outputs, rows.Err()
}
