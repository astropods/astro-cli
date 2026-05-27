package handlers

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestObservabilityResponseContract_Metrics verifies that Langfuse metrics
// responses deserialize into the ObservabilityMetricsResponse contract.
func TestObservabilityResponseContract_Metrics(t *testing.T) {
	resp := gin.H{
		"buckets": []gin.H{
			{
				"timestamp":      "2026-03-20",
				"trace_count":    10,
				"avg_latency_ms": 0,
				"input_tokens":   500,
				"output_tokens":  300,
				"error_count":    0,
			},
		},
		"time_range":       gin.H{"start": "2026-03-19T00:00:00Z", "end": "2026-03-20T00:00:00Z"},
		"interval_minutes": 1440,
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var out ObservabilityMetricsResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if out.IntervalMinutes == 0 {
		t.Errorf("interval_minutes should be non-zero")
	}
	if len(out.Buckets) == 0 {
		t.Errorf("expected at least one bucket")
	}
	if out.TimeRange.Start == "" || out.TimeRange.End == "" {
		t.Errorf("time_range start/end must be set")
	}

	bucketJSON, _ := json.Marshal(out.Buckets[0])
	var bucketMap map[string]any
	json.Unmarshal(bucketJSON, &bucketMap)

	for _, key := range []string{"timestamp", "trace_count", "avg_latency_ms", "input_tokens", "output_tokens", "error_count"} {
		if _, ok := bucketMap[key]; !ok {
			t.Errorf("bucket missing key %q", key)
		}
	}
}

// TestObservabilityResponseContract_Summary verifies that Langfuse summary
// responses deserialize into the ObservabilitySummaryResponse contract.
func TestObservabilityResponseContract_Summary(t *testing.T) {
	resp := gin.H{
		"total_traces": 42,
		"time_range":   gin.H{"start": "2026-03-19T00:00:00Z", "end": "2026-03-20T00:00:00Z"},
		"metrics": gin.H{
			"avg_latency_ms":  150.25,
			"p95_latency_ms":  320.10,
			"total_tokens":    0,
			"error_rate":      0,
			"traces_per_hour": 1.75,
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var out ObservabilitySummaryResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if out.TotalTraces == 0 {
		t.Errorf("total_traces should be non-zero")
	}
	if out.TimeRange.Start == "" || out.TimeRange.End == "" {
		t.Errorf("time_range start/end must be set")
	}

	metricsJSON, _ := json.Marshal(out.Metrics)
	var metricsMap map[string]any
	json.Unmarshal(metricsJSON, &metricsMap)

	for _, key := range []string{"avg_latency_ms", "p95_latency_ms", "total_tokens", "error_rate", "traces_per_hour"} {
		if _, ok := metricsMap[key]; !ok {
			t.Errorf("metrics missing key %q", key)
		}
	}
}

// TestObservabilityResponseContract_Traces verifies that Langfuse traces
// responses deserialize into the ObservabilityTracesResponse contract.
func TestObservabilityResponseContract_Traces(t *testing.T) {
	resp := gin.H{
		"traces": []gin.H{
			{
				"trace_id":   "def-456",
				"name":       "chat",
				"status":     "ok",
				"latency_ms": 245.5,
				"input":      "hello",
				"output":     "hi there",
				"timestamp":  "2026-03-20T10:00:00Z",
			},
		},
		"total":  100,
		"limit":  50,
		"offset": 0,
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var out ObservabilityTracesResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if out.Total == 0 {
		t.Errorf("total should be non-zero")
	}
	if len(out.Traces) == 0 {
		t.Errorf("expected at least one trace")
	}

	traceJSON, _ := json.Marshal(out.Traces[0])
	var traceMap map[string]any
	json.Unmarshal(traceJSON, &traceMap)

	for _, key := range []string{"trace_id", "name", "status", "latency_ms", "input", "output", "timestamp"} {
		if _, ok := traceMap[key]; !ok {
			t.Errorf("trace entry missing key %q", key)
		}
	}
}

// TestObservabilityResponseContract_EmptyStates verifies empty responses
// deserialize correctly.
func TestObservabilityResponseContract_EmptyStates(t *testing.T) {
	emptyMetrics := gin.H{
		"buckets":          []any{},
		"time_range":       gin.H{"start": "", "end": ""},
		"interval_minutes": 60,
	}

	emptySummary := gin.H{
		"total_traces": 0,
		"time_range":   gin.H{"start": "", "end": ""},
		"metrics": gin.H{
			"avg_latency_ms":  0,
			"p95_latency_ms":  0,
			"total_tokens":    0,
			"error_rate":      0,
			"traces_per_hour": 0,
		},
	}

	emptyTraces := gin.H{
		"traces": []any{},
		"total":  0,
		"limit":  50,
		"offset": 0,
	}

	t.Run("empty metrics", func(t *testing.T) {
		b, _ := json.Marshal(emptyMetrics)
		var out ObservabilityMetricsResponse
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(out.Buckets) != 0 {
			t.Errorf("expected 0 buckets, got %d", len(out.Buckets))
		}
	})

	t.Run("empty summary", func(t *testing.T) {
		b, _ := json.Marshal(emptySummary)
		var out ObservabilitySummaryResponse
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if out.TotalTraces != 0 {
			t.Errorf("expected 0 total_traces, got %d", out.TotalTraces)
		}
	})

	t.Run("empty traces", func(t *testing.T) {
		b, _ := json.Marshal(emptyTraces)
		var out ObservabilityTracesResponse
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(out.Traces) != 0 {
			t.Errorf("expected 0 traces, got %d", len(out.Traces))
		}
	})
}

// TestObservabilityResponseContract_DeploymentSummaries verifies the bulk
// deployment summaries response shape.
func TestObservabilityResponseContract_DeploymentSummaries(t *testing.T) {
	resp := gin.H{
		"summaries": gin.H{
			"dep-abc": gin.H{
				"total_traces":  7,
				"last_trace_at": "2026-05-01T10:00:00Z",
			},
			"dep-xyz": gin.H{
				"total_traces":  0,
				"last_trace_at": "",
			},
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var out DeploymentSummariesResponse
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(out.Summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(out.Summaries))
	}
	if out.Summaries["dep-abc"].TotalTraces != 7 {
		t.Errorf("dep-abc total_traces = %d, want 7", out.Summaries["dep-abc"].TotalTraces)
	}
	if out.Summaries["dep-abc"].LastTraceAt != "2026-05-01T10:00:00Z" {
		t.Errorf("dep-abc last_trace_at = %q, want 2026-05-01T10:00:00Z", out.Summaries["dep-abc"].LastTraceAt)
	}
	if out.Summaries["dep-xyz"].TotalTraces != 0 {
		t.Errorf("dep-xyz total_traces = %d, want 0", out.Summaries["dep-xyz"].TotalTraces)
	}
}
