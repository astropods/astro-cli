package judgmentstore

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

type Verdict string

const (
	VerdictGood    Verdict = "good"
	VerdictBad     Verdict = "bad"
	VerdictUnknown Verdict = "unknown"
)

func (v Verdict) Valid() bool {
	return v == VerdictGood || v == VerdictBad || v == VerdictUnknown
}

// ErrAlreadyJudged is returned by Insert when (eval_dataset_id, trace_id) is already present.
var ErrAlreadyJudged = errors.New("trace already judged")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Insert records a verdict for a trace. Returns ErrAlreadyJudged if a row already exists.
func (s *Store) Insert(evalDatasetID, traceID string, verdict Verdict) error {
	if !verdict.Valid() {
		return fmt.Errorf("judgmentstore insert: invalid verdict %q", verdict)
	}

	res, err := s.db.Exec(`
		INSERT INTO eval_dataset_judgments (eval_dataset_id, trace_id, verdict)
		VALUES ($1, $2, $3)
		ON CONFLICT (eval_dataset_id, trace_id) DO NOTHING
	`, evalDatasetID, traceID, string(verdict))
	if err != nil {
		return fmt.Errorf("judgmentstore insert: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("judgmentstore insert rows affected: %w", err)
	}
	if n == 0 {
		return ErrAlreadyJudged
	}
	return nil
}

// Delete removes a judgment row. It is used as best-effort compensation when
// the local duplicate gate succeeds but a later upstream write fails.
func (s *Store) Delete(evalDatasetID, traceID string) error {
	_, err := s.db.Exec(`
		DELETE FROM eval_dataset_judgments
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID)
	if err != nil {
		return fmt.Errorf("judgmentstore delete: %w", err)
	}
	return nil
}

// JudgedTraceIDs returns the subset of the input trace_ids that already have a judgment row.
func (s *Store) JudgedTraceIDs(evalDatasetID string, traceIDs []string) (map[string]bool, error) {
	if len(traceIDs) == 0 {
		return map[string]bool{}, nil
	}
	rows, err := s.db.Query(`
		SELECT trace_id
		FROM eval_dataset_judgments
		WHERE eval_dataset_id = $1 AND trace_id = ANY($2)
	`, evalDatasetID, pq.Array(traceIDs))
	if err != nil {
		return nil, fmt.Errorf("judgmentstore judged: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]bool, len(traceIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("judgmentstore judged scan: %w", err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("judgmentstore judged iter: %w", err)
	}
	return out, nil
}
