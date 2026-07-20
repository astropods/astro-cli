package handlers

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

// langfuseContext holds the resolved Langfuse client and deployment metadata.
type langfuseContext struct {
	Client       *langfuse.Client
	DeploymentID string
	UserID       string
}

// resolveLangfuseContext validates auth, looks up the deployment, and returns
// a Langfuse client using per-account credentials.
// Routes: /api/v1/deployments/:id/observability/...
func resolveLangfuseContext(
	c *gin.Context,
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
) (*langfuseContext, bool) {
	user, exists := middleware.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return nil, false
	}

	deploymentID := c.Param("id")

	dep, err := deploymentStore.GetDeploymentByID(deploymentID)
	if err != nil || dep == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return nil, false
	}

	isMember, err := accountStore.IsMember(dep.AccountID, user.ID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return nil, false
	}

	creds, err := langfuseStore.Get(dep.AccountID)
	if err != nil || creds == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "langfuse not configured for this account"})
		return nil, false
	}

	client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

	log.Debug("Resolving Langfuse observability context",
		"deployment_id", dep.ID, "user_id", user.ID,
	)

	return &langfuseContext{
		Client:       client,
		DeploymentID: dep.ID,
		UserID:       user.ID,
	}, true
}

// GetAccountLangfuseSummary returns aggregate observability data for all deployments
// in an account. Accepts optional `from`/`to` ISO-8601 query params; when absent the
// full history is queried. Two parallel Langfuse calls (current + prior period) drive
// % change stats. Active-agent count comes from the deployments table.
//
// The Insights client now omits from/to (slices client-side for instant toggles),
// so the prior-period branch never fires for that caller — it remains for any
// future / external consumer that wants change% server-side.
// GET /api/v1/accounts/:account/observability/summary
func GetAccountLangfuseSummary(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
	slackStore *slackidentity.Store,
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

		from := c.Query("from")
		to := c.Query("to")
		groupBy := c.Query("group_by") // "", "user"
		// When true, Langfuse queries skip the visible-deployment tag filter
		// so spend from archived deployments rolls into the headline KPIs and
		// the People-spend-over-time chart. Mirrors the deployments-summary
		// toggle — the Insights frontend passes both flags together.
		includeArchived := c.Query("include_archived") == "true"

		if (from == "") != (to == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from and to must both be provided or both omitted"})
			return
		}

		if groupBy != "" && groupBy != "user" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'group_by'; must be empty or 'user'"})
			return
		}

		hasPeriod := from != "" && to != ""

		if hasPeriod {
			if _, err := time.Parse(time.RFC3339, from); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'from' timestamp: must be RFC3339"})
				return
			}
			if _, err := time.Parse(time.RFC3339, to); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'to' timestamp: must be RFC3339"})
				return
			}
		}

		// Cache fast-path: canonical params (no bounded period) are pre-warmed
		// every RefreshInterval by the InsightsRefreshWorker. Bounded periods
		// still flow through live — they're rarely requested by Insights and
		// caching the cartesian product of from/to is not worth the storage.
		//
		// Cached payload is the un-resolved compute output; Slack-directory
		// merge and identity stamping happen here so profile/link churn
		// never sits in Redis. Falls through to the live compute path on
		// any unmarshal error so a poisoned key never blocks the response.
		if !hasPeriod {
			if bytes, ok := insightscache.Get(c.Request.Context(), cache, acct.ID, insightscache.EndpointSummary, insightscache.Params{
				GroupBy:         groupBy,
				IncludeArchived: includeArchived,
			}); ok {
				var cached AccountObservabilitySummaryResponse
				if uerr := json.Unmarshal(bytes, &cached); uerr == nil {
					ResolveAccountSummaryIdentities(log, slackStore, accountStore, &cached)
					c.JSON(http.StatusOK, cached)
					return
				} else {
					log.Warn("insights cache unmarshal failed; falling through to live compute",
						"account_id", acct.ID, "endpoint", "summary", "error", uerr)
				}
			}
		}

		resp, err := ComputeAccountSummary(c.Request.Context(), log, cfg, langfuseStore, deploymentStore, slackStore, acct, from, to, groupBy, includeArchived)
		if errors.Is(err, ErrAllLangfuseCallsFailed) {
			log.Warn("Langfuse account metrics unavailable; returning empty summary", "error", err)
			degraded := zeroAccountSummary(from, to, hasPeriod)
			degraded.MetricsUnavailable = true
			c.JSON(http.StatusOK, degraded)
			return
		}
		if err != nil {
			// Non-Langfuse error (e.g. deployment store DB failure). 500 so the
			// failure is visible rather than masked as a metrics outage.
			log.Error("Failed to compute account summary", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute account summary"})
			return
		}
		ResolveAccountSummaryIdentities(log, slackStore, accountStore, &resp)
		c.JSON(http.StatusOK, resp)
	}
}

// InsightsSummaryComputer adapts ComputeAccountSummary for the riverqueue
// InsightsRefreshWorker via dependency injection, breaking what would
// otherwise be a handlers⇄riverqueue import cycle.
//
// slackStore lives here so the compute path can translate linked Slack
// user_ids to their WorkOS id before aggregation — the stable identity
// layer that ends up in the cache. Dynamic profile/workspace metadata
// (name, avatar, workspace icon) is applied at read time by the
// handler's Resolve... pass, not by the worker.
type InsightsSummaryComputer struct {
	log             *logger.Logger
	cfg             *config.Config
	langfuseStore   *langfuse.Store
	deploymentStore *deploymentstore.Store
	accountStore    *account.AccountStore
	slackStore      *slackidentity.Store
}

// NewInsightsSummaryComputer wires the dependencies the worker needs.
func NewInsightsSummaryComputer(
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	deploymentStore *deploymentstore.Store,
	accountStore *account.AccountStore,
	slackStore *slackidentity.Store,
) *InsightsSummaryComputer {
	return &InsightsSummaryComputer{
		log:             log,
		cfg:             cfg,
		langfuseStore:   langfuseStore,
		deploymentStore: deploymentStore,
		accountStore:    accountStore,
		slackStore:      slackStore,
	}
}

// ComputeSummary satisfies the riverqueue.InsightsSummaryComputer contract.
// Returns JSON-marshaled response bytes the worker writes into Redis verbatim.
func (c *InsightsSummaryComputer) ComputeSummary(ctx context.Context, accountID, groupBy string, includeArchived bool) ([]byte, error) {
	acct, err := c.lookupAccount(accountID)
	if err != nil {
		return nil, err
	}
	resp, err := ComputeAccountSummary(ctx, c.log, c.cfg, c.langfuseStore, c.deploymentStore, c.slackStore, acct, "" /*from*/, "" /*to*/, groupBy, includeArchived)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}

// ComputeDeploymentsSummary satisfies the riverqueue contract.
func (c *InsightsSummaryComputer) ComputeDeploymentsSummary(ctx context.Context, accountID string, includeArchived bool) ([]byte, error) {
	acct, err := c.lookupAccount(accountID)
	if err != nil {
		return nil, err
	}
	resp, err := ComputeDeploymentsSummary(ctx, c.log, c.cfg, c.langfuseStore, c.deploymentStore, c.slackStore, acct, "" /*from*/, "" /*to*/, includeArchived)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}

// ComputeUsersSummary satisfies the riverqueue contract.
func (c *InsightsSummaryComputer) ComputeUsersSummary(ctx context.Context, accountID string) ([]byte, error) {
	acct, err := c.lookupAccount(accountID)
	if err != nil {
		return nil, err
	}
	resp, err := ComputeUsersSummary(ctx, c.log, c.cfg, c.langfuseStore, c.deploymentStore, c.accountStore, c.slackStore, acct, "" /*from*/, "" /*to*/)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}

func (c *InsightsSummaryComputer) lookupAccount(accountID string) (*account.Account, error) {
	acct, err := c.accountStore.GetByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if acct == nil {
		return nil, fmt.Errorf("account %s not found", accountID)
	}
	return acct, nil
}

// ComputeAccountSummary runs the Langfuse fan-out and assembles the
// account-summary response. Both the request handler and the periodic
// refresh worker call this; the cache layer lives in their callers.
//
// A returned error means the upstream Langfuse query failed — callers
// degrade differently (handler returns metrics_unavailable=true; worker
// preserves the previously cached value by skipping the write).
//
// Identity layering: at compute time we translate any Langfuse row whose
// user_id is a bare Slack ID with a known slack→workos link in the
// directory; aggregation then naturally folds linked Slack spend into
// the WorkOS user's bucket. Profile/workspace metadata (name, avatar,
// workspace icon) is intentionally NOT stamped here — that's left to
// ResolveAccountSummaryIdentities at response time so display data
// stays fresh without cache churn.
func ComputeAccountSummary(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	deploymentStore *deploymentstore.Store,
	slackStore *slackidentity.Store,
	acct *account.Account,
	from, to, groupBy string,
	includeArchived bool,
) (AccountObservabilitySummaryResponse, error) {
	hasPeriod := from != "" && to != ""

	creds, err := langfuseStore.Get(acct.ID)
	if err != nil || creds == nil {
		return zeroAccountSummary(from, to, hasPeriod), nil
	}

	// Scope all Langfuse queries to currently-live deployments. Deleted
	// (undeployed) deployments' historical traces are NOT surfaced — same
	// contract as the deployment-detail page.
	var deps []*deploymentstore.Deployment
	if deploymentStore != nil {
		deps, err = deploymentStore.GetVisibleDeploymentsByAccount(acct.ID)
		if err != nil {
			return AccountObservabilitySummaryResponse{}, fmt.Errorf("list deployments: %w", err)
		}
	}
	if len(deps) == 0 {
		return zeroAccountSummary(from, to, hasPeriod), nil
	}

	const maxDeployments = 100
	if len(deps) > maxDeployments {
		log.Warn("Truncating deployments for account summary",
			"account", acct.Name, "total", len(deps), "cap", maxDeployments)
		deps = deps[:maxDeployments]
	}

	visibleTagValues := make([]string, len(deps))
	for i, d := range deps {
		visibleTagValues[i] = "deployment:" + d.ID
	}

	client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var currentMetrics, priorMetrics []langfuse.DailyMetric
	var activeDepIDs map[string]bool
	var userCostRows []map[string]any
	var modelStatsRows []map[string]any
	g, gCtx := errgroup.WithContext(ctx)

	// Tally Langfuse call outcomes so we surface ErrAllLangfuseCallsFailed
	// only when *every* sub-query failed. Partial failures keep rendering
	// with whatever data did come back — same shape as ComputeDeployments-
	// Summary and ComputeUsersSummary. The prior-period goroutine is
	// already fail-open by design and is excluded from the tally.
	var lfAttempts, lfFailures atomic.Int32

	applyTagFilter := !includeArchived
	g.Go(func() error {
		var err error
		currentMetrics, activeDepIDs, err = accountDailyMetrics(gCtx, client, visibleTagValues, from, to, applyTagFilter)
		lfAttempts.Add(1)
		if err != nil {
			lfFailures.Add(1)
			log.Warn("Account daily metrics query failed", "error", err)
		}
		return nil
	})

	if hasPeriod {
		priorFrom, priorTo := shiftPrior(from, to)
		g.Go(func() error {
			// Prior-period failures degrade the % change tile to "—" but
			// shouldn't fail the whole response — fail-open. Not tallied.
			priorMetrics, _, _ = accountDailyMetrics(gCtx, client, visibleTagValues, priorFrom, priorTo, applyTagFilter)
			return nil
		})
	}

	g.Go(func() error {
		// Per-model request count + latency percentiles for the current period,
		// grouped by model with no time dimension so the percentiles are
		// computed over the whole period (averaging per-day percentiles would be
		// wrong). Cost and tokens are rolled up separately from the daily
		// metrics; this only supplies requests + p50/p95. Fail-open: a failure
		// here just leaves those fields zero, so the cost/token breakdown still
		// renders.
		qFrom, qTo := metricsTimeRange(from, to)
		q := langfuse.MetricsQuery{
			View: "observations",
			Metrics: []langfuse.MetricsQueryField{
				{Measure: "count", Aggregation: "count"},
				{Measure: "latency", Aggregation: "p50"},
				{Measure: "latency", Aggregation: "p95"},
			},
			Dimensions:    []langfuse.MetricsDimension{{Field: "providedModelName"}},
			FromTimestamp: qFrom,
			ToTimestamp:   qTo,
		}
		if applyTagFilter {
			q.Filters = []langfuse.MetricsFilter{
				{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: visibleTagValues},
			}
		}
		resp, ferr := client.GetMetrics(gCtx, q)
		lfAttempts.Add(1)
		if ferr != nil {
			lfFailures.Add(1)
			log.Warn("Per-model stats query failed; model latency/requests will be zero", "error", ferr)
			return nil
		}
		modelStatsRows = resp.Data
		return nil
	})

	if groupBy == "user" {
		g.Go(func() error {
			// View: "traces" mirrors the users-summary Q_main query so the
			// chart's per-user cost matches the table's per-user cost. The
			// observations view double-counts spans within a trace and
			// produced a chart that didn't reconcile with the row totals.
			qFrom, qTo := metricsTimeRange(from, to)
			q := langfuse.MetricsQuery{
				View: "traces",
				Metrics: []langfuse.MetricsQueryField{
					{Measure: "totalCost", Aggregation: "sum"},
					{Measure: "count", Aggregation: "count"},
					{Measure: "totalTokens", Aggregation: "sum"},
				},
				Dimensions:    []langfuse.MetricsDimension{{Field: "userId"}},
				TimeDimension: &langfuse.TimeDimension{Granularity: "day"},
				FromTimestamp: qFrom,
				ToTimestamp:   qTo,
			}
			if !includeArchived {
				q.Filters = []langfuse.MetricsFilter{
					{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: visibleTagValues},
				}
			}
			resp, ferr := client.GetMetrics(gCtx, q)
			lfAttempts.Add(1)
			if ferr != nil {
				lfFailures.Add(1)
				log.Warn("UserId-grouped cost query failed; active-users chart will be empty", "error", ferr)
				return nil
			}
			userCostRows = resp.Data
			return nil
		})
	}
	_ = g.Wait()

	attempts := lfAttempts.Load()
	if attempts > 0 && attempts == lfFailures.Load() {
		return AccountObservabilitySummaryResponse{}, ErrAllLangfuseCallsFailed
	}

	activeAgents := 0
	if hasPeriod {
		seen := make(map[string]bool)
		for _, d := range deps {
			if activeDepIDs[d.ID] {
				seen[d.AgentName] = true
			}
		}
		activeAgents = len(seen)
	} else {
		names := make(map[string]bool, len(deps))
		for _, d := range deps {
			names[d.AgentName] = true
		}
		activeAgents = len(names)
	}

	resp := buildAccountSummary(currentMetrics, priorMetrics, hasPeriod, from, to, activeAgents, parseModelStats(modelStatsRows))
	if groupBy == "user" {
		// Translate linked Slack user_ids to their WorkOS id before
		// bucketing, so Bob's bare-Slack spend and Astro spend fold
		// into one row instead of two parallel ones.
		translateLinkedSlackUserIDs(log, slackStore, "account-summary", userCostRows)
		resp.CostOverTimeByUser = buildCostOverTimeByUser(userCostRows)
		resp.CostByModel = []AccountCostByModelEntry{}
	}
	return resp, nil
}

