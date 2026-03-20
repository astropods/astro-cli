package handlers

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// resolveObservabilityLangfuseContext validates auth and returns a Langfuse client
// using per-account credentials from the database.
func resolveObservabilityLangfuseContext(
	c *gin.Context,
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	langfuseStore *langfuse.Store,
) (*langfuse.Client, string, bool) {
	user, exists := middleware.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return nil, "", false
	}

	accountName := c.Param("account")
	agentName := c.Param("name")

	acct, err := accountStore.GetByName(accountName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
		return nil, "", false
	}

	isMember, err := accountStore.IsMember(acct.ID, user.ID)
	if err != nil || !isMember {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return nil, "", false
	}

	creds, err := langfuseStore.Get(acct.ID)
	if err != nil || creds == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "langfuse not configured for this account"})
		return nil, "", false
	}

	client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

	log.Debug("Resolving Langfuse observability context",
		"account", accountName, "agent", agentName, "user_id", user.ID,
	)

	return client, agentName, true
}

// GetLangfuseMetrics returns daily metrics for an agent from Langfuse.
// GET /api/v1/agents/:account/:name/observability/langfuse/metrics
func GetLangfuseMetrics(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		client, _, ok := resolveObservabilityLangfuseContext(c, log, cfg, accountStore, langfuseStore)
		if !ok {
			return
		}

		metrics, err := client.GetDailyMetrics(c.Query("start_time"), c.Query("end_time"))
		if err != nil {
			log.Error("Failed to get Langfuse metrics", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse metrics"})
			return
		}

		buckets := make([]gin.H, 0, len(metrics.Data))
		for _, m := range metrics.Data {
			if m.CountTraces == 0 {
				continue
			}
			buckets = append(buckets, gin.H{
				"timestamp":   m.Date,
				"trace_count": m.CountTraces,
				"total_cost":  m.TotalCost,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"buckets":    buckets,
			"time_range": gin.H{"start": c.Query("start_time"), "end": c.Query("end_time")},
		})
	}
}

// GetLangfuseSummary returns summary statistics from Langfuse traces.
// GET /api/v1/agents/:account/:name/observability/langfuse/summary
func GetLangfuseSummary(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		client, agentName, ok := resolveObservabilityLangfuseContext(c, log, cfg, accountStore, langfuseStore)
		if !ok {
			return
		}

		traces, err := client.GetTraces(agentName, c.Query("start_time"), c.Query("end_time"), 0, 0)
		if err != nil {
			log.Error("Failed to get Langfuse traces for summary", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query langfuse traces"})
			return
		}

		startTime := c.Query("start_time")
		endTime := c.Query("end_time")

		total := traces.Meta.TotalItems
		if total == 0 {
			c.JSON(http.StatusOK, gin.H{
				"total_traces": 0,
				"time_range":   gin.H{"start": startTime, "end": endTime},
				"metrics": gin.H{
					"avg_latency_ms":  0,
					"p95_latency_ms":  0,
					"total_cost":      0,
					"error_rate":      0,
					"traces_per_hour": 0,
				},
			})
			return
		}

		var sumLatency float64
		var totalCost float64
		latencies := make([]float64, 0, len(traces.Data))
		for _, t := range traces.Data {
			ms := t.Latency * 1000 // Langfuse returns seconds
			sumLatency += ms
			latencies = append(latencies, ms)
			totalCost += t.TotalCost
		}

		avgLatency := sumLatency / float64(len(traces.Data))

		sort.Float64s(latencies)
		p95Idx := int(math.Ceil(0.95*float64(len(latencies)))) - 1
		if p95Idx < 0 {
			p95Idx = 0
		}
		p95Latency := latencies[p95Idx]

		var tracesPerHour float64
		start, err1 := time.Parse(time.RFC3339, startTime)
		end, err2 := time.Parse(time.RFC3339, endTime)
		if err1 == nil && err2 == nil {
			hours := end.Sub(start).Hours()
			if hours > 0 {
				tracesPerHour = float64(total) / hours
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"total_traces": total,
			"time_range":   gin.H{"start": startTime, "end": endTime},
			"metrics": gin.H{
				"avg_latency_ms":  math.Round(avgLatency*100) / 100,
				"p95_latency_ms":  math.Round(p95Latency*100) / 100,
				"total_cost":      math.Round(totalCost*10000) / 10000,
				"error_rate":      0, // Langfuse doesn't expose error status in trace list
				"traces_per_hour": math.Round(tracesPerHour*100) / 100,
			},
		})
	}
}

// GetLangfuseTraces returns a paginated list of traces from Langfuse.
// GET /api/v1/agents/:account/:name/observability/langfuse/traces
func GetLangfuseTraces(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		client, agentName, ok := resolveObservabilityLangfuseContext(c, log, cfg, accountStore, langfuseStore)
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

		traces, err := client.GetTraces(agentName, c.Query("start_time"), c.Query("end_time"), limit, offset)
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
				"input":      t.Input,
				"output":     t.Output,
				"timestamp":  t.CreatedAt,
				"total_cost": t.TotalCost,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"traces": result,
			"total":  traces.Meta.TotalItems,
			"limit":  traces.Meta.Limit,
			"offset": offset,
		})
	}
}
