// Package insightsrollup owns the durable daily-grain aggregate store behind
// the account Insights page (insights_usage_daily + insights_rollup_state).
//
// It replaces re-aggregating 90 days of Langfuse data from scratch every six
// hours and caching the rendered page in Redis: a completed day is rolled up
// once and never recomputed, so serving is a SQL aggregate and freshness stops
// being a function of recompute cost. Design doc:
// docs/01-spec/insights-rollup-spec.md.
//
// The one rule that matters here: the table holds two grains that describe the
// SAME spend, so summing across them double-counts. Every exported query takes
// a Grain argument and rejects the zero value, which is why there is no
// "aggregate everything" entry point.
package insightsrollup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrAccountGone reports that the account a roll-up job names is deleted,
// whether soft or hard. Producers return it wrapped; the worker treats it as a
// terminal skip rather than a failure to hold the watermark against.
//
// It needs its own sentinel because a hard delete leaves nowhere to record it.
// Both insights_rollup_state and insights_usage_daily are FK'd to accounts with
// ON DELETE CASCADE, so the state row is gone too, and writing the failure back
// violates the constraint. The job would then retry on an error it can never
// write down, for an account that will never come back.
var ErrAccountGone = errors.New("insightsrollup: account no longer exists")

// Grain discriminates the two descriptions of the same spend stored in
// insights_usage_daily. It is a distinct type rather than a string so a caller
// cannot pass a source or a column name by mistake, and the zero value is
// deliberately invalid so "forgot to specify" fails loudly instead of silently
// selecting both.
type Grain string

const (
	// GrainUsage is the measure grain: (deployment_id, actor_kind, actor_key).
	// Every surface on the Insights page is a GROUP BY over it.
	GrainUsage Grain = "usage"
	// GrainModel is (model) only, read from Langfuse's observations view. Kept
	// separate because observations-view cost does not reconcile with
	// traces-view cost, so it must never be mixed with GrainUsage.
	GrainModel Grain = "model"
)

func (g Grain) valid() bool { return g == GrainUsage || g == GrainModel }

// Actor kinds, mirroring the discriminated identity kinds the page renders.
// ActorKindSystem with an empty key is a trace with no user — the pinned
// system-spend row.
const (
	ActorKindMember       = "member"
	ActorKindSlack        = "slack"
	ActorKindSystem       = "system"
	ActorKindUnidentified = "unidentified"
)

// SourceAgents is the pseudo-source for spend from deployed agents, as opposed
// to a dev-tool source key like "claude-code".
const SourceAgents = "agents"

// Fact is one row at whichever grain is being written. Which dimension fields
// are populated is determined by the Grain passed to ReplaceDay; the database
// enforces the pairing with a CHECK constraint, so a producer that fills the
// wrong field fails at insert rather than corrupting a later aggregate.
//
// Requests is permanently zero for dev-tool sources — no such metric is
// emitted — so per-request derived columns must guard the denominator.
type Fact struct {
	DeploymentID string
	ActorKind    string
	ActorKey     string
	Model        string

	Requests     int64
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CostUSD      float64
	LastSeenAt   time.Time
}

