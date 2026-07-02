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

// CriterionDimension is a fixed quality axis a reviewer can cite when marking a
// trace good or bad. The set is code-owned and validated by the server.
type CriterionDimension string

const (
	DimensionAccuracy             CriterionDimension = "accuracy"
	DimensionCompleteness         CriterionDimension = "completeness"
	DimensionInstructionFollowing CriterionDimension = "instruction_following"
	DimensionScopeClarity         CriterionDimension = "scope_clarity"
	DimensionTone                 CriterionDimension = "tone"
)

// CriterionDimensions is the ordered set of valid dimensions.
var CriterionDimensions = []CriterionDimension{
	DimensionAccuracy,
	DimensionCompleteness,
	DimensionInstructionFollowing,
	DimensionScopeClarity,
	DimensionTone,
}

func (d CriterionDimension) Valid() bool {
	switch d {
	case DimensionAccuracy, DimensionCompleteness, DimensionInstructionFollowing, DimensionScopeClarity, DimensionTone:
		return true
	default:
		return false
	}
}

// CriterionCounts is the aggregate good/bad count for one criterion dimension.
type CriterionCounts struct {
	Dimension CriterionDimension
	GoodCount int
	BadCount  int
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

// DeleteReturningVerdict removes a judgment row and returns the verdict that
// was stored on it. The boolean is false when the trace was not judged for the
// dataset.
func (s *Store) DeleteReturningVerdict(evalDatasetID, traceID string) (Verdict, bool, error) {
	var raw string
	err := s.db.QueryRow(`
		DELETE FROM eval_dataset_judgments
		WHERE eval_dataset_id = $1 AND trace_id = $2
		RETURNING verdict
	`, evalDatasetID, traceID).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("judgmentstore delete returning verdict: %w", err)
	}
	verdict := Verdict(raw)
	if !verdict.Valid() {
		return "", false, fmt.Errorf("judgmentstore delete returning verdict: invalid verdict %q", raw)
	}
	return verdict, true, nil
}

// SetVerdictAndReasons sets a judgment's verdict and, when the verdict changes,
// replaces its reasons with the given set in one transaction. It returns the
// previous verdict and the reasons it replaced so the same call can reverse the
// change on rollback. found is false when the trace has no judgment row.
func (s *Store) SetVerdictAndReasons(evalDatasetID, traceID string, verdict Verdict, reasons []Reason) (Verdict, []Reason, bool, error) {
	if !verdict.Valid() {
		return "", nil, false, fmt.Errorf("judgmentstore set verdict: invalid verdict %q", verdict)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", nil, false, fmt.Errorf("judgmentstore set verdict: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var raw string
	err = tx.QueryRow(`
		WITH previous AS (
			SELECT verdict
			FROM eval_dataset_judgments
			WHERE eval_dataset_id = $1 AND trace_id = $2
		),
		updated AS (
			UPDATE eval_dataset_judgments
			SET verdict = $3
			WHERE eval_dataset_id = $1 AND trace_id = $2
			RETURNING 1
		)
		SELECT previous.verdict
		FROM previous, updated
	`, evalDatasetID, traceID, string(verdict)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("judgmentstore set verdict: %w", err)
	}
	previous := Verdict(raw)
	if !previous.Valid() {
		return "", nil, false, fmt.Errorf("judgmentstore set verdict: invalid previous verdict %q", raw)
	}

	var replaced []Reason
	if previous != verdict {
		rows, err := tx.Query(`
			DELETE FROM eval_dataset_judgment_reasons
			WHERE eval_dataset_id = $1 AND trace_id = $2
			RETURNING dimension_key, dimension_value
		`, evalDatasetID, traceID)
		if err != nil {
			return "", nil, false, fmt.Errorf("judgmentstore set verdict clear reasons: %w", err)
		}
		for rows.Next() {
			var (
				key string
				val float64
			)
			if err := rows.Scan(&key, &val); err != nil {
				_ = rows.Close()
				return "", nil, false, fmt.Errorf("judgmentstore set verdict scan reasons: %w", err)
			}
			replaced = append(replaced, Reason{Dimension: CriterionDimension(key), Value: val})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return "", nil, false, fmt.Errorf("judgmentstore set verdict iter reasons: %w", err)
		}
		_ = rows.Close()

		if len(reasons) > 0 {
			keys := make([]string, len(reasons))
			vals := make([]float64, len(reasons))
			for i, r := range reasons {
				keys[i] = string(r.Dimension)
				vals[i] = r.Value
			}
			if _, err := tx.Exec(`
				INSERT INTO eval_dataset_judgment_reasons (eval_dataset_id, trace_id, dimension_key, dimension_value)
				SELECT $1, $2, unnest($3::text[]), unnest($4::numeric[])
			`, evalDatasetID, traceID, pq.Array(keys), pq.Array(vals)); err != nil {
				return "", nil, false, fmt.Errorf("judgmentstore set verdict insert reasons: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return "", nil, false, fmt.Errorf("judgmentstore set verdict commit: %w", err)
	}
	return previous, replaced, true, nil
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

// CriterionCounts returns selected criterion counts for a dataset, grouped by
// dimension. Positive values count as good and negative values count as bad.
func (s *Store) CriterionCounts(evalDatasetID string) ([]CriterionCounts, error) {
	rows, err := s.db.Query(`
		SELECT
			dimension_key,
			COUNT(*) FILTER (WHERE dimension_value > 0) AS good_count,
			COUNT(*) FILTER (WHERE dimension_value < 0) AS bad_count
		FROM eval_dataset_judgment_reasons
		WHERE eval_dataset_id = $1
		GROUP BY dimension_key
		ORDER BY dimension_key
	`, evalDatasetID)
	if err != nil {
		return nil, fmt.Errorf("judgmentstore criterion counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []CriterionCounts
	for rows.Next() {
		var (
			key  string
			good int64
			bad  int64
		)
		if err := rows.Scan(&key, &good, &bad); err != nil {
			return nil, fmt.Errorf("judgmentstore criterion counts scan: %w", err)
		}
		out = append(out, CriterionCounts{
			Dimension: CriterionDimension(key),
			GoodCount: int(good),
			BadCount:  int(bad),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("judgmentstore criterion counts iter: %w", err)
	}
	return out, nil
}

// Reason is one selected criterion on a judgment: a dimension and the value
// captured at judgment time.
type Reason struct {
	Dimension CriterionDimension
	Value     float64
}
