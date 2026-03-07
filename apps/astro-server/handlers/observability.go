package handlers

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/galileo"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// resolveObservabilityContext validates auth, resolves the account, and returns a
// Galileo client plus the log stream name for the requested agent/deployment.
func resolveObservabilityContext(
	c *gin.Context,
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
) (*galileo.Client, string, bool) {
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

	if cfg.Deployment.GalileoAPIKey == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "observability not configured"})
		return nil, "", false
	}

	client := galileo.NewClient(
		cfg.Deployment.GalileoAPIEndpoint,
		cfg.Deployment.GalileoAPIKey,
		cfg.Deployment.GalileoProjectID,
	)

	// Resolve log stream name: {agent}-{buildID}
	logStreamName := agentName
	dep, err := deploymentStore.GetActiveDeployment(acct.ID, agentName)
	if err == nil && dep != nil && dep.BuildID != "" {
		logStreamName = fmt.Sprintf("%s-%s", agentName, dep.BuildID)
	}

	log.Debug("Resolving observability context",
		"account", accountName, "agent", agentName,
		"log_stream", logStreamName, "user_id", user.ID,
	)

	return client, logStreamName, true
}

// GetObservabilityMetrics returns bucketed metrics for an agent's traces.
// GET /api/v1/agents/:account/:name/observability/metrics
func GetObservabilityMetrics(
	log *logger.Logger,
	cfg *config.Config,
	deploymentStore *deploymentstore.Store,
	accountStore *account.AccountStore,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		client, logStreamName, ok := resolveObservabilityContext(c, log, cfg, accountStore, deploymentStore)
		if !ok {
			return
		}

		// Resolve log stream ID
		streams, err := client.SearchLogStreams(cfg.Deployment.GalileoProjectID, logStreamName)
		if err != nil {
			log.Error("Failed to search log streams", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query observability backend"})
			return
		}
		if len(streams) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"buckets":          []any{},
				"time_range":       gin.H{"start": c.Query("start_time"), "end": c.Query("end_time")},
				"interval_minutes": 60,
			})
			return
		}

		intervalStr := c.DefaultQuery("interval", "60")
		interval, _ := strconv.Atoi(intervalStr)
		if interval <= 0 {
			interval = 60
		}

		metrics, err := client.SearchMetrics(
			cfg.Deployment.GalileoProjectID,
			streams[0].ID,
			c.Query("start_time"),
			c.Query("end_time"),
			interval,
		)
		if err != nil {
			log.Error("Failed to search metrics", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query metrics"})
			return
		}

		// Transform Galileo bucketed_metrics into frontend format
		buckets := transformMetricsBuckets(metrics)

		c.JSON(http.StatusOK, gin.H{
			"buckets":          buckets,
			"time_range":       gin.H{"start": c.Query("start_time"), "end": c.Query("end_time")},
			"interval_minutes": interval,
		})
	}
}

// GetObservabilitySummary returns summary statistics for an agent's traces.
// GET /api/v1/agents/:account/:name/observability/summary
func GetObservabilitySummary(
	log *logger.Logger,
	cfg *config.Config,
	deploymentStore *deploymentstore.Store,
	accountStore *account.AccountStore,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		client, logStreamName, ok := resolveObservabilityContext(c, log, cfg, accountStore, deploymentStore)
		if !ok {
			return
		}

		streams, err := client.SearchLogStreams(cfg.Deployment.GalileoProjectID, logStreamName)
		if err != nil {
			log.Error("Failed to search log streams", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query observability backend"})
			return
		}
		if len(streams) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"total_traces": 0,
				"time_range":   gin.H{"start": c.Query("start_time"), "end": c.Query("end_time")},
				"metrics": gin.H{
					"avg_latency_ms":  0,
					"p95_latency_ms":  0,
					"total_tokens":    0,
					"error_rate":      0,
					"traces_per_hour": 0,
				},
			})
			return
		}

		// Fetch all traces for aggregation
		traces, err := client.SearchTraces(
			cfg.Deployment.GalileoProjectID,
			streams[0].ID,
			c.Query("start_time"),
			c.Query("end_time"),
			0, 0, "",
		)
		if err != nil {
			log.Error("Failed to search traces for summary", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query traces"})
			return
		}

		summary := computeSummary(traces, c.Query("start_time"), c.Query("end_time"))
		c.JSON(http.StatusOK, summary)
	}
}