// key returns the primary-key tuple of the dimension columns, used to fold
// duplicates before insert.
func (f Fact) key() [4]string {
	return [4]string{f.DeploymentID, f.ActorKind, f.ActorKey, f.Model}
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// maxInsertRows caps rows per INSERT statement. Each row binds 13 parameters,
// so this stays well under Postgres' 65535-parameter limit while keeping the
// number of round trips low.
const maxInsertRows = 500

// ReplaceDay makes (account, grain, day, source) exactly equal to facts, inside
// a single transaction. Full replace rather than merge is what makes the
// roll-up idempotent: reruns, overlapping ticks, and the trailing re-roll all
// converge on the same state, and there are no incremental merge semantics to
// get wrong.
//
// The delete is scoped by grain as well as by (account, day, source), because a
// producer that emits only one grain must not wipe the other's rows for that
// day.
//
// Facts are folded by their dimension tuple first, and that fold is load-bearing
// rather than defensive. Langfuse groups by the whole `tags` array, so a single
// deployment legitimately arrives as several groups — `[deployment:x]` and
// `[deployment:x, env:prod]` are distinct upstream rows that map to the same
// deployment_id. Summing them is correct; passing both through to one INSERT
// would violate the primary key, and ON CONFLICT cannot help because Postgres
// refuses to let one statement touch the same row twice.
func (s *Store) ReplaceDay(
	ctx context.Context,
	accountID string,
	grain Grain,
	day time.Time,
	source string,
	facts []Fact,
) error {
	if !grain.valid() {
		return fmt.Errorf("insightsrollup: replace day: invalid grain %q", grain)
	}
	if accountID == "" || source == "" {
		return fmt.Errorf("insightsrollup: replace day: account and source are required")
	}

	folded := foldFacts(facts)
	d := day.UTC().Format(time.DateOnly)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("insightsrollup: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rolled back only if Commit didn't run

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM insights_usage_daily
		 WHERE account_id = $1 AND grain = $2 AND day = $3 AND source = $4`,
		accountID, string(grain), d, source); err != nil {
		return fmt.Errorf("insightsrollup: clear day: %w", err)
	}

	for start := 0; start < len(folded); start += maxInsertRows {
		chunk := folded[start:min(start+maxInsertRows, len(folded))]
		var (
			sb   strings.Builder
			args []any
		)
		sb.WriteString(`INSERT INTO insights_usage_daily
			(account_id, grain, day, source, deployment_id, actor_kind, actor_key, model,
			 requests, input_tokens, output_tokens, total_tokens, cost_usd, last_seen_at)
			VALUES `)
		for i, f := range chunk {
			if i > 0 {
				sb.WriteString(", ")
			}
			base := i * 14
			fmt.Fprintf(&sb, "($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
				base+1, base+2, base+3, base+4, base+5, base+6, base+7,
				base+8, base+9, base+10, base+11, base+12, base+13, base+14)

			var lastSeen any
			if !f.LastSeenAt.IsZero() {
				lastSeen = f.LastSeenAt.UTC()
			}
			args = append(args,
				accountID, string(grain), d, source,
				f.DeploymentID, f.ActorKind, f.ActorKey, f.Model,
				f.Requests, f.InputTokens, f.OutputTokens, f.TotalTokens, f.CostUSD, lastSeen,
			)
		}
		if _, err := tx.ExecContext(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("insightsrollup: insert facts: %w", err)
		}
	}

	return tx.Commit()
}

// foldFacts sums facts sharing a dimension tuple, taking the newest LastSeenAt.
// Iteration order of the result is unspecified; callers only insert it.
func foldFacts(facts []Fact) []Fact {
	if len(facts) <= 1 {
		return facts
	}
	byKey := make(map[[4]string]*Fact, len(facts))
	order := make([][4]string, 0, len(facts))
	for _, f := range facts {
		k := f.key()
		existing, ok := byKey[k]
		if !ok {
			copied := f
			byKey[k] = &copied
			order = append(order, k)
			continue
		}
		existing.Requests += f.Requests
		existing.InputTokens += f.InputTokens
		existing.OutputTokens += f.OutputTokens
		existing.TotalTokens += f.TotalTokens
		existing.CostUSD += f.CostUSD
		if f.LastSeenAt.After(existing.LastSeenAt) {
			existing.LastSeenAt = f.LastSeenAt
		}
	}
	out := make([]Fact, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// State is the roll-up watermark for one (account, source).
type State struct {
	// RolledUpThrough is the last day considered complete. Zero means nothing
	// has ever been rolled up, which the read path surfaces as an absent
	// `as_of` rather than as zeroed metrics.
	RolledUpThrough   time.Time
	LastRunAt         time.Time
	LastError         string
	ConsecutiveErrors int
}

// State returns the watermark for (account, source). A missing row is not an
// error — it is the initial state, and reports a zero RolledUpThrough.
func (s *Store) State(ctx context.Context, accountID, source string) (State, error) {
	var (
		st      State
		through sql.NullTime
		lastRun sql.NullTime
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT rolled_up_through, last_run_at, last_error, consecutive_errors
		 FROM insights_rollup_state WHERE account_id = $1 AND source = $2`,
		accountID, source).Scan(&through, &lastRun, &st.LastError, &st.ConsecutiveErrors)
	if err == sql.ErrNoRows {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("insightsrollup: read state: %w", err)
	}
	if through.Valid {
		st.RolledUpThrough = through.Time
	}
	if lastRun.Valid {
		st.LastRunAt = lastRun.Time
	}
	return st, nil
}

// Advance moves the watermark to through and clears the error state. Callers
// advance only after every day up to `through` has been committed, so the
// watermark never claims more than the facts support.
func (s *Store) Advance(ctx context.Context, accountID, source string, through time.Time) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO insights_rollup_state
		   (account_id, source, rolled_up_through, last_run_at, last_error, consecutive_errors)
		 VALUES ($1, $2, $3, now(), '', 0)
		 ON CONFLICT (account_id, source) DO UPDATE SET
		   rolled_up_through = GREATEST(
		     EXCLUDED.rolled_up_through,
		     COALESCE(insights_rollup_state.rolled_up_through, EXCLUDED.rolled_up_through)),
		   last_run_at = now(), last_error = '', consecutive_errors = 0`,
		accountID, source, through.UTC().Format(time.DateOnly)); err != nil {
		return fmt.Errorf("insightsrollup: advance watermark: %w", err)
	}
	return nil
}

// RecordFailure leaves the watermark where it is and records why. Holding the
// watermark back is the point: a failed run must not let the page claim
// coverage it doesn't have, and a stalled watermark is a visible state rather
// than a silently stale cache entry.
func (s *Store) RecordFailure(ctx context.Context, accountID, source, reason string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO insights_rollup_state
		   (account_id, source, last_run_at, last_error, consecutive_errors)
		 VALUES ($1, $2, now(), $3, 1)
		 ON CONFLICT (account_id, source) DO UPDATE SET
		   last_run_at = now(), last_error = $3,
		   consecutive_errors = insights_rollup_state.consecutive_errors + 1`,
		accountID, source, reason); err != nil {
		return fmt.Errorf("insightsrollup: record failure: %w", err)
	}
	return nil
}

// DeleteBefore drops facts older than cutoff across all accounts, returning the
// number of rows removed. Retention is a bounded DELETE rather than a partition
// detach: the table is small enough that partitioning would buy nothing, and
// this schema is applied by declarative Atlas, which would read job-created
// partitions as drift.
func (s *Store) DeleteBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM insights_usage_daily WHERE day < $1`,
		cutoff.UTC().Format(time.DateOnly))
	if err != nil {
		return 0, fmt.Errorf("insightsrollup: delete before: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil // driver doesn't report it; the delete still succeeded
	}
	return n, nil
}