// buildCostOverTimeByUser groups the per-(user, day) Langfuse rows into per-day
// entries with the user breakdown nested inside. Sorted by date ascending.
// Each entry carries cost + requests + tokens so the client can slice the
// per-(day, user) data into any range window without an extra round-trip.
//
// Emits rows keyed by the user_id values it receives. Linked Slack IDs are
// translated before this function runs; ResolveAccountSummaryIdentities only
// hydrates display/profile metadata at response time.
func buildCostOverTimeByUser(rows []map[string]any) []AccountCostOverTimeByUserEntry {
	type userBucket struct {
		userID   string
		cost     float64
		requests int
		tokens   int
	}
	byDateUser := make(map[string]map[string]*userBucket)
	for _, row := range rows {
		ts, _ := row[langfuseTimeDimensionKey].(string)
		if ts == "" {
			continue
		}
		// Langfuse returns the day bucket as RFC3339; strip to YYYY-MM-DD so it
		// lines up with the model-grouped cost_over_time dates.
		date := ts
		if len(ts) >= 10 {
			date = ts[:10]
		}
		userID, _ := row["userId"].(string)
		userID = normalizeUserID(userID)
		cost := toFloat(row["sum_totalCost"])
		requests := toInt(row["count_count"])
		tokens := toInt(row["sum_totalTokens"])
		if cost <= 0 && requests == 0 && tokens == 0 {
			continue
		}
		byUser, ok := byDateUser[date]
		if !ok {
			byUser = make(map[string]*userBucket)
			byDateUser[date] = byUser
		}
		bucket, ok := byUser[userID]
		if !ok {
			bucket = &userBucket{userID: userID}
			byUser[userID] = bucket
		}
		bucket.cost += cost
		bucket.requests += requests
		bucket.tokens += tokens
	}

	dates := make([]string, 0, len(byDateUser))
	for d := range byDateUser {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	out := make([]AccountCostOverTimeByUserEntry, 0, len(dates))
	for _, d := range dates {
		byUser := byDateUser[d]
		users := make([]AccountUserCost, 0, len(byUser))
		for _, b := range byUser {
			users = append(users, AccountUserCost{
				UserIdentity: UserIdentity{
					UserID:      b.userID,
					UserDetails: UserDetails{Kind: classifyUserID(b.userID)},
				},
				CostUSD:  math.Round(b.cost*10000) / 10000,
				Requests: b.requests,
				Tokens:   b.tokens,
			})
		}
		out = append(out, AccountCostOverTimeByUserEntry{Date: d, Users: users})
	}
	return out
}

// translateLinkedSlackUserIDs rewrites the "userId" field of every raw
// Langfuse row whose value is a bare Slack user id with a known
// slack→workos link in the directory. Downstream aggregation then sums
// linked Slack and Astro spend into the same WorkOS-keyed bucket
// instead of producing two parallel rows. Unlinked bare-Slack rows are
// left alone — they survive the cache write and pick up profile +
// workspace metadata via the read-time resolvers.
//
// rowSets covers the typical "main metrics + tags attribution" pair —
// pass any number of pre-aggregation row slices and they're rewritten
// in place. A nil store, an empty row set, or a directory lookup error
// is treated as a no-op (rows aggregate as raw Slack ids; the page
// degrades to the pre-link shape rather than failing).
func translateLinkedSlackUserIDs(
	log *logger.Logger,
	slackStore *slackidentity.Store,
	contextLabel string,
	rowSets ...[]map[string]any,
) {
	if slackStore == nil {
		return
	}
	bare := map[string]struct{}{}
	for _, rows := range rowSets {
		for _, row := range rows {
			uid, _ := row["userId"].(string)
			uid = normalizeUserID(uid)
			if slackidentity.IsBareSlackUserID(uid) {
				bare[uid] = struct{}{}
			}
		}
	}
	if len(bare) == 0 {
		return
	}
	ids := make([]string, 0, len(bare))
	for id := range bare {
		ids = append(ids, id)
	}
	entries, err := slackStore.DirectoryEntriesForSlackUserIDs(ids)
	if err != nil {
		log.Warn(contextLabel+": slack→workos link lookup failed; rows aggregate as raw slack ids", "error", err)
		return
	}
	linkMap := make(map[string]string, len(entries))
	for slackID, entry := range entries {
		if entry.WorkOSUserID != "" {
			linkMap[slackID] = entry.WorkOSUserID
		}
	}
	applyLinkedSlackUserIDTranslation(linkMap, rowSets...)
}

// applyLinkedSlackUserIDTranslation does the pure rewrite step from
// translateLinkedSlackUserIDs. Exposed so tests can drive it with a
// synthetic link map instead of standing up a Slack store.
func applyLinkedSlackUserIDTranslation(linkMap map[string]string, rowSets ...[]map[string]any) {
	if len(linkMap) == 0 {
		return
	}
	for _, rows := range rowSets {
		for _, row := range rows {
			uid, _ := row["userId"].(string)
			if mapped, ok := linkMap[uid]; ok {
				row["userId"] = mapped
			}
		}
	}
}

// zeroAccountSummary returns an empty response with the correct shape when
// Langfuse is not configured for the account.
func zeroAccountSummary(from, to string, hasPeriod bool) AccountObservabilitySummaryResponse {
	resp := AccountObservabilitySummaryResponse{
		Period:       buildPeriod(from, to),
		Totals:       AccountSummaryTotals{},
		DailyAvg:     AccountSummaryDailyAvg{},
		CostOverTime: []AccountCostOverTimeEntry{},
		CostByModel:  []AccountCostByModelEntry{},
	}
	if hasPeriod {
		resp.Change = &AccountSummaryChange{} // all nil fields — prior period is also zero
	}
	return resp
}

// modelStats holds the per-model request count and latency percentiles from the
// model-grouped observations query (period-level, not per-day).
type modelStats struct {
	Requests int
	P50Ms    float64
	P95Ms    float64
}

// parseModelStats turns the raw model-grouped observations rows into a map keyed
// by model. Latency is returned by Langfuse in seconds; convert to ms. Rows
// without a model (non-LLM observations) are skipped. Tolerant of missing keys
// so a partial or empty response degrades to zeroed fields.
func parseModelStats(rows []map[string]any) map[string]modelStats {
	out := make(map[string]modelStats, len(rows))
	for _, row := range rows {
		model, _ := row["providedModelName"].(string)
		if model == "" {
			continue
		}
		out[model] = modelStats{
			Requests: toInt(row["count_count"]),
			P50Ms:    toFloat(row["p50_latency"]) * 1000,
			P95Ms:    toFloat(row["p95_latency"]) * 1000,
		}
	}
	return out
}

// buildAccountSummary aggregates DailyMetric slices into the full response shape.
func buildAccountSummary(
	current, prior []langfuse.DailyMetric,
	hasPeriod bool,
	from, to string,
	activeAgents int,
	modelStatsByModel map[string]modelStats,
) AccountObservabilitySummaryResponse {
	// Aggregate current period.
	var totalCost float64
	var totalRequests, totalInput, totalOutput int
	costByDay := make(map[string][]AccountModelCost)
	costByModel := make(map[string]float64)
	tokensByModel := make(map[string]int)

	for _, m := range current {
		totalRequests += m.CountTraces
		totalCost += m.TotalCost
		totalInput += m.InputTokens()
		totalOutput += m.OutputTokens()

		if len(m.Usage) > 0 {
			models := make([]AccountModelCost, 0, len(m.Usage))
			for _, u := range m.Usage {
				tokensByModel[u.Model] += u.InputUsage + u.OutputUsage
				if u.TotalCost > 0 {
					models = append(models, AccountModelCost{Model: u.Model, CostUSD: u.TotalCost})
					costByModel[u.Model] += u.TotalCost
				}
			}
			if len(models) > 0 {
				costByDay[m.Date] = models
			}
		}
	}
	totalModelTokens := totalInput + totalOutput

	// Build per-day sparkline values (all dates, not just days with model breakdowns).
	allDays := make(map[string]struct{})
	dailyCost := make(map[string]float64)
	dailyRequests := make(map[string]int)
	dailyTokens := make(map[string]int)
	for _, m := range current {
		allDays[m.Date] = struct{}{}
		dailyCost[m.Date] = m.TotalCost
		dailyRequests[m.Date] = m.CountTraces
		dailyTokens[m.Date] = m.InputTokens() + m.OutputTokens()
	}

	// cost_over_time: sorted by date ascending.
	dates := make([]string, 0, len(allDays))
	for d := range allDays {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	costOverTime := make([]AccountCostOverTimeEntry, 0, len(dates))
	for _, d := range dates {
		if models, ok := costByDay[d]; ok {
			costOverTime = append(costOverTime, AccountCostOverTimeEntry{Date: d, Models: models})
		}
	}

	sparklines := AccountSparklines{
		Cost:     make([]float64, 0, len(dates)),
		Requests: make([]int, 0, len(dates)),
		Tokens:   make([]int, 0, len(dates)),
	}
	for _, d := range dates {
		sparklines.Cost = append(sparklines.Cost, math.Round(dailyCost[d]*10000)/10000)
		sparklines.Requests = append(sparklines.Requests, dailyRequests[d])
		sparklines.Tokens = append(sparklines.Tokens, dailyTokens[d])
	}

	// cost_by_model: sorted by cost descending with percentage.
	modelEntries := make([]AccountCostByModelEntry, 0, len(costByModel))
	for model, cost := range costByModel {
		modelEntries = append(modelEntries, AccountCostByModelEntry{Model: model, CostUSD: cost})
	}
	sort.Slice(modelEntries, func(i, j int) bool {
		return modelEntries[i].CostUSD > modelEntries[j].CostUSD
	})
	for i := range modelEntries {
		if totalCost > 0 {
			modelEntries[i].CostPct = math.Round(modelEntries[i].CostUSD/totalCost*1000) / 10
		}
		model := modelEntries[i].Model
		modelEntries[i].TotalTokens = tokensByModel[model]
		if totalModelTokens > 0 {
			modelEntries[i].TokenPct = math.Round(float64(tokensByModel[model])/float64(totalModelTokens)*1000) / 10
		}
		if s, ok := modelStatsByModel[model]; ok {
			modelEntries[i].Requests = s.Requests
			modelEntries[i].P50LatencyMs = math.Round(s.P50Ms*10) / 10
			modelEntries[i].P95LatencyMs = math.Round(s.P95Ms*10) / 10
		}
	}

	period := buildPeriod(from, to)
	days := float64(period.Days)

	dailyAvg := AccountSummaryDailyAvg{}
	if days > 0 {
		dailyAvg = AccountSummaryDailyAvg{
			CostUSD:  math.Round(totalCost/days*100) / 100,
			Requests: math.Round(float64(totalRequests)/days*100) / 100,
			Tokens:   math.Round(float64(totalInput+totalOutput)/days*100) / 100,
		}
	}

	totals := AccountSummaryTotals{
		CostUSD:      math.Round(totalCost*100) / 100,
		Requests:     totalRequests,
		InputTokens:  totalInput,
		OutputTokens: totalOutput,
		TotalTokens:  totalInput + totalOutput,
		ActiveAgents: activeAgents,
	}

	// Change vs prior period.
	var change *AccountSummaryChange
	if hasPeriod {
		var priorCost float64
		var priorRequests, priorInput, priorOutput int
		for _, m := range prior {
			priorRequests += m.CountTraces
			priorCost += m.TotalCost
			priorInput += m.InputTokens()
			priorOutput += m.OutputTokens()
		}
		change = &AccountSummaryChange{
			CostPct:     pctChange(totalCost, priorCost),
			RequestsPct: pctChange(float64(totalRequests), float64(priorRequests)),
			TokensPct:   pctChange(float64(totalInput+totalOutput), float64(priorInput+priorOutput)),
		}
	}

	return AccountObservabilitySummaryResponse{
		Period:       period,
		Totals:       totals,
		DailyAvg:     dailyAvg,
		Change:       change,
		CostOverTime: costOverTime,
		CostByModel:  modelEntries,
		Sparklines:   sparklines,
	}
}

// buildPeriod parses from/to into an AccountSummaryPeriod.
func buildPeriod(from, to string) AccountSummaryPeriod {
	if from == "" || to == "" {
		return AccountSummaryPeriod{}
	}
	f, err1 := time.Parse(time.RFC3339, from)
	t, err2 := time.Parse(time.RFC3339, to)
	if err1 != nil || err2 != nil {
		return AccountSummaryPeriod{Start: from, End: to}
	}
	days := int(t.Sub(f).Hours() / 24)
	return AccountSummaryPeriod{Start: from, End: to, Days: days}
}

// shiftPrior computes the prior period of equal length immediately before from.
// Precondition: from and to are valid RFC3339 strings (enforced by the handler).
func shiftPrior(from, to string) (string, string) {
	f, _ := time.Parse(time.RFC3339, from)
	t, _ := time.Parse(time.RFC3339, to)
	d := t.Sub(f)
	return f.Add(-d).UTC().Format(time.RFC3339), f.UTC().Format(time.RFC3339)
}

// pctChange returns the % change from prior to current rounded to one decimal place,
// or nil when prior is 0 (undefined).
func pctChange(current, prior float64) *float64 {
	if prior == 0 {
		return nil
	}
	v := math.Round((current-prior)/prior*1000) / 10
	return &v
}

// metricsTimeRange returns a (from, to) pair safe to pass to /api/public/metrics,
// which 400s on empty timestamps. When the caller asked for all-time (both
// inputs empty), backfill a 5-year lookback ending now — long enough to be
// "all-time" for any account in practice and avoids divergent semantics
// between the legacy /metrics/daily endpoint (which accepts empty
// timestamps natively) and /metrics (which doesn't).
//
// The Insights People + Agents tables render server-aggregated per-user
// and per-deployment totals over the FULL query window — they're
// independent of the range toggle, which only resizes charts and KPIs.
// So this fallback width directly determines how far back lifetime
// totals reach. Bounding it tighter would clip the "$X over your time on
// Astro" semantics those tables are supposed to convey. Q_main's load
// problem is addressed via day-granularity bucketing in
// ComputeUsersSummary, not by clipping the time window here.
func metricsTimeRange(from, to string) (string, string) {
	if from != "" && to != "" {
		return from, to
	}
	now := time.Now().UTC()
	return now.AddDate(-5, 0, 0).Format(time.RFC3339), now.Format(time.RFC3339)
}

// normalizeUserID collapses the SDK-emitted "-" sentinel into "" so callers
// only need to check one shape of "no user".
func normalizeUserID(s string) string {
	if s == "-" {
		return ""
	}
	return s
}

// classifyUserID picks the discriminator for a Langfuse user_id based on
// the id's shape alone — no directory lookup. A bare Slack id maps to
// "slack" (even when we have no profile data for them); a WorkOS-prefixed
// id maps to "astro"; anything else (or empty) is "unknown".
func classifyUserID(userID string) UserDetailsKind {
	if userID == "" {
		return UserDetailsKindUnknown
	}
	if slackidentity.IsBareSlackUserID(userID) {
		return UserDetailsKindSlack
	}
	if strings.HasPrefix(userID, "user_") {
		return UserDetailsKindAstro
	}
	return UserDetailsKindUnknown
}

// userDetailsFromEntry builds a UserDetails for the given user_id,
// folding in profile metadata when entry is non-nil and the user_id
// classifies as Slack. For astro / unknown / Slack-without-entry rows
// it returns a kind-only UserDetails with no other fields set.
func userDetailsFromEntry(userID string, entry *slackidentity.DirectoryEntry) UserDetails {
	kind := classifyUserID(userID)
	if kind != UserDetailsKindSlack || entry == nil {
		return UserDetails{Kind: kind}
	}
	return UserDetails{
		Kind:        UserDetailsKindSlack,
		TeamID:      entry.TeamID,
		DisplayName: entry.Profile.DisplayName,
		Username:    entry.Profile.Username,
		AvatarURL:   entry.Profile.AvatarURL,
		IsBot:       entry.Profile.IsBot,
		Deleted:     entry.Profile.Deleted,
	}
}

type slackDirectoryEntries map[string]slackidentity.DirectoryEntry

func lookupSlackDirectoryEntries(log *logger.Logger, slackStore *slackidentity.Store, rows []UserSummaryEntry, contextLabel string) slackDirectoryEntries {
	out := slackDirectoryEntries{}
	if slackStore == nil || len(rows) == 0 {
		return out
	}

	unscopedSlackIDs := make([]string, 0, len(rows))
	seenUnscoped := make(map[string]struct{}, len(rows))
	for _, u := range rows {
		if !slackidentity.IsBareSlackUserID(u.UserID) {
			continue
		}
		if _, ok := seenUnscoped[u.UserID]; ok {
			continue
		}
		seenUnscoped[u.UserID] = struct{}{}
		unscopedSlackIDs = append(unscopedSlackIDs, u.UserID)
	}

	if len(unscopedSlackIDs) > 0 {
		entries, err := slackStore.DirectoryEntriesForSlackUserIDs(unscopedSlackIDs)
		if err != nil {
			log.Warn(contextLabel+": unscoped slack directory lookup failed; legacy deep links unavailable", "error", err)
		} else {
			out = entries
		}
	}
	return out
}

func traceIdentityLookupRows(traces []langfuse.Trace) []UserSummaryEntry {
	rows := make([]UserSummaryEntry, 0, len(traces))
	for _, t := range traces {
		if uid := normalizeUserID(t.UserID); uid != "" {
			rows = append(rows, UserSummaryEntry{UserIdentity: UserIdentity{UserID: uid}})
		}
	}
	return rows
}

// traceUserDetails builds the per-trace UserDetails. Returns nil when
// the trace has no user — the JSON encoder drops the field on output.
func traceUserDetails(log *logger.Logger, slackStore *slackidentity.Store, userID string, contextLabel string) *UserDetails {
	userID = normalizeUserID(userID)
	if userID == "" {
		return nil
	}
	rows := []UserSummaryEntry{{UserIdentity: UserIdentity{UserID: userID}}}
	return traceUserDetailsFromDirectory(userID, lookupSlackDirectoryEntries(log, slackStore, rows, contextLabel))
}

// traceUserDetailsFromDirectory is the directory-driven inner of
// traceUserDetails — exposed so the per-trace-list batch can do one
// directory lookup and then build details per row.
func traceUserDetailsFromDirectory(userID string, entries slackDirectoryEntries) *UserDetails {
	userID = normalizeUserID(userID)
	if userID == "" {
		return nil
	}
	if entry, ok := entries[userID]; ok {
		d := userDetailsFromEntry(userID, &entry)
		return &d
	}
	d := userDetailsFromEntry(userID, nil)
	return &d
}

func traceUserDetailsFromHydrator(userID string, hydrator *userDetailsHydrator) *UserDetails {
	details := traceUserDetailsFromDirectory(userID, hydrator.slack)
	if details != nil {
		hydrator.stamp(userID, details)
	}
	return details
}

// tagStrings flattens a Langfuse `tags` group-by value, which can come back
// as either a single string or a JSON array, into a slice.
func tagStrings(v any) []string {
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// accountDailyMetrics builds the per-day account timeline from two batched
// /metrics queries instead of the N per-deployment fan-out we used to do.
//
//   - Q_traces: traces view, grouped by [tags, time]. Per-(deployment, day)
//     trace count — drives CountTraces per day plus the active-deployment set
//     (any tag with count > 0 in any day).
//   - Q_obs: observations view, grouped by [providedModelName, time]. Per-
//     (model, day) totalCost + input/output usage — drives Usage breakdown and
//     daily totalCost (sum across models matches the legacy /metrics/daily
//     invariant). Observations view does NOT support `tags` as a grouping
//     dimension (only as a filter), so we cannot get per-(deployment, model)
//     split — only the global model breakdown.
//
// Returns merged []DailyMetric + active deps + error. Caller's downstream
// logic (buildAccountSummary) is unchanged.
// applyTagFilter=false drops the deployment-tag scope entirely so archived
// deployments' historical traces flow into the response. Callers pass false
// when the Insights "Show deleted" toggle is on. tagValues is still required
// (it scopes the `activeDepIDs` map on the live-only path); ignored when
// applyTagFilter is false.
func accountDailyMetrics(
	ctx context.Context,
	client *langfuse.Client,
	tagValues []string,
	from, to string,
	applyTagFilter bool,
) ([]langfuse.DailyMetric, map[string]bool, error) {
	if applyTagFilter && len(tagValues) == 0 {
		return nil, map[string]bool{}, nil
	}

	qFrom, qTo := metricsTimeRange(from, to)
	var tagFilter []langfuse.MetricsFilter
	if applyTagFilter {
		tagFilter = []langfuse.MetricsFilter{
			{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: tagValues},
		}
	}

	var (
		tracesRows []map[string]any
		obsRows    []map[string]any
	)
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		q := langfuse.MetricsQuery{
			View: "traces",
			Metrics: []langfuse.MetricsQueryField{
				{Measure: "count", Aggregation: "count"},
			},
			Dimensions:    []langfuse.MetricsDimension{{Field: "tags"}},
			TimeDimension: &langfuse.TimeDimension{Granularity: "day"},
			Filters:       tagFilter,
			FromTimestamp: qFrom,
			ToTimestamp:   qTo,
		}
		resp, err := client.GetMetrics(gCtx, q)
		if err != nil {
			return fmt.Errorf("traces query: %w", err)
		}
		tracesRows = resp.Data
		return nil
	})

	g.Go(func() error {
		q := langfuse.MetricsQuery{
			View: "observations",
			Metrics: []langfuse.MetricsQueryField{
				{Measure: "totalCost", Aggregation: "sum"},
				{Measure: "inputTokens", Aggregation: "sum"},
				{Measure: "outputTokens", Aggregation: "sum"},
			},
			Dimensions:    []langfuse.MetricsDimension{{Field: "providedModelName"}},
			TimeDimension: &langfuse.TimeDimension{Granularity: "day"},
			Filters:       tagFilter,
			FromTimestamp: qFrom,
			ToTimestamp:   qTo,
		}
		resp, err := client.GetMetrics(gCtx, q)
		if err != nil {
			return fmt.Errorf("observations query: %w", err)
		}
		obsRows = resp.Data
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	// Q_traces → per-day count + active deployments.
	countByDate := make(map[string]int)
	activeDeps := make(map[string]bool)
	for _, row := range tracesRows {
		date := dateFromTimeDim(row[langfuseTimeDimensionKey])
		cnt := toInt(row["count_count"])
		if cnt == 0 {
			continue
		}
		countByDate[date] += cnt
		for _, tag := range tagStrings(row["tags"]) {
			if strings.HasPrefix(tag, "deployment:") {
				activeDeps[strings.TrimPrefix(tag, "deployment:")] = true
			}
		}
	}

	// Q_obs → per-(date, model) usage.
	usageByDate := make(map[string]map[string]langfuse.DailyMetricUsage)
	for _, row := range obsRows {
		date := dateFromTimeDim(row[langfuseTimeDimensionKey])
		if date == "" {
			continue
		}
		model, _ := row["providedModelName"].(string)
		if model == "" {
			// Langfuse returns nil for traces without a model attribution
			// (non-LLM observations). Skip — they don't belong in the model
			// breakdown and their cost is captured via the traces-view query.
			continue
		}
		cost := toFloat(row["sum_totalCost"])
		input := toInt(row["sum_inputTokens"])
		output := toInt(row["sum_outputTokens"])
		if cost == 0 && input == 0 && output == 0 {
			continue
		}
		byModel := usageByDate[date]
		if byModel == nil {
			byModel = make(map[string]langfuse.DailyMetricUsage)
			usageByDate[date] = byModel
		}
		prev := byModel[model]
		byModel[model] = langfuse.DailyMetricUsage{
			Model:       model,
			InputUsage:  prev.InputUsage + input,
			OutputUsage: prev.OutputUsage + output,
			TotalUsage:  prev.TotalUsage + input + output,
			TotalCost:   prev.TotalCost + cost,
		}
	}

	// Merge into DailyMetric shape. Dates can appear in either query (e.g.
	// non-LLM days appear only in traces). Use union of both date sets.
	allDates := make(map[string]struct{})
	for d := range countByDate {
		allDates[d] = struct{}{}
	}
	for d := range usageByDate {
		allDates[d] = struct{}{}
	}

	out := make([]langfuse.DailyMetric, 0, len(allDates))
	for date := range allDates {
		usage := make([]langfuse.DailyMetricUsage, 0, len(usageByDate[date]))
		var totalCost float64
		for _, u := range usageByDate[date] {
			usage = append(usage, u)
			totalCost += u.TotalCost
		}
		out = append(out, langfuse.DailyMetric{
			Date:        date,
			CountTraces: countByDate[date],
			TotalCost:   totalCost,
			Usage:       usage,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, activeDeps, nil
}

// dateFromTimeDim normalizes Langfuse's RFC3339 time-bucket value to YYYY-MM-DD
// to match the legacy /metrics/daily endpoint's Date field shape.
func dateFromTimeDim(v any) string {
	ts, _ := v.(string)
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// ── deployments-summary ───────────────────────────────────────────────────────

// deploymentMetrics holds the raw Langfuse data fetched for one deployment.
type deploymentMetrics struct {
	DeploymentID string
	AgentName    string
	DailyMetrics []langfuse.DailyMetric
	P95LatencyMs float64
}

// fetchDeploymentDaily fetches the per-deployment daily metrics (cost / tokens
// / request count + per-model usage). Stays per-deployment because the legacy
// /metrics/daily endpoint is the only path to the per-(deployment, model)
// breakdown that powers TopModel — Langfuse's /metrics endpoint can't group
// observations by tags, so it can't produce per-(deployment, model) rows.
//
// On error the returned deploymentMetrics still has DeploymentID + AgentName
// populated so the row renders with zeros (per-row fail-open). The error is
// also returned so the compute path can tally whether every Langfuse call
// failed and surface MetricsUnavailable.
func fetchDeploymentDaily(ctx context.Context, client *langfuse.Client, dep *deploymentstore.Deployment, from, to string) (deploymentMetrics, error) {
	result := deploymentMetrics{DeploymentID: dep.ID, AgentName: dep.AgentName}
	daily, err := client.GetDailyMetrics(ctx, dep.ID, from, to)
	if err != nil {
		return result, err
	}
	result.DailyMetrics = daily
	return result, nil
}

// batchedP95Latencies fetches per-deployment P95 latency in a single batched
// /metrics call (traces view grouped by tags) instead of N separate ones.
// Returns map[deploymentID]p95Ms. On error returns an empty map AND the error
// — caller decides whether to log + fail-open (column renders as zero) or
// roll the failure into an all-failed tally to surface MetricsUnavailable.
func batchedP95Latencies(
	ctx context.Context,
	client *langfuse.Client,
	tagValues []string,
	from, to string,
) (map[string]float64, error) {
	out := make(map[string]float64)
	if len(tagValues) == 0 {
		return out, nil
	}
	qFrom, qTo := metricsTimeRange(from, to)
	q := langfuse.MetricsQuery{
		View:       "traces",
		Metrics:    []langfuse.MetricsQueryField{{Measure: "latency", Aggregation: "p95"}},
		Dimensions: []langfuse.MetricsDimension{{Field: "tags"}},
		Filters: []langfuse.MetricsFilter{
			{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: tagValues},
		},
		FromTimestamp: qFrom,
		ToTimestamp:   qTo,
	}
	resp, err := client.GetMetrics(ctx, q)
	if err != nil {
		return out, err
	}
	for _, row := range resp.Data {
		p95 := toFloat(row["p95_latency"])
		if p95 <= 0 {
			continue
		}
		for _, tag := range tagStrings(row["tags"]) {
			if strings.HasPrefix(tag, "deployment:") {
				depID := strings.TrimPrefix(tag, "deployment:")
				// Take max across rows that might both touch a deployment
				// (Langfuse can return tag-array rows per group).
				if p95 > out[depID] {
					out[depID] = p95
				}
			}
		}
	}
	return out, nil
}

func buildDeploymentUserRows(tagsRows []map[string]any) map[string][]UserSummaryEntry {
	byDep := make(map[string]map[string]UserSummaryEntry)
	for _, row := range tagsRows {
		userID, _ := row["userId"].(string)
		userID = normalizeUserID(userID)
		if userID == "" {
			continue
		}
		for _, tag := range tagStrings(row["tags"]) {
			if !strings.HasPrefix(tag, "deployment:") {
				continue
			}
			depID := strings.TrimPrefix(tag, "deployment:")
			set, exists := byDep[depID]
			if !exists {
				set = make(map[string]UserSummaryEntry)
				byDep[depID] = set
			}
			set[userID] = UserSummaryEntry{
				UserIdentity: UserIdentity{
					UserID:      userID,
					UserDetails: UserDetails{Kind: classifyUserID(userID)},
				},
			}
		}
	}

	out := make(map[string][]UserSummaryEntry, len(byDep))
	for depID, set := range byDep {
		rows := make([]UserSummaryEntry, 0, len(set))
		for _, row := range set {
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].UserID < rows[j].UserID
		})
		out[depID] = rows
	}
	return out
}

// buildDeploymentSummary turns the per-deployment metrics into one
// DeploymentSummaryEntry per deployment, sorted by cost descending. tagsRows
// is the per-(userId, tag) Langfuse response used to populate users_used;
// pass nil to skip. deployments carries the deployment-store rows so display
// metadata (display_name, namespace) can be threaded into each entry.
//
// Unlike the previous build helper, this one does NOT roll up by
// agent_name — multi-region deployments of the same blueprint show up as
// separate rows.
func buildDeploymentSummary(
	metrics []deploymentMetrics,
	tagsRows []map[string]any,
	deployments []*deploymentstore.Deployment,
	archivedIDs map[string]struct{},
) []DeploymentSummaryEntry {
	return buildDeploymentSummaryWithUsers(
		metrics,
		buildDeploymentUserRows(tagsRows),
		deployments,
		archivedIDs,
	)
}

func buildDeploymentSummaryWithUsers(
	metrics []deploymentMetrics,
	usersByDep map[string][]UserSummaryEntry,
	deployments []*deploymentstore.Deployment,
	archivedIDs map[string]struct{},
) []DeploymentSummaryEntry {
	// Sidecar: deployment_id → display metadata. Walk the deployments slice
	// once so the per-row build below can look up display_name / namespace
	// without scanning.
	type meta struct {
		displayName  string
		namespace    string
		undeployedAt *time.Time
	}
	depMeta := make(map[string]meta, len(deployments))
	for _, d := range deployments {
		depMeta[d.ID] = meta{
			displayName:  d.DisplayName,
			namespace:    d.Namespace,
			undeployedAt: d.UndeployedAt,
		}
	}

	entries := make([]DeploymentSummaryEntry, 0, len(metrics))
	for _, m := range metrics {
		// Per-day rollups for this single deployment.
		modelCosts := make(map[string]float64)
		dayCosts := make(map[string]float64)
		dayRequests := make(map[string]int)
		dayTokens := make(map[string][2]int)

		var requests, inputTokens, outputTokens int
		var costUSD float64
		for _, d := range m.DailyMetrics {
			requests += d.CountTraces
			costUSD += d.TotalCost
			inputTokens += d.InputTokens()
			outputTokens += d.OutputTokens()
			dayCosts[d.Date] += d.TotalCost
			dayRequests[d.Date] += d.CountTraces
			prev := dayTokens[d.Date]
			dayTokens[d.Date] = [2]int{prev[0] + d.InputTokens(), prev[1] + d.OutputTokens()}
			for _, u := range d.Usage {
				modelCosts[u.Model] += u.TotalCost
			}
		}

		topModel := ""
		var maxModelCost float64
		for model, cost := range modelCosts {
			if cost > maxModelCost {
				maxModelCost = cost
				topModel = model
			}
		}

		var costPerRequest, tokPerRequest float64
		if requests > 0 {
			costPerRequest = math.Round(costUSD/float64(requests)*10000) / 10000
			tokPerRequest = math.Round(float64(inputTokens+outputTokens)/float64(requests)*10) / 10
		}

		// Build *_over_time slices sorted by date ascending.
		dates := make([]string, 0, len(dayCosts))
		for d := range dayCosts {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		costOverTime := make([]DeploymentDailyCost, 0, len(dates))
		requestsOverTime := make([]DeploymentDailyRequests, 0, len(dates))
		tokensOverTime := make([]DeploymentDailyTokens, 0, len(dates))
		for _, d := range dates {
			costOverTime = append(costOverTime, DeploymentDailyCost{
				Date:    d,
				CostUSD: math.Round(dayCosts[d]*10000) / 10000,
			})
			requestsOverTime = append(requestsOverTime, DeploymentDailyRequests{
				Date:     d,
				Requests: dayRequests[d],
			})
			tok := dayTokens[d]
			tokensOverTime = append(tokensOverTime, DeploymentDailyTokens{
				Date:         d,
				InputTokens:  tok[0],
				OutputTokens: tok[1],
				TotalTokens:  tok[0] + tok[1],
			})
		}

		// User rows are post-translation: bare-Slack rows whose user has
		// a known slack→workos link already aggregated under the WorkOS
		// id in buildDeploymentUserRows, so no merge pass is needed
		// here. Bare-Slack rows that pass through are the unlinked /
		// unknown ones — Resolve...Identities stamps profile and
		// workspace fields on them at read time.
		userRows := usersByDep[m.DeploymentID]
		usersUsedSet := make(map[string]struct{}, len(userRows))
		for _, row := range userRows {
			usersUsedSet[row.UserID] = struct{}{}
		}
		usersUsed := make([]string, 0, len(usersUsedSet))
		for uid := range usersUsedSet {
			usersUsed = append(usersUsed, uid)
		}
		sort.Strings(usersUsed)
		usersUsedDetails := make([]UserIdentity, 0, len(userRows))
		for _, row := range userRows {
			usersUsedDetails = append(usersUsedDetails, row.UserIdentity)
		}
		sort.Slice(usersUsedDetails, func(i, j int) bool {
			return usersUsedDetails[i].UserID < usersUsedDetails[j].UserID
		})

		md := depMeta[m.DeploymentID]
		_, isArchived := archivedIDs[m.DeploymentID]
		// Drop archived deployments that contributed nothing to the
		// selected range — tombstones are only useful when there's spend to
		// preserve. Live deployments with zero spend still surface (a
		// configured-but-unused agent is meaningful signal).
		if isArchived && requests == 0 && costUSD == 0 {
			continue
		}
		entries = append(entries, DeploymentSummaryEntry{
			DeploymentID:     m.DeploymentID,
			AgentName:        m.AgentName,
			DisplayName:      md.displayName,
			Namespace:        md.namespace,
			Requests:         requests,
			CostUSD:          math.Round(costUSD*10000) / 10000,
			CostPerRequest:   costPerRequest,
			InputTokens:      inputTokens,
			OutputTokens:     outputTokens,
			TotalTokens:      inputTokens + outputTokens,
			TokPerRequest:    tokPerRequest,
			P95LatencyMs:     int(math.Round(m.P95LatencyMs)),
			TopModel:         topModel,
			CostOverTime:     costOverTime,
			RequestsOverTime: requestsOverTime,
			TokensOverTime:   tokensOverTime,
			UsersUsed:        usersUsed,
			UsersUsedDetails: usersUsedDetails,
			UndeployedAt:     md.undeployedAt,
			IsArchived:       isArchived,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CostUSD > entries[j].CostUSD
	})

	return entries
}

// discoverTombstoneIDs scans a Q_tags response for deployment tags that don't
// belong to any live deployment in the account — those IDs are soft-deleted
// deployments with spend in the window, which Insights renders as tombstoned
// rows. Live deployments are looked up by ID; the Q_tags row's `tags` field is
// either a single string or a JSON array (tagStrings normalises both shapes).
func discoverTombstoneIDs(tagsRows []map[string]any, live []*deploymentstore.Deployment) []string {
	liveSet := make(map[string]struct{}, len(live))
	for _, d := range live {
		liveSet[d.ID] = struct{}{}
	}
	seen := make(map[string]struct{})
	for _, row := range tagsRows {
		for _, tag := range tagStrings(row["tags"]) {
			if !strings.HasPrefix(tag, "deployment:") {
				continue
			}
			depID := strings.TrimPrefix(tag, "deployment:")
			if _, isLive := liveSet[depID]; isLive {
				continue
			}
			seen[depID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	// Sort for deterministic ordering — when the caller hits the
	// maxTombstones cap and truncates, the same subset is selected on
	// every request instead of map-iteration roulette.
	sort.Strings(ids)
	return ids
}

// zeroDeploymentEntries returns an empty deployments response when Langfuse is not configured.
func zeroDeploymentEntries(from, to string) AccountDeploymentsSummaryResponse {
	return AccountDeploymentsSummaryResponse{
		Deployments: []DeploymentSummaryEntry{},
		Period:      buildPeriod(from, to),
	}
}

// GetAccountDeploymentsSummary returns per-deployment cost, tokens, requests,
// and P95 latency by fanning out to Langfuse across all visible account
// deployments. One row per deployment — multi-region deployments of the same
// blueprint surface as separate entries.
// GET /api/v1/accounts/:account/observability/deployments-summary
func GetAccountDeploymentsSummary(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
	slackStore *slackidentity.Store,
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

		from := c.Query("from")
		to := c.Query("to")

		if (from == "") != (to == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from and to must both be provided or both omitted"})
			return
		}

		hasPeriod := from != "" && to != ""
		includeArchived := c.Query("include_archived") == "true"

		if hasPeriod {
			if _, err := time.Parse(time.RFC3339, from); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'from' timestamp: must be RFC3339"})
				return
			}
			if _, err := time.Parse(time.RFC3339, to); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'to' timestamp: must be RFC3339"})
				return
			}
		}

		// Cached payload is un-resolved; resolve identities at read time.
		// Falls through to the live compute path on any unmarshal error.
		if !hasPeriod {
			if bytes, ok := insightscache.Get(c.Request.Context(), cache, acct.ID, insightscache.EndpointDeploymentsSummary, insightscache.Params{
				IncludeArchived: includeArchived,
			}); ok {
				var cached AccountDeploymentsSummaryResponse
				if uerr := json.Unmarshal(bytes, &cached); uerr == nil {
					ResolveDeploymentsSummaryIdentities(log, slackStore, accountStore, &cached)
					c.JSON(http.StatusOK, cached)
					return
				} else {
					log.Warn("insights cache unmarshal failed; falling through to live compute",
						"account_id", acct.ID, "endpoint", "deployments-summary", "error", uerr)
				}
			}
		}

		resp, err := ComputeDeploymentsSummary(c.Request.Context(), log, cfg, langfuseStore, deploymentStore, slackStore, acct, from, to, includeArchived)
		if errors.Is(err, ErrAllLangfuseCallsFailed) {
			log.Warn("Langfuse deployments metrics unavailable; returning empty list", "error", err)
			degraded := zeroDeploymentEntries(from, to)
			degraded.MetricsUnavailable = true
			c.JSON(http.StatusOK, degraded)
			return
		}
		if err != nil {
			log.Error("Failed to compute deployments summary", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute deployments summary"})
			return
		}
		ResolveDeploymentsSummaryIdentities(log, slackStore, accountStore, &resp)
		c.JSON(http.StatusOK, resp)
	}
}

// ErrAllLangfuseCallsFailed is re-exported from insightscache so existing
// callers in this package (handlers + ComputeAccountSummary tests) keep
// working without an import flip. Canonical definition lives in
// insightscache so the periodic refresh worker can errors.Is on it without
// pulling the handlers package into riverqueue (which would close the
// handlers→riverqueue→handlers cycle).
var ErrAllLangfuseCallsFailed = insightscache.ErrAllLangfuseCallsFailed

// ComputeDeploymentsSummary runs the per-deployment Langfuse fan-out and
// assembles the deployments-summary response. Returns ErrAllLangfuseCallsFailed
// when every Langfuse sub-query failed (banner-worthy); partial failures
// continue to render with missing per-field data and nil error (existing
// fail-open behavior).
//
// Linked Slack user_ids are translated to their WorkOS id before per-
// deployment user roll-up, so users_used and users_used_details contain
// one entry per resolved identity rather than separate Slack/Astro rows.
// Profile/workspace stamping for the bare-Slack rows that survive
// translation happens in ResolveDeploymentsSummaryIdentities at read time.
func ComputeDeploymentsSummary(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	deploymentStore *deploymentstore.Store,
	slackStore *slackidentity.Store,
	acct *account.Account,
	from, to string,
	includeArchived bool,
) (AccountDeploymentsSummaryResponse, error) {
	creds, err := langfuseStore.Get(acct.ID)
	if err != nil || creds == nil {
		return zeroDeploymentEntries(from, to), nil
	}

	if deploymentStore == nil {
		return zeroDeploymentEntries(from, to), nil
	}

	deployments, err := deploymentStore.GetVisibleDeploymentsByAccount(acct.ID)
	if err != nil {
		return AccountDeploymentsSummaryResponse{}, fmt.Errorf("list deployments: %w", err)
	}

	// Fast-path: no live deployments AND caller didn't ask for tombstones
	// → there's nothing to surface. Skip the Langfuse fan-out entirely
	// rather than burning request budget assembling an empty response.
	// When includeArchived=true we still proceed because the Q_tags probe
	// is also our tombstone-discovery channel.
	if len(deployments) == 0 && !includeArchived {
		return zeroDeploymentEntries(from, to), nil
	}

	client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

	const maxDeployments = 100
	if len(deployments) > maxDeployments {
		log.Warn("Truncating deployments for deployments summary",
			"account", acct.Name, "total", len(deployments), "cap", maxDeployments)
		deployments = deployments[:maxDeployments]
	}

	tagValues := make([]string, len(deployments))
	for i, d := range deployments {
		tagValues[i] = "deployment:" + d.ID
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Tally Langfuse call outcomes across the live fan-out + the batched
	// queries. After g.Wait, if attempts > 0 && failures == attempts we
	// surface ErrAllLangfuseCallsFailed; partial failures keep rendering
	// with per-field zeros.
	var lfAttempts, lfFailures atomic.Int32

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	var p95ByDep map[string]float64
	g.Go(func() error {
		m, perr := batchedP95Latencies(gCtx, client, tagValues, from, to)
		lfAttempts.Add(1)
		if perr != nil {
			lfFailures.Add(1)
			log.Warn("Batched P95 query failed — per-blueprint latency will render as zero", "error", perr)
		}
		p95ByDep = m
		return nil
	})

	results := make([]deploymentMetrics, len(deployments))
	for i, dep := range deployments {
		g.Go(func() error {
			m, ferr := fetchDeploymentDaily(gCtx, client, dep, from, to)
			lfAttempts.Add(1)
			if ferr != nil {
				lfFailures.Add(1)
			}
			results[i] = m
			return nil
		})
	}

	var tagsRows []map[string]any
	g.Go(func() error {
		qFrom, qTo := metricsTimeRange(from, to)
		q := langfuse.MetricsQuery{
			View:    "traces",
			Metrics: []langfuse.MetricsQueryField{{Measure: "count", Aggregation: "count"}},
			Dimensions: []langfuse.MetricsDimension{
				{Field: "userId"},
				{Field: "tags"},
			},
			FromTimestamp: qFrom,
			ToTimestamp:   qTo,
		}
		if !includeArchived {
			q.Filters = []langfuse.MetricsFilter{
				{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: tagValues},
			}
		}
		resp, ferr := client.GetMetrics(gCtx, q)
		lfAttempts.Add(1)
		if ferr != nil {
			lfFailures.Add(1)
			log.Warn("Failed to fetch users-per-deployment for deployments summary", "error", ferr)
			return nil
		}
		tagsRows = resp.Data
		if includeArchived {
			log.Debug("Q_tags unfiltered response", "account", acct.Name, "rows", len(resp.Data))
		}
		return nil
	})
	_ = g.Wait()

	for i, dep := range deployments {
		results[i].P95LatencyMs = p95ByDep[dep.ID]
	}

	archivedIDs := make(map[string]struct{})
	var tombstoneIDs []string
	if includeArchived {
		tombstoneIDs = discoverTombstoneIDs(tagsRows, deployments)
		const maxTombstones = 50
		if len(tombstoneIDs) > maxTombstones {
			log.Warn("Truncating tombstoned deployments for deployments summary",
				"account", acct.Name, "total", len(tombstoneIDs), "cap", maxTombstones)
			tombstoneIDs = tombstoneIDs[:maxTombstones]
		}
		for _, id := range tombstoneIDs {
			archivedIDs[id] = struct{}{}
		}
	}
	if len(tombstoneIDs) > 0 {
		tombstones, terr := deploymentStore.GetDeploymentsByIDsForAccount(acct.ID, tombstoneIDs)
		if terr != nil {
			log.Warn("Failed to load tombstoned deployments for deployments summary", "error", terr)
		} else if len(tombstones) > 0 {
			tombstoneTags := make([]string, len(tombstones))
			for i, d := range tombstones {
				tombstoneTags[i] = "deployment:" + d.ID
			}
			tombstoneResults := make([]deploymentMetrics, len(tombstones))
			var tombstoneP95 map[string]float64
			g2, g2Ctx := errgroup.WithContext(ctx)
			g2.SetLimit(10)
			g2.Go(func() error {
				m, perr := batchedP95Latencies(g2Ctx, client, tombstoneTags, from, to)
				lfAttempts.Add(1)
				if perr != nil {
					lfFailures.Add(1)
					log.Warn("Tombstone P95 query failed", "error", perr)
				}
				tombstoneP95 = m
				return nil
			})
			for i, dep := range tombstones {
				g2.Go(func() error {
					m, ferr := fetchDeploymentDaily(g2Ctx, client, dep, from, to)
					lfAttempts.Add(1)
					if ferr != nil {
						lfFailures.Add(1)
					}
					tombstoneResults[i] = m
					return nil
				})
			}
			_ = g2.Wait()
			for i, dep := range tombstones {
				tombstoneResults[i].P95LatencyMs = tombstoneP95[dep.ID]
			}
			deployments = append(deployments, tombstones...)
			// makezero: deliberate. results was pre-sized for the live
			// fan-out (parallel index writes); we're concatenating the
			// tombstone fan-out's pre-sized slice after, not zero-padding.
			results = append(results, tombstoneResults...) //nolint:makezero
		}
	}

	attempts := lfAttempts.Load()
	if attempts > 0 && attempts == lfFailures.Load() {
		return AccountDeploymentsSummaryResponse{}, ErrAllLangfuseCallsFailed
	}

	translateLinkedSlackUserIDs(log, slackStore, "deployments-summary", tagsRows)
	usersByDep := buildDeploymentUserRows(tagsRows)

	return AccountDeploymentsSummaryResponse{
		Deployments: buildDeploymentSummaryWithUsers(results, usersByDep, deployments, archivedIDs),
		Period:      buildPeriod(from, to),
	}, nil
}

// ── users-summary ─────────────────────────────────────────────────────────────

// userAgg holds in-flight per-user state while we accumulate Q_main rows.
type userAgg struct {
	userID     string
	requests   int
	cost       float64
	tokens     int    // combined input + output — traces view only exposes the sum
	lastSeenTS string // RFC3339 — Langfuse's hour-bucket timestamp, kept as-is for the response
}

// maxAgentsPerUser caps the size of agents_used in each row of the response so
// the JSON stays small for high-fan-out users.
const maxAgentsPerUser = 10

// maxTagFilterValues caps the deployment-tag list passed to Langfuse's
// arrayOptions filter. Langfuse hasn't published an explicit limit but very
// large any-of lists slow query planning and risk URL/body bloat. Accounts
// with more deployments degrade to the top-N tag set (truncation is logged).
const maxTagFilterValues = 100

// maxUsersInResponse caps the per-user rows returned by users-summary so a
// single account with very high userId cardinality (e.g. public-facing agents
// with thousands of end users) can't return an unbounded response. Top spenders
// are kept; the rest is truncated with a warn log. Real pagination is tracked
// as a follow-up.
const maxUsersInResponse = 500

// GetAccountUsersSummary returns per-user aggregated cost / tokens / requests +
// last-seen and the set of agents touched.
// Two parallel Langfuse queries (see docs/01-spec/users-toggle-spec.md):
//   - Q_main: traces grouped by userId at hour granularity →
//     totals per user + last_seen (max non-zero hour-bucket).
//   - Q_tags: traces grouped by [userId, tags] → deployment-tag set per user,
//     mapped to agent_name via the deployments table.
//
// GET /api/v1/accounts/:account/observability/users-summary
func GetAccountUsersSummary(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
	slackStore *slackidentity.Store,
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

		from := c.Query("from")
		to := c.Query("to")
		hasPeriod := from != "" && to != ""

		if (from == "") != (to == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "from and to must both be provided or both omitted"})
			return
		}

		if from != "" {
			if _, err := time.Parse(time.RFC3339, from); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'from' timestamp: must be RFC3339"})
				return
			}
		}
		if to != "" {
			if _, err := time.Parse(time.RFC3339, to); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'to' timestamp: must be RFC3339"})
				return
			}
		}

		// Cached payload is un-resolved; resolve identities at read time.
		// Falls through to the live compute path on any unmarshal error.
		if !hasPeriod {
			if bytes, ok := insightscache.Get(c.Request.Context(), cache, acct.ID, insightscache.EndpointUsersSummary, insightscache.Params{}); ok {
				var cached AccountUsersSummaryResponse
				if uerr := json.Unmarshal(bytes, &cached); uerr == nil {
					ResolveUsersSummaryIdentities(log, slackStore, accountStore, &cached)
					c.JSON(http.StatusOK, cached)
					return
				} else {
					log.Warn("insights cache unmarshal failed; falling through to live compute",
						"account_id", acct.ID, "endpoint", "users-summary", "error", uerr)
				}
			}
		}

		resp, err := ComputeUsersSummary(c.Request.Context(), log, cfg, langfuseStore, deploymentStore, accountStore, slackStore, acct, from, to)
		if errors.Is(err, ErrAllLangfuseCallsFailed) {
			log.Warn("Langfuse users metrics unavailable; returning empty users list", "error", err)
			c.JSON(http.StatusOK, AccountUsersSummaryResponse{
				Users:              []UserSummaryEntry{},
				Period:             buildPeriod(from, to),
				MetricsUnavailable: true,
			})
			return
		}
		if err != nil {
			log.Error("Failed to compute users summary", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute users summary"})
			return
		}
		ResolveUsersSummaryIdentities(log, slackStore, accountStore, &resp)
		c.JSON(http.StatusOK, resp)
	}
}

// ComputeUsersSummary runs the per-user Langfuse fan-out and assembles the
// users-summary response. Returns ErrAllLangfuseCallsFailed when the
// primary metrics query (Q_main) fails — even if the attribution query
// (Q_tags) succeeds — so callers preserve the prior cache rather than
// caching zero-metric results. Q_tags failure alone is non-fatal.
//
// Linked Slack user_ids in the raw Langfuse rows are translated to their
// WorkOS id before aggregation; the cache therefore stores one row per
// resolved identity instead of separate Slack/Astro rows for a linked
// user. The remaining bare-Slack rows (unlinked / unknown) are stamped
// with profile + workspace metadata at response time by
// ResolveUsersSummaryIdentities — those are the dynamic bits.
func ComputeUsersSummary(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	deploymentStore *deploymentstore.Store,
	accountStore *account.AccountStore,
	slackStore *slackidentity.Store,
	acct *account.Account,
	from, to string,
) (AccountUsersSummaryResponse, error) {
	creds, err := langfuseStore.Get(acct.ID)
	if err != nil || creds == nil {
		return AccountUsersSummaryResponse{
			Users:  []UserSummaryEntry{},
			Period: buildPeriod(from, to),
		}, nil
	}

	depToAgent := make(map[string]UserAgentRef)
	if deploymentStore != nil {
		deployments, derr := deploymentStore.GetVisibleDeploymentsByAccount(acct.ID)
		if derr != nil {
			return AccountUsersSummaryResponse{}, fmt.Errorf("list deployments: %w", derr)
		}
		srcAccountIDs := make(map[string]struct{})
		for _, d := range deployments {
			if d.SourceAccountID != nil && *d.SourceAccountID != "" && *d.SourceAccountID != acct.ID {
				srcAccountIDs[*d.SourceAccountID] = struct{}{}
			}
		}
		srcAccountName := make(map[string]string, len(srcAccountIDs))
		var srcMu sync.Mutex
		lookupGroup, lookupCtx := errgroup.WithContext(ctx)
		lookupGroup.SetLimit(10)
		for id := range srcAccountIDs {
			lookupGroup.Go(func() error {
				if lookupCtx.Err() != nil {
					return nil
				}
				if a, lookupErr := accountStore.GetByID(id); lookupErr == nil && a != nil {
					srcMu.Lock()
					srcAccountName[id] = a.Name
					srcMu.Unlock()
				}
				return nil
			})
		}
		_ = lookupGroup.Wait()
		for _, d := range deployments {
			avatarAccount := acct.Name
			if d.SourceAccountID != nil && *d.SourceAccountID != "" && *d.SourceAccountID != acct.ID {
				if name, ok := srcAccountName[*d.SourceAccountID]; ok && name != "" {
					avatarAccount = name
				}
			}
			depToAgent[d.ID] = UserAgentRef{DeploymentID: d.ID, Name: d.AgentName, Account: avatarAccount}
		}
	}

	if len(depToAgent) == 0 {
		return AccountUsersSummaryResponse{
			Users:  []UserSummaryEntry{},
			Period: buildPeriod(from, to),
		}, nil
	}

	visibleTagValues := make([]string, 0, len(depToAgent))
	for id := range depToAgent {
		visibleTagValues = append(visibleTagValues, "deployment:"+id)
	}
	if len(visibleTagValues) > maxTagFilterValues {
		log.Warn("Truncating deployment-tag filter for users-summary",
			"total", len(visibleTagValues), "cap", maxTagFilterValues)
		sort.Strings(visibleTagValues)
		visibleTagValues = visibleTagValues[:maxTagFilterValues]
	}
	tagFilter := []langfuse.MetricsFilter{
		{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: visibleTagValues},
	}

	client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Q_main carries the actual metrics (cost/tokens/requests/last_seen);
	// Q_tags is attribution-only (which agents a user touched). If Q_main
	// fails — even with Q_tags succeeding — the response degrades to a list
	// of users with all-zero metrics, which is worse than no data: the
	// refresh worker would happily cache those zeros for the next 6h. So
	// treat any Q_main failure as ErrAllLangfuseCallsFailed and let the
	// caller preserve the prior cache entry. Q_tags failure alone keeps
	// real metrics and just leaves agents_used empty — acceptable to cache.
	var mainQueryFailed atomic.Bool

	g, gCtx := errgroup.WithContext(ctx)
	qFrom, qTo := metricsTimeRange(from, to)
	var mainRows, tagsRows []map[string]any

	g.Go(func() error {
		// Day granularity, not hour: the only thing we read out of the
		// per-bucket timestamp is `last_seen` (max non-zero bucket), and
		// day-resolution is plenty for that UX. Hourly was costing us
		// 24× the bucket count for zero product gain and is the single
		// biggest contributor to Q_main's ClickHouse cost over the 90d
		// lookback. With day granularity, Q_main shape is at most
		// ~90 buckets × N users — still tractable for the full-window
		// People table totals.
		q := langfuse.MetricsQuery{
			View: "traces",
			Metrics: []langfuse.MetricsQueryField{
				{Measure: "totalCost", Aggregation: "sum"},
				{Measure: "totalTokens", Aggregation: "sum"},
				{Measure: "count", Aggregation: "count"},
			},
			Dimensions:    []langfuse.MetricsDimension{{Field: "userId"}},
			TimeDimension: &langfuse.TimeDimension{Granularity: "day"},
			Filters:       tagFilter,
			FromTimestamp: qFrom,
			ToTimestamp:   qTo,
		}
		resp, ferr := client.GetMetrics(gCtx, q)
		if ferr != nil {
			mainQueryFailed.Store(true)
			log.Warn("Users Q_main query failed", "error", ferr)
			return nil
		}
		mainRows = resp.Data
		return nil
	})

	g.Go(func() error {
		q := langfuse.MetricsQuery{
			View: "traces",
			Metrics: []langfuse.MetricsQueryField{
				{Measure: "count", Aggregation: "count"},
			},
			Dimensions:    []langfuse.MetricsDimension{{Field: "userId"}, {Field: "tags"}},
			Filters:       tagFilter,
			FromTimestamp: qFrom,
			ToTimestamp:   qTo,
		}
		resp, ferr := client.GetMetrics(gCtx, q)
		if ferr != nil {
			log.Warn("Users Q_tags query failed; agents_used will be empty", "error", ferr)
			return nil
		}
		tagsRows = resp.Data
		return nil
	})
	_ = g.Wait()

	if mainQueryFailed.Load() {
		return AccountUsersSummaryResponse{}, ErrAllLangfuseCallsFailed
	}

	translateLinkedSlackUserIDs(log, slackStore, "users-summary", mainRows, tagsRows)
	users := buildUsersSummary(mainRows, tagsRows, depToAgent)

	// Sort by cost descending and cap at maxUsersInResponse. The
	// translation step above already rolled linked Slack spend into the
	// user's WorkOS row, so this cap is now on the resolved identity
	// set — the old "drop a bare-Slack row whose linked twin survives"
	// hazard is gone.
	sort.Slice(users, func(i, j int) bool {
		return users[i].CostUSD > users[j].CostUSD
	})
	if len(users) > maxUsersInResponse {
		log.Warn("Truncating users-summary response", "total", len(users), "cap", maxUsersInResponse)
		users = users[:maxUsersInResponse]
	}
	return AccountUsersSummaryResponse{
		Users:  users,
		Period: buildPeriod(from, to),
	}, nil
}

// buildUsersSummary aggregates the two Langfuse query responses into per-user
// summary rows sorted by cost descending. Rows are bucketed on raw user_id
// — translation upstream has already collapsed linked Slack ids onto the
// matching WorkOS id, so we never get two parallel rows for the same human.
func buildUsersSummary(mainRows, tagsRows []map[string]any, depToAgent map[string]UserAgentRef) []UserSummaryEntry {
	aggs := make(map[string]*userAgg)
	getOrCreate := func(userID string) *userAgg {
		a, ok := aggs[userID]
		if !ok {
			a = &userAgg{userID: userID}
			aggs[userID] = a
		}
		return a
	}

	// Q_main rollup: sum metrics across hour-buckets, track max-non-zero bucket
	// timestamp as last_seen.
	for _, row := range mainRows {
		userID, _ := row["userId"].(string)
		userID = normalizeUserID(userID)
		count := toInt(row["count_count"])
		cost := toFloat(row["sum_totalCost"])
		tokens := toInt(row["sum_totalTokens"])
		ts, _ := row[langfuseTimeDimensionKey].(string)

		a := getOrCreate(userID)
		a.requests += count
		a.cost += cost
		a.tokens += tokens
		if count > 0 && ts > a.lastSeenTS {
			// String compare on RFC3339 timestamps is lexicographically correct.
			a.lastSeenTS = ts
		}
	}

	// Q_tags rollup: extract deployment tags per user, map to one ref per
	// deployment. The Langfuse `tags` column is an array on the source
	// trace; when grouped, Langfuse may return the value either as a
	// single string (one row per tag) or as the full JSON array. Handle
	// both — earlier code assumed only string and silently dropped every
	// row in the array case, leaving agents_used empty.
	//
	// Dedupe key is depID so two deployments of the same blueprint each
	// surface as their own chip in the People-tab "Agents Used" column.
	// Mirrors the Agents-tab "one row per deployment" shape; the client
	// uses the per-deployment route segment for click-through and looks
	// up display_name/namespace via the deployments-summary response.
	agentsByUser := make(map[string]map[string]UserAgentRef)
	for _, row := range tagsRows {
		userID, _ := row["userId"].(string)
		userID = normalizeUserID(userID)
		for _, tag := range tagStrings(row["tags"]) {
			if !strings.HasPrefix(tag, "deployment:") {
				continue
			}
			depID := strings.TrimPrefix(tag, "deployment:")
			ref, ok := depToAgent[depID]
			if !ok || ref.Name == "" {
				continue
			}
			if _, exists := aggs[userID]; !exists {
				// Tag-only user (no cost in Q_main) shouldn't really happen —
				// if it does, surface them with zero metrics.
				getOrCreate(userID)
			}
			set := agentsByUser[userID]
			if set == nil {
				set = make(map[string]UserAgentRef)
				agentsByUser[userID] = set
			}
			set[depID] = ref
		}
	}

	out := make([]UserSummaryEntry, 0, len(aggs))
	for userID, a := range aggs {
		agents := make([]UserAgentRef, 0, len(agentsByUser[userID]))
		for _, ref := range agentsByUser[userID] {
			agents = append(agents, ref)
		}
		sort.Slice(agents, func(i, j int) bool {
			if agents[i].Name != agents[j].Name {
				return agents[i].Name < agents[j].Name
			}
			if agents[i].Account != agents[j].Account {
				return agents[i].Account < agents[j].Account
			}
			return agents[i].DeploymentID < agents[j].DeploymentID
		})
		if len(agents) > maxAgentsPerUser {
			agents = agents[:maxAgentsPerUser]
		}
		out = append(out, UserSummaryEntry{
			UserIdentity: UserIdentity{
				UserID:      a.userID,
				UserDetails: UserDetails{Kind: classifyUserID(a.userID)},
			},
			Requests:   a.requests,
			CostUSD:    math.Round(a.cost*10000) / 10000,
			Tokens:     a.tokens,
			LastSeen:   a.lastSeenTS,
			AgentsUsed: agents,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].CostUSD > out[j].CostUSD
	})

	return out
}

// granularityIntervalMinutes maps Langfuse granularity names to their bucket
// size in minutes so the client knows the width of each bar.
var granularityIntervalMinutes = map[string]int{
	"hour":  60,
	"day":   1440,
	"week":  10080,
	"month": 43200, // approximate
}

// langfuseTimeDimensionKey is the response field Langfuse uses for time buckets.
const langfuseTimeDimensionKey = "time_dimension"

// GetLangfuseMetrics returns token usage metrics for a deployment from Langfuse,
// bucketed at the requested granularity (hour, day, week, month).
// GET /api/v1/deployments/:id/observability/metrics
func GetLangfuseMetrics(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		granularity := c.DefaultQuery("granularity", "day")
		if _, ok := granularityIntervalMinutes[granularity]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid granularity; must be hour, day, week, or month"})
			return
		}

		filters := []langfuse.MetricsFilter{
			{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: []string{"deployment:" + lctx.DeploymentID}},
		}
		timeDim := &langfuse.TimeDimension{Granularity: granularity}
		fromTS := c.Query("start_time")
		toTS := c.Query("end_time")

		obsQ := langfuse.MetricsQuery{
			View: "observations",
			Metrics: []langfuse.MetricsQueryField{
				{Measure: "inputTokens", Aggregation: "sum"},
				{Measure: "outputTokens", Aggregation: "sum"},
				{Measure: "count", Aggregation: "count"},
			},
			TimeDimension: timeDim,
			Filters:       filters,
			FromTimestamp: fromTS,
			ToTimestamp:   toTS,
		}

		obsResp, err := lctx.Client.GetMetrics(c.Request.Context(), obsQ)
		if err != nil {
			log.Error("Failed to get Langfuse metrics", "error", err, "granularity", granularity)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse metrics"})
			return
		}

		// Latency comes from the traces view — observation latency would
		// include sub-spans, which understates request duration.
		traceQ := langfuse.MetricsQuery{
			View: "traces",
			Metrics: []langfuse.MetricsQueryField{
				{Measure: "latency", Aggregation: "avg"},
				{Measure: "latency", Aggregation: "p95"},
				{Measure: "latency", Aggregation: "min"},
				{Measure: "latency", Aggregation: "max"},
			},
			TimeDimension: timeDim,
			Filters:       filters,
			FromTimestamp: fromTS,
			ToTimestamp:   toTS,
		}

		type latencyAgg struct{ avg, p95, min, max float64 }
		traceResp, terr := lctx.Client.GetMetrics(c.Request.Context(), traceQ)
		latencyByTS := map[string]latencyAgg{}
		if terr != nil {
			log.Warn("Failed to get Langfuse trace latency metrics — bucket latency will be zero", "error", terr)
		} else {
			for _, row := range traceResp.Data {
				ts, _ := row[langfuseTimeDimensionKey].(string)
				if ts == "" {
					continue
				}
				// Langfuse's metrics API returns latency in milliseconds
				// (unlike the per-trace REST endpoint, which uses seconds).
				latencyByTS[ts] = latencyAgg{
					avg: toFloat(row["avg_latency"]),
					p95: toFloat(row["p95_latency"]),
					min: toFloat(row["min_latency"]),
					max: toFloat(row["max_latency"]),
				}
			}
		}

		buckets := make([]gin.H, 0, len(obsResp.Data))
		for _, row := range obsResp.Data {
			ts, _ := row[langfuseTimeDimensionKey].(string)
			if ts == "" {
				continue
			}
			lat := latencyByTS[ts]
			buckets = append(buckets, gin.H{
				"timestamp":      ts,
				"trace_count":    toInt(row["count_count"]),
				"avg_latency_ms": math.Round(lat.avg*100) / 100,
				"p95_latency_ms": math.Round(lat.p95*100) / 100,
				"min_latency_ms": math.Round(lat.min*100) / 100,
				"max_latency_ms": math.Round(lat.max*100) / 100,
				"input_tokens":   toInt(row["sum_inputTokens"]),
				"output_tokens":  toInt(row["sum_outputTokens"]),
				"error_count":    0,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"buckets":          buckets,
			"time_range":       gin.H{"start": c.Query("start_time"), "end": c.Query("end_time")},
			"interval_minutes": granularityIntervalMinutes[granularity],
		})
	}
}

// toInt converts a metrics response value (may be string or float64 from JSON) to int.
func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}

// toFloat converts a metrics response value (may be string, float64, or nil from JSON) to float64.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}

// GetLangfuseSummary returns summary statistics from Langfuse traces.
// GET /api/v1/deployments/:id/observability/summary
func GetLangfuseSummary(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		traces, err := lctx.Client.GetTraces(c.Request.Context(), lctx.DeploymentID, c.Query("start_time"), c.Query("end_time"), 0, 0)
		if err != nil {
			log.Error("Failed to get Langfuse traces for summary", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse traces"})
			return
		}

		// Token totals come from daily metrics, not the trace list (which has no
		// per-trace usage). A daily-metrics failure leaves tokens at 0 rather
		// than failing the whole summary.
		totalTokens := 0
		dailyMetrics, dmErr := lctx.Client.GetDailyMetrics(c.Request.Context(), lctx.DeploymentID, c.Query("start_time"), c.Query("end_time"))
		if dmErr != nil {
			log.Warn("Failed to get Langfuse daily metrics for summary", "error", dmErr)
		} else {
			for _, m := range dailyMetrics {
				totalTokens += m.InputTokens() + m.OutputTokens()
			}
		}

		c.JSON(http.StatusOK, computeLangfuseSummary(
			traces.Data, traces.Meta.TotalItems, totalTokens,
			c.Query("start_time"), c.Query("end_time"),
		))
	}
}

// GetLangfuseSummaries returns summary statistics for all active deployments
// in an account. Reads come from Redis only — the obs summary cache is
// written periodically by ObsSummaryRefreshWorker (see internal/riverqueue)
// so the agents page never waits on Langfuse during a request. Deployments
// without a cache entry yet (brand-new, or refreshed-failed) are silently
// omitted from the response; the frontend already handles missing summaries
// by hiding the sparkline.
//
// GET /api/v1/accounts/:account/observability/deployment-summaries
func GetLangfuseSummaries(
	log *logger.Logger,
	deploymentStore *deploymentstore.Store,
	cache k8scache.Cache,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		deployments, err := deploymentStore.GetActiveDeploymentsByAccount(acct.ID)
		if err != nil {
			log.Error("Failed to list deployments for bulk summary", "account_id", acct.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
			return
		}

		if len(deployments) == 0 || cache == nil {
			c.JSON(http.StatusOK, gin.H{"summaries": gin.H{}})
			return
		}

		// Parallel Redis reads. The cache wrapper itself is small (one GET
		// per call), so 10-way concurrency is plenty for typical accounts.
		type result struct {
			id    string
			entry *obssummary.Entry
		}
		results := make([]result, len(deployments))
		var g errgroup.Group
		g.SetLimit(10)
		for i, dep := range deployments {
			id := dep.ID
			g.Go(func() error {
				entry, _, err := obssummary.Get(c.Request.Context(), cache, id)
				if err != nil {
					log.Warn("Obs summary cache read", "deployment_id", id, "error", err)
					return nil
				}
				if entry != nil {
					results[i] = result{id: id, entry: entry}
				}
				return nil
			})
		}
		_ = g.Wait()

		summaries := make(map[string]DeploymentTraceSummary, len(deployments))
		for _, r := range results {
			if r.id == "" || r.entry == nil {
				continue
			}
			summaries[r.id] = DeploymentTraceSummary{
				TotalTraces:   r.entry.TotalTraces,
				LastTraceAt:   r.entry.LastTraceAt,
				CostUSD:       r.entry.CostUSD,
				RequestSeries: r.entry.RequestSeries,
				TokenSeries:   r.entry.TokenSeries,
				CostSeries:    r.entry.CostSeries,
			}
		}

		c.JSON(http.StatusOK, DeploymentSummariesResponse{Summaries: summaries})
	}
}

// maxTracesLimit is the largest page size the Langfuse traces API accepts; it
// rejects anything greater. Callers wanting a wider window paginate via offset.
const maxTracesLimit = 100

type traceListCriteria struct {
	search    string
	userID    string
	noUser    bool
	sortKey   string
	direction string
}

func parseTraceListCriteria(c *gin.Context) traceListCriteria {
	criteria := traceListCriteria{
		search:    strings.ToLower(strings.TrimSpace(c.Query("search"))),
		userID:    strings.TrimSpace(c.Query("user_id")),
		noUser:    c.Query("no_user") == "true",
		sortKey:   c.DefaultQuery("sort", "timestamp"),
		direction: c.DefaultQuery("direction", "desc"),
	}
	if criteria.noUser {
		criteria.userID = ""
	}
	if criteria.sortKey != "timestamp" && criteria.sortKey != "latency" && criteria.sortKey != "cost" {
		criteria.sortKey = "timestamp"
	}
	if criteria.direction != "asc" && criteria.direction != "desc" {
		criteria.direction = "desc"
	}
	return criteria
}

func (c traceListCriteria) upstreamOrderBy() (string, bool) {
	if c.search != "" || c.sortKey != "timestamp" {
		return "", false
	}
	return "timestamp." + c.direction, true
}

func (c traceListCriteria) upstreamFilters(
	deploymentID, startTime, endTime string,
) []langfuse.TraceFilter {
	filters := []langfuse.TraceFilter{{
		Type: "arrayOptions", Column: "tags", Operator: "all of",
		Value: []string{"deployment:" + deploymentID},
	}}
	if startTime != "" {
		filters = append(filters, langfuse.TraceFilter{
			Type: "datetime", Column: "timestamp", Operator: ">=", Value: startTime,
		})
	}
	if endTime != "" {
		filters = append(filters, langfuse.TraceFilter{
			Type: "datetime", Column: "timestamp", Operator: "<", Value: endTime,
		})
	}
	if c.noUser {
		filters = append(filters, langfuse.TraceFilter{
			Type: "null", Column: "userId", Operator: "is null",
		})
	} else if c.userID != "" {
		filters = append(filters, langfuse.TraceFilter{
			Type: "string", Column: "userId", Operator: "=", Value: c.userID,
		})
	}
	return filters
}

func appendTraceFilter(
	base []langfuse.TraceFilter,
	filter langfuse.TraceFilter,
) []langfuse.TraceFilter {
	result := make([]langfuse.TraceFilter, len(base), len(base)+1)
	copy(result, base)
	return append(result, filter)
}

func traceEntryFromLangfuse(t langfuse.Trace, details *UserDetails) TraceEntry {
	return TraceEntry{
		TraceID:     t.ID,
		Name:        t.Name,
		Status:      "ok",
		LatencyMs:   t.Latency * 1000,
		TotalCost:   t.TotalCost,
		Timestamp:   t.CreatedAt,
		UserID:      t.UserID,
		UserDetails: details,
	}
}

func traceEntryMatchesSearch(trace TraceEntry, query string) bool {
	if query == "" {
		return true
	}
	terms := []string{trace.Name, trace.TraceID, trace.UserID}
	if trace.UserDetails != nil {
		terms = append(terms, trace.UserDetails.DisplayName, trace.UserDetails.Username)
	}
	for _, term := range terms {
		if strings.Contains(strings.ToLower(strings.TrimSpace(term)), query) {
			return true
		}
	}
	return false
}

func compareTraceEntries(a, b TraceEntry, criteria traceListCriteria) int {
	var comparison int
	if criteria.sortKey == "timestamp" {
		comparison = strings.Compare(a.Timestamp, b.Timestamp)
	} else {
		aValue, bValue := a.LatencyMs, b.LatencyMs
		if criteria.sortKey == "cost" {
			aValue, bValue = a.TotalCost, b.TotalCost
		}
		comparison = cmp.Compare(aValue, bValue)
	}
	if criteria.direction == "desc" {
		comparison = -comparison
	}
	if comparison != 0 {
		return comparison
	}
	return strings.Compare(a.TraceID, b.TraceID)
}

func filterAndSortTraceEntries(traces []TraceEntry, criteria traceListCriteria) []TraceEntry {
	filtered := make([]TraceEntry, 0, len(traces))
	for _, trace := range traces {
		if criteria.noUser && trace.UserID != "" {
			continue
		}
		if criteria.userID != "" && trace.UserID != criteria.userID {
			continue
		}
		if !traceEntryMatchesSearch(trace, criteria.search) {
			continue
		}
		filtered = append(filtered, trace)
	}
	slices.SortFunc(filtered, func(a, b TraceEntry) int {
		return compareTraceEntries(a, b, criteria)
	})
	return filtered
}

func pageTraceEntries(traces []TraceEntry, offset, limit int) []TraceEntry {
	if offset >= len(traces) {
		return []TraceEntry{}
	}
	end := min(offset+limit, len(traces))
	return traces[offset:end]
}

type tracePageFetcher func(context.Context, int, int) (*langfuse.TracesResponse, error)

const (
	traceCriteriaCacheTTL         = 5 * time.Minute
	traceCriteriaCacheMaxEntries  = 16
	traceCriteriaLoadTimeout      = 2 * time.Minute
	maxTraceCriteriaItems         = 1000
	maxTraceSearchIdentitySources = 5
)

type traceCriteriaResult struct {
	traces       []TraceEntry
	truncated    bool
	scannedCount int
}

type traceCriteriaCacheEntry struct {
	result    traceCriteriaResult
	expiresAt time.Time
}

type traceCriteriaCache struct {
	mu      sync.Mutex
	entries map[string]traceCriteriaCacheEntry
	loads   singleflight.Group
}

func newTraceCriteriaCache() *traceCriteriaCache {
	return &traceCriteriaCache{entries: make(map[string]traceCriteriaCacheEntry)}
}

func (cache *traceCriteriaCache) getOrLoad(
	key string,
	load func() (traceCriteriaResult, error),
) (traceCriteriaResult, error) {
	if result, ok := cache.get(key); ok {
		return result, nil
	}
	value, err, _ := cache.loads.Do(key, func() (any, error) {
		if result, ok := cache.get(key); ok {
			return result, nil
		}
		result, err := load()
		if err != nil {
			return nil, err
		}
		cache.put(key, result)
		return result, nil
	})
	if err != nil {
		return traceCriteriaResult{}, err
	}
	return value.(traceCriteriaResult), nil
}

func (cache *traceCriteriaCache) get(key string) (traceCriteriaResult, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(cache.entries, key)
		return traceCriteriaResult{}, false
	}
	return entry.result, true
}

func (cache *traceCriteriaCache) put(key string, result traceCriteriaResult) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	now := time.Now()
	for entryKey, entry := range cache.entries {
		if now.After(entry.expiresAt) {
			delete(cache.entries, entryKey)
		}
	}
	if len(cache.entries) >= traceCriteriaCacheMaxEntries {
		var oldestKey string
		var oldestExpiry time.Time
		for entryKey, entry := range cache.entries {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = entryKey
				oldestExpiry = entry.expiresAt
			}
		}
		delete(cache.entries, oldestKey)
	}
	cache.entries[key] = traceCriteriaCacheEntry{
		result:    result,
		expiresAt: now.Add(traceCriteriaCacheTTL),
	}
}

