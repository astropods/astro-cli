// Package classification stores Claude Code prompt labels, their daily
// aggregates, and the pass watermark. The per-trace primary key is what makes
// re-runs free: only prompts Postgres says are new reach inference.
package classification

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Axis is a distinct type so a label or source cannot be passed positionally.
type Axis string

const (
	AxisPurpose Axis = "purpose"
	AxisTopic   Axis = "topic"
	AxisTask    Axis = "task"
)

func (a Axis) valid() bool { return a == AxisPurpose || a == AxisTopic || a == AxisTask }

// Only UnitTurn is produced today — Claude Code sends no session id.
type UnitKind string

const UnitTurn UnitKind = "turn"

const SourceClaudeCode = "claude-code"

type Result struct {
	UnitKind   UnitKind
	UnitID     string
	Axis       Axis
	Label      string
	Score      float64
	OccurredAt time.Time
	UserEmail  string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// 10 params per row, well under Postgres' 65535 limit.
const maxInsertRows = 500

// Duplicates are folded first: one statement cannot touch a row twice, and
// ON CONFLICT does not rescue it.
func (s *Store) SaveResults(ctx context.Context, accountID, source, modelVersion string, results []Result) error {
	if modelVersion == "" {
		return fmt.Errorf("classification: model version required")
	}
	deduped := foldResults(results)
	if len(deduped) == 0 {
		return nil
	}
	for _, r := range deduped {
		if !r.Axis.valid() {
			return fmt.Errorf("classification: invalid axis %q", r.Axis)
		}
		if r.Label == "" || len(r.Label) > 64 {
			return fmt.Errorf("classification: label %q outside the 64-char column", r.Label)
		}
		if r.Score < 0 || r.Score > 1 {
			return fmt.Errorf("classification: score %v outside [0,1]", r.Score)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("classification: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for start := 0; start < len(deduped); start += maxInsertRows {
		end := min(start+maxInsertRows, len(deduped))
		if err := insertResults(ctx, tx, accountID, source, modelVersion, deduped[start:end]); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("classification: commit: %w", err)
	}
	return nil
}

func insertResults(ctx context.Context, tx *sql.Tx, accountID, source, modelVersion string, batch []Result) error {
	var sb strings.Builder
	sb.WriteString(`INSERT INTO trace_classifications
		(account_id, source, unit_kind, unit_id, axis, label, score, model_version, occurred_at, user_email)
		VALUES `)
	args := make([]any, 0, len(batch)*10)
	for i, r := range batch {
		if i > 0 {
			sb.WriteString(",")
		}
		n := i * 10
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8, n+9, n+10)
		args = append(args, accountID, source, string(r.UnitKind), r.UnitID,
			string(r.Axis), r.Label, r.Score, modelVersion, r.OccurredAt, r.UserEmail)
	}
	// A retrain writes a new row, so each generation survives for retraining.
	sb.WriteString(` ON CONFLICT (account_id, unit_kind, unit_id, axis, model_version) DO UPDATE SET
		label = EXCLUDED.label,
		score = EXCLUDED.score,
		occurred_at = EXCLUDED.occurred_at,
		user_email = EXCLUDED.user_email,
		updated_at = now()`)

	if _, err := tx.ExecContext(ctx, sb.String(), args...); err != nil {
		return fmt.Errorf("classification: insert results: %w", err)
	}
	return nil
}

// Keeps the last occurrence of each (unit_kind, unit_id, axis).
func foldResults(results []Result) []Result {
	idx := make(map[[3]string]int, len(results))
	out := make([]Result, 0, len(results))
	for _, r := range results {
		k := [3]string{string(r.UnitKind), r.UnitID, string(r.Axis)}
		if i, ok := idx[k]; ok {
			out[i] = r
			continue
		}
		idx[k] = len(out)
		out = append(out, r)
	}
	return out
}

// ClassifiedAxes reports pairs already at modelVersion; older ones read as absent.
func (s *Store) ClassifiedAxes(
	ctx context.Context,
	accountID, source, modelVersion string,
	unitKind UnitKind,
	unitIDs []string,
) (map[string]map[Axis]bool, error) {
	out := map[string]map[Axis]bool{}
	if len(unitIDs) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT unit_id, axis FROM trace_classifications
		WHERE account_id = $1 AND source = $2 AND unit_kind = $3
		  AND model_version = $4 AND unit_id = ANY($5)`,
		accountID, source, string(unitKind), modelVersion, pq.Array(unitIDs))
	if err != nil {
		return nil, fmt.Errorf("classification: query classified: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var unitID, axis string
		if err := rows.Scan(&unitID, &axis); err != nil {
			return nil, fmt.Errorf("classification: scan classified: %w", err)
		}
		if out[unitID] == nil {
			out[unitID] = map[Axis]bool{}
		}
		out[unitID][Axis(axis)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("classification: iterate classified: %w", err)
	}
	return out, nil
}

type LabelCount struct {
	Axis      Axis
	Label     string
	UserEmail string
	Traces    int64
}

// Scoped to one unit_kind and one model_version: the table keeps every
// generation of a retrain, so an unscoped tally counts a trace once per one.
func (s *Store) CountsForDay(
	ctx context.Context,
	accountID, source, modelVersion string,
	unitKind UnitKind,
	day time.Time,
) ([]LabelCount, error) {
	if modelVersion == "" {
		return nil, fmt.Errorf("classification: counts for day: model version required")
	}
	start := day.UTC().Truncate(24 * time.Hour)
	rows, err := s.db.QueryContext(ctx, `
		SELECT axis, label, user_email, count(*)
		FROM trace_classifications
		WHERE account_id = $1 AND source = $2 AND unit_kind = $3 AND model_version = $4
		  AND occurred_at >= $5 AND occurred_at < $6
		GROUP BY axis, label, user_email`,
		accountID, source, string(unitKind), modelVersion, start, start.AddDate(0, 0, 1))
	if err != nil {
		return nil, fmt.Errorf("classification: counts for day: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []LabelCount
	for rows.Next() {
		var c LabelCount
		var axis string
		if err := rows.Scan(&axis, &c.Label, &c.UserEmail, &c.Traces); err != nil {
			return nil, fmt.Errorf("classification: scan counts: %w", err)
		}
		c.Axis = Axis(axis)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("classification: iterate counts: %w", err)
	}
	return out, nil
}

type DailyFact struct {
	Axis      Axis
	Label     string
	ActorKind string
	ActorKey  string
	Traces    int64
	CostUSD   float64
}

func (f DailyFact) key() [4]string {
	return [4]string{string(f.Axis), f.Label, f.ActorKind, f.ActorKey}
}

// Full replace rather than merge, which is what keeps reruns idempotent.
func (s *Store) ReplaceDayAggregates(
	ctx context.Context,
	accountID string,
	day time.Time,
	source string,
	facts []DailyFact,
) error {
	folded := foldFacts(facts)
	start := day.UTC().Truncate(24 * time.Hour)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("classification: begin aggregates: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM insights_classification_daily
		WHERE account_id = $1 AND day = $2 AND source = $3`,
		accountID, start, source); err != nil {
		return fmt.Errorf("classification: delete aggregates: %w", err)
	}

	for begin := 0; begin < len(folded); begin += maxInsertRows {
		end := min(begin+maxInsertRows, len(folded))
		if err := insertFacts(ctx, tx, accountID, start, source, folded[begin:end]); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("classification: commit aggregates: %w", err)
	}
	return nil
}

func insertFacts(ctx context.Context, tx *sql.Tx, accountID string, day time.Time, source string, batch []DailyFact) error {
	var sb strings.Builder
	sb.WriteString(`INSERT INTO insights_classification_daily
		(account_id, day, source, axis, label, actor_kind, actor_key, traces, cost_usd)
		VALUES `)
	args := make([]any, 0, len(batch)*9)
	for i, f := range batch {
		if i > 0 {
			sb.WriteString(",")
		}
		n := i * 9
		fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			n+1, n+2, n+3, n+4, n+5, n+6, n+7, n+8, n+9)
		args = append(args, accountID, day, source, string(f.Axis), f.Label,
			f.ActorKind, f.ActorKey, f.Traces, f.CostUSD)
	}
	if _, err := tx.ExecContext(ctx, sb.String(), args...); err != nil {
		return fmt.Errorf("classification: insert aggregates: %w", err)
	}
	return nil
}

