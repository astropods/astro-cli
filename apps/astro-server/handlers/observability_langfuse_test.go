package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		GetAccountLangfuseSummary(log, cfg, accountStore, nil, langfuseStore, nil, nil))

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
	result := computeLangfuseSummary(nil, 0, 0, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

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

func TestComputeLangfuseSummary_TotalTokens(t *testing.T) {
	traces := []langfuse.Trace{{Latency: 0.1}}
	result := computeLangfuseSummary(traces, 1, 4200, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

	b, _ := json.Marshal(result)
	var out ObservabilitySummaryResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Metrics.TotalTokens != 4200 {
		t.Errorf("total_tokens = %d, want 4200", out.Metrics.TotalTokens)
	}
}

func TestLangfuseSummary_TokensFromDailyMetrics(t *testing.T) {
	// The trace list carries no per-trace usage, so the summary sources token
	// totals from the daily-metrics endpoint, the same as the refresh worker.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"date": "2026-03-19", "countTraces": 2, "usage": []map[string]any{
					{"model": "gpt-4o", "inputUsage": 100, "outputUsage": 50},
				}},
				{"date": "2026-03-20", "countTraces": 3, "usage": []map[string]any{
					{"model": "gpt-4o", "inputUsage": 200, "outputUsage": 75},
				}},
			},
			"meta": map[string]any{"page": 1, "totalPages": 1},
		})
	}))
	defer srv.Close()
	client := langfuse.NewClient(srv.URL, "pk", "sk")

	dailyMetrics, err := client.GetDailyMetrics(context.Background(), "dep-a", "2026-03-19T00:00:00Z", "2026-03-21T00:00:00Z")
	if err != nil {
		t.Fatalf("get daily metrics: %v", err)
	}
	totalTokens := 0
	for _, m := range dailyMetrics {
		totalTokens += m.InputTokens() + m.OutputTokens()
	}

	result := computeLangfuseSummary([]langfuse.Trace{{Latency: 0.1}}, 5, totalTokens,
		"2026-03-19T00:00:00Z", "2026-03-21T00:00:00Z")

	b, _ := json.Marshal(result)
	var out ObservabilitySummaryResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// 100+50 + 200+75 = 425
	if out.Metrics.TotalTokens != 425 {
		t.Errorf("total_tokens = %d, want 425", out.Metrics.TotalTokens)
	}
}