func traceCriteriaKey(
	deploymentID, startTime, endTime string,
	criteria traceListCriteria,
) string {
	return strings.Join([]string{
		deploymentID,
		startTime,
		endTime,
		criteria.search,
		criteria.userID,
		strconv.FormatBool(criteria.noUser),
		criteria.sortKey,
		criteria.direction,
	}, "\x00")
}

func newTraceCriteriaLoadContext(requestCtx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(requestCtx), traceCriteriaLoadTimeout)
}

func loadAllTracePages(
	ctx context.Context,
	first *langfuse.TracesResponse,
	fetch tracePageFetcher,
) ([]langfuse.Trace, bool, error) {
	totalPageCount := (first.Meta.TotalItems + maxTracesLimit - 1) / maxTracesLimit
	maxPageCount := (maxTraceCriteriaItems + maxTracesLimit - 1) / maxTracesLimit
	pageCount := min(totalPageCount, maxPageCount)
	truncated := first.Meta.TotalItems > maxTraceCriteriaItems
	if pageCount <= 1 {
		return first.Data, truncated, nil
	}
	pages := make([][]langfuse.Trace, pageCount)
	pages[0] = first.Data
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for pageIndex := 1; pageIndex < pageCount; pageIndex++ {
		group.Go(func() error {
			page, err := fetch(groupCtx, maxTracesLimit, pageIndex*maxTracesLimit)
			if err != nil {
				return err
			}
			pages[pageIndex] = page.Data
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, false, err
	}
	all := make([]langfuse.Trace, 0, min(first.Meta.TotalItems, maxTraceCriteriaItems))
	for _, page := range pages {
		all = append(all, page...)
	}
	if len(all) > maxTraceCriteriaItems {
		all = all[:maxTraceCriteriaItems]
		truncated = true
	}
	return all, truncated, nil
}

func resolveTraceEntries(
	log *logger.Logger,
	accountStore *account.AccountStore,
	slackStore *slackidentity.Store,
	traces []langfuse.Trace,
) []TraceEntry {
	identityRows := traceIdentityLookupRows(traces)
	userIDs := make([]string, 0, len(identityRows))
	for _, row := range identityRows {
		userIDs = append(userIDs, row.UserID)
	}
	hydrator := newUserDetailsHydrator(log, slackStore, accountStore, userIDs, "deployment-traces")
	result := make([]TraceEntry, 0, len(traces))
	for _, trace := range traces {
		result = append(result, traceEntryFromLangfuse(
			trace,
			traceUserDetailsFromHydrator(trace.UserID, hydrator),
		))
	}
	return result
}

func buildTraceUserFacets(
	log *logger.Logger,
	accountStore *account.AccountStore,
	slackStore *slackidentity.Store,
	rows []map[string]any,
) []TraceUserFacet {
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		userID, _ := row["userId"].(string)
		counts[normalizeUserID(userID)] += toInt(row["count_count"])
	}

	userIDs := make([]string, 0, len(counts))
	for userID := range counts {
		if userID != "" {
			userIDs = append(userIDs, userID)
		}
	}
	hydrator := newUserDetailsHydrator(log, slackStore, accountStore, userIDs, "deployment-trace-users")

	result := make([]TraceUserFacet, 0, len(counts))
	for userID, count := range counts {
		if count <= 0 {
			continue
		}
		result = append(result, TraceUserFacet{
			UserID:      userID,
			UserDetails: traceUserDetailsFromHydrator(userID, hydrator),
			Count:       count,
		})
	}
	slices.SortFunc(result, func(a, b TraceUserFacet) int {
		return strings.Compare(a.UserID, b.UserID)
	})
	return result
}

func getTraceUserFacets(
	ctx context.Context,
	client *langfuse.Client,
	deploymentID, startTime, endTime string,
	log *logger.Logger,
	accountStore *account.AccountStore,
	slackStore *slackidentity.Store,
) ([]TraceUserFacet, error) {
	from, to := metricsTimeRange(startTime, endTime)
	response, err := client.GetMetrics(ctx, langfuse.MetricsQuery{
		View:       "traces",
		Metrics:    []langfuse.MetricsQueryField{{Measure: "count", Aggregation: "count"}},
		Dimensions: []langfuse.MetricsDimension{{Field: "userId"}},
		Filters: []langfuse.MetricsFilter{{
			Type: "arrayOptions", Column: "tags", Operator: "any of",
			Value: []string{"deployment:" + deploymentID},
		}},
		FromTimestamp: from,
		ToTimestamp:   to,
	})
	if err != nil {
		return nil, err
	}
	return buildTraceUserFacets(log, accountStore, slackStore, response.Data), nil
}

func matchingTraceIdentityUserIDs(
	facets []TraceUserFacet,
	criteria traceListCriteria,
) ([]string, bool) {
	if criteria.noUser || criteria.search == "" {
		return nil, false
	}
	result := make([]string, 0)
	for _, facet := range facets {
		if facet.UserID == "" || facet.UserDetails == nil {
			continue
		}
		if criteria.userID != "" && facet.UserID != criteria.userID {
			continue
		}
		terms := []string{facet.UserDetails.DisplayName, facet.UserDetails.Username}
		matches := false
		for _, term := range terms {
			if strings.Contains(strings.ToLower(strings.TrimSpace(term)), criteria.search) {
				matches = true
				break
			}
		}
		if !matches {
			continue
		}
		if len(result) == maxTraceSearchIdentitySources {
			return result, true
		}
		result = append(result, facet.UserID)
	}
	return result, false
}

func traceFilterSources(
	base []langfuse.TraceFilter,
	criteria traceListCriteria,
	identityUserIDs []string,
) [][]langfuse.TraceFilter {
	if criteria.search == "" {
		return [][]langfuse.TraceFilter{base}
	}

	columns := []string{"id", "name"}
	if !criteria.noUser {
		columns = append(columns, "userId")
	}
	// Langfuse's string "contains" predicate is case-sensitive while Astro's
	// search contract is not. Keep one bounded base stream as a compatibility
	// fallback, then union it with complete server-filtered streams for raw
	// fields and identity-specific streams for enriched names.
	sources := make([][]langfuse.TraceFilter, 0, 1+len(columns)+len(identityUserIDs))
	sources = append(sources, slices.Clone(base))
	for _, column := range columns {
		sources = append(sources, appendTraceFilter(base, langfuse.TraceFilter{
			Type: "string", Column: column, Operator: "contains", Value: criteria.search,
		}))
	}
	for _, userID := range identityUserIDs {
		if criteria.userID != "" {
			// The base already contains the selected user. Adding it without a
			// raw-field search includes every trace whose enriched identity matched.
			sources = append(sources, slices.Clone(base))
			continue
		}
		sources = append(sources, appendTraceFilter(base, langfuse.TraceFilter{
			Type: "string", Column: "userId", Operator: "=", Value: userID,
		}))
	}
	return sources
}

func loadFilteredTraceCriteria(
	ctx context.Context,
	client *langfuse.Client,
	deploymentID, startTime, endTime string,
	criteria traceListCriteria,
	sources [][]langfuse.TraceFilter,
	identitySourcesTruncated bool,
	log *logger.Logger,
	accountStore *account.AccountStore,
	slackStore *slackidentity.Store,
) (traceCriteriaResult, error) {
	orderBy := "timestamp.desc"
	if criteria.sortKey == "timestamp" {
		orderBy = "timestamp." + criteria.direction
	}

	pages := make([][]langfuse.Trace, len(sources))
	truncatedSources := make([]bool, len(sources))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for sourceIndex, filters := range sources {
		group.Go(func() error {
			fetch := func(fetchCtx context.Context, limit, offset int) (*langfuse.TracesResponse, error) {
				return client.GetTracesFilteredOrdered(
					fetchCtx,
					deploymentID, startTime, endTime,
					filters,
					limit, offset,
					orderBy,
				)
			}
			first, err := fetch(groupCtx, maxTracesLimit, 0)
			if err != nil {
				return err
			}
			pages[sourceIndex], truncatedSources[sourceIndex], err = loadAllTracePages(groupCtx, first, fetch)
			return err
		})
	}
	if err := group.Wait(); err != nil {
		return traceCriteriaResult{}, err
	}

	unique := make(map[string]langfuse.Trace)
	truncated := identitySourcesTruncated
	for sourceIndex, traces := range pages {
		truncated = truncated || truncatedSources[sourceIndex]
		for _, trace := range traces {
			unique[trace.ID] = trace
		}
	}
	candidates := make([]langfuse.Trace, 0, len(unique))
	for _, trace := range unique {
		candidates = append(candidates, trace)
	}
	scannedCount := len(candidates)
	resolved := filterAndSortTraceEntries(
		resolveTraceEntries(log, accountStore, slackStore, candidates),
		criteria,
	)
	if len(resolved) > maxTraceCriteriaItems {
		resolved = resolved[:maxTraceCriteriaItems]
		truncated = true
	}
	return traceCriteriaResult{
		traces: resolved, truncated: truncated, scannedCount: scannedCount,
	}, nil
}

// GetLangfuseTraceUsers returns complete user facets for the selected window.
// GET /api/v1/deployments/:id/observability/trace-users
func GetLangfuseTraceUsers(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
	slackStore *slackidentity.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}
		facets, err := getTraceUserFacets(
			c.Request.Context(), lctx.Client, lctx.DeploymentID,
			c.Query("start_time"), c.Query("end_time"),
			log, accountStore, slackStore,
		)
		if err != nil {
			log.Error("Failed to get Langfuse trace user facets", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse trace users"})
			return
		}
		c.JSON(http.StatusOK, TraceUserFacetsResponse{Users: facets})
	}
}

