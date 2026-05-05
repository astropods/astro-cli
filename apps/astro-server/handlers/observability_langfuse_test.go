package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

var accountCols = []string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links"}

func TestGetAccountLangfuseSummary_NotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	accountDB, accountMock, _ := sqlmock.New()
	langfuseDB, langfuseMock, _ := sqlmock.New()

	accountStore := account.NewAccountStore(accountDB)
	langfuseStore := langfuse.NewStore(langfuseDB)
	log := logger.New("error", "json")
	cfg := &config.Config{}

	now := time.Now()
	accountMock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(accountCols).
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil))
	accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	langfuseMock.ExpectQuery("SELECT .+ FROM account_langfuse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "langfuse_project_id", "langfuse_public_key", "langfuse_secret_key", "encrypted_data_key", "nonce", "created_at"}))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-1"})
		c.Next()
	})
	router.GET("/api/v1/accounts/:account/observability/summary",
		GetAccountLangfuseSummary(log, cfg, accountStore, langfuseStore))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/myorg/observability/summary?start_time=2026-04-01T00:00:00Z&end_time=2026-04-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["total_traces"] != float64(0) {
		t.Errorf("total_traces = %v, want 0", resp["total_traces"])
	}
}

func TestComputeLangfuseSummary_Empty(t *testing.T) {
	result := computeLangfuseSummary(nil, 0, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

	b, _ := json.Marshal(result)
	var out ObservabilitySummaryResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TotalTraces != 0 {
		t.Errorf("total_traces = %d, want 0", out.TotalTraces)
	}
	if out.Metrics.AvgLatencyMs != 0 || out.Metrics.P95LatencyMs != 0 || out.Metrics.TracesPerHour != 0 {
		t.Errorf("expected all zero metrics, got %+v", out.Metrics)
	}
}

func TestComputeLangfuseSummary_SingleTrace(t *testing.T) {
	traces := []langfuse.Trace{
		{Latency: 0.250}, // 250ms
	}
	result := computeLangfuseSummary(traces, 1, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

	b, _ := json.Marshal(result)
	var out ObservabilitySummaryResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.TotalTraces != 1 {
		t.Errorf("total_traces = %d, want 1", out.TotalTraces)
	}
	if out.Metrics.AvgLatencyMs != 250 {
		t.Errorf("avg_latency_ms = %v, want 250", out.Metrics.AvgLatencyMs)
	}
	// Single trace: p95 = the only value
	if out.Metrics.P95LatencyMs != 250 {
		t.Errorf("p95_latency_ms = %v, want 250", out.Metrics.P95LatencyMs)
	}
}

func TestComputeLangfuseSummary_MultipleTraces(t *testing.T) {
	// 20 traces with latencies 0.1s, 0.2s, ..., 2.0s
	traces := make([]langfuse.Trace, 20)
	for i := range traces {
		traces[i].Latency = float64(i+1) * 0.1
	}
	result := computeLangfuseSummary(traces, 20, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

	b, _ := json.Marshal(result)
	var out ObservabilitySummaryResponse
	json.Unmarshal(b, &out)

	// avg = (100+200+...+2000)/20 = 21000/20 = 1050ms
	if out.Metrics.AvgLatencyMs != 1050 {
		t.Errorf("avg_latency_ms = %v, want 1050", out.Metrics.AvgLatencyMs)
	}

	// p95 index = ceil(0.95*20) - 1 = 19 - 1 = 18 → latencies[18] = 1900ms
	if out.Metrics.P95LatencyMs != 1900 {
		t.Errorf("p95_latency_ms = %v, want 1900", out.Metrics.P95LatencyMs)
	}
}

func TestComputeLangfuseSummary_P95_TwoTraces(t *testing.T) {
	traces := []langfuse.Trace{
		{Latency: 0.1}, // 100ms
		{Latency: 0.5}, // 500ms
	}
	result := computeLangfuseSummary(traces, 2, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

	b, _ := json.Marshal(result)
	var out ObservabilitySummaryResponse
	json.Unmarshal(b, &out)

	// p95 index = ceil(0.95*2) - 1 = 2 - 1 = 1 → latencies[1] = 500ms
	if out.Metrics.P95LatencyMs != 500 {
		t.Errorf("p95_latency_ms = %v, want 500", out.Metrics.P95LatencyMs)
	}
}

func TestComputeLangfuseSummary_TracesPerHour(t *testing.T) {
	traces := []langfuse.Trace{{Latency: 0.1}, {Latency: 0.1}}
	// 48 traces over 24 hours = 2.0 traces/hr
	result := computeLangfuseSummary(traces, 48, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

	m := result["metrics"].(gin.H)
	if m["traces_per_hour"] != 2.0 {
		t.Errorf("traces_per_hour = %v, want 2.0", m["traces_per_hour"])
	}
}

func TestComputeLangfuseSummary_InvalidTimeRange(t *testing.T) {
	traces := []langfuse.Trace{{Latency: 0.1}}
	result := computeLangfuseSummary(traces, 1, "not-a-time", "also-not-a-time")

	m := result["metrics"].(gin.H)
	if m["traces_per_hour"] != 0.0 {
		t.Errorf("traces_per_hour = %v, want 0 for invalid time range", m["traces_per_hour"])
	}
}

func TestComputeLangfuseSummary_ZeroDurationRange(t *testing.T) {
	traces := []langfuse.Trace{{Latency: 0.1}}
	result := computeLangfuseSummary(traces, 1, "2026-03-20T00:00:00Z", "2026-03-20T00:00:00Z")

	m := result["metrics"].(gin.H)
	if m["traces_per_hour"] != 0.0 {
		t.Errorf("traces_per_hour = %v, want 0 for zero duration", m["traces_per_hour"])
	}
}

func TestLangfuseMetricsBucketFiltering(t *testing.T) {
	metrics := []langfuse.DailyMetric{
		{Date: "2026-03-18", CountTraces: 10, Usage: []langfuse.DailyMetricUsage{{InputUsage: 100, OutputUsage: 50}}},
		{Date: "2026-03-19", CountTraces: 0}, // should be filtered out
		{Date: "2026-03-20", CountTraces: 5, Usage: []langfuse.DailyMetricUsage{{InputUsage: 200, OutputUsage: 80}}},
	}

	var buckets []gin.H
	for _, m := range metrics {
		if m.CountTraces == 0 {
			continue
		}
		buckets = append(buckets, gin.H{
			"timestamp":      m.Date,
			"trace_count":    m.CountTraces,
			"avg_latency_ms": 0,
			"input_tokens":   m.InputTokens(),
			"output_tokens":  m.OutputTokens(),
			"error_count":    0,
		})
	}

	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}
	if buckets[0]["timestamp"] != "2026-03-18" {
		t.Errorf("bucket[0] timestamp = %v, want 2026-03-18", buckets[0]["timestamp"])
	}
	if buckets[1]["timestamp"] != "2026-03-20" {
		t.Errorf("bucket[1] timestamp = %v, want 2026-03-20", buckets[1]["timestamp"])
	}
	if buckets[0]["input_tokens"] != 100 {
		t.Errorf("bucket[0] input_tokens = %v, want 100", buckets[0]["input_tokens"])
	}
	if buckets[1]["output_tokens"] != 80 {
		t.Errorf("bucket[1] output_tokens = %v, want 80", buckets[1]["output_tokens"])
	}
}
