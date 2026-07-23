package judgmentstore

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

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

// Prediction is an Astro-managed judge's stored prediction for one trace.
type Prediction struct {
	VerdictScore float64
	Confidence   int
	Explanation  string
	JudgeVersion string
	Criteria     []PredictionCriterion
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// PredictionCriterion is one predicted score on a server-owned criterion
// dimension.
type PredictionCriterion struct {
	Dimension CriterionDimension
	Value     float64
}

// ErrAlreadyJudged is returned by Insert when (eval_dataset_id, trace_id) is already present.
var ErrAlreadyJudged = errors.New("trace already judged")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// GetPredictions returns stored predictions for the requested trace IDs, keyed
// by trace ID. Criteria are ordered by dimension key.
func (s *Store) GetPredictions(evalDatasetID string, traceIDs []string) (map[string]Prediction, error) {
	out := make(map[string]Prediction, len(traceIDs))
	if len(traceIDs) == 0 {
		return out, nil
	}

	rows, err := s.db.Query(`
		SELECT p.trace_id, p.verdict_score, p.confidence, p.explanation,
		       p.judge_version, p.created_at, p.updated_at,
		       c.dimension_key, c.dimension_value
		FROM eval_dataset_judgment_predictions p
		LEFT JOIN eval_dataset_judgment_prediction_criteria c
		  ON c.eval_dataset_id = p.eval_dataset_id
		 AND c.trace_id = p.trace_id
		WHERE p.eval_dataset_id = $1 AND p.trace_id = ANY($2)
		ORDER BY p.trace_id, c.dimension_key
	`, evalDatasetID, pq.Array(traceIDs))
	if err != nil {
		return nil, fmt.Errorf("judgmentstore get predictions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			traceID        string
			prediction     Prediction
			dimensionKey   sql.NullString
			dimensionValue sql.NullFloat64
		)
		if err := rows.Scan(
			&traceID,
			&prediction.VerdictScore,
			&prediction.Confidence,
			&prediction.Explanation,
			&prediction.JudgeVersion,
			&prediction.CreatedAt,
			&prediction.UpdatedAt,
			&dimensionKey,
			&dimensionValue,
		); err != nil {
			return nil, fmt.Errorf("judgmentstore get predictions scan: %w", err)
		}

		if existing, ok := out[traceID]; ok {
			prediction.Criteria = existing.Criteria
		}
		if dimensionKey.Valid && dimensionValue.Valid {
			prediction.Criteria = append(prediction.Criteria, PredictionCriterion{
				Dimension: CriterionDimension(dimensionKey.String),
				Value:     dimensionValue.Float64,
			})
		}
		out[traceID] = prediction
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("judgmentstore get predictions iter: %w", err)
	}
	return out, nil
}

// UpsertPrediction stores a prediction and completely replaces its criteria in
// one transaction. An update preserves created_at and refreshes updated_at.
func (s *Store) UpsertPrediction(evalDatasetID, traceID string, prediction Prediction) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("judgmentstore upsert prediction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		INSERT INTO eval_dataset_judgment_predictions (
			eval_dataset_id, trace_id, verdict_score, confidence, explanation, judge_version
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (eval_dataset_id, trace_id) DO UPDATE SET
			verdict_score = EXCLUDED.verdict_score,
			confidence = EXCLUDED.confidence,
			explanation = EXCLUDED.explanation,
			judge_version = EXCLUDED.judge_version,
			updated_at = now()
	`, evalDatasetID, traceID, prediction.VerdictScore, prediction.Confidence, prediction.Explanation, prediction.JudgeVersion)
	if err != nil {
		return fmt.Errorf("judgmentstore upsert prediction row: %w", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM eval_dataset_judgment_prediction_criteria
		WHERE eval_dataset_id = $1 AND trace_id = $2
	`, evalDatasetID, traceID); err != nil {
		return fmt.Errorf("judgmentstore replace prediction criteria delete: %w", err)
	}

	if len(prediction.Criteria) > 0 {
		keys := make([]string, len(prediction.Criteria))
		values := make([]float64, len(prediction.Criteria))
		for i, criterion := range prediction.Criteria {
			keys[i] = string(criterion.Dimension)
			values[i] = criterion.Value
		}
		if _, err := tx.Exec(`
			INSERT INTO eval_dataset_judgment_prediction_criteria (
				eval_dataset_id, trace_id, dimension_key, dimension_value
			)
			SELECT $1, $2, unnest($3::text[]), unnest($4::numeric[])
		`, evalDatasetID, traceID, pq.Array(keys), pq.Array(values)); err != nil {
			return fmt.Errorf("judgmentstore replace prediction criteria insert: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("judgmentstore upsert prediction commit: %w", err)
	}
	return nil
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
		replaced, err = replaceReasonsTx(tx, evalDatasetID, traceID, reasons)
		if err != nil {
			return "", nil, false, fmt.Errorf("judgmentstore set verdict: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", nil, false, fmt.Errorf("judgmentstore set verdict commit: %w", err)
	}
	return previous, replaced, true, nil
}

// ReplaceReasons replaces an existing judgment's reasons with the given set in
// one transaction, returning the verdict and the replaced (previous) reasons.
// The reasons' values are stored as given, so callers control the scale. It does
// not modify a judgment whose verdict is unknown. found is false when the trace
// has no judgment row.
func (s *Store) ReplaceReasons(evalDatasetID, traceID string, reasons []Reason) (verdict Verdict, previous []Reason, found bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", nil, false, fmt.Errorf("judgmentstore replace reasons: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var raw string
	err = tx.QueryRow(`
		SELECT verdict FROM eval_dataset_judgments
		WHERE eval_dataset_id = $1 AND trace_id = $2
		FOR UPDATE
	`, evalDatasetID, traceID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, fmt.Errorf("judgmentstore replace reasons: %w", err)
	}
	verdict = Verdict(raw)
	if !verdict.Valid() {
		return "", nil, false, fmt.Errorf("judgmentstore replace reasons: invalid verdict %q", raw)
	}
	if verdict == VerdictUnknown {
		return verdict, nil, true, nil
	}

	previous, err = replaceReasonsTx(tx, evalDatasetID, traceID, reasons)
	if err != nil {
		return "", nil, false, fmt.Errorf("judgmentstore replace reasons: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", nil, false, fmt.Errorf("judgmentstore replace reasons commit: %w", err)
	}
	return verdict, previous, true, nil
}

// replaceReasonsTx deletes a judgment's reasons (returning them) and inserts the
// given set, within tx. Shared by SetVerdictAndReasons and ReplaceReasons.
func replaceReasonsTx(tx *sql.Tx, evalDatasetID, traceID string, reasons []Reason) ([]Reason, error) {
	rows, err := tx.Query(`
		DELETE FROM eval_dataset_judgment_reasons
		WHERE eval_dataset_id = $1 AND trace_id = $2
		RETURNING dimension_key, dimension_value
	`, evalDatasetID, traceID)
	if err != nil {
		return nil, fmt.Errorf("delete reasons: %w", err)
	}
	var previous []Reason
	for rows.Next() {
		var (
			key string
			val float64
		)
		if err := rows.Scan(&key, &val); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan reasons: %w", err)
		}
		previous = append(previous, Reason{Dimension: CriterionDimension(key), Value: val})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iter reasons: %w", err)
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
			return nil, fmt.Errorf("insert reasons: %w", err)
		}
	}
	return previous, nil
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
