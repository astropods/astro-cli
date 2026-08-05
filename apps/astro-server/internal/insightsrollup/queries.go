package insightsrollup

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Window is an inclusive day range. Both bounds are UTC dates; the time of day
// is ignored.
type Window struct {
	From time.Time
	To   time.Time
}

// Filter narrows an aggregate to what the caller is allowed and asked to see.
//
// HideSources is the negative form on purpose, mirroring the `hide_sources`
// request param: absent means every source is included, so a new dev-tool source
// appears without anyone remembering to add it to an allowlist.
type Filter struct {
	HideSources []string
	// OnlySources restricts to specific sources. Distinct from the inverse of
	// HideSources: this is for asking about one source directly ("does this
	// dev-tool have any spend?"), not for applying the user's filter.
	OnlySources []string
	// Untagged restricts to rows with no deployment id. Those split two ways:
	// dev-tool spend, which never has one, and agent usage that didn't report
	// which agent it came from. Both need a synthetic row, so both are read
	// through this rather than dropped.
	Untagged bool
	// RestrictActorKey, when set, limits rows to a single actor. This is how
	// per-developer visibility is enforced for non-admins: the restriction is
	// applied in SQL, so raw per-developer spend never reaches the process, let
	// alone the client.
	RestrictActorKey string
}

// Totals is the stat-card aggregate over a window.
type Totals struct {
	CostUSD  float64
	Requests int64
	Tokens   int64
}