// GetLangfuseTraces returns a paginated list of traces from Langfuse.
// Filter by one identity with user_id, or by traces without an identity with
// no_user=true. If both are provided, no_user takes precedence.
// GET /api/v1/deployments/:id/observability/traces
func GetLangfuseTraces(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
	slackStore *slackidentity.Store,
) gin.HandlerFunc {
	criteriaCache := newTraceCriteriaCache()
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		limitStr := c.DefaultQuery("limit", "50")
		limit, _ := strconv.Atoi(limitStr)
		if limit <= 0 {
			limit = 50
		}
		if limit > maxTracesLimit {
			limit = maxTracesLimit
		}

		offsetStr := c.DefaultQuery("offset", "0")
		offset, _ := strconv.Atoi(offsetStr)
		if offset < 0 {
			offset = 0
		}
		criteria := parseTraceListCriteria(c)
		startTime, endTime := c.Query("start_time"), c.Query("end_time")
		fetchDeploymentPage := func(orderBy string) tracePageFetcher {
			return func(ctx context.Context, pageLimit, pageOffset int) (*langfuse.TracesResponse, error) {
				return lctx.Client.GetTracesOrdered(ctx, lctx.DeploymentID, startTime, endTime, pageLimit, pageOffset, orderBy)
			}
		}

		var result []TraceEntry
		var total int
		var truncated bool
		var scannedCount int
		if orderBy, supported := criteria.upstreamOrderBy(); supported {
			var traces *langfuse.TracesResponse
			var err error
			if criteria.noUser {
				traces, err = lctx.Client.GetTracesFilteredOrdered(
					c.Request.Context(),
					lctx.DeploymentID, startTime, endTime,
					criteria.upstreamFilters(lctx.DeploymentID, startTime, endTime),
					limit, offset, orderBy,
				)
			} else if criteria.userID != "" {
				traces, err = lctx.Client.GetUserTracesOrdered(
					c.Request.Context(), lctx.DeploymentID,
					criteria.userID,
					startTime, endTime, limit, offset, orderBy,
				)
			} else {
				traces, err = fetchDeploymentPage(orderBy)(c.Request.Context(), limit, offset)
			}
			if err != nil {
				log.Error("Failed to get ordered Langfuse traces", "error", err)
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse traces"})
				return
			}
			result = resolveTraceEntries(log, accountStore, slackStore, traces.Data)
			total = traces.Meta.TotalItems
		} else {
			cacheKey := traceCriteriaKey(lctx.DeploymentID, startTime, endTime, criteria)
			resolved, err := criteriaCache.getOrLoad(cacheKey, func() (traceCriteriaResult, error) {
				loadCtx, cancel := newTraceCriteriaLoadContext(c.Request.Context())
				defer cancel()

				var identityUserIDs []string
				var identitySourcesTruncated bool
				if criteria.search != "" && !criteria.noUser {
					facets, err := getTraceUserFacets(
						loadCtx, lctx.Client, lctx.DeploymentID,
						startTime, endTime,
						log, accountStore, slackStore,
					)
					if err != nil {
						return traceCriteriaResult{}, err
					}
					identityUserIDs, identitySourcesTruncated = matchingTraceIdentityUserIDs(facets, criteria)
				}

				baseFilters := criteria.upstreamFilters(lctx.DeploymentID, startTime, endTime)
				return loadFilteredTraceCriteria(
					loadCtx, lctx.Client,
					lctx.DeploymentID, startTime, endTime,
					criteria,
					traceFilterSources(baseFilters, criteria, identityUserIDs),
					identitySourcesTruncated,
					log, accountStore, slackStore,
				)
			})
			if err != nil {
				log.Error("Failed to resolve filtered Langfuse traces", "error", err)
				c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse traces"})
				return
			}
			total = len(resolved.traces)
			result = pageTraceEntries(resolved.traces, offset, limit)
			truncated = resolved.truncated
			scannedCount = resolved.scannedCount
		}

		c.JSON(http.StatusOK, gin.H{
			"traces":        result,
			"total":         total,
			"limit":         limit,
			"offset":        offset,
			"truncated":     truncated,
			"scanned_count": scannedCount,
		})
	}
}

