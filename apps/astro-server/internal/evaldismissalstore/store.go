package evaldismissalstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

var ErrIsDatasetItem = errors.New("trace is a dataset item")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Dismiss records the dismissal, rejecting a trace that is already a
// committed dataset item. A racing Add can still slip past this check;
// evalitemstore.Remove clears the stray dismissal when that happens.
// Dismissing twice succeeds.
func (s *Store) Dismiss(ctx context.Context, evalDatasetID, traceID string) error {
	var isItem bool
	if err := s.db.QueryRowContext(ctx, `
		WITH inserted AS (
			INSERT INTO eval_dataset_dismissed_traces (eval_dataset_id, trace_id)
			SELECT $1, $2
			WHERE NOT EXISTS (
				SELECT 1
				FROM eval_dataset_items
				WHERE eval_dataset_id = $1 AND trace_id = $2
			)
			ON CONFLICT (eval_dataset_id, trace_id) DO NOTHING
			RETURNING 1
		)
		SELECT EXISTS (
			SELECT 1
			FROM eval_dataset_items
			WHERE eval_dataset_id = $1 AND trace_id = $2
		)
	`, evalDatasetID, traceID).Scan(&isItem); err != nil {
		return fmt.Errorf("evaldismissalstore dismiss: %w", err)
	}
	if !isItem {
		return nil
	}
	return ErrIsDatasetItem
}

// Restore returns the trace to the review queue. Restoring an undismissed trace
// succeeds.
func (s *Store) Restore(ctx context.Context, evalDatasetID, traceID string) error {
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM eval_dataset_dismissed_traces
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID); err != nil {
		return fmt.Errorf("evaldismissalstore restore: %w", err)
	}
	return nil
}

// DismissedTraceIDs returns the subset of the given trace IDs dismissed from the queue.
func (s *Store) DismissedTraceIDs(ctx context.Context, evalDatasetID string, traceIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(traceIDs))
	if len(traceIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT trace_id
		FROM eval_dataset_dismissed_traces
		WHERE eval_dataset_id = $1 AND trace_id = ANY($2)
	`, evalDatasetID, pq.Array(traceIDs))
	if err != nil {
		return nil, fmt.Errorf("evaldismissalstore dismissed trace ids: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var traceID string
		if err := rows.Scan(&traceID); err != nil {
			return nil, fmt.Errorf("evaldismissalstore dismissed trace ids scan: %w", err)
		}
		out[traceID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("evaldismissalstore dismissed trace ids iter: %w", err)
	}
	return out, nil
}
