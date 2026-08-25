package evalitemstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrAlreadyAdded = errors.New("trace already in the dataset")

type Item struct {
	EvalDatasetID         string
	TraceID               string
	EvaluationRef         string
	SourceEvaluationRunID *string
	AddedByUserID         string
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
	payload, err := json.Marshal(outputs)
	if err != nil {
		return fmt.Errorf("evalitemstore add: marshal outputs: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("evalitemstore add: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO eval_dataset_items (
			eval_dataset_id, trace_id, evaluation_ref, source_evaluation_run_id, added_by_user_id
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (eval_dataset_id, trace_id) DO NOTHING
	`, item.EvalDatasetID, item.TraceID, item.EvaluationRef, item.SourceEvaluationRunID, item.AddedByUserID)
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

	if len(outputs) > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO eval_dataset_item_evaluator_outputs (
				eval_dataset_id, trace_id, evaluator_key, value_json
			)
			SELECT $1, $2, output->>'key', output->'value'
			FROM jsonb_array_elements($3::jsonb) AS output
		`, item.EvalDatasetID, item.TraceID, string(payload)); err != nil {
			return fmt.Errorf("evalitemstore add outputs: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("evalitemstore add: commit: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, evalDatasetID, traceID string) error {
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM eval_dataset_items
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID); err != nil {
		return fmt.Errorf("evalitemstore delete: %w", err)
	}
	return nil
}
