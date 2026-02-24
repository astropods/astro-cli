package galileo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchLogStreams(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/v2/projects/proj-1/log_streams/search" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Header.Get("Galileo-Api-Key") != "test-key" {
				t.Errorf("expected Galileo-API-Key header")
			}

			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			filters, _ := body["filters"].([]any)
			if len(filters) == 0 {
				t.Errorf("expected filters in request body")
			}

			_ = json.NewEncoder(w).Encode(map[string]any{
				"log_streams": []map[string]string{
					{"id": "ls-1", "name": "my-agent-build-1"},
					{"id": "ls-2", "name": "my-agent-build-2"},
				},
			})
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "test-key", "proj-1")
		streams, err := c.SearchLogStreams("proj-1", "my-agent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(streams) != 2 {
			t.Fatalf("expected 2 streams, got %d", len(streams))
		}
		if streams[0].ID != "ls-1" {
			t.Errorf("expected first stream ID ls-1, got %s", streams[0].ID)
		}
		if streams[1].Name != "my-agent-build-2" {
			t.Errorf("expected second stream name my-agent-build-2, got %s", streams[1].Name)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"log_streams": []any{}})
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "key", "proj")
		streams, err := c.SearchLogStreams("proj", "no-match")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(streams) != 0 {
			t.Errorf("expected 0 streams, got %d", len(streams))
		}
	})

	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal"}`))
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "key", "proj")
		_, err := c.SearchLogStreams("proj", "agent")
		if err == nil {
			t.Fatal("expected error on 500 response")
		}
	})
}

func TestSearchMetrics(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/v2/projects/proj-1/metrics/search" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["log_stream_id"] != "ls-1" {
				t.Errorf("expected log_stream_id=ls-1, got %v", body["log_stream_id"])
			}
			if body["start_time"] != "2024-01-01T00:00:00Z" {
				t.Errorf("unexpected start_time: %v", body["start_time"])
			}
			if body["interval"] != float64(30) {
				t.Errorf("expected interval=30, got %v", body["interval"])
			}

			_ = json.NewEncoder(w).Encode(MetricsResponse{
				AggregateMetrics: AggregateMetrics{
					RequestsCount: 10,
					AvgDurationNs: 50500000,
				},
				BucketedMetrics: map[string][]MetricsBucket{
					"all": {
						{StartBucketTime: "2024-01-01T00:00:00Z", EndBucketTime: "2024-01-01T01:00:00Z", RequestsCount: 10, AvgDurationNs: 50500000},
					},
				},
			})
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "key", "proj-1")
		resp, err := c.SearchMetrics("proj-1", "ls-1", "2024-01-01T00:00:00Z", "2024-01-01T01:00:00Z", 30)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		buckets := resp.BucketedMetrics["all"]
		if len(buckets) != 1 {
			t.Fatalf("expected 1 bucket, got %d", len(buckets))
		}
		if buckets[0].RequestsCount != 10 {
			t.Errorf("expected requests_count=10, got %d", buckets[0].RequestsCount)
		}
		if resp.AggregateMetrics.AvgDurationNs != 50500000 {
			t.Errorf("expected avg_duration_ns=50500000, got %f", resp.AggregateMetrics.AvgDurationNs)
		}
	})

	t.Run("zero interval omitted", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["interval"]; ok {
				t.Errorf("expected interval to be omitted, got %v", body["interval"])
			}
			_ = json.NewEncoder(w).Encode(MetricsResponse{})
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "key", "proj")
		_, err := c.SearchMetrics("proj", "ls-1", "start", "end", 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestSearchTraces(t *testing.T) {
	t.Run("success with all params", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/v2/projects/proj-1/traces/search" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["log_stream_id"] != "ls-1" {
				t.Errorf("expected log_stream_id=ls-1")
			}
			if body["limit"] != float64(25) {
				t.Errorf("expected limit=25, got %v", body["limit"])
			}
			if body["starting_token"] != float64(10) {
				t.Errorf("expected starting_token=10, got %v", body["starting_token"])
			}

			_ = json.NewEncoder(w).Encode(TracesResponse{
				Records: []TraceEntry{
					{TraceID: "t1", Name: "agent/llm-call", StatusCode: 1, Metrics: TraceMetrics{DurationNs: 120500000}},
				},
				NumRecords:    100,
				Limit:         25,
				StartingToken: 10,
			})
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "key", "proj-1")
		resp, err := c.SearchTraces("proj-1", "ls-1", "start", "end", 25, 10, "error")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Records) != 1 {
			t.Fatalf("expected 1 trace, got %d", len(resp.Records))
		}
		if resp.Records[0].TraceID != "t1" {
			t.Errorf("expected trace_id=t1, got %s", resp.Records[0].TraceID)
		}
		if resp.NumRecords != 100 {
			t.Errorf("expected num_records=100, got %d", resp.NumRecords)
		}
	})

	t.Run("optional params omitted", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if _, ok := body["limit"]; ok {
				t.Errorf("expected limit to be omitted, got %v", body["limit"])
			}
			if _, ok := body["starting_token"]; ok {
				t.Errorf("expected starting_token to be omitted, got %v", body["starting_token"])
			}
			_ = json.NewEncoder(w).Encode(TracesResponse{})
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "key", "proj")
		_, err := c.SearchTraces("proj", "ls-1", "start", "end", 0, 0, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "key", "proj")
		_, err := c.SearchTraces("proj", "ls-1", "start", "end", 0, 0, "")
		if err == nil {
			t.Fatal("expected error on invalid JSON")
		}
	})
}

func TestAuthenticationHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Galileo-Api-Key")
		_ = json.NewEncoder(w).Encode(map[string]any{"log_streams": []any{}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "my-secret-key", "proj")
	_, _ = c.SearchLogStreams("proj", "agent")

	want := "my-secret-key"
	if gotKey != want {
		t.Errorf("Galileo-API-Key header: expected %q, got %q", want, gotKey)
	}
}