// Totals sums a window at the given grain. Callers wanting the change-percentage
// call this twice, once per window — which is now honest for every source
// combination, because both windows come from the same durable history rather
// than from agent spend alone.
func (s *Store) Totals(ctx context.Context, accountID string, grain Grain, w Window, f Filter) (Totals, error) {
	if !grain.valid() {
		return Totals{}, fmt.Errorf("insightsrollup: totals: invalid grain %q", grain)
	}
	where, args := buildWhere(accountID, grain, w, f)

	var t Totals
	// COALESCE because SUM over no rows is NULL, and a brand-new account
	// legitimately has no rows.
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0), COALESCE(SUM(requests), 0), COALESCE(SUM(total_tokens), 0)
		 FROM insights_usage_daily WHERE `+where, args...).
		Scan(&t.CostUSD, &t.Requests, &t.Tokens)
	if err != nil {
		return Totals{}, fmt.Errorf("insightsrollup: totals: %w", err)
	}
	return t, nil
}

// DayPoint is one day of a time series.
type DayPoint struct {
	Day time.Time
	// Key is the series discriminator — a deployment id for the agent chart,
	// empty for account-wide series.
	Key        string
	CostUSD    float64
	Requests   int64
	Tokens     int64
	ActorCount int64
}

// DailyByDeployment returns the agent spend chart's series: cost per
// (day, deployment). The caller picks the top N by cost and folds the rest,
// which stays a presentation choice rather than something baked into storage.
func (s *Store) DailyByDeployment(ctx context.Context, accountID string, w Window, f Filter) ([]DayPoint, error) {
	where, args := buildWhere(accountID, GrainUsage, w, f)
	return s.dayPoints(ctx,
		`SELECT day, deployment_id, COALESCE(SUM(cost_usd), 0), COALESCE(SUM(requests), 0),
		        COALESCE(SUM(total_tokens), 0), 0
		 FROM insights_usage_daily WHERE `+where+`
		 GROUP BY day, deployment_id
		 ORDER BY day, deployment_id`, args)
}

// DailyTotals returns the People chart's series: cost per day plus the number of
// distinct actors active that day.
//
// COUNT(DISTINCT actor_key) is why this can't be derived from DailyByDeployment:
// the same person appears under several deployments, so summing per-deployment
// actor counts would over-count them.
func (s *Store) DailyTotals(ctx context.Context, accountID string, w Window, f Filter) ([]DayPoint, error) {
	where, args := buildWhere(accountID, GrainUsage, w, f)
	return s.dayPoints(ctx,
		`SELECT day, '', COALESCE(SUM(cost_usd), 0), COALESCE(SUM(requests), 0),
		        COALESCE(SUM(total_tokens), 0),
		        COUNT(DISTINCT actor_key) FILTER (WHERE actor_key <> '')
		 FROM insights_usage_daily WHERE `+where+`
		 GROUP BY day ORDER BY day`, args)
}

// DailyByActor returns cost per (day, actor), with the actor key in Key.
//
// The People spend chart needs the per-actor breakdown rather than a daily
// total, because it reports how many distinct people were active each day —
// a number that cannot be recovered from a summed series.
func (s *Store) DailyByActor(ctx context.Context, accountID string, w Window, f Filter) ([]DayPoint, error) {
	where, args := buildWhere(accountID, GrainUsage, w, f)
	return s.dayPoints(ctx,
		`SELECT day, actor_key, COALESCE(SUM(cost_usd), 0), COALESCE(SUM(requests), 0),
		        COALESCE(SUM(total_tokens), 0), 0
		 FROM insights_usage_daily WHERE `+where+`
		 GROUP BY day, actor_key
		 ORDER BY day, actor_key`, args)
}

// DailyBySource returns cost per (day, source), with the source in Key.
//
// Dev-tool spend carries no deployment id, so it never appears in
// DailyByDeployment and can't reach the stat cards or the agent chart that way.
// This is how those surfaces pick it up.
func (s *Store) DailyBySource(ctx context.Context, accountID string, w Window, f Filter) ([]DayPoint, error) {
	where, args := buildWhere(accountID, GrainUsage, w, f)
	return s.dayPoints(ctx,
		`SELECT day, source, COALESCE(SUM(cost_usd), 0), COALESCE(SUM(requests), 0),
		        COALESCE(SUM(total_tokens), 0), 0
		 FROM insights_usage_daily WHERE `+where+`
		 GROUP BY day, source
		 ORDER BY day, source`, args)
}

func (s *Store) dayPoints(ctx context.Context, query string, args []any) ([]DayPoint, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insightsrollup: daily series: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []DayPoint
	for rows.Next() {
		var p DayPoint
		if err := rows.Scan(&p.Day, &p.Key, &p.CostUSD, &p.Requests, &p.Tokens, &p.ActorCount); err != nil {
			return nil, fmt.Errorf("insightsrollup: scan daily series: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Row is one aggregated table row, already carrying its share of the total and
// the unfiltered row count so the caller needs no second query.
type Row struct {
	// DeploymentID is set for agent rows; ActorKind/ActorKey for people rows.
	DeploymentID string
	ActorKind    string
	ActorKey     string

	Requests int64
	CostUSD  float64
	Tokens   int64
	// CostPct is this row's share of the window's total spend across every
	// included source, computed in the same pass via a window function.
	CostPct  float64
	LastSeen sql.NullTime

	// Deleted reports that no deployments row matched — the agent existed when
	// the spend happened and has since been removed. Only meaningful for agent
	// rows, and only possible because the facts outlive the deployment.
	Deleted bool
	// TotalRows is the unfiltered count for this table, repeated on every row by
	// COUNT(*) OVER (). Drives the view-toggle pill counts.
	TotalRows int64
}

// AgentRowOptions controls sorting and paging of the agents table.
type AgentRowOptions struct {
	// SortColumn must already be whitelisted by the caller; it is interpolated
	// into SQL, so an unvalidated value here would be an injection.
	SortColumn string
	Descending bool
	Limit      int
	Offset     int
}

// agentSortColumns maps the API's sort keys to SQL expressions. Interpolating a
// column name is only safe against a fixed map like this, never against the
// request value — the request key is looked up, and an unknown key falls back to
// cost rather than reaching the query.
var agentSortColumns = map[string]string{
	"cost":     "cost_usd",
	"requests": "requests",
	"tokens":   "tokens",
	"name":     "sort_name",
}

// AgentRows returns the agents table: every visible deployment, LEFT JOINed to
// its facts.
//
// Deployments are the row set and facts only supply the metrics. That direction
// matters: a fact table has rows only where something happened, so aggregating
// facts alone would silently drop a deployed-but-idle agent that the page shows
// today with a not_instrumented marker. It also matches the standing rule that
// "what did we deploy" is a database question.
//
// The FULL OUTER-ish shape (UNION of deployments and orphaned facts) is what
// keeps spend from deleted deployments visible instead of vanishing with the
// row.
func (s *Store) AgentRows(
	ctx context.Context,
	accountID string,
	w Window,
	f Filter,
	opts AgentRowOptions,
) ([]Row, error) {
	sortCol, ok := agentSortColumns[opts.SortColumn]
	if !ok {
		sortCol = "cost_usd"
	}
	direction := "ASC"
	if opts.Descending {
		direction = "DESC"
	}

	where, args := buildWhere(accountID, GrainUsage, w, f)
	// $N for the account id is reused by the deployments side of the join.
	acctParam := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, accountID)
	limitParam := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, normalizeLimit(opts.Limit))
	offsetParam := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, max(opts.Offset, 0))

	// facts: per-deployment aggregate over the window.
	// grand: the denominator for cost_pct — every included source's full total,
	//        so a percentage means the same thing in the agents and people
	//        tables. Computed as a scalar subquery rather than SUM() OVER () so
	//        it survives the LEFT JOIN's zero-activity rows.
	query := `
	WITH facts AS (
		SELECT deployment_id,
		       SUM(requests) AS requests,
		       SUM(cost_usd) AS cost_usd,
		       SUM(total_tokens) AS tokens,
		       MAX(last_seen_at) AS last_seen
		FROM insights_usage_daily WHERE ` + where + `
		GROUP BY deployment_id
	), grand AS (
		SELECT COALESCE(SUM(cost_usd), 0) AS total
		FROM insights_usage_daily WHERE ` + where + `
	), joined AS (
		SELECT d.id                       AS deployment_id,
		       COALESCE(d.display_name, '') AS sort_name,
		       COALESCE(f.requests, 0)    AS requests,
		       COALESCE(f.cost_usd, 0)    AS cost_usd,
		       COALESCE(f.tokens, 0)      AS tokens,
		       f.last_seen                AS last_seen,
		       false                      AS deleted
		FROM deployments d
		LEFT JOIN facts f ON f.deployment_id = d.id
		WHERE d.account_id = ` + acctParam + ` AND d.status <> 'undeployed'
		UNION ALL
		-- Spend whose deployment no longer exists. '' is excluded: that is
		-- untagged/dev-tool spend, which belongs to the synthetic source row
		-- rather than to a deleted agent.
		SELECT f.deployment_id, '', f.requests, f.cost_usd, f.tokens, f.last_seen, true
		FROM facts f
		WHERE f.deployment_id <> ''
		  AND NOT EXISTS (SELECT 1 FROM deployments d WHERE d.id = f.deployment_id)
	)
	SELECT deployment_id, requests, cost_usd, tokens,
	       CASE WHEN (SELECT total FROM grand) > 0
	            THEN cost_usd / (SELECT total FROM grand) * 100 ELSE 0 END,
	       last_seen, deleted, COUNT(*) OVER () AS total_rows
	FROM joined
	ORDER BY ` + sortCol + ` ` + direction + `, deployment_id
	LIMIT ` + limitParam + ` OFFSET ` + offsetParam

	return s.scanRows(ctx, query, args, false)
}

var peopleSortColumns = map[string]string{
	"cost":      "cost_usd",
	"requests":  "requests",
	"tokens":    "tokens",
	"last_seen": "last_seen",
}

// PeopleRows returns the People table, aggregated per actor.
//
// System spend sorts last regardless of the requested direction, matching the
// page's pinned system row. Doing it here rather than in Go keeps the pinning
// correct across pagination — a Go-side re-sort of one page cannot pin a row to
// the end of a list it can't see.
func (s *Store) PeopleRows(
	ctx context.Context,
	accountID string,
	w Window,
	f Filter,
	opts AgentRowOptions,
) ([]Row, error) {
	sortCol, ok := peopleSortColumns[opts.SortColumn]
	if !ok {
		sortCol = "cost_usd"
	}
	direction := "ASC"
	if opts.Descending {
		direction = "DESC"
	}

	where, args := buildWhere(accountID, GrainUsage, w, f)
	limitParam := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, normalizeLimit(opts.Limit))
	offsetParam := fmt.Sprintf("$%d", len(args)+1)
	args = append(args, max(opts.Offset, 0))

	query := `
	WITH agg AS (
		SELECT actor_kind, actor_key,
		       SUM(requests) AS requests,
		       SUM(cost_usd) AS cost_usd,
		       SUM(total_tokens) AS tokens,
		       MAX(last_seen_at) AS last_seen
		FROM insights_usage_daily WHERE ` + where + `
		GROUP BY actor_kind, actor_key
	)
	SELECT actor_kind, actor_key, requests, cost_usd, tokens,
	       CASE WHEN SUM(cost_usd) OVER () > 0
	            THEN cost_usd / SUM(cost_usd) OVER () * 100 ELSE 0 END,
	       last_seen, COUNT(*) OVER () AS total_rows
	FROM agg
	ORDER BY (actor_kind = '` + ActorKindSystem + `') ASC, ` + sortCol + ` ` + direction + `, actor_key
	LIMIT ` + limitParam + ` OFFSET ` + offsetParam

	return s.scanRows(ctx, query, args, true)
}

func (s *Store) scanRows(ctx context.Context, query string, args []any, people bool) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insightsrollup: rows: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []Row
	for rows.Next() {
		var r Row
		if people {
			err = rows.Scan(&r.ActorKind, &r.ActorKey, &r.Requests, &r.CostUSD, &r.Tokens,
				&r.CostPct, &r.LastSeen, &r.TotalRows)
		} else {
			err = rows.Scan(&r.DeploymentID, &r.Requests, &r.CostUSD, &r.Tokens,
				&r.CostPct, &r.LastSeen, &r.Deleted, &r.TotalRows)
		}
		if err != nil {
			return nil, fmt.Errorf("insightsrollup: scan row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Pair is one (deployment, actor) coincidence, driving the used_by and
// agents_used chips. Presence only — these are never summed, because the measure
// grain already carries the spend.
type Pair struct {
	DeploymentID string
	ActorKind    string
	ActorKey     string
}

// Pairs returns the distinct (deployment, actor) combinations in the window.
// They come free from the measure grain, which is the main dividend of carrying
// both dimensions on one row.
func (s *Store) Pairs(ctx context.Context, accountID string, w Window, f Filter) ([]Pair, error) {
	where, args := buildWhere(accountID, GrainUsage, w, f)
	query := pairsSelect + where + pairsTail //nolint:gosec // G202: `where` is fixed predicates and $N placeholders; values bind through args
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insightsrollup: pairs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []Pair
	for rows.Next() {
		var p Pair
		if err := rows.Scan(&p.DeploymentID, &p.ActorKind, &p.ActorKey); err != nil {
			return nil, fmt.Errorf("insightsrollup: scan pair: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ActorSource is one (actor, source) coincidence — which tools and pipelines a
// person's usage came through. Presence only, never summed.
type ActorSource struct {
	ActorKey string
	Source   string
}

// ActorSources returns the distinct (actor, source) pairs in the window.
//
// Distinct from Pairs, which answers the same question for deployments and so
// requires a deployment id. Dev-tool usage has none, so it is invisible to Pairs
// and needs this to reach the chips on a person's row.
func (s *Store) ActorSources(ctx context.Context, accountID string, w Window, f Filter) ([]ActorSource, error) {
	where, args := buildWhere(accountID, GrainUsage, w, f)
	query := actorSourcesSelect + where + actorSourcesTail //nolint:gosec // G202: see pairsSelect
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("insightsrollup: actor sources: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var out []ActorSource
	for rows.Next() {
		var a ActorSource
		if err := rows.Scan(&a.ActorKey, &a.Source); err != nil {
			return nil, fmt.Errorf("insightsrollup: scan actor source: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Query fragments for the two DISTINCT lookups. Split around the shared
// predicate so the concatenation is a single expression: gosec reports G202
// across the whole concatenation, and a directive cannot live inside a raw
// string literal.
//
// Safe to concatenate because buildWhere never interpolates a value — see its
// contract below.
const (
	pairsSelect = `SELECT DISTINCT deployment_id, actor_kind, actor_key
		FROM insights_usage_daily WHERE `
	pairsTail = ` AND deployment_id <> '' AND actor_key <> ''`

	actorSourcesSelect = `SELECT DISTINCT actor_key, source
		FROM insights_usage_daily WHERE `
	actorSourcesTail = ` AND actor_key <> ''
		ORDER BY actor_key, source`
)

// buildWhere assembles the shared predicate and its arguments. Every aggregate
// goes through it, which is what guarantees no query can omit the grain filter —
// the one mistake that would double-count.
//
// The returned string contains only fixed predicate text and $N placeholders;
// every value is returned in args and bound by the driver. Callers may therefore
// concatenate it into a query safely, which is what the G202 suppressions above
// rely on. Never interpolate a value here.
func buildWhere(accountID string, grain Grain, w Window, f Filter) (string, []any) {
	clauses := []string{"account_id = $1", "grain = $2", "day BETWEEN $3 AND $4"}
	args := []any{
		accountID,
		string(grain),
		w.From.UTC().Format(time.DateOnly),
		w.To.UTC().Format(time.DateOnly),
	}

	// pq.Array, not the bare slice: lib/pq cannot convert []string on its own and
	// fails at query time with "unsupported type []string". Unit tests on this
	// function don't catch that, because the conversion happens in the driver.
	if len(f.HideSources) > 0 {
		args = append(args, pq.Array(f.HideSources))
		clauses = append(clauses, fmt.Sprintf("source <> ALL($%d)", len(args)))
	}
	if len(f.OnlySources) > 0 {
		args = append(args, pq.Array(f.OnlySources))
		clauses = append(clauses, fmt.Sprintf("source = ANY($%d)", len(args)))
	}
	if f.Untagged {
		clauses = append(clauses, "deployment_id = ''")
	}
	if f.RestrictActorKey != "" {
		args = append(args, f.RestrictActorKey)
		clauses = append(clauses, fmt.Sprintf("actor_key = $%d", len(args)))
	}
	return strings.Join(clauses, " AND "), args
}

// normalizeLimit clamps paging to the same bounds the v1 request normalizer
// uses, so v2 can't be coaxed into materializing more than v1 would.
func normalizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return 25
	case limit > 5000:
		return 5000
	default:
		return limit
	}
}
