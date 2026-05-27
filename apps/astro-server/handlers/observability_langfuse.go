package handlers

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

// langfuseContext holds the resolved Langfuse client and deployment metadata.
type langfuseContext struct {
	Client       *langfuse.Client
	DeploymentID string
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

	return &langfuseContext{Client: client, DeploymentID: dep.ID}, true
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

		creds, err := langfuseStore.Get(acct.ID)
		if err != nil || creds == nil {
			c.JSON(http.StatusOK, zeroAccountSummary(from, to, hasPeriod))
			return
		}

		// Scope all Langfuse queries to currently-live deployments. Deleted
		// (undeployed) deployments' historical traces are NOT surfaced — same
		// contract as the deployment-detail page.
		var deps []*deploymentstore.Deployment
		if deploymentStore != nil {
			deps, err = deploymentStore.GetVisibleDeploymentsByAccount(acct.ID)
			if err != nil {
				log.Error("Failed to list visible deployments for account summary", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
				return
			}
		}
		if len(deps) == 0 {
			c.JSON(http.StatusOK, zeroAccountSummary(from, to, hasPeriod))
			return
		}

		// Cap fan-out to bound worst-case Langfuse load — same threshold the
		// blueprints-summary handler enforces.
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

		// Bound the per-request Langfuse work so a slow upstream can't pin a
		// gin worker indefinitely — matches the blueprints-summary handler.
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		// Parallel fetch: current period + prior period (only when bounded) + optional user-grouped breakdown.
		var currentMetrics, priorMetrics []langfuse.DailyMetric
		var activeDepIDs map[string]bool
		var userCostRows []map[string]any
		g, gCtx := errgroup.WithContext(ctx)

		g.Go(func() error {
			var err error
			currentMetrics, activeDepIDs, err = accountDailyMetrics(gCtx, client, visibleTagValues, from, to)
			return err
		})

		if hasPeriod {
			priorFrom, priorTo := shiftPrior(from, to)
			g.Go(func() error {
				// Prior-period failures degrade the % change tile to "—" but
				// shouldn't fail the whole response — fail-open.
				priorMetrics, _, _ = accountDailyMetrics(gCtx, client, visibleTagValues, priorFrom, priorTo)
				return nil
			})
		}

		if groupBy == "user" {
			g.Go(func() error {
				// View: "traces" mirrors the users-summary Q_main query so the
				// chart's per-user cost matches the table's per-user cost. The
				// observations view double-counts spans within a trace and
				// produced a chart that didn't reconcile with the row totals.
				//
				// We pull cost + count + totalTokens so the client can slice
				// the per-(day, user) data into a range and recompute per-user
				// totals without an extra round-trip on every range toggle.
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
					Filters: []langfuse.MetricsFilter{
						{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: visibleTagValues},
					},
					FromTimestamp: qFrom,
					ToTimestamp:   qTo,
				}
				resp, ferr := client.GetMetrics(gCtx, q)
				if ferr != nil {
					return ferr
				}
				userCostRows = resp.Data
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			log.Error("Failed to get Langfuse account metrics", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse metrics"})
			return
		}

		// Active agents = currently-live agents that drove ≥1 trace in the
		// period. When no window is bounded, falls back to live-agent count.
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

		resp := buildAccountSummary(currentMetrics, priorMetrics, hasPeriod, from, to, activeAgents)
		if groupBy == "user" {
			resp.CostOverTimeByUser = buildCostOverTimeByUser(userCostRows)
			// Model-mode chart data isn't shown in user view; keep cost_over_time
			// populated for sparklines/totals consumers but blank cost_by_model
			// since the donut isn't rendered.
			resp.CostByModel = []AccountCostByModelEntry{}
		}
		c.JSON(http.StatusOK, resp)
	}
}

// buildCostOverTimeByUser groups the per-(user, day) Langfuse rows into per-day
// entries with the user breakdown nested inside. Sorted by date ascending.
// Each entry carries cost + requests + tokens so the client can slice the
// per-(day, user) data into any range window without an extra round-trip.
func buildCostOverTimeByUser(rows []map[string]any) []AccountCostOverTimeByUserEntry {
	type userBucket struct {
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
			bucket = &userBucket{}
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
		for uid, b := range byUser {
			users = append(users, AccountUserCost{
				UserID:   uid,
				CostUSD:  math.Round(b.cost*10000) / 10000,
				Requests: b.requests,
				Tokens:   b.tokens,
			})
		}
		out = append(out, AccountCostOverTimeByUserEntry{Date: d, Users: users})
	}
	return out
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

// buildAccountSummary aggregates DailyMetric slices into the full response shape.
func buildAccountSummary(
	current, prior []langfuse.DailyMetric,
	hasPeriod bool,
	from, to string,
	activeAgents int,
) AccountObservabilitySummaryResponse {
	// Aggregate current period.
	var totalCost float64
	var totalRequests, totalInput, totalOutput int
	costByDay := make(map[string][]AccountModelCost)
	costByModel := make(map[string]float64)

	for _, m := range current {
		totalRequests += m.CountTraces
		totalCost += m.TotalCost
		totalInput += m.InputTokens()
		totalOutput += m.OutputTokens()

		if len(m.Usage) > 0 {
			models := make([]AccountModelCost, 0, len(m.Usage))
			for _, u := range m.Usage {
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
// "all-time" for any account in practice and avoids divergent semantics between
// the legacy /metrics/daily endpoint (which accepts empty timestamps natively)
// and /metrics (which doesn't).
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
func accountDailyMetrics(
	ctx context.Context,
	client *langfuse.Client,
	tagValues []string,
	from, to string,
) ([]langfuse.DailyMetric, map[string]bool, error) {
	if len(tagValues) == 0 {
		return nil, map[string]bool{}, nil
	}

	qFrom, qTo := metricsTimeRange(from, to)
	tagFilter := []langfuse.MetricsFilter{
		{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: tagValues},
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

// ── blueprints-summary ────────────────────────────────────────────────────────

// deploymentMetrics holds the raw Langfuse data fetched for one deployment.
type deploymentMetrics struct {
	AgentName    string
	DailyMetrics []langfuse.DailyMetric
	P95LatencyMs float64
}

// fetchDeploymentDaily fetches the per-deployment daily metrics (cost / tokens
// / request count + per-model usage). Stays per-deployment because the legacy
// /metrics/daily endpoint is the only path to the per-(deployment, model)
// breakdown that powers TopModel — Langfuse's /metrics endpoint can't group
// observations by tags, so it can't produce per-(deployment, model) rows.
// Errors are swallowed — missing data is treated as zero so the blueprint row
// still appears in the aggregated response.
func fetchDeploymentDaily(ctx context.Context, client *langfuse.Client, dep *deploymentstore.Deployment, from, to string) deploymentMetrics {
	result := deploymentMetrics{AgentName: dep.AgentName}
	daily, err := client.GetDailyMetrics(ctx, dep.ID, from, to)
	if err == nil {
		result.DailyMetrics = daily
	}
	return result
}

// batchedP95Latencies fetches per-deployment P95 latency in a single batched
// /metrics call (traces view grouped by tags) instead of N separate ones.
// Returns map[deploymentID]p95Ms. Failures fail-open: returns empty map; the
// per-blueprint P95 column then renders as "—" but the rest of the row is
// untouched.
func batchedP95Latencies(
	ctx context.Context,
	client *langfuse.Client,
	log *logger.Logger,
	tagValues []string,
	from, to string,
) map[string]float64 {
	out := make(map[string]float64)
	if len(tagValues) == 0 {
		return out
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
		log.Warn("Batched P95 query failed — per-blueprint latency will render as zero", "error", err)
		return out
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
	return out
}

// buildBlueprintsSummary aggregates per-deployment metrics into per-agent-name
// blueprint entries, sorted by cost descending.
func buildBlueprintsSummary(metrics []deploymentMetrics) []BlueprintSummaryEntry {
	type group struct {
		requests     int
		costUSD      float64
		inputTokens  int
		outputTokens int
		p95LatencyMs float64
		modelCosts   map[string]float64
		dayCosts     map[string]float64 // date → total cost for this agent
		dayRequests  map[string]int     // date → trace count for this agent
		dayTokens    map[string][2]int  // date → [inputTokens, outputTokens]
	}

	groups := make(map[string]*group)
	// order preserves agent_name insertion order so sort is the only reordering.
	order := make([]string, 0, len(metrics))

	for _, m := range metrics {
		g, exists := groups[m.AgentName]
		if !exists {
			g = &group{
				modelCosts:  make(map[string]float64),
				dayCosts:    make(map[string]float64),
				dayRequests: make(map[string]int),
				dayTokens:   make(map[string][2]int),
			}
			groups[m.AgentName] = g
			order = append(order, m.AgentName)
		}
		for _, d := range m.DailyMetrics {
			g.requests += d.CountTraces
			g.costUSD += d.TotalCost
			g.inputTokens += d.InputTokens()
			g.outputTokens += d.OutputTokens()
			g.dayCosts[d.Date] += d.TotalCost
			g.dayRequests[d.Date] += d.CountTraces
			prev := g.dayTokens[d.Date]
			g.dayTokens[d.Date] = [2]int{prev[0] + d.InputTokens(), prev[1] + d.OutputTokens()}
			for _, u := range d.Usage {
				g.modelCosts[u.Model] += u.TotalCost
			}
		}
		if m.P95LatencyMs > g.p95LatencyMs {
			g.p95LatencyMs = m.P95LatencyMs
		}
	}

	entries := make([]BlueprintSummaryEntry, 0, len(groups))
	for _, name := range order {
		g := groups[name]

		topModel := ""
		var maxModelCost float64
		for model, cost := range g.modelCosts {
			if cost > maxModelCost {
				maxModelCost = cost
				topModel = model
			}
		}

		var costPerRequest, tokPerRequest float64
		if g.requests > 0 {
			costPerRequest = math.Round(g.costUSD/float64(g.requests)*10000) / 10000
			tokPerRequest = math.Round(float64(g.inputTokens+g.outputTokens)/float64(g.requests)*10) / 10
		}

		// Build *_over_time slices sorted by date ascending.
		dates := make([]string, 0, len(g.dayCosts))
		for d := range g.dayCosts {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		costOverTime := make([]BlueprintDailyCost, 0, len(dates))
		requestsOverTime := make([]BlueprintDailyRequests, 0, len(dates))
		tokensOverTime := make([]BlueprintDailyTokens, 0, len(dates))
		for _, d := range dates {
			costOverTime = append(costOverTime, BlueprintDailyCost{
				Date:    d,
				CostUSD: math.Round(g.dayCosts[d]*10000) / 10000,
			})
			requestsOverTime = append(requestsOverTime, BlueprintDailyRequests{
				Date:     d,
				Requests: g.dayRequests[d],
			})
			tok := g.dayTokens[d]
			tokensOverTime = append(tokensOverTime, BlueprintDailyTokens{
				Date:         d,
				InputTokens:  tok[0],
				OutputTokens: tok[1],
				TotalTokens:  tok[0] + tok[1],
			})
		}

		entries = append(entries, BlueprintSummaryEntry{
			AgentName:        name,
			Requests:         g.requests,
			CostUSD:          math.Round(g.costUSD*10000) / 10000,
			CostPerRequest:   costPerRequest,
			InputTokens:      g.inputTokens,
			OutputTokens:     g.outputTokens,
			TotalTokens:      g.inputTokens + g.outputTokens,
			TokPerRequest:    tokPerRequest,
			P95LatencyMs:     int(math.Round(g.p95LatencyMs)),
			TopModel:         topModel,
			CostOverTime:     costOverTime,
			RequestsOverTime: requestsOverTime,
			TokensOverTime:   tokensOverTime,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CostUSD > entries[j].CostUSD
	})

	return entries
}

// zeroBlueprintEntries returns an empty blueprints response when Langfuse is not configured.
func zeroBlueprintEntries(from, to string) AccountBlueprintsSummaryResponse {
	return AccountBlueprintsSummaryResponse{
		Blueprints: []BlueprintSummaryEntry{},
		Period:     buildPeriod(from, to),
	}
}

// GetAccountBlueprintsSummary returns per-blueprint aggregated cost, tokens, requests,
// and P95 latency by fanning out to Langfuse across all account deployments.
// GET /api/v1/accounts/:account/observability/blueprints-summary
func GetAccountBlueprintsSummary(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
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

		creds, err := langfuseStore.Get(acct.ID)
		if err != nil || creds == nil {
			c.JSON(http.StatusOK, zeroBlueprintEntries(from, to))
			return
		}

		if deploymentStore == nil {
			c.JSON(http.StatusOK, zeroBlueprintEntries(from, to))
			return
		}

		deployments, err := deploymentStore.GetVisibleDeploymentsByAccount(acct.ID)
		if err != nil {
			log.Error("Failed to list deployments for blueprints summary", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
			return
		}

		client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

		const maxDeployments = 100
		if len(deployments) > maxDeployments {
			log.Warn("Truncating deployments for blueprints summary",
				"account", acct.Name, "total", len(deployments), "cap", maxDeployments)
			deployments = deployments[:maxDeployments]
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		tagValues := make([]string, len(deployments))
		for i, d := range deployments {
			tagValues[i] = "deployment:" + d.ID
		}

		// Per-deployment daily metrics fan-out + ONE batched P95 query run in
		// parallel. The daily fan-out is unavoidable today (per-deployment
		// per-model breakdown isn't expressible in a batched /metrics query
		// because Langfuse's observations view can't group by trace tags);
		// the P95 batch saves N calls.
		g, gCtx := errgroup.WithContext(ctx)
		g.SetLimit(10)

		var p95ByDep map[string]float64
		g.Go(func() error {
			p95ByDep = batchedP95Latencies(gCtx, client, log, tagValues, from, to)
			return nil
		})

		results := make([]deploymentMetrics, len(deployments))
		for i, dep := range deployments {
			g.Go(func() error {
				results[i] = fetchDeploymentDaily(gCtx, client, dep, from, to)
				return nil
			})
		}
		_ = g.Wait()

		// Stitch the batched P95 into per-deployment results.
		for i, dep := range deployments {
			results[i].P95LatencyMs = p95ByDep[dep.ID]
		}

		c.JSON(http.StatusOK, AccountBlueprintsSummaryResponse{
			Blueprints: buildBlueprintsSummary(results),
			Period:     buildPeriod(from, to),
		})
	}
}

// ── users-summary ─────────────────────────────────────────────────────────────

// userAgg holds in-flight per-user state while we accumulate Q_main rows.
type userAgg struct {
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

		creds, err := langfuseStore.Get(acct.ID)
		if err != nil || creds == nil {
			c.JSON(http.StatusOK, AccountUsersSummaryResponse{
				Users:  []UserSummaryEntry{},
				Period: buildPeriod(from, to),
			})
			return
		}

		// Deployment ID → agent ref (name + publishing account). Only currently-live
		// (non-undeployed) deployments are included — deleted deployments' traces
		// are excluded from totals via the tag filter below (deployment-detail-page
		// contract). The publishing account is the SourceAccountID when set (cross-
		// account / public-blueprint deploys), otherwise the deploying account —
		// the client needs this to construct avatar URLs that actually resolve.
		depToAgent := make(map[string]UserAgentRef)
		if deploymentStore != nil {
			deployments, derr := deploymentStore.GetVisibleDeploymentsByAccount(acct.ID)
			if derr != nil {
				// Failing this silently would surface as an empty users view —
				// indistinguishable from "no deployments" to the user. Mirror
				// the summary endpoint and 500 so the failure is visible.
				log.Error("Failed to list deployments for users summary", "error", derr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
				return
			}
			srcAccountIDs := make(map[string]struct{})
			for _, d := range deployments {
				if d.SourceAccountID != nil && *d.SourceAccountID != "" && *d.SourceAccountID != acct.ID {
					srcAccountIDs[*d.SourceAccountID] = struct{}{}
				}
			}
			// Lookup failures fall back to the deploying account name below —
			// the avatar will 404 to the placeholder rather than break the page.
			srcAccountName := make(map[string]string, len(srcAccountIDs))
			var srcMu sync.Mutex
			lookupGroup, lookupCtx := errgroup.WithContext(c.Request.Context())
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
				depToAgent[d.ID] = UserAgentRef{Name: d.AgentName, Account: avatarAccount}
			}
		}

		if len(depToAgent) == 0 {
			c.JSON(http.StatusOK, AccountUsersSummaryResponse{
				Users:  []UserSummaryEntry{},
				Period: buildPeriod(from, to),
			})
			return
		}

		visibleTagValues := make([]string, 0, len(depToAgent))
		for id := range depToAgent {
			visibleTagValues = append(visibleTagValues, "deployment:"+id)
		}
		if len(visibleTagValues) > maxTagFilterValues {
			log.Warn("Truncating deployment-tag filter for users-summary",
				"total", len(visibleTagValues), "cap", maxTagFilterValues)
			// Stable order so the same accounts get truncated reproducibly across
			// requests, making partial-data symptoms easier to diagnose.
			sort.Strings(visibleTagValues)
			visibleTagValues = visibleTagValues[:maxTagFilterValues]
		}
		tagFilter := []langfuse.MetricsFilter{
			{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: visibleTagValues},
		}

		client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		g, gCtx := errgroup.WithContext(ctx)

		// Backfill empty all-time timestamps so /api/public/metrics doesn't 400.
		qFrom, qTo := metricsTimeRange(from, to)

		var mainRows, tagsRows []map[string]any

		// Q_main: per-(user, hour) trace count / total cost / total tokens.
		// Uses the *traces* view so "requests" counts user-facing requests
		// (1 trace = 1 request), matching the agent-view denominator.
		// totalTokens is combined; traces view does not expose input/output split.
		g.Go(func() error {
			q := langfuse.MetricsQuery{
				View: "traces",
				Metrics: []langfuse.MetricsQueryField{
					{Measure: "totalCost", Aggregation: "sum"},
					{Measure: "totalTokens", Aggregation: "sum"},
					{Measure: "count", Aggregation: "count"},
				},
				Dimensions:    []langfuse.MetricsDimension{{Field: "userId"}},
				TimeDimension: &langfuse.TimeDimension{Granularity: "hour"},
				Filters:       tagFilter,
				FromTimestamp: qFrom,
				ToTimestamp:   qTo,
			}
			resp, ferr := client.GetMetrics(gCtx, q)
			if ferr != nil {
				return ferr
			}
			mainRows = resp.Data
			return nil
		})

		// Q_tags: per-(user, tag) — value ignored, only the tag dim matters.
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
				return ferr
			}
			tagsRows = resp.Data
			return nil
		})

		if err := g.Wait(); err != nil {
			log.Error("Failed to get Langfuse users metrics", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse metrics"})
			return
		}

		users := buildUsersSummary(mainRows, tagsRows, depToAgent)
		if len(users) > maxUsersInResponse {
			log.Warn("Truncating users-summary response", "total", len(users), "cap", maxUsersInResponse)
			users = users[:maxUsersInResponse]
		}
		c.JSON(http.StatusOK, AccountUsersSummaryResponse{
			Users:  users,
			Period: buildPeriod(from, to),
		})
	}
}

// buildUsersSummary aggregates the two Langfuse query responses into per-user
// summary rows sorted by cost descending.
func buildUsersSummary(mainRows, tagsRows []map[string]any, depToAgent map[string]UserAgentRef) []UserSummaryEntry {
	aggs := make(map[string]*userAgg)
	getOrCreate := func(userID string) *userAgg {
		a, ok := aggs[userID]
		if !ok {
			a = &userAgg{}
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

	// Q_tags rollup: extract deployment tags per user, map to (agent_name, account).
	// The Langfuse `tags` column is an array on the source trace. When grouped,
	// Langfuse may return the value either as a single string (one row per tag)
	// or as the full JSON array. Handle both shapes — earlier code assumed
	// only string and silently dropped every row in the array case, leaving
	// agents_used empty.
	// Dedupe key is "account/name" so two different accounts publishing the
	// same agent name don't collapse to one entry.
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
			set[ref.Account+"/"+ref.Name] = ref
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
			return agents[i].Account < agents[j].Account
		})
		if len(agents) > maxAgentsPerUser {
			agents = agents[:maxAgentsPerUser]
		}
		out = append(out, UserSummaryEntry{
			UserID:     userID,
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

		c.JSON(http.StatusOK, computeLangfuseSummary(
			traces.Data, traces.Meta.TotalItems,
			c.Query("start_time"), c.Query("end_time"),
		))
	}
}

// GetLangfuseSummaries returns summary statistics for all active deployments in an account.
// GET /api/v1/accounts/:account/observability/deployment-summaries
func GetLangfuseSummaries(
	log *logger.Logger,
	cfg *config.Config,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		creds, err := langfuseStore.Get(acct.ID)
		if err != nil || creds == nil {
			c.JSON(http.StatusOK, gin.H{"summaries": gin.H{}})
			return
		}

		deployments, err := deploymentStore.GetActiveDeploymentsByAccount(acct.ID)
		if err != nil {
			log.Error("Failed to list deployments for bulk summary", "account_id", acct.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list deployments"})
			return
		}

		if len(deployments) == 0 {
			c.JSON(http.StatusOK, gin.H{"summaries": gin.H{}})
			return
		}

		client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		type entry struct {
			id      string
			summary gin.H
		}
		results := make([]entry, len(deployments))

		var g errgroup.Group
		g.SetLimit(10)
		for i, dep := range deployments {
			id := dep.ID
			g.Go(func() error {
				traces, err := client.GetTraces(ctx, id, "", "", 1, 0)
				if err != nil {
					log.Warn("Failed to get Langfuse traces for deployment summary", "deployment_id", id, "error", err)
					return nil
				}
				var lastTraceAt string
				if len(traces.Data) > 0 {
					lastTraceAt = traces.Data[0].CreatedAt
				}
				results[i] = entry{id: id, summary: gin.H{
					"total_traces":  traces.Meta.TotalItems,
					"last_trace_at": lastTraceAt,
				}}
				return nil
			})
		}
		_ = g.Wait()

		if ctx.Err() != nil {
			return
		}

		summaries := make(gin.H, len(deployments))
		for _, r := range results {
			if r.id != "" {
				summaries[r.id] = r.summary
			}
		}

		c.JSON(http.StatusOK, gin.H{"summaries": summaries})
	}
}

// GetLangfuseTraces returns a paginated list of traces from Langfuse.
// GET /api/v1/deployments/:id/observability/traces
func GetLangfuseTraces(
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

		limitStr := c.DefaultQuery("limit", "50")
		limit, _ := strconv.Atoi(limitStr)
		if limit <= 0 {
			limit = 50
		}

		offsetStr := c.DefaultQuery("offset", "0")
		offset, _ := strconv.Atoi(offsetStr)

		traces, err := lctx.Client.GetTraces(c.Request.Context(), lctx.DeploymentID, c.Query("start_time"), c.Query("end_time"), limit, offset)
		if err != nil {
			log.Error("Failed to get Langfuse traces", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse traces"})
			return
		}

		result := make([]gin.H, 0, len(traces.Data))
		for _, t := range traces.Data {
			result = append(result, gin.H{
				"trace_id":   t.ID,
				"name":       t.Name,
				"status":     "ok",
				"latency_ms": t.Latency * 1000,
				"total_cost": t.TotalCost,
				"input":      t.Input,
				"output":     t.Output,
				"timestamp":  t.CreatedAt,
				"user_id":    t.UserID,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"traces": result,
			"total":  traces.Meta.TotalItems,
			"limit":  traces.Meta.Limit,
			"offset": (traces.Meta.Page - 1) * traces.Meta.Limit,
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

		// Verify the trace actually belongs to this deployment via tag (defense
		// in depth: the URL is account-scoped through resolveLangfuseContext,
		// but a malicious caller could pass a traceId from another project).
		if !traceHasDeploymentTag(detail.Tags, lctx.DeploymentID) {
			c.JSON(http.StatusNotFound, gin.H{"error": "trace not found"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"trace":        projectTrace(detail),
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

// projectObservations normalizes observations to a flat client-friendly shape.
func projectObservations(obs []langfuse.Observation) []gin.H {
	out := make([]gin.H, 0, len(obs))
	for _, o := range obs {
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
			"input":          o.Input,
			"output":         o.Output,
			"metadata":       o.Metadata,
			"cost":           o.CalculatedTotalCost,
		}
		if o.Model != "" {
			row["model"] = o.Model
		}
		if len(o.ModelParameters) > 0 {
			row["model_parameters"] = o.ModelParameters
		}
		if o.Usage != nil {
			row["usage"] = gin.H{
				"input":  o.Usage.Input,
				"output": o.Usage.Output,
				"total":  o.Usage.Total,
				"unit":   o.Usage.Unit,
			}
		}
		out = append(out, row)
	}
	return out
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
// matching the standardized observability response contract.
func computeLangfuseSummary(traces []langfuse.Trace, totalItems int, startTime, endTime string) gin.H {
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
			"total_tokens":    0, // Langfuse doesn't provide per-trace token counts
			"error_rate":      0, // Langfuse doesn't expose error status in trace list
			"traces_per_hour": math.Round(tracesPerHour*100) / 100,
		},
	}
}