// GetLangfuseTraceDetail returns a single trace with its observations and scores.
// GET /api/v1/deployments/:id/observability/traces/:traceId
func GetLangfuseTraceDetail(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
	slackStore *slackidentity.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		traceID := c.Param("traceId")
		if traceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing traceId"})
			return
		}

		detail, err := lctx.Client.GetTrace(c.Request.Context(), traceID)
		if err != nil {
			if errors.Is(err, langfuse.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
				return
			}
			log.Error("Failed to get Langfuse trace detail", "error", err, "trace_id", traceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse trace"})
			return
		}
		// Verify the trace belongs to this deployment (defense in depth: the URL
		// is account-scoped but a caller could pass a traceId from another project).
		if !traceHasDeploymentTag(detail.Tags, lctx.DeploymentID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
			return
		}

		trace := projectTrace(detail)
		if details := traceUserDetails(log, slackStore, detail.UserID, "trace-detail"); details != nil {
			trace["user_details"] = details
		}

		c.JSON(http.StatusOK, gin.H{
			"trace":        trace,
			"observations": projectObservations(detail.Observations),
			"scores":       projectScores(detail.Scores),
		})
	}
}

// traceHasDeploymentTag checks whether the trace was tagged with this deployment.
func traceHasDeploymentTag(tags []string, deploymentID string) bool {
	return slices.Contains(tags, "deployment:"+deploymentID)
}

