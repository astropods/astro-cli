package handlers

import (
	"context"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// GetAccountInsights returns the complete server-owned view model for the
// Insights page, read from the durable daily fact table.
// GET /api/v1/accounts/:account/insights
//
// The lower-level observability endpoints remain reusable API primitives; this
// endpoint owns page semantics.
//
// Sort, filter and pagination still happen in Go. The pushdown aggregates exist
// (internal/insightsrollup/queries.go: cost_pct via window function, ORDER
// BY/LIMIT/OFFSET, the system row pinned in SQL) and can replace them; keeping
// one assembly for every surface is what makes the tables and the cards agree by
// construction.
func GetAccountInsights(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	slackStore *slackidentity.Store,
	rollups *insightsrollup.Store,
	cache k8scache.Cache,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		acct, err := accountStore.GetByName(c.Param("account"))
		if err != nil || acct == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		isMember, err := accountStore.IsMember(acct.ID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		// Per-developer dev-tool spend is admin-only, members see only their own
		// row, and aggregate spend stays visible to all.
		params := parseInsightsRequestParams(c)
		// Daily facts make a range change a window change, so the tables respect
		// the range chip above them.
		params.TableDays = parseInsightsTableDays(c)
		if !middleware.HasAccountPermission(c, accountStore, acct, user, "org:admin") {
			params.RestrictDevtoolToKey = "member:" + user.ID
		}

		resp, err := ComputeInsightsFromRollups(c.Request.Context(), log,
			accountStore, deploymentStore, slackStore, rollups, acct,
			cache, time.Now().UTC(), params)
		if err != nil {
			log.Error("Failed to compute insights from rollups", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute insights"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ComputeInsightsFromRollups assembles the Insights response from
// insights_usage_daily.
//
// One fetch at the widest range feeds every range, because
// buildInsightsViewWithParams slices the per-deployment daily series itself. So a
// request is a handful of SQL aggregates, and no request touches Langfuse.
func ComputeInsightsFromRollups(
	ctx context.Context,
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	slackStore *slackidentity.Store,
	rollups *insightsrollup.Store,
	acct *account.Account,
	cache k8scache.Cache,
	now time.Time,
	params insightsRequestParams,
) (InsightsResponse, error) {
	// Every window ends on the watermark, not on today. The facts hold complete
	// days only (insightsrollup.DaysToRoll), so a window ending today carries an
	// empty trailing day: a chart bar that can never fill, and stat cards that
	// compare N-1 days of spend against N prior days. Reporting the horizon we
	// have keeps both honest, and as_of tells the client which day that is.
	state, serr := rollups.State(ctx, acct.ID, insightsrollup.SourceAgents)
	if serr != nil {
		// Non-fatal: without the watermark we still know the facts stop at the last
		// complete day, which is where the watermark sits on a healthy account.
		log.Warn("Insights rollup: failed to read watermark, falling back to last complete day",
			"account_id", acct.ID, "error", serr)
	}
	asOf := insightsAsOfDay(state, now)

	widest := widestInsightsRange()
	_, fromDate, toDate := insightsPeriod(asOf, widest.days)

	window := insightsrollup.Window{
		From: parseInsightsDate(fromDate),
		To:   parseInsightsDate(toDate),
	}
	filter := insightsrollup.Filter{HideSources: hiddenSourceKeys(params.HideSources)}

	// The tables scope to the selected range while the charts keep the widest
	// window, because one response carries every range. The agents table is
	// sliced from the same entries the charts use; the People rows have to be
	// queried at the table window, since a person's row has no daily series to
	// slice afterwards.
	tableWindow := window
	if params.TableDays > 0 {
		_, tFrom, tTo := insightsPeriod(asOf, params.TableDays)
		tableWindow = insightsrollup.Window{From: parseInsightsDate(tFrom), To: parseInsightsDate(tTo)}
	}

	deployments, err := rollupDeploymentEntries(ctx, log, rollups, deploymentStore, acct, window, filter)
	if err != nil {
		return InsightsResponse{}, err
	}
	enrichDeploymentLastSeen(ctx, log, cache, &deployments)
	users, err := rollupUserEntries(ctx, accountStore, rollups, deploymentStore, acct, tableWindow, filter)
	if err != nil {
		return InsightsResponse{}, err
	}
	summary, err := rollupSummary(ctx, rollups, acct, window, filter)
	if err != nil {
		return InsightsResponse{}, err
	}

	// Identity decoration stays at read time: Slack display names and member
	// profiles change independently of spend, so the facts hold only stable keys
	// and the labels are resolved on every read.
	ResolveUsersSummaryIdentities(log, slackStore, accountStore, &users)
	members, err := insightsMemberProfiles(log, accountStore, acct.ID)
	if err != nil {
		return InsightsResponse{}, err
	}

	// Dev-tool spend reaches every surface through the same lineage as agent
	// spend: the deployment entries feed the cards, chart and agents table, the
	// actor grain feeds the People surfaces. Only the Sources filter needs to know
	// which sources exist, and it needs them unfiltered by hide_sources, or a
	// hidden source could never be switched back on.
	present, err := rollupPresentDevtoolSources(ctx, rollups, acct, window)
	if err != nil {
		return InsightsResponse{}, err
	}

	resp := buildInsightsViewWithParams(acct.Name, summary, deployments, users,
		members, present, asOf, params)
	// Only reported when a watermark exists. On a cold account the window still
	// ends at the last complete day, but claiming coverage through it would be a
	// lie — the rollup has never run.
	if !state.RolledUpThrough.IsZero() {
		resp.AsOf = asOf.Format(time.DateOnly)
	}
	return resp, nil
}

// insightsAsOfDay resolves the day every reported window should end on: the
// rollup watermark, or the last complete UTC day when there is none.
//
// Clamped to the last complete day because that is the most the facts can hold
// — a watermark ahead of it would mean a partial day had been written, and the
// page must not report a window it has only part of.
func insightsAsOfDay(state insightsrollup.State, now time.Time) time.Time {
	lastComplete := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	if state.RolledUpThrough.IsZero() || state.RolledUpThrough.UTC().After(lastComplete) {
		return lastComplete
	}
	return state.RolledUpThrough.UTC().Truncate(24 * time.Hour)
}

// rollupDeploymentEntries builds the per-deployment rows the view model expects,
// including the daily series it slices per range.
//
// Deployments are the row set and facts supply the metrics — a deployed-but-idle
// agent must still appear, with requests == 0 driving the not_instrumented
// marker, and a fact table only has rows where something happened.
//
// P95LatencyMs is deliberately left at 0 for wire compatibility. The Agents
// table now shows LastSeen instead; the table stores no mergeable latency, because
// nothing can supply an additive one. Langfuse's histogram aggregation is
// ClickHouse's adaptive variant (per-query bin boundaries, so days can't be
// merged), its filters resolve only to dimensions with no HAVING (so buckets
// can't be counted by hand), and agent latency exists only as span timestamps
// rather than as a metric — the collector exports to Langfuse, not to
// VictoriaMetrics. Deriving it from the table needs fixed-boundary histograms at
// ingest (a spanmetrics connector); until then the column is intentionally
// empty. See docs/01-spec/insights-rollup-spec.md.
func rollupDeploymentEntries(
	ctx context.Context,
	log *logger.Logger,
	rollups *insightsrollup.Store,
	deploymentStore *deploymentstore.Store,
	acct *account.Account,
	window insightsrollup.Window,
	filter insightsrollup.Filter,
) (AccountDeploymentsSummaryResponse, error) {
	daily, err := rollups.DailyByDeployment(ctx, acct.ID, window, filter)
	if err != nil {
		return AccountDeploymentsSummaryResponse{}, err
	}
	type acc struct {
		entry DeploymentSummaryEntry
	}
	byID := map[string]*acc{}
	ensure := func(id string) *acc {
		a, ok := byID[id]
		if !ok {
			a = &acc{entry: DeploymentSummaryEntry{DeploymentID: id}}
			byID[id] = a
		}
		return a
	}

	// Deployments first, so idle ones exist with zeroed metrics.
	if deploymentStore != nil {
		deps, derr := deploymentStore.GetVisibleDeploymentsByAccount(acct.ID)
		if derr != nil {
			return AccountDeploymentsSummaryResponse{}, derr
		}
		for _, d := range deps {
			a := ensure(d.ID)
			a.entry.AgentName = d.AgentName
			a.entry.DisplayName = d.DisplayName
			a.entry.Namespace = d.Namespace
		}
	}

	for _, p := range daily {
		if p.Key == "" {
			// Spend with no deployment id. Dev-tool spend is added below as its
			// own entries below, keyed by source — including the unattributed
			// bucket for agent usage that never reported an agent.
			continue
		}
		a := ensure(p.Key)
		date := p.Day.UTC().Format(time.DateOnly)
		a.entry.Requests += int(p.Requests)
		a.entry.CostUSD += p.CostUSD
		a.entry.TotalTokens += int(p.Tokens)
		a.entry.CostOverTime = append(a.entry.CostOverTime,
			DeploymentDailyCost{Date: date, CostUSD: p.CostUSD})
		a.entry.RequestsOverTime = append(a.entry.RequestsOverTime,
			DeploymentDailyRequests{Date: date, Requests: int(p.Requests)})
		a.entry.TokensOverTime = append(a.entry.TokensOverTime,
			DeploymentDailyTokens{Date: date, TotalTokens: int(p.Tokens)})
		if p.Requests > 0 && date > a.entry.LastSeen {
			a.entry.LastSeen = date
		}
	}

	// Everything with no deployment id, split by source: dev-tool spend, which
	// never has one, and agent usage that didn't report which agent produced it.
	// Both get a synthetic entry so they reach the cards, chart and table through
	// the same lineage as real agents.
	untaggedFilter := filter
	untaggedFilter.Untagged = true
	untagged, derr := rollups.DailyBySource(ctx, acct.ID, window, untaggedFilter)
	if derr != nil {
		return AccountDeploymentsSummaryResponse{}, derr
	}
	for _, p := range untagged {
		var a *acc
		switch ad, ok := devtoolAdapterByKey(p.Key); {
		case ok:
			a = ensure(ad.Key)
			a.entry.DevtoolSourceKey = ad.Key
			a.entry.AgentName = ad.Label
		case p.Key == insightsrollup.SourceAgents:
			a = ensure(unattributedAgentKey)
			a.entry.IsUnattributed = true
			a.entry.AgentName = "Unattributed usage"
		default:
			// An unregistered source: no adapter to label or brand it, so there is
			// nothing honest to render. Skipping keeps it out of the table while
			// the actor grain still counts it in the People surfaces.
			continue
		}
		date := p.Day.UTC().Format(time.DateOnly)
		a.entry.Requests += int(p.Requests)
		a.entry.CostUSD += p.CostUSD
		a.entry.TotalTokens += int(p.Tokens)
		a.entry.CostOverTime = append(a.entry.CostOverTime,
			DeploymentDailyCost{Date: date, CostUSD: p.CostUSD})
		a.entry.RequestsOverTime = append(a.entry.RequestsOverTime,
			DeploymentDailyRequests{Date: date, Requests: int(p.Requests)})
		a.entry.TokensOverTime = append(a.entry.TokensOverTime,
			DeploymentDailyTokens{Date: date, TotalTokens: int(p.Tokens)})
		if (p.Requests > 0 || p.CostUSD > 0 || p.Tokens > 0) && date > a.entry.LastSeen {
			a.entry.LastSeen = date
		}
	}

	// Deployments that only appear in the facts are archived or deleted. Their
	// spend is deliberately kept, but without a name they render as blank rows —
	// so resolve them from the DB, which retains the row after undeploy, and
	// flag them archived. Only their names are missing; the metrics are already
	// aggregated above.
	var archivedIDs []string
	for id, a := range byID {
		if a.entry.AgentName == "" && a.entry.DisplayName == "" {
			archivedIDs = append(archivedIDs, id)
		}
	}
	if len(archivedIDs) > 0 && deploymentStore != nil {
		// Sorted so the lookup and any log line are deterministic across runs.
		sort.Strings(archivedIDs)
		archived, aerr := deploymentStore.GetDeploymentsByIDsForAccount(acct.ID, archivedIDs)
		if aerr != nil {
			// Non-fatal: the rows still carry correct spend, they just fall back
			// to being labelled by id rather than name.
			log.Warn("Insights rollup: failed to resolve archived deployment names",
				"account_id", acct.ID, "count", len(archivedIDs), "error", aerr)
		}
		for _, d := range archived {
			a, ok := byID[d.ID]
			if !ok {
				continue
			}
			a.entry.AgentName = d.AgentName
			a.entry.DisplayName = d.DisplayName
			a.entry.Namespace = d.Namespace
			a.entry.IsArchived = true
		}
	}

	// used_by chips come free from the measure grain — the same rows that carry
	// the spend already record which actors touched which deployment.
	pairs, err := rollups.Pairs(ctx, acct.ID, window, filter)
	if err != nil {
		return AccountDeploymentsSummaryResponse{}, err
	}
	for _, p := range pairs {
		if a, ok := byID[p.DeploymentID]; ok {
			a.entry.UsersUsed = append(a.entry.UsersUsed, p.ActorKey)
		}
	}

	out := make([]DeploymentSummaryEntry, 0, len(byID))
	for _, a := range byID {
		e := a.entry
		if e.AgentName == "" && e.DisplayName == "" {
			// Purged from the deployments table entirely, so no name survives.
			// Label it by id: the spend is real and hiding the row would
			// understate account cost, while a blank row reads as a bug.
			e.AgentName = e.DeploymentID
			e.IsArchived = true
		}
		if e.Requests > 0 {
			e.CostPerRequest = e.CostUSD / float64(e.Requests)
			// One decimal, because the client renders this through
			// formatCompact, which falls through to toLocaleString() below 1000
			// and would otherwise show three decimals of false precision.
			// Cost fields need no such treatment: formatCost already collapses
			// them to two decimals, so rounding here would only discard
			// precision the facts actually have.
			e.TokPerRequest = math.Round(float64(e.TotalTokens)/float64(e.Requests)*10) / 10
		}
		if e.UsersUsed == nil {
			e.UsersUsed = []string{}
		}
		out = append(out, e)
	}
	return AccountDeploymentsSummaryResponse{Deployments: out}, nil
}

// rollupUserEntries builds the People rows from the usage grain.
func rollupUserEntries(
	ctx context.Context,
	accountStore *account.AccountStore,
	rollups *insightsrollup.Store,
	deploymentStore *deploymentstore.Store,
	acct *account.Account,
	window insightsrollup.Window,
	filter insightsrollup.Filter,
) (AccountUsersSummaryResponse, error) {
	rows, err := rollups.PeopleRows(ctx, acct.ID, window, filter,
		// No paging here: the view model still sorts and pages in Go, so it
		// needs the full set. The clamp matches maxInsightsTableLimit, so a huge
		// account can't turn one request into an unbounded scan.
		insightsrollup.AgentRowOptions{SortColumn: "cost", Descending: true, Limit: 5000})
	if err != nil {
		return AccountUsersSummaryResponse{}, err
	}

	pairs, err := rollups.Pairs(ctx, acct.ID, window, filter)
	if err != nil {
		return AccountUsersSummaryResponse{}, err
	}

	pairedIDs := make([]string, 0, len(pairs))
	seen := map[string]struct{}{}
	for _, p := range pairs {
		if _, ok := seen[p.DeploymentID]; ok {
			continue
		}
		seen[p.DeploymentID] = struct{}{}
		pairedIDs = append(pairedIDs, p.DeploymentID)
	}
	depToAgent, err := rollupAgentRefs(accountStore, deploymentStore, acct, pairedIDs)
	if err != nil {
		return AccountUsersSummaryResponse{}, err
	}

	agentsByActor := map[string][]UserAgentRef{}
	for _, p := range pairs {
		if ref, ok := depToAgent[p.DeploymentID]; ok {
			agentsByActor[p.ActorKey] = append(agentsByActor[p.ActorKey], ref)
		}
	}

	// Dev-tool usage has no deployment id, so Pairs can't see it and a person who
	// used Claude Code would show no chip for it. These come from the (actor,
	// source) pairs instead, and are appended after the agent chips so a row reads
	// deployed agents first, then dev tools.
	actorSources, err := rollups.ActorSources(ctx, acct.ID, window, filter)
	if err != nil {
		return AccountUsersSummaryResponse{}, err
	}
	devtoolByActor := map[string][]string{}
	for _, as := range actorSources {
		if _, ok := devtoolAdapterByKey(as.Source); !ok {
			continue // 'agents' usage is already covered by the agent chips.
		}
		devtoolByActor[as.ActorKey] = append(devtoolByActor[as.ActorKey], as.Source)
	}

	users := make([]UserSummaryEntry, 0, len(rows))
	for _, r := range rows {
		entry := UserSummaryEntry{
			UserIdentity: UserIdentity{
				UserID:      r.ActorKey,
				UserDetails: UserDetails{Kind: classifyUserID(r.ActorKey)},
			},
			Requests:          int(r.Requests),
			CostUSD:           r.CostUSD,
			Tokens:            int(r.Tokens),
			AgentsUsed:        agentsByActor[r.ActorKey],
			DevtoolSourceKeys: devtoolByActor[r.ActorKey],
		}
		if r.LastSeen.Valid {
			// Date only: last_seen_at is stored at day granularity,
			// so an RFC3339 stamp would imply a precision the facts don't have.
			entry.LastSeen = r.LastSeen.Time.UTC().Format(time.DateOnly)
		}
		if entry.AgentsUsed == nil {
			entry.AgentsUsed = []UserAgentRef{}
		}
		users = append(users, entry)
	}
	return AccountUsersSummaryResponse{Users: users}, nil
}

// rollupAgentRefs resolves the agent chips shown on People rows.
//
// `Account` is the avatar/route-segment account, and it is NOT always this
// account: a deployment sourced from a public blueprint renders the publishing
// account's avatar, so hardcoding the viewing account gives every cross-account
// agent the wrong icon.
//
// extraIDs covers deployments referenced by the facts that are no longer
// visible — archived agents whose spend is retained. Without them a person whose
// only agent has since been deleted shows no chips at all, while the agents
// table still lists them under used_by.
func rollupAgentRefs(
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	acct *account.Account,
	extraIDs []string,
) (map[string]UserAgentRef, error) {
	out := map[string]UserAgentRef{}
	if deploymentStore == nil {
		return out, nil
	}

	deps, err := deploymentStore.GetVisibleDeploymentsByAccount(acct.ID)
	if err != nil {
		return nil, err
	}
	known := make(map[string]struct{}, len(deps))
	for _, d := range deps {
		known[d.ID] = struct{}{}
	}
	var missing []string
	for _, id := range extraIDs {
		if _, ok := known[id]; !ok && id != "" {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		archived, aerr := deploymentStore.GetDeploymentsByIDsForAccount(acct.ID, missing)
		if aerr != nil {
			return nil, aerr
		}
		deps = append(deps, archived...)
	}

	// Resolve publishing-account names once per source account rather than per
	// deployment; several deployments commonly share one blueprint source.
	srcNames := map[string]string{}
	for _, d := range deps {
		if d.SourceAccountID == nil || *d.SourceAccountID == "" || *d.SourceAccountID == acct.ID {
			continue
		}
		if _, done := srcNames[*d.SourceAccountID]; done {
			continue
		}
		srcNames[*d.SourceAccountID] = ""
		if a, gerr := accountStore.GetByID(*d.SourceAccountID); gerr == nil && a != nil {
			srcNames[*d.SourceAccountID] = a.Name
		}
	}

	for _, d := range deps {
		out[d.ID] = UserAgentRef{
			DeploymentID: d.ID,
			Name:         d.AgentName,
			Account:      agentAvatarAccount(acct, d.SourceAccountID, srcNames),
		}
	}
	return out, nil
}

// agentAvatarAccount picks the account whose avatar and route segment a chip
// should use: the publishing account for a deployment sourced from another
// account's public blueprint, otherwise the viewing account.
//
// Falls back to the viewing account when the source name couldn't be resolved,
// because a chip pointing at an empty account segment is a broken link, while
// the viewing account is at worst the wrong avatar.
func agentAvatarAccount(acct *account.Account, sourceAccountID *string, srcNames map[string]string) string {
	if sourceAccountID == nil || *sourceAccountID == "" || *sourceAccountID == acct.ID {
		return acct.Name
	}
	if name := srcNames[*sourceAccountID]; name != "" {
		return name
	}
	return acct.Name
}

// rollupSummary supplies the account-level series the view model reads: the
// per-day cost timeline and the per-day per-user breakdown behind the People
// chart.
func rollupSummary(
	ctx context.Context,
	rollups *insightsrollup.Store,
	acct *account.Account,
	window insightsrollup.Window,
	filter insightsrollup.Filter,
) (AccountObservabilitySummaryResponse, error) {
	// Per-actor, not per-day: the People chart counts distinct active people per
	// day, which a summed daily series cannot answer.
	daily, err := rollups.DailyByActor(ctx, acct.ID, window, filter)
	if err != nil {
		return AccountObservabilitySummaryResponse{}, err
	}

	var (
		summary  AccountObservabilitySummaryResponse
		byDate   = map[string]int{} // date → index into CostOverTimeByUser
		dayTotal = map[string]float64{}
		dayOrder []string
	)
	for _, p := range daily {
		date := p.Day.UTC().Format(time.DateOnly)
		idx, seen := byDate[date]
		if !seen {
			idx = len(summary.CostOverTimeByUser)
			byDate[date] = idx
			dayOrder = append(dayOrder, date)
			summary.CostOverTimeByUser = append(summary.CostOverTimeByUser,
				AccountCostOverTimeByUserEntry{Date: date})
		}
		// Key is the actor key; '' is system spend, passed through rather than
		// dropped so the People chart totals match the cards.
		summary.CostOverTimeByUser[idx].Users = append(summary.CostOverTimeByUser[idx].Users,
			AccountUserCost{
				UserIdentity: UserIdentity{UserID: p.Key},
				CostUSD:      p.CostUSD,
				Requests:     int(p.Requests),
				Tokens:       int(p.Tokens),
			})

		dayTotal[date] += p.CostUSD
		summary.Totals.CostUSD += p.CostUSD
		summary.Totals.Requests += int(p.Requests)
		summary.Totals.TotalTokens += int(p.Tokens)
	}

	summary.CostOverTime = make([]AccountCostOverTimeEntry, 0, len(dayOrder))
	for _, date := range dayOrder {
		summary.CostOverTime = append(summary.CostOverTime, AccountCostOverTimeEntry{
			Date: date,
			// NB: this field is named Models but carries deployment-scoped cost
			// on the account timeline — a pre-existing naming trap kept because
			// the wire contract is frozen.
			Models: []AccountModelCost{},
		})
	}
	return summary, nil
}

// rollupPresentDevtoolSources lists the dev-tool sources with any usage in the
// window, in registry order, for the Sources filter.
//
// A source unused in the window isn't offered at all: the filter would show a
// row that can only ever toggle zero spend.
func rollupPresentDevtoolSources(
	ctx context.Context,
	rollups *insightsrollup.Store,
	acct *account.Account,
	window insightsrollup.Window,
) ([]DevtoolSourceRef, error) {
	out := make([]DevtoolSourceRef, 0, len(devtoolAdapters))
	for _, ad := range devtoolAdapters {
		t, err := rollups.Totals(ctx, acct.ID, insightsrollup.GrainUsage, window,
			insightsrollup.Filter{OnlySources: []string{ad.Key}})
		if err != nil {
			return nil, err
		}
		if t.CostUSD == 0 && t.Tokens == 0 {
			continue
		}
		// A registered adapter is exactly what the filter renders, so the ref is a
		// conversion rather than a mapping.
		out = append(out, DevtoolSourceRef(ad))
	}
	return out, nil
}

func hiddenSourceKeys(hidden map[string]bool) []string {
	if len(hidden) == 0 {
		return nil
	}
	out := make([]string, 0, len(hidden))
	for key, on := range hidden {
		if on {
			out = append(out, key)
		}
	}
	return out
}

// parseInsightsDate parses the YYYY-MM-DD dates insightsPeriod produces.
func parseInsightsDate(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
