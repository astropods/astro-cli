package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

var accountCols = account.SQLMockScanColumns

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
			AddRow("acct-1", "myorg", "organization", nil, nil, now, now, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))
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

// ── buildUsersSummary ─────────────────────────────────────────────────────────

func TestBuildUsersSummary_AggregationAndAgents(t *testing.T) {
	// Two hour-buckets per user → verify sums + last_seen takes the max ts.
	mainRows := []map[string]any{
		{"userId": "u_alice", "count_count": 10.0, "sum_totalCost": 1.5, "sum_totalTokens": 500.0, "time_dimension": "2026-04-01T10:00:00Z"},
		{"userId": "u_alice", "count_count": 5.0, "sum_totalCost": 0.5, "sum_totalTokens": 200.0, "time_dimension": "2026-04-01T14:00:00Z"},
		{"userId": "u_bob", "count_count": 3.0, "sum_totalCost": 0.25, "sum_totalTokens": 100.0, "time_dimension": "2026-04-01T11:00:00Z"},
	}
	// Tags fan-out: alice hit dep-1 and dep-2 (with dep-2 appearing twice → dedup), bob only dep-1.
	tagsRows := []map[string]any{
		{"userId": "u_alice", "tags": "deployment:dep-1"},
		{"userId": "u_alice", "tags": "deployment:dep-2"},
		{"userId": "u_alice", "tags": "deployment:dep-2"},
		{"userId": "u_bob", "tags": "deployment:dep-1"},
		// Unknown tag — should be ignored.
		{"userId": "u_alice", "tags": "env:prod"},
	}
	// dep-2 is a cross-account/public-blueprint deployment — its avatar account
	// (publisher) differs from the deploying account, exercising the new
	// per-entry account field.
	depToAgent := map[string]UserAgentRef{
		"dep-1": {Name: "customer-support", Account: "acme"},
		"dep-2": {Name: "code-reviewer", Account: "anthropic-public"},
	}

	out := buildUsersSummary(mainRows, tagsRows, depToAgent)

	if len(out) != 2 {
		t.Fatalf("expected 2 users, got %d", len(out))
	}
	// Sorted by cost desc: alice (2.0) before bob (0.25).
	if out[0].UserID != "u_alice" {
		t.Errorf("first user = %q, want u_alice", out[0].UserID)
	}
	if out[0].Requests != 15 {
		t.Errorf("alice.requests = %d, want 15", out[0].Requests)
	}
	if out[0].CostUSD != 2.0 {
		t.Errorf("alice.cost = %v, want 2.0", out[0].CostUSD)
	}
	if out[0].Tokens != 700 {
		t.Errorf("alice.tokens = %d, want 700", out[0].Tokens)
	}
	// last_seen tracks the most recent non-zero bucket.
	if out[0].LastSeen != "2026-04-01T14:00:00Z" {
		t.Errorf("alice.last_seen = %q, want 2026-04-01T14:00:00Z", out[0].LastSeen)
	}
	// agents_used deduplicated + sorted by name. Each entry carries its
	// publishing account so the client can resolve avatars correctly.
	if len(out[0].AgentsUsed) != 2 {
		t.Fatalf("alice.agents_used len = %d, want 2: %+v", len(out[0].AgentsUsed), out[0].AgentsUsed)
	}
	if out[0].AgentsUsed[0].Name != "code-reviewer" || out[0].AgentsUsed[0].Account != "anthropic-public" {
		t.Errorf("alice[0] = %+v, want {code-reviewer, anthropic-public}", out[0].AgentsUsed[0])
	}
	if out[0].AgentsUsed[1].Name != "customer-support" || out[0].AgentsUsed[1].Account != "acme" {
		t.Errorf("alice[1] = %+v, want {customer-support, acme}", out[0].AgentsUsed[1])
	}
	if out[1].UserID != "u_bob" || len(out[1].AgentsUsed) != 1 || out[1].AgentsUsed[0].Name != "customer-support" {
		t.Errorf("bob row mismatch: %+v", out[1])
	}
}

func TestBuildUsersSummary_TagsReturnedAsArray(t *testing.T) {
	// Langfuse's grouped `tags` value can come back as an array rather than a
	// single string. Earlier code asserted string and silently dropped every
	// row in the array case, leaving agents_used empty.
	mainRows := []map[string]any{
		{"userId": "u_alice", "count_count": 1.0, "sum_totalCost": 1.0, "sum_totalTokens": 100.0, "time_dimension": "2026-04-01T00:00:00Z"},
	}
	tagsRows := []map[string]any{
		{"userId": "u_alice", "tags": []any{"deployment:dep-1", "env:prod", "deployment:dep-2"}},
	}
	depToAgent := map[string]UserAgentRef{
		"dep-1": {Name: "customer-support", Account: "acme"},
		"dep-2": {Name: "code-reviewer", Account: "acme"},
	}

	out := buildUsersSummary(mainRows, tagsRows, depToAgent)
	if len(out) != 1 {
		t.Fatalf("expected 1 user, got %d", len(out))
	}
	if len(out[0].AgentsUsed) != 2 {
		t.Fatalf("expected 2 agents (env:prod ignored), got %v", out[0].AgentsUsed)
	}
	if out[0].AgentsUsed[0].Name != "code-reviewer" || out[0].AgentsUsed[1].Name != "customer-support" {
		t.Errorf("agents_used = %v, want sorted [code-reviewer customer-support]", out[0].AgentsUsed)
	}
}