// projectTrace flattens a Langfuse trace into our stable response shape.
func projectTrace(t *langfuse.TraceDetail) gin.H {
	return gin.H{
		"trace_id":    t.ID,
		"name":        t.Name,
		"timestamp":   t.CreatedAt,
		"latency_ms":  t.Latency * 1000, // langfuse trace latency is seconds
		"total_cost":  t.TotalCost,
		"input":       t.Input,
		"output":      t.Output,
		"session_id":  t.SessionID,
		"user_id":     t.UserID,
		"tags":        t.Tags,
		"metadata":    t.Metadata,
		"environment": t.Environment,
		"release":     t.Release,
		"version":     t.Version,
	}
}

// projectObservation normalizes a single observation to its client-facing shape.
// I/O fields (input, output, metadata, model_parameters) are intentionally absent
// from this base projection — Langfuse skips them at the ClickHouse level when the
// tree skeleton is fetched, and the frontend loads them on demand via the observation
// detail endpoint.
func projectObservation(o langfuse.Observation) gin.H {
	row := gin.H{
		"id":             o.ID,
		"parent_id":      o.ParentObservationID,
		"type":           strings.ToLower(o.Type),
		"name":           o.Name,
		"start_time":     o.StartTime,
		"end_time":       o.EndTime,
		"latency_ms":     o.Latency * 1000, // langfuse observation latency is seconds
		"level":          strings.ToLower(o.Level),
		"status_message": o.StatusMessage,
		"cost":           o.CalculatedTotalCost,
	}
	if o.Model != "" {
		row["model"] = o.Model
	}
	if o.Usage != nil {
		row["usage"] = gin.H{
			"input":  o.Usage.Input,
			"output": o.Usage.Output,
			"total":  o.Usage.Total,
			"unit":   o.Usage.Unit,
		}
	}
	return row
}