func TestComputeLangfuseSummary_SingleTrace(t *testing.T) {
	traces := []langfuse.Trace{
		{Latency: 0.250}, // 250ms
	}
	result := computeLangfuseSummary(traces, 1, 0, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

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
	result := computeLangfuseSummary(traces, 20, 0, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

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
	result := computeLangfuseSummary(traces, 2, 0, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

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
	result := computeLangfuseSummary(traces, 48, 0, "2026-03-19T00:00:00Z", "2026-03-20T00:00:00Z")

	m := result["metrics"].(gin.H)
	if m["traces_per_hour"] != 2.0 {
		t.Errorf("traces_per_hour = %v, want 2.0", m["traces_per_hour"])
	}
}

func TestComputeLangfuseSummary_InvalidTimeRange(t *testing.T) {
	traces := []langfuse.Trace{{Latency: 0.1}}
	result := computeLangfuseSummary(traces, 1, 0, "not-a-time", "also-not-a-time")

	m := result["metrics"].(gin.H)
	if m["traces_per_hour"] != 0.0 {
		t.Errorf("traces_per_hour = %v, want 0 for invalid time range", m["traces_per_hour"])
	}
}

func TestComputeLangfuseSummary_ZeroDurationRange(t *testing.T) {
	traces := []langfuse.Trace{{Latency: 0.1}}
	result := computeLangfuseSummary(traces, 1, 0, "2026-03-20T00:00:00Z", "2026-03-20T00:00:00Z")

	m := result["metrics"].(gin.H)
	if m["traces_per_hour"] != 0.0 {
		t.Errorf("traces_per_hour = %v, want 0 for zero duration", m["traces_per_hour"])
	}
}

func TestFilterAndSortTraceEntries(t *testing.T) {
	traces := []TraceEntry{
		{
			TraceID:   "trace-ada-slow",
			Name:      "Chat completion",
			LatencyMs: 900,
			TotalCost: 0.03,
			Timestamp: "2026-07-15T03:00:00Z",
			UserID:    "U-ADA",
			UserDetails: &UserDetails{
				Kind:        UserDetailsKindSlack,
				DisplayName: "Ada Lovelace",
				Username:    "ada",
			},
		},
		{
			TraceID:   "trace-bob-fast",
			Name:      "Tool call",
			LatencyMs: 100,
			TotalCost: 0.01,
			Timestamp: "2026-07-15T01:00:00Z",
			UserID:    "U-BOB",
		},
		{
			TraceID:   "trace-no-user",
			Name:      "Background task",
			LatencyMs: 400,
			TotalCost: 0.02,
			Timestamp: "2026-07-15T02:00:00Z",
		},
	}

	t.Run("searches enriched user fields", func(t *testing.T) {
		got := filterAndSortTraceEntries(traces, traceListCriteria{
			search: "lovelace", sortKey: "timestamp", direction: "desc",
		})
		if len(got) != 1 || got[0].TraceID != "trace-ada-slow" {
			t.Fatalf("got trace IDs %v, want [trace-ada-slow]", traceEntryIDs(got))
		}
	})

	t.Run("filters traces without a user", func(t *testing.T) {
		got := filterAndSortTraceEntries(traces, traceListCriteria{
			noUser: true, sortKey: "timestamp", direction: "desc",
		})
		if len(got) != 1 || got[0].TraceID != "trace-no-user" {
			t.Fatalf("got trace IDs %v, want [trace-no-user]", traceEntryIDs(got))
		}
	})

	t.Run("orders the complete result before pagination", func(t *testing.T) {
		ordered := filterAndSortTraceEntries(traces, traceListCriteria{
			sortKey: "latency", direction: "asc",
		})
		got := pageTraceEntries(ordered, 1, 1)
		if len(got) != 1 || got[0].TraceID != "trace-no-user" {
			t.Fatalf("got trace IDs %v, want [trace-no-user]", traceEntryIDs(got))
		}
	})
}

func TestTraceEntryFromLangfuseOmitsBodies(t *testing.T) {
	entry := traceEntryFromLangfuse(langfuse.Trace{
		ID:     "trace-1",
		Input:  map[string]any{"large": "request"},
		Output: map[string]any{"large": "response"},
	}, nil)
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["input"]; ok {
		t.Fatal("trace list entry retained input")
	}
	if _, ok := payload["output"]; ok {
		t.Fatal("trace list entry retained output")
	}
}

func TestTraceListCriteriaUpstreamOrderBy(t *testing.T) {
	tests := []struct {
		name     string
		criteria traceListCriteria
		want     string
		ok       bool
	}{
		{name: "default", criteria: traceListCriteria{sortKey: "timestamp", direction: "desc"}, want: "timestamp.desc", ok: true},
		{name: "ascending", criteria: traceListCriteria{sortKey: "timestamp", direction: "asc"}, want: "timestamp.asc", ok: true},
		{name: "exact user", criteria: traceListCriteria{userID: "U-ADA", sortKey: "timestamp", direction: "desc"}, want: "timestamp.desc", ok: true},
		{name: "identity search", criteria: traceListCriteria{search: "ada", sortKey: "timestamp", direction: "desc"}},
		{name: "latency", criteria: traceListCriteria{sortKey: "latency", direction: "asc"}},
		{name: "missing user", criteria: traceListCriteria{noUser: true, sortKey: "timestamp", direction: "desc"}, want: "timestamp.desc", ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.criteria.upstreamOrderBy()
			if got != tt.want || ok != tt.ok {
				t.Fatalf("upstreamOrderBy() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestTraceListCriteriaUpstreamFilters(t *testing.T) {
	filters := (traceListCriteria{noUser: true}).upstreamFilters(
		"dep-1", "2026-07-01T00:00:00Z", "2026-07-02T00:00:00Z",
	)
	if len(filters) != 4 {
		t.Fatalf("got %d filters, want 4", len(filters))
	}
	if filters[0].Column != "tags" || filters[0].Operator != "all of" {
		t.Fatalf("deployment filter = %+v", filters[0])
	}
	if filters[1].Column != "timestamp" || filters[1].Operator != ">=" {
		t.Fatalf("start filter = %+v", filters[1])
	}
	if filters[2].Column != "timestamp" || filters[2].Operator != "<" {
		t.Fatalf("end filter = %+v", filters[2])
	}
	if filters[3].Type != "null" || filters[3].Column != "userId" || filters[3].Operator != "is null" {
		t.Fatalf("missing-user filter = %+v", filters[3])
	}
}

func TestTraceFilterSourcesPushSearchPredicatesUpstream(t *testing.T) {
	base := (traceListCriteria{noUser: true}).upstreamFilters("dep-1", "", "")
	sources := traceFilterSources(base, traceListCriteria{search: "ada", noUser: true}, nil)

	if len(sources) != 3 {
		t.Fatalf("got %d sources, want bounded fallback plus id and name filters", len(sources))
	}
	for index, column := range []string{"id", "name"} {
		filters := sources[index+1]
		searchFilter := filters[len(filters)-1]
		if searchFilter.Column != column || searchFilter.Operator != "contains" || searchFilter.Value != "ada" {
			t.Fatalf("source %d search filter = %+v", index+1, searchFilter)
		}
		if filters[len(filters)-2].Type != "null" {
			t.Fatalf("source %d lost missing-user predicate: %+v", index+1, filters)
		}
	}
}

func TestMatchingTraceIdentityUserIDsUsesEnrichedIdentity(t *testing.T) {
	facets := []TraceUserFacet{
		{UserID: "U-ADA", UserDetails: &UserDetails{DisplayName: "Ada Lovelace", Username: "ada"}},
		{UserID: "U-GRACE", UserDetails: &UserDetails{DisplayName: "Grace Hopper", Username: "grace"}},
	}

	got, truncated := matchingTraceIdentityUserIDs(facets, traceListCriteria{search: "lovelace"})
	if truncated || len(got) != 1 || got[0] != "U-ADA" {
		t.Fatalf("matching identity IDs = %v, truncated=%v; want [U-ADA], false", got, truncated)
	}
}

func TestParseTraceListCriteriaUsesStructuredUserParams(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantUserID string
		wantNoUser bool
	}{
		{name: "user ID", query: "?user_id=U-ADA", wantUserID: "U-ADA"},
		{name: "no user", query: "?no_user=true", wantNoUser: true},
		{name: "no user takes precedence", query: "?user_id=U-ADA&no_user=true", wantNoUser: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodGet, "/traces"+tt.query, nil)

			criteria := parseTraceListCriteria(context)
			if criteria.userID != tt.wantUserID || criteria.noUser != tt.wantNoUser {
				t.Fatalf(
					"user criteria = (userID %q, noUser %v), want (%q, %v)",
					criteria.userID, criteria.noUser, tt.wantUserID, tt.wantNoUser,
				)
			}
		})
	}
}

func TestTraceCriteriaCacheReusesResolvedResult(t *testing.T) {
	cache := newTraceCriteriaCache()
	loads := 0
	load := func() (traceCriteriaResult, error) {
		loads++
		return traceCriteriaResult{
			traces:       []TraceEntry{{TraceID: "trace-1"}, {TraceID: "trace-2"}},
			truncated:    true,
			scannedCount: 2000,
		}, nil
	}

	first, err := cache.getOrLoad("same-window", load)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := cache.getOrLoad("same-window", load)
	if err != nil {
		t.Fatalf("cached load: %v", err)
	}
	if loads != 1 {
		t.Fatalf("load called %d times, want 1", loads)
	}
	if len(first.traces) != 2 || len(second.traces) != 2 {
		t.Fatalf("cache returned lengths %d and %d, want 2", len(first.traces), len(second.traces))
	}
	if !second.truncated || second.scannedCount != 2000 {
		t.Fatalf("cache lost criteria metadata: %+v", second)
	}
}

func TestTraceCriteriaCacheEvictsOldestEntry(t *testing.T) {
	cache := newTraceCriteriaCache()
	now := time.Now()
	for index := range traceCriteriaCacheMaxEntries {
		key := "entry-" + strconv.Itoa(index)
		cache.entries[key] = traceCriteriaCacheEntry{
			result:    traceCriteriaResult{traces: []TraceEntry{{TraceID: key}}},
			expiresAt: now.Add(time.Duration(index+1) * time.Minute),
		}
	}

	cache.put("new-entry", traceCriteriaResult{traces: []TraceEntry{{TraceID: "new-entry"}}})

	if _, ok := cache.entries["entry-0"]; ok {
		t.Fatal("oldest cache entry was not evicted")
	}
	if _, ok := cache.entries["new-entry"]; !ok {
		t.Fatal("new cache entry was not stored")
	}
	if len(cache.entries) != traceCriteriaCacheMaxEntries {
		t.Fatalf("cache contains %d entries, want %d", len(cache.entries), traceCriteriaCacheMaxEntries)
	}
}

func TestTraceCriteriaLoadContextOutlivesRequestCancellation(t *testing.T) {
	type contextKey string
	requestCtx, cancelRequest := context.WithCancel(
		context.WithValue(context.Background(), contextKey("viewer"), "viewer-1"),
	)
	cancelRequest()

	loadCtx, cancelLoad := newTraceCriteriaLoadContext(requestCtx)
	defer cancelLoad()

	select {
	case <-loadCtx.Done():
		t.Fatalf("shared load context ended with request: %v", loadCtx.Err())
	default:
	}
	if _, ok := loadCtx.Deadline(); !ok {
		t.Fatal("shared load context must have a deadline")
	}
	if got := loadCtx.Value(contextKey("viewer")); got != "viewer-1" {
		t.Fatalf("shared load context lost request values: got %v", got)
	}
}

func traceEntryIDs(traces []TraceEntry) []string {
	ids := make([]string, 0, len(traces))
	for _, trace := range traces {
		ids = append(ids, trace.TraceID)
	}
	return ids
}

func TestLoadAllTracePages(t *testing.T) {
	first := &langfuse.TracesResponse{Data: []langfuse.Trace{{ID: "page-1"}}}
	first.Meta.TotalItems = 250
	fetch := func(_ context.Context, _ int, offset int) (*langfuse.TracesResponse, error) {
		return &langfuse.TracesResponse{Data: []langfuse.Trace{{ID: "page-" + strconv.Itoa(offset/maxTracesLimit+1)}}}, nil
	}

	got, truncated, err := loadAllTracePages(context.Background(), first, fetch)
	if err != nil {
		t.Fatalf("loadAllTracePages returned error: %v", err)
	}
	if truncated {
		t.Fatal("loadAllTracePages unexpectedly truncated a three-page result")
	}
	want := []string{"page-1", "page-2", "page-3"}
	if len(got) != len(want) {
		t.Fatalf("got %d traces, want %d", len(got), len(want))
	}
	for i, trace := range got {
		if trace.ID != want[i] {
			t.Fatalf("trace %d = %q, want %q", i, trace.ID, want[i])
		}
	}
}

func TestLoadAllTracePagesCapsCriteriaWindow(t *testing.T) {
	const pageCount = 101
	first := &langfuse.TracesResponse{Data: []langfuse.Trace{{ID: "page-1"}}}
	first.Meta.TotalItems = pageCount * maxTracesLimit
	fetch := func(_ context.Context, _ int, offset int) (*langfuse.TracesResponse, error) {
		return &langfuse.TracesResponse{
			Data: []langfuse.Trace{{ID: "page-" + strconv.Itoa(offset/maxTracesLimit+1)}},
		}, nil
	}

	got, truncated, err := loadAllTracePages(context.Background(), first, fetch)
	if err != nil {
		t.Fatalf("loadAllTracePages returned error: %v", err)
	}
	if !truncated {
		t.Fatal("loadAllTracePages did not report truncation")
	}
	wantPages := maxTraceCriteriaItems / maxTracesLimit
	if len(got) != wantPages || got[len(got)-1].ID != "page-10" {
		t.Fatalf("loaded %d page markers, last=%q; want %d/page-10", len(got), got[len(got)-1].ID, wantPages)
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
	resp := buildAccountSummary(nil, nil, false, "", "", 0, nil)

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

	resp := buildAccountSummary(current, nil, true, "2026-04-01T00:00:00Z", "2026-04-08T00:00:00Z", 3, nil)

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

func TestBuildAccountSummary_PerModelBreakdown(t *testing.T) {
	current := []langfuse.DailyMetric{
		{
			Date: "2026-04-01", CountTraces: 100, TotalCost: 5.0,
			Usage: []langfuse.DailyMetricUsage{
				{Model: "claude-sonnet", InputUsage: 1000, OutputUsage: 500, TotalCost: 3.0},
				{Model: "claude-haiku", InputUsage: 200, OutputUsage: 100, TotalCost: 1.0},
			},
		},
		{
			Date: "2026-04-02", CountTraces: 50, TotalCost: 1.0,
			Usage: []langfuse.DailyMetricUsage{
				{Model: "claude-sonnet", InputUsage: 500, OutputUsage: 250, TotalCost: 1.0},
			},
		},
	}
	stats := map[string]modelStats{
		"claude-sonnet": {Requests: 120, P50Ms: 0.8 * 1000, P95Ms: 1.9 * 1000},
		"claude-haiku":  {Requests: 30, P50Ms: 0.3 * 1000, P95Ms: 0.6 * 1000},
	}

	resp := buildAccountSummary(current, nil, false, "", "", 0, stats)

	byModel := map[string]AccountCostByModelEntry{}
	for _, e := range resp.CostByModel {
		byModel[e.Model] = e
	}
	// Total tokens: sonnet (1500+750)=2250, haiku 300, total 2550.
	sonnet := byModel["claude-sonnet"]
	if sonnet.TotalTokens != 2250 {
		t.Errorf("sonnet total_tokens = %d, want 2250", sonnet.TotalTokens)
	}
	if sonnet.Requests != 120 {
		t.Errorf("sonnet requests = %d, want 120", sonnet.Requests)
	}
	if sonnet.P95LatencyMs != 1900 {
		t.Errorf("sonnet p95_latency_ms = %v, want 1900", sonnet.P95LatencyMs)
	}
	if sonnet.LastSeen != "2026-04-02" {
		t.Errorf("sonnet last_seen = %q, want 2026-04-02", sonnet.LastSeen)
	}
	// token_pct: 2250/2550 ≈ 88.2
	if sonnet.TokenPct < 88 || sonnet.TokenPct > 89 {
		t.Errorf("sonnet token_pct = %v, want ~88.2", sonnet.TokenPct)
	}
	haiku := byModel["claude-haiku"]
	if haiku.TotalTokens != 300 || haiku.Requests != 30 || haiku.P50LatencyMs != 300 {
		t.Errorf("haiku entry wrong: %+v", haiku)
	}
}

func TestParseModelStats(t *testing.T) {
	rows := []map[string]any{
		{"providedModelName": "gpt-4o", "count_count": float64(42), "p50_latency": 0.5, "p95_latency": 1.25},
		{"providedModelName": nil},  // non-LLM observation, skipped
		{"count_count": float64(9)}, // no model, skipped
	}
	got := parseModelStats(rows)
	if len(got) != 1 {
		t.Fatalf("want 1 model, got %d", len(got))
	}
	s := got["gpt-4o"]
	if s.Requests != 42 || s.P50Ms != 500 || s.P95Ms != 1250 {
		t.Errorf("gpt-4o stats wrong: %+v", s)
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

	resp := buildAccountSummary(current, nil, false, "", "", 0, nil)

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

	resp := buildAccountSummary(current, nil, false, "", "", 0, nil)

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

	resp := buildAccountSummary(current, prior, true, "2026-04-08T00:00:00Z", "2026-04-15T00:00:00Z", 0, nil)

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
	resp := buildAccountSummary(current, nil, true, "2026-04-08T00:00:00Z", "2026-04-15T00:00:00Z", 0, nil)

	if resp.Change == nil {
		t.Fatal("change key should exist when hasPeriod=true")
	}
	// Prior cost was 0 → division by zero → null
	if resp.Change.CostPct != nil {
		t.Errorf("cost_pct should be nil when prior=0, got %v", *resp.Change.CostPct)
	}
}

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
		"dep-1": {DeploymentID: "dep-1", Name: "customer-support", Account: "acme"},
		"dep-2": {DeploymentID: "dep-2", Name: "code-reviewer", Account: "anthropic-public"},
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
		"dep-1": {DeploymentID: "dep-1", Name: "customer-support", Account: "acme"},
		"dep-2": {DeploymentID: "dep-2", Name: "code-reviewer", Account: "acme"},
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

// Two deployments of the same blueprint surface as two separate refs
// — the People-tab "Agents Used" column mirrors the Agents-tab "one row
// per deployment" shape rather than collapsing to one chip per blueprint.
// Differentiation between identical-avatar chips happens on the client
// (tooltip with display_name/namespace + per-deployment click target).
func TestBuildUsersSummary_SameBlueprintDifferentDeployments(t *testing.T) {
	mainRows := []map[string]any{
		{"userId": "u_alice", "count_count": 3.0, "sum_totalCost": 1.5, "sum_totalTokens": 300.0, "time_dimension": "2026-04-01T12:00:00Z"},
	}
	tagsRows := []map[string]any{
		{"userId": "u_alice", "tags": "deployment:dep-1"},
		{"userId": "u_alice", "tags": "deployment:dep-2"},
	}
	depToAgent := map[string]UserAgentRef{
		"dep-1": {DeploymentID: "dep-1", Name: "weather-poet", Account: "acme"},
		"dep-2": {DeploymentID: "dep-2", Name: "weather-poet", Account: "acme"},
	}

	out := buildUsersSummary(mainRows, tagsRows, depToAgent)
	if len(out) != 1 || len(out[0].AgentsUsed) != 2 {
		t.Fatalf("expected 1 user with 2 deployment refs, got %+v", out)
	}
	ids := map[string]bool{}
	for _, a := range out[0].AgentsUsed {
		if a.Name != "weather-poet" || a.Account != "acme" {
			t.Errorf("ref name/account mismatch: %+v", a)
		}
		ids[a.DeploymentID] = true
	}
	if !ids["dep-1"] || !ids["dep-2"] {
		t.Errorf("expected both dep-1 and dep-2 in refs, got %+v", out[0].AgentsUsed)
	}
}

// ── Translation + aggregation correctness ───────────────────────────────
//
// These tests pin the contract that linked Slack ids get rewritten to
// the WorkOS id BEFORE buildUsersSummary runs, so the resulting per-
// user buckets sum cost/requests/tokens exactly across what were
// previously two parallel rows. The compute path applies
// applyLinkedSlackUserIDTranslation directly; this is the same flow.

// Linked Slack and WorkOS spend for the same human folds into one
// row keyed by the WorkOS id after translation. Verifies the four
// metric invariants: cost+requests+tokens sum exactly, last_seen takes
// the max, agents_used unions on deployment_id without duplicates.
func TestBuildUsersSummary_LinkedSlackFoldsIntoWorkOSWithCorrectMetrics(t *testing.T) {
	mainRows := []map[string]any{
		// Bob's bare-Slack spend before he linked his account.
		{"userId": "U07BOBBOB1", "count_count": 10.0, "sum_totalCost": 5.0, "sum_totalTokens": 1000.0, "time_dimension": "2026-04-01T00:00:00Z"},
		// Bob's WorkOS spend after he linked.
		{"userId": "user_01HXX_bob", "count_count": 3.0, "sum_totalCost": 2.5, "sum_totalTokens": 300.0, "time_dimension": "2026-06-01T12:00:00Z"},
	}
	tagsRows := []map[string]any{
		// Pre-link activity tagged to dep-old; post-link tagged to dep-new.
		{"userId": "U07BOBBOB1", "tags": "deployment:dep-old"},
		{"userId": "user_01HXX_bob", "tags": "deployment:dep-new"},
	}
	depToAgent := map[string]UserAgentRef{
		"dep-old": {DeploymentID: "dep-old", Name: "old-bot", Account: "postman"},
		"dep-new": {DeploymentID: "dep-new", Name: "new-bot", Account: "postman"},
	}
	linkMap := map[string]string{"U07BOBBOB1": "user_01HXX_bob"}

	applyLinkedSlackUserIDTranslation(linkMap, mainRows, tagsRows)
	out := buildUsersSummary(mainRows, tagsRows, depToAgent)

	if len(out) != 1 {
		t.Fatalf("expected 1 row after translation, got %d: %+v", len(out), out)
	}
	bob := out[0]
	if bob.UserID != "user_01HXX_bob" {
		t.Errorf("merged row should key on WorkOS id, got %q", bob.UserID)
	}
	if bob.Requests != 13 || bob.CostUSD != 7.5 || bob.Tokens != 1300 {
		t.Errorf("metrics did not sum exactly: requests=%d cost=%v tokens=%d (want 13 / 7.5 / 1300)",
			bob.Requests, bob.CostUSD, bob.Tokens)
	}
	if bob.LastSeen != "2026-06-01T12:00:00Z" {
		t.Errorf("last_seen = %q, want max timestamp 2026-06-01T12:00:00Z", bob.LastSeen)
	}
	if len(bob.AgentsUsed) != 2 {
		t.Errorf("agents_used should union to 2 refs (no dup deployment_id), got %d: %+v",
			len(bob.AgentsUsed), bob.AgentsUsed)
	}
	depIDs := map[string]bool{}
	for _, a := range bob.AgentsUsed {
		depIDs[a.DeploymentID] = true
	}
	if !depIDs["dep-old"] || !depIDs["dep-new"] {
		t.Errorf("agents_used missing one of dep-old/dep-new: %+v", bob.AgentsUsed)
	}
}

// Unlinked Slack and Astro rows for different humans must NOT fold
// together — translation only rewrites ids the directory says are
// linked. Belt-and-suspenders against a regression where the rewrite
// accidentally affects non-linked ids.
func TestBuildUsersSummary_UnlinkedSlackKeepsItsOwnRow(t *testing.T) {
	mainRows := []map[string]any{
		{"userId": "U07GHOSTLY", "count_count": 5.0, "sum_totalCost": 2.0, "sum_totalTokens": 500.0, "time_dimension": "2026-04-01T00:00:00Z"},
		{"userId": "user_01HXX_alice", "count_count": 3.0, "sum_totalCost": 1.5, "sum_totalTokens": 300.0, "time_dimension": "2026-04-02T00:00:00Z"},
	}
	tagsRows := []map[string]any{}
	depToAgent := map[string]UserAgentRef{}
	// Empty link map — no rewrite. Both rows stay separate.
	applyLinkedSlackUserIDTranslation(map[string]string{}, mainRows, tagsRows)

	out := buildUsersSummary(mainRows, tagsRows, depToAgent)
	if len(out) != 2 {
		t.Fatalf("expected 2 distinct rows, got %d: %+v", len(out), out)
	}
}

// Cost-over-time per-day per-user totals sum exactly across the same
// day when translation collapses two parallel rows into one bucket.
// Different days must remain separate entries.
func TestBuildCostOverTimeByUser_LinkedRowsSumExactlyPerDay(t *testing.T) {
	rows := []map[string]any{
		// Day 1: bare-Slack and WorkOS spend on the same day — should fold.
		{"userId": "U07BOBBOB1", "sum_totalCost": 3.0, "count_count": 4.0, "sum_totalTokens": 400.0, "time_dimension": "2026-04-01T08:00:00Z"},
		{"userId": "user_01HXX_bob", "sum_totalCost": 2.0, "count_count": 1.0, "sum_totalTokens": 100.0, "time_dimension": "2026-04-01T16:00:00Z"},
		// Day 2: only WorkOS spend — stays as its own entry on its own day.
		{"userId": "user_01HXX_bob", "sum_totalCost": 1.5, "count_count": 2.0, "sum_totalTokens": 200.0, "time_dimension": "2026-04-02T08:00:00Z"},
	}
	applyLinkedSlackUserIDTranslation(map[string]string{"U07BOBBOB1": "user_01HXX_bob"}, rows)
	out := buildCostOverTimeByUser(rows)

	if len(out) != 2 {
		t.Fatalf("expected 2 date entries, got %d: %+v", len(out), out)
	}
	if out[0].Date != "2026-04-01" || out[1].Date != "2026-04-02" {
		t.Errorf("dates not sorted ascending: %q / %q", out[0].Date, out[1].Date)
	}
	if len(out[0].Users) != 1 {
		t.Fatalf("day-1: expected 1 merged user, got %+v", out[0].Users)
	}
	day1 := out[0].Users[0]
	if day1.UserID != "user_01HXX_bob" || day1.CostUSD != 5.0 || day1.Requests != 5 || day1.Tokens != 500 {
		t.Errorf("day-1 metrics mismatch: %+v (want user_01HXX_bob / 5.0 / 5 / 500)", day1)
	}
	day2 := out[1].Users[0]
	if day2.CostUSD != 1.5 || day2.Requests != 2 || day2.Tokens != 200 {
		t.Errorf("day-2 metrics mismatch: %+v", day2)
	}
}

// Deployments-summary's users_used + users_used_details fold linked
// rows into one entry per resolved user_id post-translation. Two
// deployments touched by the linked user collapse independently per
// deployment (no cross-deployment merge).
func TestBuildDeploymentSummary_LinkedSlackFoldsPerDeployment(t *testing.T) {
	metrics := []deploymentMetrics{
		{DeploymentID: "dep-1", AgentName: "code-reviewer", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 1, TotalCost: 1.0}}},
		{DeploymentID: "dep-2", AgentName: "swipefile", DailyMetrics: []langfuse.DailyMetric{{CountTraces: 1, TotalCost: 1.0}}},
	}
	// Both the bare-Slack id and the WorkOS id touch dep-1; only the
	// WorkOS id touches dep-2.
	tagsRows := []map[string]any{
		{"userId": "U07BOBBOB1", "tags": []any{"deployment:dep-1"}},
		{"userId": "user_01HXX_bob", "tags": []any{"deployment:dep-1"}},
		{"userId": "user_01HXX_bob", "tags": []any{"deployment:dep-2"}},
	}
	applyLinkedSlackUserIDTranslation(map[string]string{"U07BOBBOB1": "user_01HXX_bob"}, tagsRows)
	out := buildDeploymentSummaryWithUsers(metrics, buildDeploymentUserRows(tagsRows), nil, nil)

	byID := make(map[string]DeploymentSummaryEntry, len(out))
	for _, d := range out {
		byID[d.DeploymentID] = d
	}
	if len(byID["dep-1"].UsersUsed) != 1 || byID["dep-1"].UsersUsed[0] != "user_01HXX_bob" {
		t.Errorf("dep-1 should have one user (user_01HXX_bob), got %+v", byID["dep-1"].UsersUsed)
	}
	if len(byID["dep-1"].UsersUsedDetails) != 1 || byID["dep-1"].UsersUsedDetails[0].UserID != "user_01HXX_bob" {
		t.Errorf("dep-1 users_used_details should fold to one entry, got %+v", byID["dep-1"].UsersUsedDetails)
	}
	if len(byID["dep-2"].UsersUsed) != 1 || byID["dep-2"].UsersUsed[0] != "user_01HXX_bob" {
		t.Errorf("dep-2 should have user_01HXX_bob, got %+v", byID["dep-2"].UsersUsed)
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
		depToAgent[depID] = UserAgentRef{DeploymentID: depID, Name: "agent-" + strconv.Itoa(i), Account: "acme"}
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

// The Slack-directory merge for cost-over-time has moved out of
// buildCostOverTimeByUser entirely. Linked-Slack rows are now rewritten
// in the raw Langfuse rows BEFORE buildCostOverTimeByUser runs (via
// translateLinkedSlackUserIDs at the compute layer), so the bucketing
// happens naturally on the resolved user_id. ResolveAccountSummaryIdentities
// only stamps team_id on the unlinked bare-Slack rows that survived.
// Those two layers each have their own tests below.

// TestBuildCostOverTimeByUser_LinkedRowsBucketByWorkOSId pins the contract
// that buildCostOverTimeByUser groups by whatever user_id is in the row —
// the compute layer is expected to have already translated linked Slack
// user_ids to their WorkOS id.
func TestBuildCostOverTimeByUser_LinkedRowsBucketByWorkOSId(t *testing.T) {
	rows := []map[string]any{
		// Caller (compute path) translated U07BOBBOB1 → user_bob before
		// calling buildCostOverTimeByUser. Two rows for user_bob on the
		// same day should merge.
		{"userId": "user_bob", "sum_totalCost": 3.0, "count_count": 4.0, "sum_totalTokens": 400.0, "time_dimension": "2026-04-01T08:00:00.000Z"},
		{"userId": "user_bob", "sum_totalCost": 2.0, "count_count": 1.0, "sum_totalTokens": 100.0, "time_dimension": "2026-04-01T16:00:00.000Z"},
	}

	out := buildCostOverTimeByUser(rows)

	if len(out) != 1 {
		t.Fatalf("expected 1 date entry, got %d", len(out))
	}
	if len(out[0].Users) != 1 {
		t.Fatalf("expected one user_bob bucket post-translation, got %+v", out[0].Users)
	}
	got := out[0].Users[0]
	if got.UserID != "user_bob" || got.UserDetails.Kind != UserDetailsKindAstro {
		t.Fatalf("identity mismatch: %+v", got)
	}
	if got.CostUSD != 5.0 || got.Requests != 5 || got.Tokens != 500 {
		t.Fatalf("merged metrics mismatch: %+v", got)
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

// ── accountDailyMetrics (batched /metrics) ──────────────────────────────────

// batchedMetricsHandler responds to /api/public/metrics with one of two row
// sets depending on the query view: traces (per-(tags, day) count) or
// observations (per-(model, day) cost + tokens). Returns 500 to either by
// setting the corresponding response to nil.
func batchedMetricsHandler(t *testing.T, tracesRows, obsRows []map[string]any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		var parsed struct {
			View string `json:"view"`
		}
		_ = json.Unmarshal([]byte(q), &parsed)
		var rows []map[string]any
		switch parsed.View {
		case "traces":
			rows = tracesRows
		case "observations":
			rows = obsRows
		}
		if rows == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": rows})
	}
}

func TestAccountDailyMetrics_MergesPerDateAndTracksActiveDeps(t *testing.T) {
	// Q_traces — per-(tags, day) trace count
	tracesRows := []map[string]any{
		{"time_dimension": "2026-04-01T00:00:00Z", "tags": "deployment:dep-a", "count_count": 10},
		{"time_dimension": "2026-04-01T00:00:00Z", "tags": "deployment:dep-b", "count_count": 3},
		{"time_dimension": "2026-04-02T00:00:00Z", "tags": "deployment:dep-a", "count_count": 5},
		// dep-c sends nothing — should NOT show up in activeDeps.
	}
	// Q_obs — per-(providedModelName, day) cost + tokens
	obsRows := []map[string]any{
		{"time_dimension": "2026-04-01T00:00:00Z", "providedModelName": "gpt-4o",
			"sum_totalCost": 1.2, "sum_inputTokens": 120, "sum_outputTokens": 60},
		{"time_dimension": "2026-04-02T00:00:00Z", "providedModelName": "gpt-4o",
			"sum_totalCost": 0.5, "sum_inputTokens": 50, "sum_outputTokens": 25},
	}
	srv := httptest.NewServer(batchedMetricsHandler(t, tracesRows, obsRows))
	defer srv.Close()
	client := langfuse.NewClient(srv.URL, "pk", "sk")

	out, activeDeps, err := accountDailyMetrics(context.Background(), client,
		[]string{"deployment:dep-a", "deployment:dep-b", "deployment:dep-c"},
		"2026-04-01T00:00:00Z", "2026-04-03T00:00:00Z", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 merged date buckets, got %d", len(out))
	}
	if out[0].Date != "2026-04-01" || out[1].Date != "2026-04-02" {
		t.Errorf("dates not sorted: %q, %q", out[0].Date, out[1].Date)
	}
	// 2026-04-01: traces = 10 (dep-a) + 3 (dep-b) = 13; cost from obs = 1.2.
	if out[0].CountTraces != 13 || out[0].TotalCost != 1.2 {
		t.Errorf("2026-04-01: traces=%d cost=%v, want 13 / 1.2", out[0].CountTraces, out[0].TotalCost)
	}
	if !activeDeps["dep-a"] || !activeDeps["dep-b"] {
		t.Errorf("dep-a/dep-b should be active, got %+v", activeDeps)
	}
	if activeDeps["dep-c"] {
		t.Errorf("dep-c should not be active, got active=true")
	}
}

func TestAccountDailyMetrics_TracesQueryFailFailsAll(t *testing.T) {
	// Batched semantics: any sub-query failure surfaces an error (no partial
	// success). The handler's prior-period path wraps this in fail-open via
	// the caller; the function itself returns the error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	client := langfuse.NewClient(srv.URL, "pk", "sk")

	_, _, err := accountDailyMetrics(context.Background(), client,
		[]string{"deployment:dep-a"}, "2026-04-01T00:00:00Z", "2026-04-03T00:00:00Z", true)
	if err == nil {
		t.Fatal("expected error when /metrics returns 500")
	}
}

func TestAccountDailyMetrics_NoTagsReturnsEmpty(t *testing.T) {
	// Empty tag list = no live deployments. Should return zero state without
	// issuing any requests.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("expected zero HTTP calls when tag list is empty")
	}))
	defer srv.Close()
	client := langfuse.NewClient(srv.URL, "pk", "sk")

	out, active, err := accountDailyMetrics(context.Background(), client, nil, "", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 || len(active) != 0 {
		t.Errorf("expected empty results, got out=%d active=%d", len(out), len(active))
	}
}

func TestBuildTraceUserFacetsAggregatesFullWindowCounts(t *testing.T) {
	rows := []map[string]any{
		{"userId": "user_sohum", "count_count": float64(120)},
		{"userId": "user_sohum", "count_count": "30"},
		{"userId": "-", "count_count": float64(7)},
	}

	got := buildTraceUserFacets(nil, nil, nil, rows)
	if len(got) != 2 {
		t.Fatalf("expected two facets, got %#v", got)
	}
	if got[0].UserID != "" || got[0].Count != 7 || got[0].UserDetails != nil {
		t.Errorf("unexpected no-user facet: %#v", got[0])
	}
	if got[1].UserID != "user_sohum" || got[1].Count != 150 {
		t.Errorf("unexpected Astro user facet: %#v", got[1])
	}
	if got[1].UserDetails == nil || got[1].UserDetails.Kind != UserDetailsKindAstro {
		t.Errorf("expected resolved Astro identity kind, got %#v", got[1].UserDetails)
	}
}

func TestAstroProfileDetailsParticipateInTraceSearch(t *testing.T) {
	hydrator := &userDetailsHydrator{astro: map[string]account.PersonalProfile{
		"user_sohum": {UserID: "user_sohum", Name: "sohum", DisplayName: "Sohum Dalal"},
	}}
	details := traceUserDetailsFromHydrator("user_sohum", hydrator)
	trace := TraceEntry{TraceID: "trace-1", UserID: "user_sohum", UserDetails: details}

	if details == nil || details.DisplayName != "Sohum Dalal" || details.Username != "sohum" {
		t.Fatalf("unexpected Astro details: %#v", details)
	}
	if !traceEntryMatchesSearch(trace, "sohum dalal") {
		t.Fatal("resolved Astro display name did not match trace search")
	}
}
