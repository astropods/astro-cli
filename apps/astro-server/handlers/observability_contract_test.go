package handlers

import (
	"encoding/json"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestObservabilityResponseContract_Metrics verifies that both Galileo and
// Langfuse metrics responses deserialize into the same ObservabilityMetricsResponse.
func TestObservabilityResponseContract_Metrics(t *testing.T) {
	galileo := gin.H{
		"buckets": []gin.H{
			{
				"timestamp":      "2026-03-20T00:00:00Z",
				"trace_count":    10,
				"avg_latency_ms": 123.45,
				"input_tokens":   500,
				"output_tokens":  300,
				"error_count":    1,
			},
		},
		"time_range":       gin.H{"start": "2026-03-19T00:00:00Z", "end": "2026-03-20T00:00:00Z"},
		"interval_minutes": 60,
	}

	langfuse := gin.H{
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

	for name, resp := range map[string]gin.H{"galileo": galileo, "langfuse": langfuse} {
		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("%s: marshal error: %v", name, err)
		}

		var out ObservabilityMetricsResponse
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("%s: unmarshal error: %v", name, err)
		}

		if out.IntervalMinutes == 0 {
			t.Errorf("%s: interval_minutes should be non-zero", name)
		}
		if len(out.Buckets) == 0 {
			t.Errorf("%s: expected at least one bucket", name)
		}
		if out.TimeRange.Start == "" || out.TimeRange.End == "" {
			t.Errorf("%s: time_range start/end must be set", name)
		}

		// Verify all bucket fields are present by re-marshalling the typed struct
		// and checking no keys were lost.
		bucketJSON, _ := json.Marshal(out.Buckets[0])
		var bucketMap map[string]any
		json.Unmarshal(bucketJSON, &bucketMap)

		for _, key := range []string{"timestamp", "trace_count", "avg_latency_ms", "input_tokens", "output_tokens", "error_count"} {
			if _, ok := bucketMap[key]; !ok {
				t.Errorf("%s: bucket missing key %q", name, key)
			}
		}
	}
}

// TestObservabilityResponseContract_Summary verifies that both Galileo and
// Langfuse summary responses deserialize into the same ObservabilitySummaryResponse.
func TestObservabilityResponseContract_Summary(t *testing.T) {
	galileo := gin.H{
		"total_traces": 42,
		"time_range":   gin.H{"start": "2026-03-19T00:00:00Z", "end": "2026-03-20T00:00:00Z"},
		"metrics": gin.H{
			"avg_latency_ms":  150.25,
			"p95_latency_ms":  320.10,
			"total_tokens":    0,
			"error_rate":      0.05,
			"traces_per_hour": 1.75,
		},
	}

	langfuse := gin.H{
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

	for name, resp := range map[string]gin.H{"galileo": galileo, "langfuse": langfuse} {
		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("%s: marshal error: %v", name, err)
		}

		var out ObservabilitySummaryResponse
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("%s: unmarshal error: %v", name, err)
		}

		if out.TotalTraces == 0 {
			t.Errorf("%s: total_traces should be non-zero", name)
		}
		if out.TimeRange.Start == "" || out.TimeRange.End == "" {
			t.Errorf("%s: time_range start/end must be set", name)
		}

		// Verify all metrics fields are present
		metricsJSON, _ := json.Marshal(out.Metrics)
		var metricsMap map[string]any
		json.Unmarshal(metricsJSON, &metricsMap)

		for _, key := range []string{"avg_latency_ms", "p95_latency_ms", "total_tokens", "error_rate", "traces_per_hour"} {
			if _, ok := metricsMap[key]; !ok {
				t.Errorf("%s: metrics missing key %q", name, key)
			}
		}
	}
}

// TestObservabilityResponseContract_Traces verifies that both Galileo and
// Langfuse traces responses deserialize into the same ObservabilityTracesResponse.
func TestObservabilityResponseContract_Traces(t *testing.T) {
	galileo := gin.H{
		"traces": []gin.H{
			{
				"trace_id":   "abc-123",
				"name":       "chat",
				"status":     "error",
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

	langfuse := gin.H{
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

	for name, resp := range map[string]gin.H{"galileo": galileo, "langfuse": langfuse} {
		b, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("%s: marshal error: %v", name, err)
		}

		var out ObservabilityTracesResponse
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("%s: unmarshal error: %v", name, err)
		}

		if out.Total == 0 {
			t.Errorf("%s: total should be non-zero", name)
		}
		if len(out.Traces) == 0 {
			t.Errorf("%s: expected at least one trace", name)
		}

		// Verify all trace entry fields are present
		traceJSON, _ := json.Marshal(out.Traces[0])
		var traceMap map[string]any
		json.Unmarshal(traceJSON, &traceMap)

		for _, key := range []string{"trace_id", "name", "status", "latency_ms", "input", "output", "timestamp"} {
			if _, ok := traceMap[key]; !ok {
				t.Errorf("%s: trace entry missing key %q", name, key)
			}
		}
	}
}

// TestObservabilityResponseContract_EmptyStates verifies empty responses match
// across both backends.
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