func projectObservations(obs []langfuse.Observation) []gin.H {
	out := make([]gin.H, 0, len(obs))
	for _, o := range obs {
		out = append(out, projectObservation(o))
	}
	return out
}

// GetLangfuseObservationDetail returns a single observation with full input/output/metadata.
// The tree endpoint omits I/O to keep the initial load fast; this endpoint is called
// on demand when a node is selected in the trace tree.
// GET /api/v1/deployments/:id/observability/observations/:observationId
func GetLangfuseObservationDetail(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		lctx, ok := resolveLangfuseContext(c, log, cfg, accountStore, deploymentStore, langfuseStore)
		if !ok {
			return
		}

		observationID := c.Param("observationId")
		if observationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing observationId"})
			return
		}

		obs, err := lctx.Client.GetObservation(c.Request.Context(), observationID)
		if err != nil {
			if errors.Is(err, langfuse.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "observation not found"})
				return
			}
			log.Error("Failed to get Langfuse observation detail", "error", err, "observation_id", observationID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse observation"})
			return
		}

		parent, err := lctx.Client.GetTraceCore(c.Request.Context(), obs.TraceID)
		if err != nil {
			if errors.Is(err, langfuse.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "observation not found"})
				return
			}
			log.Error("Failed to verify observation ownership", "error", err, "trace_id", obs.TraceID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse observation"})
			return
		}
		if !traceHasDeploymentTag(parent.Tags, lctx.DeploymentID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "observation not found"})
			return
		}

		row := projectObservation(*obs)
		if obs.Input != nil {
			row["input"] = obs.Input
		}
		if obs.Output != nil {
			row["output"] = obs.Output
		}
		if len(obs.Metadata) > 0 {
			row["metadata"] = obs.Metadata
		}
		if len(obs.ModelParameters) > 0 {
			row["model_parameters"] = obs.ModelParameters
		}
		c.JSON(http.StatusOK, row)
	}
}

