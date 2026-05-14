package handlers

import (
	"context"
	"errors"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
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
			c.JSON(http.StatusOK, zeroAccountSummary(from, to, hasPeriod))
			return
		}

		client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

		// Parallel fetch: current period + prior period (only when bounded).
		var currentMetrics, priorMetrics []langfuse.DailyMetric
		g, gCtx := errgroup.WithContext(c.Request.Context())

		g.Go(func() error {
			m, ferr := client.GetDailyMetrics(gCtx, "", from, to)
			currentMetrics = m
			return ferr
		})

		if hasPeriod {
			priorFrom, priorTo := shiftPrior(from, to)
			g.Go(func() error {
				m, ferr := client.GetDailyMetrics(gCtx, "", priorFrom, priorTo)
				priorMetrics = m
				return ferr
			})
		}

		if err := g.Wait(); err != nil {
			log.Error("Failed to get Langfuse account metrics", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse metrics"})
			return
		}

		// Active-agent count: always a snapshot of currently-deployed agents,
		// independent of the selected time window.
		activeAgents := 0
		if deploymentStore != nil {
			now := time.Now()
			var agentErr error
			activeAgents, agentErr = deploymentStore.CountActiveAgentsDuringPeriod(acct.ID, now, now)
			if agentErr != nil {
				log.Error("Failed to count active agents", "error", agentErr)
			}
		}

		resp := buildAccountSummary(currentMetrics, priorMetrics, hasPeriod, from, to, activeAgents)
		c.JSON(http.StatusOK, resp)
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

// ── blueprints-summary ────────────────────────────────────────────────────────

// deploymentMetrics holds the raw Langfuse data fetched for one deployment.
type deploymentMetrics struct {
	AgentName    string
	DailyMetrics []langfuse.DailyMetric
	P95LatencyMs float64
}

// fetchDeploymentMetrics fetches cost/token/request metrics and P95 latency for one
// deployment. Errors are swallowed — missing data is treated as zero so the
// blueprint row still appears in the aggregated response.
func fetchDeploymentMetrics(ctx context.Context, client *langfuse.Client, dep *deploymentstore.Deployment, from, to string) deploymentMetrics {
	result := deploymentMetrics{AgentName: dep.AgentName}

	daily, err := client.GetDailyMetrics(ctx, dep.ID, from, to)
	if err == nil {
		result.DailyMetrics = daily
	}

	traceQ := langfuse.MetricsQuery{
		View:    "traces",
		Metrics: []langfuse.MetricsQueryField{{Measure: "latency", Aggregation: "p95"}},
		Filters: []langfuse.MetricsFilter{
			{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: []string{"deployment:" + dep.ID}},
		},
		FromTimestamp: from,
		ToTimestamp:   to,
	}
	traceResp, err := client.GetMetrics(ctx, traceQ)
	if err == nil && len(traceResp.Data) > 0 {
		result.P95LatencyMs = toFloat(traceResp.Data[0]["p95_latency"])
	}

	return result
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
			})
		}

		entries = append(entries, BlueprintSummaryEntry{
			AgentName:        name,
			Requests:         g.requests,
			CostUSD:          math.Round(g.costUSD*10000) / 10000,
			CostPerRequest:   costPerRequest,
			InputTokens:      g.inputTokens,
			OutputTokens:     g.outputTokens,
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
			deployments = deployments[:maxDeployments]
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()

		g, gCtx := errgroup.WithContext(ctx)
		g.SetLimit(10)

		results := make([]deploymentMetrics, len(deployments))
		for i, dep := range deployments {
			g.Go(func() error {
				results[i] = fetchDeploymentMetrics(gCtx, client, dep, from, to)
				return nil // errors swallowed inside fetchDeploymentMetrics
			})
		}
		_ = g.Wait()

		c.JSON(http.StatusOK, AccountBlueprintsSummaryResponse{
			Blueprints: buildBlueprintsSummary(results),
			Period:     buildPeriod(from, to),
		})
	}
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