func TestBuildUsersSummary_MaxAgentsPerUserCap(t *testing.T) {
	mainRows := []map[string]any{
		{"userId": "u_heavy", "count_count": 1.0, "sum_totalCost": 1.0, "sum_totalTokens": 100.0, "time_dimension": "2026-04-01T00:00:00Z"},
	}
	// 15 distinct deployments tagged to one user — cap should trim to maxAgentsPerUser=10.
	depToAgent := make(map[string]UserAgentRef, 15)
	tagsRows := make([]map[string]any, 0, 15)
	for i := 0; i < 15; i++ {
		depID := "dep-" + strconv.Itoa(i)
		depToAgent[depID] = UserAgentRef{Name: "agent-" + strconv.Itoa(i), Account: "acme"}
		tagsRows = append(tagsRows, map[string]any{"userId": "u_heavy", "tags": "deployment:" + depID})
	}

	out := buildUsersSummary(mainRows, tagsRows, depToAgent)
	if len(out) != 1 {
		t.Fatalf("expected 1 user, got %d", len(out))
	}
	if len(out[0].AgentsUsed) != maxAgentsPerUser {
		t.Errorf("agents_used len = %d, want %d", len(out[0].AgentsUsed), maxAgentsPerUser)
	}
}

// ── buildCostOverTimeByUser ───────────────────────────────────────────────────

func TestBuildCostOverTimeByUser_AggregatesSameUserPerDate(t *testing.T) {
	// Two rows for u_alice on the same day with different RFC3339 timestamps
	// (Langfuse can return multiple rows per day-bucket). Expect one entry
	// with summed cost.
	rows := []map[string]any{
		{"userId": "u_alice", "sum_totalCost": 1.0, "time_dimension": "2026-04-01T08:00:00.000Z"},
		{"userId": "u_alice", "sum_totalCost": 0.5, "time_dimension": "2026-04-01T16:00:00.000Z"},
		{"userId": "u_bob", "sum_totalCost": 2.0, "time_dimension": "2026-04-01T12:00:00.000Z"},
	}

	out := buildCostOverTimeByUser(rows)
	if len(out) != 1 {
		t.Fatalf("expected 1 date entry, got %d", len(out))
	}
	users := out[0].Users
	if len(users) != 2 {
		t.Fatalf("expected 2 user entries (alice merged, bob), got %d: %+v", len(users), users)
	}
	costByUser := map[string]float64{}
	for _, u := range users {
		costByUser[u.UserID] = u.CostUSD
	}
	if costByUser["u_alice"] != 1.5 {
		t.Errorf("u_alice cost = %v, want 1.5 (1.0 + 0.5 merged)", costByUser["u_alice"])
	}
	if costByUser["u_bob"] != 2.0 {
		t.Errorf("u_bob cost = %v, want 2.0", costByUser["u_bob"])
	}
}

func TestBuildCostOverTimeByUser_TruncatesDateAndExcludesZeroCost(t *testing.T) {
	rows := []map[string]any{
		// Same date bucket as a different row; both contribute.
		{"userId": "u_alice", "sum_totalCost": 1.25, "time_dimension": "2026-04-02T08:00:00.000Z"},
		{"userId": "u_bob", "sum_totalCost": 0.75, "time_dimension": "2026-04-02T16:00:00.000Z"},
		// Zero-cost row — must be excluded.
		{"userId": "u_carol", "sum_totalCost": 0.0, "time_dimension": "2026-04-02T18:00:00.000Z"},
		// Earlier date — verifies ascending sort.
		{"userId": "u_alice", "sum_totalCost": 0.5, "time_dimension": "2026-04-01T08:00:00.000Z"},
	}

	out := buildCostOverTimeByUser(rows)

	if len(out) != 2 {
		t.Fatalf("expected 2 date entries, got %d (%+v)", len(out), out)
	}
	if out[0].Date != "2026-04-01" || out[1].Date != "2026-04-02" {
		t.Errorf("dates not sorted ascending: %q, %q", out[0].Date, out[1].Date)
	}
	if len(out[0].Users) != 1 || out[0].Users[0].UserID != "u_alice" || out[0].Users[0].CostUSD != 0.5 {
		t.Errorf("first day mismatch: %+v", out[0])
	}
	// u_carol's zero-cost row is filtered before bucketing → only alice + bob present.
	if len(out[1].Users) != 2 {
		t.Errorf("2026-04-02 should have 2 users (zero-cost excluded), got %d: %+v", len(out[1].Users), out[1].Users)
	}
}

// ── mergedDailyMetrics ────────────────────────────────────────────────────────