// projectScores normalizes scores to a flat client-friendly shape.
func projectScores(scores []langfuse.Score) []gin.H {
	out := make([]gin.H, 0, len(scores))
	for _, s := range scores {
		out = append(out, gin.H{
			"id":             s.ID,
			"name":           s.Name,
			"value":          s.Value,
			"string_value":   s.StringValue,
			"data_type":      strings.ToLower(s.DataType),
			"comment":        s.Comment,
			"observation_id": s.ObservationID,
			"source":         s.Source,
			"created_at":     s.CreatedAt,
		})
	}
	return out
}

// computeLangfuseSummary aggregates Langfuse traces into summary statistics
// matching the standardized observability response contract. totalTokens comes
// from the daily-metrics endpoint since the trace list carries no per-trace
// usage.
func computeLangfuseSummary(traces []langfuse.Trace, totalItems, totalTokens int, startTime, endTime string) gin.H {
	if totalItems == 0 {
		return gin.H{
			"total_traces": 0,
			"time_range":   gin.H{"start": startTime, "end": endTime},
			"metrics": gin.H{
				"avg_latency_ms":  0,
				"p95_latency_ms":  0,
				"total_tokens":    0,
				"error_rate":      0,
				"traces_per_hour": 0,
			},
		}
	}

	var sumLatency float64
	latencies := make([]float64, 0, len(traces))
	for _, t := range traces {
		ms := t.Latency * 1000 // Langfuse returns seconds
		sumLatency += ms
		latencies = append(latencies, ms)
	}

	avgLatency := sumLatency / float64(len(traces))

	sort.Float64s(latencies)
	p95Idx := int(math.Ceil(0.95*float64(len(latencies)))) - 1
	p95Idx = max(p95Idx, 0)
	p95Latency := latencies[p95Idx]

	var tracesPerHour float64
	start, err1 := time.Parse(time.RFC3339, startTime)
	end, err2 := time.Parse(time.RFC3339, endTime)
	if err1 == nil && err2 == nil {
		hours := end.Sub(start).Hours()
		if hours > 0 {
			tracesPerHour = float64(totalItems) / hours
		}
	}

	return gin.H{
		"total_traces": totalItems,
		"time_range":   gin.H{"start": startTime, "end": endTime},
		"metrics": gin.H{
			"avg_latency_ms":  math.Round(avgLatency*100) / 100,
			"p95_latency_ms":  math.Round(p95Latency*100) / 100,
			"total_tokens":    totalTokens,
			"error_rate":      0, // Langfuse doesn't expose error status in trace list
			"traces_per_hour": math.Round(tracesPerHour*100) / 100,
		},
	}
}
