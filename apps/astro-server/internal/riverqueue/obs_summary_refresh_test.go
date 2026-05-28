package riverqueue

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/obssummary"
)

// mapCache is a thread-safe in-memory implementation of k8scache.Cache used
// across these worker tests so we can assert exactly what the worker wrote.
type mapCache struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMapCache() *mapCache { return &mapCache{data: make(map[string][]byte)} }

func (c *mapCache) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[key]
	return v, ok
}

func (c *mapCache) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = data
	return nil
}

func (c *mapCache) Invalidate(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

// Ensure mapCache satisfies the cache interface used by obssummary.
var _ k8scache.Cache = (*mapCache)(nil)

// fakeClient is a langfuseSummaryClient stub keyed by deployment id.
type fakeClient struct {
	traces    map[string]*langfuse.TracesResponse
	daily     map[string][]langfuse.DailyMetric
	tracesErr error
	dailyErr  error
}

func (f *fakeClient) GetTraces(_ context.Context, deploymentID, _, _ string, _, _ int) (*langfuse.TracesResponse, error) {
	if f.tracesErr != nil {
		return nil, f.tracesErr
	}
	if r, ok := f.traces[deploymentID]; ok {
		return r, nil
	}
	return &langfuse.TracesResponse{}, nil
}

func (f *fakeClient) GetDailyMetrics(_ context.Context, deploymentID, _, _ string) ([]langfuse.DailyMetric, error) {
	if f.dailyErr != nil {
		return nil, f.dailyErr
	}
	return f.daily[deploymentID], nil
}

// ────────────────────────────────────────────────────────────────────────────

func TestObsSummaryRefreshArgs_Kind(t *testing.T) {
	if kind := (ObsSummaryRefreshArgs{}).Kind(); kind != "obs.summary_refresh" {
		t.Errorf("Kind() = %q, want obs.summary_refresh", kind)
	}
}

func TestRefreshWindow_Shape(t *testing.T) {
	start, end, dates := refreshWindow()
	if len(dates) != obssummary.DaysOfHistory {
		t.Fatalf("len(dates) = %d, want %d", len(dates), obssummary.DaysOfHistory)
	}
	// Each date should be YYYY-MM-DD and dates must be strictly ascending.
	for i, d := range dates {
		if _, err := time.Parse("2006-01-02", d); err != nil {
			t.Errorf("dates[%d] = %q is not YYYY-MM-DD: %v", i, d, err)
		}
		if i > 0 && dates[i] <= dates[i-1] {
			t.Errorf("dates not ascending: %q <= %q at index %d", dates[i], dates[i-1], i)
		}
	}
	if _, err := time.Parse(time.RFC3339, start); err != nil {
		t.Errorf("startTime not RFC3339: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, end); err != nil {
		t.Errorf("endTime not RFC3339: %v", err)
	}
}

func TestRefreshOne_WritesEntryWithSeries(t *testing.T) {
	cache := newMapCache()
	_, _, dates := refreshWindow()
	// Pick the latest day so the value lands at index DaysOfHistory-1.
	lastDay := dates[len(dates)-1]

	w := &ObsSummaryRefreshWorker{
		cache: cache,
		log:   logger.New("error", "text"),
	}

	client := &fakeClient{
		traces: map[string]*langfuse.TracesResponse{
			"dep-1": {
				Data: []langfuse.Trace{{ID: "t1", CreatedAt: "2026-05-27T10:00:00Z"}},
				Meta: struct {
					Page       int `json:"page"`
					Limit      int `json:"limit"`
					TotalItems int `json:"totalItems"`
					TotalPages int `json:"totalPages"`
				}{TotalItems: 42},
			},
		},
		daily: map[string][]langfuse.DailyMetric{
			"dep-1": {
				{
					Date:        lastDay,
					CountTraces: 7,
					Usage: []langfuse.DailyMetricUsage{
						{InputUsage: 100, OutputUsage: 50},
					},
				},
			},
		},
	}

	if err := w.refreshOne(context.Background(), client, "dep-1", "ignored-start", "ignored-end", dates); err != nil {
		t.Fatalf("refreshOne returned error: %v", err)
	}

	raw, ok := cache.Get(context.Background(), obssummary.KeyFor("dep-1"))
	if !ok {
		t.Fatal("no cache entry was written")
	}
	var entry obssummary.Entry
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if entry.TotalTraces != 42 {
		t.Errorf("TotalTraces = %d, want 42", entry.TotalTraces)
	}
	if entry.LastTraceAt != "2026-05-27T10:00:00Z" {
		t.Errorf("LastTraceAt = %q, want 2026-05-27T10:00:00Z", entry.LastTraceAt)
	}
	if len(entry.RequestSeries) != obssummary.DaysOfHistory {
		t.Fatalf("RequestSeries len = %d, want %d", len(entry.RequestSeries), obssummary.DaysOfHistory)
	}
	if got := entry.RequestSeries[len(entry.RequestSeries)-1]; got != 7 {
		t.Errorf("last RequestSeries = %d, want 7", got)
	}
	if got := entry.TokenSeries[len(entry.TokenSeries)-1]; got != 150 {
		t.Errorf("last TokenSeries = %d, want 150 (input+output)", got)
	}
	// All other days should be zero-padded.
	for i := 0; i < len(entry.RequestSeries)-1; i++ {
		if entry.RequestSeries[i] != 0 || entry.TokenSeries[i] != 0 {
			t.Errorf("non-spike day %d not zero-padded: req=%d tok=%d", i, entry.RequestSeries[i], entry.TokenSeries[i])
		}
	}
}

func TestRefreshOne_DailyMetricsError_StillWritesEntry(t *testing.T) {
	// Failure to fetch daily metrics should NOT drop the entry — total_traces
	// and last_trace_at are still useful. Series are zero-padded.
	cache := newMapCache()
	_, _, dates := refreshWindow()
	w := &ObsSummaryRefreshWorker{cache: cache, log: logger.New("error", "text")}

	client := &fakeClient{
		traces: map[string]*langfuse.TracesResponse{
			"dep-2": {
				Meta: struct {
					Page       int `json:"page"`
					Limit      int `json:"limit"`
					TotalItems int `json:"totalItems"`
					TotalPages int `json:"totalPages"`
				}{TotalItems: 5},
			},
		},
		dailyErr: errSimulated,
	}

	if err := w.refreshOne(context.Background(), client, "dep-2", "", "", dates); err != nil {
		t.Fatalf("refreshOne returned error: %v", err)
	}

	raw, ok := cache.Get(context.Background(), obssummary.KeyFor("dep-2"))
	if !ok {
		t.Fatal("no cache entry was written")
	}
	var entry obssummary.Entry
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if entry.TotalTraces != 5 {
		t.Errorf("TotalTraces = %d, want 5", entry.TotalTraces)
	}
	for i := range entry.RequestSeries {
		if entry.RequestSeries[i] != 0 || entry.TokenSeries[i] != 0 {
			t.Errorf("series not zero at index %d: req=%d tok=%d", i, entry.RequestSeries[i], entry.TokenSeries[i])
		}
	}
}

func TestRefreshOne_TracesError_NoWrite(t *testing.T) {
	// If we can't even get the trace count, skip this deployment entirely —
	// don't write a misleading "0 traces" entry that would replace a valid
	// older one.
	cache := newMapCache()
	_, _, dates := refreshWindow()
	w := &ObsSummaryRefreshWorker{cache: cache, log: logger.New("error", "text")}

	client := &fakeClient{tracesErr: errSimulated}

	err := w.refreshOne(context.Background(), client, "dep-3", "", "", dates)
	if err == nil {
		t.Fatal("expected error from refreshOne when traces call fails")
	}
	if _, ok := cache.Get(context.Background(), obssummary.KeyFor("dep-3")); ok {
		t.Error("cache entry should NOT have been written on traces error")
	}
}

// errSimulated is a sentinel for fake-client error injection.
var errSimulated = simulatedError{}

type simulatedError struct{}

func (simulatedError) Error() string { return "simulated error" }