func foldFacts(facts []DailyFact) []DailyFact {
	idx := make(map[[4]string]int, len(facts))
	out := make([]DailyFact, 0, len(facts))
	for _, f := range facts {
		if i, ok := idx[f.key()]; ok {
			out[i].Traces += f.Traces
			out[i].CostUSD += f.CostUSD
			continue
		}
		idx[f.key()] = len(out)
		out = append(out, f)
	}
	return out
}

// Returned unfolded so one fetch of the widest window serves every range.
type AggRow struct {
	Day       time.Time
	Axis      Axis
	Label     string
	ActorKind string
	ActorKey  string
	Traces    int64
	CostUSD   float64
}

// Inclusive day range. Rows carry their actor unfiltered — the caller folds
// them, and gates the per-developer view.
func (s *Store) Aggregates(
	ctx context.Context,
	accountID, source string,
	from, to time.Time,
) ([]AggRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT day, axis, label, actor_kind, actor_key, traces, cost_usd
		FROM insights_classification_daily
		WHERE account_id = $1 AND source = $2 AND day >= $3 AND day <= $4`,
		accountID, source, from.UTC().Truncate(24*time.Hour), to.UTC().Truncate(24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("classification: aggregates: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []AggRow
	for rows.Next() {
		var r AggRow
		var axis string
		if err := rows.Scan(&r.Day, &axis, &r.Label, &r.ActorKind, &r.ActorKey, &r.Traces, &r.CostUSD); err != nil {
			return nil, fmt.Errorf("classification: scan aggregates: %w", err)
		}
		r.Axis = Axis(axis)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("classification: iterate aggregates: %w", err)
	}
	return out, nil
}

// Floors the backfill: telemetry cannot predate its ingest key. Revoked keys
// count, or a rotation would truncate history.
func (s *Store) EarliestDataDay(ctx context.Context, accountID string) (*time.Time, error) {
	var earliest *time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT min(created_at) FROM otel_ingest_tokens WHERE account_id = $1`,
		accountID).Scan(&earliest)
	if err != nil {
		return nil, fmt.Errorf("classification: earliest data day: %w", err)
	}
	if earliest == nil {
		return nil, nil
	}
	day := earliest.UTC().Truncate(24 * time.Hour)
	return &day, nil
}