// GetObservabilityTraces returns a paginated list of traces for an agent.
// GET /api/v1/agents/:account/:name/observability/traces
func GetObservabilityTraces(
	log *logger.Logger,
	cfg *config.Config,
	deploymentStore *deploymentstore.Store,
	accountStore *account.AccountStore,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		client, logStreamName, ok := resolveObservabilityContext(c, log, cfg, accountStore, deploymentStore)
		if !ok {
			return
		}

		streams, err := client.SearchLogStreams(cfg.Deployment.GalileoProjectID, logStreamName)
		if err != nil {
			log.Error("Failed to search log streams", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query observability backend"})
			return
		}
		if len(streams) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"traces": []any{},
				"total":  0,
				"limit":  50,
				"offset": 0,
			})
			return
		}

		limitStr := c.DefaultQuery("limit", "50")
		limit, _ := strconv.Atoi(limitStr)
		if limit <= 0 {
			limit = 50
		}

		offsetStr := c.DefaultQuery("offset", "0")
		offset, _ := strconv.Atoi(offsetStr)

		status := c.Query("status")

		traces, err := client.SearchTraces(
			cfg.Deployment.GalileoProjectID,
			streams[0].ID,
			c.Query("start_time"),
			c.Query("end_time"),
			limit, offset, status,
		)
		if err != nil {
			log.Error("Failed to search traces", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to query traces"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"traces": transformTraces(traces.Records),
			"total":  traces.NumRecords,
			"limit":  traces.Limit,
			"offset": traces.StartingToken,
		})
	}
}

// transformMetricsBuckets converts Galileo bucketed metrics to the frontend format.
func transformMetricsBuckets(metrics *galileo.MetricsResponse) []gin.H {
	buckets := metrics.BucketedMetrics["all"]
	if len(buckets) == 0 {
		return []gin.H{}
	}

	// Filter out empty buckets and convert
	var result []gin.H
	for _, b := range buckets {
		if b.RequestsCount == 0 {
			continue
		}
		result = append(result, gin.H{
			"timestamp":      b.StartBucketTime,
			"trace_count":    b.RequestsCount,
			"avg_latency_ms": b.AvgDurationNs / 1e6,
			"input_tokens":   b.InputTokens,
			"output_tokens":  b.OutputTokens,
			"error_count":    b.FailuresCount,
		})
	}
	if result == nil {
		return []gin.H{}
	}
	return result
}

// transformTraces converts Galileo trace records to the frontend format.
func transformTraces(records []galileo.TraceEntry) []gin.H {
	result := make([]gin.H, 0, len(records))
	for _, r := range records {
		status := "ok"
		if r.StatusCode != 0 {
			status = "error"
		}
		result = append(result, gin.H{
			"trace_id":   r.TraceID,
			"name":       r.Name,
			"status":     status,
			"latency_ms": r.Metrics.DurationNs / 1e6,
			"input":      r.Input,
			"output":     r.Output,
			"timestamp":  r.CreatedAt,
		})
	}
	return result
}

// computeSummary aggregates trace records into summary statistics.
func computeSummary(traces *galileo.TracesResponse, startTime, endTime string) gin.H {
	total := traces.NumRecords
	records := traces.Records

	if total == 0 {
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
	var errorCount int
	latencies := make([]float64, 0, len(records))
	for _, r := range records {
		ms := r.Metrics.DurationNs / 1e6
		sumLatency += ms
		latencies = append(latencies, ms)
		if r.StatusCode != 0 {
			errorCount++
		}
	}

	avgLatency := sumLatency / float64(len(records))

	// P95
	sort.Float64s(latencies)
	p95Idx := int(math.Ceil(0.95*float64(len(latencies)))) - 1
	if p95Idx < 0 {
		p95Idx = 0
	}
	p95Latency := latencies[p95Idx]

	// Traces per hour
	var tracesPerHour float64
	start, err1 := time.Parse(time.RFC3339, startTime)
	end, err2 := time.Parse(time.RFC3339, endTime)
	if err1 == nil && err2 == nil {
		hours := end.Sub(start).Hours()
		if hours > 0 {
			tracesPerHour = float64(total) / hours
		}
	}

	errorRate := float64(errorCount) / float64(len(records))

	return gin.H{
		"total_traces": total,
		"time_range":   gin.H{"start": startTime, "end": endTime},
		"metrics": gin.H{
			"avg_latency_ms":  math.Round(avgLatency*100) / 100,
			"p95_latency_ms":  math.Round(p95Latency*100) / 100,
			"total_tokens":    0, // Galileo doesn't provide per-trace token counts in trace records
			"error_rate":      math.Round(errorRate*10000) / 10000,
			"traces_per_hour": math.Round(tracesPerHour*100) / 100,
		},
	}
}
