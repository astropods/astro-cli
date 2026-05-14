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

var accountCols = []string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "account_number", "bio", "location", "email", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}

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
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
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
	// deploymentStore is nil — not reached in the "not configured" early-return path.
	router.GET("/api/v1/accounts/:account/observability/summary",
		GetAccountLangfuseSummary(log, cfg, accountStore, nil, langfuseStore))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/myorg/observability/summary", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp AccountObservabilitySummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Totals.Requests != 0 {
		t.Errorf("totals.requests = %d, want 0", resp.Totals.Requests)
	}
	if len(resp.CostOverTime) != 0 {
		t.Errorf("cost_over_time len = %d, want 0", len(resp.CostOverTime))
	}
	if len(resp.CostByModel) != 0 {
		t.Errorf("cost_by_model len = %d, want 0", len(resp.CostByModel))
	}
	// No from/to — change key should be absent.
	if resp.Change != nil {
		t.Errorf("change should be nil when no period provided, got %+v", resp.Change)
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

// ── buildAccountSummary ───────────────────────────────────────────────────────

func TestBuildAccountSummary_Empty(t *testing.T) {
	resp := buildAccountSummary(nil, nil, false, "", "", 0)

	if resp.Totals.Requests != 0 || resp.Totals.CostUSD != 0 {
		t.Errorf("expected zero totals, got %+v", resp.Totals)
	}
	if resp.Change != nil {
		t.Errorf("change should be nil when hasPeriod=false")
	}
	if len(resp.CostOverTime) != 0 {
		t.Errorf("cost_over_time should be empty, got %d entries", len(resp.CostOverTime))
	}
	if len(resp.CostByModel) != 0 {
		t.Errorf("cost_by_model should be empty, got %d entries", len(resp.CostByModel))
	}
}

func TestBuildAccountSummary_Totals(t *testing.T) {
	current := []langfuse.DailyMetric{
		{
			Date: "2026-04-01", CountTraces: 100, TotalCost: 5.0,
			Usage: []langfuse.DailyMetricUsage{
				{Model: "claude-sonnet", InputUsage: 1000, OutputUsage: 500, TotalCost: 3.0},
				{Model: "claude-haiku", InputUsage: 200, OutputUsage: 100, TotalCost: 2.0},
			},
		},
		{
			Date: "2026-04-02", CountTraces: 50, TotalCost: 2.5,
			Usage: []langfuse.DailyMetricUsage{
				{Model: "claude-sonnet", InputUsage: 500, OutputUsage: 250, TotalCost: 2.5},
			},
		},
	}

	resp := buildAccountSummary(current, nil, false, "2026-04-01T00:00:00Z", "2026-04-08T00:00:00Z", 3)

	if resp.Totals.Requests != 150 {
		t.Errorf("requests = %d, want 150", resp.Totals.Requests)
	}
	if resp.Totals.InputTokens != 1700 {
		t.Errorf("input_tokens = %d, want 1700", resp.Totals.InputTokens)
	}
	if resp.Totals.OutputTokens != 850 {
		t.Errorf("output_tokens = %d, want 850", resp.Totals.OutputTokens)
	}
	if resp.Totals.ActiveAgents != 3 {
		t.Errorf("active_agents = %d, want 3", resp.Totals.ActiveAgents)
	}
	// cost_usd: 7.5 rounded to 2dp
	if resp.Totals.CostUSD != 7.5 {
		t.Errorf("cost_usd = %v, want 7.5", resp.Totals.CostUSD)
	}
	// Period is 7 days; daily_avg requests = 150/7 ≈ 21.43
	if resp.DailyAvg.Requests == 0 {
		t.Errorf("daily_avg.requests should be non-zero")
	}
	if resp.Period.Days != 7 {
		t.Errorf("period.days = %d, want 7", resp.Period.Days)
	}
}

func TestBuildAccountSummary_CostOverTime(t *testing.T) {
	current := []langfuse.DailyMetric{
		{
			Date: "2026-04-02", CountTraces: 10, TotalCost: 2.0,
			Usage: []langfuse.DailyMetricUsage{{Model: "gpt-4", TotalCost: 2.0}},
		},
		{
			Date: "2026-04-01", CountTraces: 5, TotalCost: 1.0,
			Usage: []langfuse.DailyMetricUsage{{Model: "gpt-4", TotalCost: 1.0}},
		},
	}

	resp := buildAccountSummary(current, nil, false, "", "", 0)

	// cost_over_time should be sorted by date ascending
	if len(resp.CostOverTime) != 2 {
		t.Fatalf("expected 2 cost_over_time entries, got %d", len(resp.CostOverTime))
	}
	if resp.CostOverTime[0].Date != "2026-04-01" {
		t.Errorf("first date = %q, want 2026-04-01", resp.CostOverTime[0].Date)
	}
	if resp.CostOverTime[1].Date != "2026-04-02" {
		t.Errorf("second date = %q, want 2026-04-02", resp.CostOverTime[1].Date)
	}
}

func TestBuildAccountSummary_CostByModel(t *testing.T) {
	current := []langfuse.DailyMetric{
		{
			Date: "2026-04-01", TotalCost: 10.0,
			Usage: []langfuse.DailyMetricUsage{
				{Model: "claude-haiku", TotalCost: 3.0},
				{Model: "claude-sonnet", TotalCost: 7.0},
			},
		},
	}

	resp := buildAccountSummary(current, nil, false, "", "", 0)

	if len(resp.CostByModel) != 2 {
		t.Fatalf("expected 2 cost_by_model entries, got %d", len(resp.CostByModel))
	}
	// Sorted by cost desc: sonnet first
	if resp.CostByModel[0].Model != "claude-sonnet" {
		t.Errorf("first model = %q, want claude-sonnet", resp.CostByModel[0].Model)
	}
	if resp.CostByModel[0].CostPct != 70.0 {
		t.Errorf("sonnet cost_pct = %v, want 70.0", resp.CostByModel[0].CostPct)
	}
	if resp.CostByModel[1].CostPct != 30.0 {
		t.Errorf("haiku cost_pct = %v, want 30.0", resp.CostByModel[1].CostPct)
	}
}

func TestBuildAccountSummary_ChangeWithPrior(t *testing.T) {
	current := []langfuse.DailyMetric{
		{Date: "2026-04-08", CountTraces: 200, TotalCost: 10.0,
			Usage: []langfuse.DailyMetricUsage{{Model: "m", InputUsage: 200, OutputUsage: 200, TotalCost: 10.0}}},
	}
	prior := []langfuse.DailyMetric{
		{Date: "2026-04-01", CountTraces: 100, TotalCost: 5.0,
			Usage: []langfuse.DailyMetricUsage{{Model: "m", InputUsage: 100, OutputUsage: 100, TotalCost: 5.0}}},
	}

	resp := buildAccountSummary(current, prior, true, "2026-04-08T00:00:00Z", "2026-04-15T00:00:00Z", 0)

	if resp.Change == nil {
		t.Fatal("change should be present when hasPeriod=true")
	}
	// cost: (10-5)/5 * 100 = 100%
	if resp.Change.CostPct == nil || *resp.Change.CostPct != 100.0 {
		t.Errorf("cost_pct = %v, want 100.0", resp.Change.CostPct)
	}
	// requests: (200-100)/100 * 100 = 100%
	if resp.Change.RequestsPct == nil || *resp.Change.RequestsPct != 100.0 {
		t.Errorf("requests_pct = %v, want 100.0", resp.Change.RequestsPct)
	}
}

func TestBuildAccountSummary_ChangeNullWhenPriorZero(t *testing.T) {
	current := []langfuse.DailyMetric{
		{Date: "2026-04-08", CountTraces: 100, TotalCost: 5.0,
			Usage: []langfuse.DailyMetricUsage{{Model: "m", TotalCost: 5.0}}},
	}

	// Empty prior (no data in the prior period)
	resp := buildAccountSummary(current, nil, true, "2026-04-08T00:00:00Z", "2026-04-15T00:00:00Z", 0)

	if resp.Change == nil {
		t.Fatal("change key should exist when hasPeriod=true")
	}
	// Prior cost was 0 → division by zero → null
	if resp.Change.CostPct != nil {
		t.Errorf("cost_pct should be nil when prior=0, got %v", *resp.Change.CostPct)
	}
}

// ── pctChange ─────────────────────────────────────────────────────────────────

func TestPctChange(t *testing.T) {
	tests := []struct {
		current, prior float64
		want           *float64
	}{
		{200, 100, ptr(100.0)},
		{50, 100, ptr(-50.0)},
		{100, 100, ptr(0.0)},
		{100, 0, nil}, // division by zero → nil
		{0, 100, ptr(-100.0)},
	}
	for _, tt := range tests {
		got := pctChange(tt.current, tt.prior)
		if tt.want == nil {
			if got != nil {
				t.Errorf("pctChange(%v, %v) = %v, want nil", tt.current, tt.prior, *got)
			}
			continue
		}
		if got == nil || *got != *tt.want {
			t.Errorf("pctChange(%v, %v) = %v, want %v", tt.current, tt.prior, got, *tt.want)
		}
	}
}

func ptr(f float64) *float64 { return &f }