// [BackfilledFrom, ClassifiedThrough] is the fully-classified window.
type State struct {
	ClassifiedThrough *time.Time
	BackfilledFrom    *time.Time
	BackfillComplete  bool
	LastRunAt         *time.Time
	LastError         string
	ConsecutiveErrors int
}

// GetState returns a zero State when the account has never run.
func (s *Store) GetState(ctx context.Context, accountID, source string) (State, error) {
	var st State
	err := s.db.QueryRowContext(ctx, `
		SELECT classified_through, backfilled_from, backfill_complete,
		       last_run_at, last_error, consecutive_errors
		FROM classification_state WHERE account_id = $1 AND source = $2`,
		accountID, source).Scan(&st.ClassifiedThrough, &st.BackfilledFrom, &st.BackfillComplete,
		&st.LastRunAt, &st.LastError, &st.ConsecutiveErrors)
	if err == sql.ErrNoRows {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("classification: get state: %w", err)
	}
	return st, nil
}

// Cursors only widen the window; a nil argument leaves that edge untouched.
func (s *Store) SetCursors(
	ctx context.Context,
	accountID, source string,
	through, from *time.Time,
	backfillComplete bool,
) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO classification_state
			(account_id, source, classified_through, backfilled_from, backfill_complete,
			 last_run_at, last_error, consecutive_errors)
		VALUES ($1, $2, $3, $4, $5, now(), '', 0)
		ON CONFLICT (account_id, source) DO UPDATE SET
			classified_through = GREATEST(
				COALESCE(EXCLUDED.classified_through, classification_state.classified_through),
				COALESCE(classification_state.classified_through, EXCLUDED.classified_through)),
			backfilled_from = LEAST(
				COALESCE(EXCLUDED.backfilled_from, classification_state.backfilled_from),
				COALESCE(classification_state.backfilled_from, EXCLUDED.backfilled_from)),
			backfill_complete = classification_state.backfill_complete OR EXCLUDED.backfill_complete,
			last_run_at = now(), last_error = '', consecutive_errors = 0`,
		accountID, source, truncDay(through), truncDay(from), backfillComplete)
	if err != nil {
		return fmt.Errorf("classification: set cursors: %w", err)
	}
	return nil
}

func truncDay(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Truncate(24 * time.Hour)
}

// MarkFailure leaves the watermark alone so the day is retried next tick.
func (s *Store) MarkFailure(ctx context.Context, accountID, source, msg string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO classification_state (account_id, source, last_run_at, last_error, consecutive_errors)
		VALUES ($1, $2, now(), $3, 1)
		ON CONFLICT (account_id, source) DO UPDATE SET
			last_run_at = now(), last_error = $3,
			consecutive_errors = classification_state.consecutive_errors + 1`,
		accountID, source, truncateErr(msg))
	if err != nil {
		return fmt.Errorf("classification: mark failure: %w", err)
	}
	return nil
}

func truncateErr(s string) string {
	const maxErr = 500
	if len(s) <= maxErr {
		return s
	}
	return s[:maxErr] + "..."
}
