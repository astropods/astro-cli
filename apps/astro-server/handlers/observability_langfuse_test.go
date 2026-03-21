package handlers

import (
	"encoding/json"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/gin-gonic/gin"
)

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
