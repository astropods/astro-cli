package handlers

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
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

// GetAccountLangfuseSummary returns the total trace count for all deployments
// in an account over a given time window. Uses the account's Langfuse project
// credentials without a deployment tag filter, so it aggregates across all deployments.
// GET /api/v1/accounts/:account/observability/summary
func GetAccountLangfuseSummary(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
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

		creds, err := langfuseStore.Get(acct.ID)
		if err != nil || creds == nil {
			c.JSON(http.StatusOK, gin.H{
				"total_traces":  0,
				"input_tokens":  0,
				"output_tokens": 0,
				"time_range":    gin.H{"start": c.Query("start_time"), "end": c.Query("end_time")},
			})
			return
		}

		client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

		// Empty deploymentID = no tag filter; queries all traces in the account's Langfuse project.
		metrics, err := client.GetDailyMetrics("", c.Query("start_time"), c.Query("end_time"))
		if err != nil {
			log.Error("Failed to get Langfuse account metrics", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse metrics"})
			return
		}

		var totalTraces, inputTokens, outputTokens int
		for _, m := range metrics.Data {
			totalTraces += m.CountTraces
			inputTokens += m.InputTokens()
			outputTokens += m.OutputTokens()
		}

		c.JSON(http.StatusOK, gin.H{
			"total_traces":  totalTraces,
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
			"time_range":    gin.H{"start": c.Query("start_time"), "end": c.Query("end_time")},
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

		q := langfuse.MetricsQuery{
			View: "observations",
			Metrics: []langfuse.MetricsQueryField{
				{Measure: "inputTokens", Aggregation: "sum"},
				{Measure: "outputTokens", Aggregation: "sum"},
				{Measure: "count", Aggregation: "count"},
			},
			TimeDimension: &langfuse.TimeDimension{Granularity: granularity},
			Filters: []langfuse.MetricsFilter{
				{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: []string{"deployment:" + lctx.DeploymentID}},
			},
			FromTimestamp: c.Query("start_time"),
			ToTimestamp:   c.Query("end_time"),
		}

		resp, err := lctx.Client.GetMetrics(q)
		if err != nil {
			log.Error("Failed to get Langfuse metrics", "error", err, "granularity", granularity)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse metrics"})
			return
		}

		buckets := make([]gin.H, 0, len(resp.Data))
		for _, row := range resp.Data {
			ts, _ := row[langfuseTimeDimensionKey].(string)
			if ts == "" {
				continue
			}
			buckets = append(buckets, gin.H{
				"timestamp":      ts,
				"trace_count":    toInt(row["count_count"]),
				"avg_latency_ms": 0,
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
				"trace_id":    t.ID,
				"name":        t.Name,
				"status":      "ok",
				"latency_ms":  t.Latency * 1000,
				"total_cost":  t.TotalCost,
				"input":       t.Input,
				"output":      t.Output,
				"timestamp":   t.CreatedAt,
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