// dailyMetricsHandler builds an httptest handler that responds to
// /api/public/metrics/daily based on the `tags` query param, looking up the
// per-deployment response in the supplied table. If a depID is absent, returns
// 500 — used to simulate per-deployment failures.
func dailyMetricsHandler(t *testing.T, responses map[string]langfuse.DailyMetricsResponse) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		tag := r.URL.Query().Get("tags")
		depID := strings.TrimPrefix(tag, "deployment:")
		resp, ok := responses[depID]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if resp.Meta.TotalPages == 0 {
			resp.Meta.TotalPages = 1
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func TestMergedDailyMetrics_MergesPerDateAndTracksActiveDeps(t *testing.T) {
	responses := map[string]langfuse.DailyMetricsResponse{
		"dep-a": {Data: []langfuse.DailyMetric{
			{Date: "2026-04-01", CountTraces: 10, TotalCost: 1.0,
				Usage: []langfuse.DailyMetricUsage{{Model: "gpt-4o", InputUsage: 100, OutputUsage: 50, TotalCost: 1.0}}},
			{Date: "2026-04-02", CountTraces: 5, TotalCost: 0.5,
				Usage: []langfuse.DailyMetricUsage{{Model: "gpt-4o", InputUsage: 50, OutputUsage: 25, TotalCost: 0.5}}},
		}},
		"dep-b": {Data: []langfuse.DailyMetric{
			{Date: "2026-04-01", CountTraces: 3, TotalCost: 0.2,
				Usage: []langfuse.DailyMetricUsage{{Model: "gpt-4o", InputUsage: 20, OutputUsage: 10, TotalCost: 0.2}}},
		}},
		// dep-c returns an empty page — should not appear in activeDeps.
		"dep-c": {Data: []langfuse.DailyMetric{}},
	}
	srv := httptest.NewServer(dailyMetricsHandler(t, responses))
	defer srv.Close()
	client := langfuse.NewClient(srv.URL, "pk", "sk")
	log := logger.New("error", "json")

	out, activeDeps, err := mergedDailyMetrics(context.Background(), client, log,
		[]string{"dep-a", "dep-b", "dep-c"}, "2026-04-01T00:00:00Z", "2026-04-03T00:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 merged date buckets, got %d", len(out))
	}
	// Ascending date order.
	if out[0].Date != "2026-04-01" || out[1].Date != "2026-04-02" {
		t.Errorf("dates not sorted: %q, %q", out[0].Date, out[1].Date)
	}
	// 2026-04-01 = dep-a (10 traces, $1.0) + dep-b (3 traces, $0.2)
	if out[0].CountTraces != 13 || out[0].TotalCost != 1.2 {
		t.Errorf("2026-04-01 merge: traces=%d cost=%v, want 13 / 1.2", out[0].CountTraces, out[0].TotalCost)
	}
	// activeDeps only includes dep-a and dep-b — dep-c had zero rows.
	if !activeDeps["dep-a"] || !activeDeps["dep-b"] {
		t.Errorf("dep-a/dep-b should be active, got %+v", activeDeps)
	}
	if activeDeps["dep-c"] {
		t.Errorf("dep-c should not be active (zero rows), got active=true")
	}
}

func TestMergedDailyMetrics_AllFailReturnsError(t *testing.T) {
	// Every request returns 500 → all per-dep calls error → mergedDailyMetrics
	// returns a non-nil error so the handler can respond 502.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	client := langfuse.NewClient(srv.URL, "pk", "sk")
	log := logger.New("error", "json")

	_, _, err := mergedDailyMetrics(context.Background(), client, log,
		[]string{"dep-a", "dep-b"}, "2026-04-01T00:00:00Z", "2026-04-03T00:00:00Z")
	if err == nil {
		t.Fatal("expected error when all per-deployment calls fail, got nil")
	}
}

func TestMergedDailyMetrics_PartialFailReturnsNoError(t *testing.T) {
	// dep-a succeeds, dep-b returns 500 → partial failure → no error returned
	// (the caller still gets dep-a's data; partial failure is logged at WARN).
	responses := map[string]langfuse.DailyMetricsResponse{
		"dep-a": {Data: []langfuse.DailyMetric{
			{Date: "2026-04-01", CountTraces: 10, TotalCost: 1.0,
				Usage: []langfuse.DailyMetricUsage{{Model: "gpt-4o", TotalCost: 1.0}}},
		}},
	}
	srv := httptest.NewServer(dailyMetricsHandler(t, responses))
	defer srv.Close()
	client := langfuse.NewClient(srv.URL, "pk", "sk")
	log := logger.New("error", "json")

	out, activeDeps, err := mergedDailyMetrics(context.Background(), client, log,
		[]string{"dep-a", "dep-b"}, "2026-04-01T00:00:00Z", "2026-04-03T00:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error on partial failure: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 date bucket (only dep-a succeeded), got %d", len(out))
	}
	if !activeDeps["dep-a"] {
		t.Errorf("dep-a should be active")
	}
	if activeDeps["dep-b"] {
		t.Errorf("dep-b should not be active (call failed)")
	}
}
