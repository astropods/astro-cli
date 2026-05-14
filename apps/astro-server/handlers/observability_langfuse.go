package handlers

import (
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
		g, _ := errgroup.WithContext(c.Request.Context()) //nolint:errcheck // ctx unused until doGet accepts context

		g.Go(func() error {
			m, ferr := client.GetDailyMetrics("", from, to)
			currentMetrics = m
			return ferr
		})

		if hasPeriod {
			priorFrom, priorTo := shiftPrior(from, to)
			g.Go(func() error {
				m, ferr := client.GetDailyMetrics("", priorFrom, priorTo)
				priorMetrics = m
				return ferr
			})
		}

		if err := g.Wait(); err != nil {
			log.Error("Failed to get Langfuse account metrics", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse metrics"})
			return
		}

		// Active-agent count: time-bounded or snapshot (now/now for "All").
		activeAgents := 0
		if deploymentStore != nil {
			fromT, toT := periodTimes(from, to)
			var agentErr error
			activeAgents, agentErr = deploymentStore.CountActiveAgentsDuringPeriod(acct.ID, fromT, toT)
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

	// cost_over_time: sorted by date ascending.
	dates := make([]string, 0, len(costByDay))
	for d := range costByDay {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	costOverTime := make([]AccountCostOverTimeEntry, 0, len(dates))
	for _, d := range dates {
		costOverTime = append(costOverTime, AccountCostOverTimeEntry{Date: d, Models: costByDay[d]})
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

// periodTimes parses from/to into time.Time values. For "All" mode (empty strings)
// both are set to time.Now() so CountActiveAgentsDuringPeriod returns currently-active agents.
func periodTimes(from, to string) (time.Time, time.Time) {
	now := time.Now().UTC()
	fromT, toT := now, now
	if from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			fromT = t
		}
	}
	if to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			toT = t
		}
	}
	return fromT, toT
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

		obsResp, err := lctx.Client.GetMetrics(obsQ)
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
		traceResp, terr := lctx.Client.GetMetrics(traceQ)
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

		traces, err := lctx.Client.GetTraces(lctx.DeploymentID, c.Query("start_time"), c.Query("end_time"), 0, 0)
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

		traces, err := lctx.Client.GetTraces(lctx.DeploymentID, c.Query("start_time"), c.Query("end_time"), limit, offset)
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

		detail, err := lctx.Client.GetTrace(traceID)
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
