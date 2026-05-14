package langfuse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetTraces_QueryParams(t *testing.T) {
	tests := []struct {
		name         string
		deploymentID string
		startTime    string
		endTime      string
		limit        int
		offset       int
		wantPage     string // "" means absent
		wantLimit    string
		wantTags     string
		wantFrom     string
		wantTo       string
	}{
		{
			name:         "offset=0 limit=50 does not set page",
			deploymentID: "dep-1",
			limit:        50,
			offset:       0,
			wantPage:     "",
			wantLimit:    "50",
			wantTags:     "deployment:dep-1",
		},
		{
			name:         "offset=50 limit=50 sets page=2",
			deploymentID: "dep-2",
			limit:        50,
			offset:       50,
			wantPage:     "2",
			wantLimit:    "50",
			wantTags:     "deployment:dep-2",
		},
		{
			name:         "offset=1 limit=0 sets page=2",
			deploymentID: "dep-3",
			limit:        0,
			offset:       1,
			wantPage:     "2",
			wantLimit:    "",
			wantTags:     "deployment:dep-3",
		},
		{
			name:         "offset=0 limit=0 sets neither page nor limit",
			deploymentID: "dep-4",
			limit:        0,
			offset:       0,
			wantPage:     "",
			wantLimit:    "",
			wantTags:     "deployment:dep-4",
		},
		{
			name:         "timestamps included when non-empty",
			deploymentID: "dep-5",
			startTime:    "2026-01-01T00:00:00Z",
			endTime:      "2026-01-02T00:00:00Z",
			limit:        10,
			wantLimit:    "10",
			wantTags:     "deployment:dep-5",
			wantFrom:     "2026-01-01T00:00:00Z",
			wantTo:       "2026-01-02T00:00:00Z",
		},
		{
			name:         "timestamps excluded when empty",
			deploymentID: "dep-6",
			limit:        25,
			wantLimit:    "25",
			wantTags:     "deployment:dep-6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotQuery map[string][]string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.Query()
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(TracesResponse{})
			}))
			defer srv.Close()

			c := NewClient(srv.URL, "pk", "sk")
			_, err := c.GetTraces(context.Background(), tt.deploymentID, tt.startTime, tt.endTime, tt.limit, tt.offset)
			if err != nil {
				t.Fatalf("GetTraces returned error: %v", err)
			}

			assertParam(t, gotQuery, "tags", tt.wantTags)
			assertParam(t, gotQuery, "page", tt.wantPage)
			assertParam(t, gotQuery, "limit", tt.wantLimit)
			assertParam(t, gotQuery, "fromTimestamp", tt.wantFrom)
			assertParam(t, gotQuery, "toTimestamp", tt.wantTo)
		})
	}
}

func TestGetDailyMetrics_QueryParams(t *testing.T) {
	tests := []struct {
		name         string
		deploymentID string
		startTime    string
		endTime      string
		wantTags     string
		wantFrom     string
		wantTo       string
	}{
		{
			name:     "empty deploymentID skips tags",
			wantTags: "",
		},
		{
			name:         "non-empty deploymentID sets tags",
			deploymentID: "dep-abc",
			wantTags:     "deployment:dep-abc",
		},
		{
			name:         "timestamps optional",
			deploymentID: "dep-abc",
			startTime:    "2026-03-01T00:00:00Z",
			wantTags:     "deployment:dep-abc",
			wantFrom:     "2026-03-01T00:00:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotQuery map[string][]string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.Query()
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(DailyMetricsResponse{})
			}))
			defer srv.Close()

			c := NewClient(srv.URL, "pk", "sk")
			_, err := c.GetDailyMetrics(context.Background(), tt.deploymentID, tt.startTime, tt.endTime)
			if err != nil {
				t.Fatalf("GetDailyMetrics returned error: %v", err)
			}

			assertParam(t, gotQuery, "tags", tt.wantTags)
			assertParam(t, gotQuery, "fromTimestamp", tt.wantFrom)
			assertParam(t, gotQuery, "toTimestamp", tt.wantTo)
		})
	}
}

func TestDoGet_BasicAuth(t *testing.T) {
	pk, sk := "my-public-key", "my-secret-key"
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(pk+":"+sk))

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TracesResponse{})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, pk, sk)
	_, err := c.GetTraces(context.Background(), "dep", "", "", 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != wantAuth {
		t.Errorf("Authorization = %q, want %q", gotAuth, wantAuth)
	}
}

func TestDoGet_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "pk", "sk")
	_, err := c.GetTraces(context.Background(), "dep", "", "", 50, 0)
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not contain 403", err)
	}
}

func TestDoGet_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "pk", "sk")
	_, err := c.GetTraces(context.Background(), "dep", "", "", 50, 0)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error %q does not mention decode", err)
	}
}

func TestDailyMetric_InputOutputTokens(t *testing.T) {
	tests := []struct {
		name       string
		usage      []DailyMetricUsage
		wantInput  int
		wantOutput int
	}{
		{"empty usage", nil, 0, 0},
		{
			"single model",
			[]DailyMetricUsage{{Model: "gpt-4", InputUsage: 100, OutputUsage: 200}},
			100, 200,
		},
		{
			"multiple models summed",
			[]DailyMetricUsage{
				{Model: "gpt-4", InputUsage: 100, OutputUsage: 200},
				{Model: "claude-3", InputUsage: 300, OutputUsage: 400},
				{Model: "gemini", InputUsage: 50, OutputUsage: 75},
			},
			450, 675,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := DailyMetric{Usage: tt.usage}
			if got := m.InputTokens(); got != tt.wantInput {
				t.Errorf("InputTokens() = %d, want %d", got, tt.wantInput)
			}
			if got := m.OutputTokens(); got != tt.wantOutput {
				t.Errorf("OutputTokens() = %d, want %d", got, tt.wantOutput)
			}
		})
	}
}

func TestGetDailyMetrics_Pagination(t *testing.T) {
	page1 := DailyMetricsResponse{
		Data: []DailyMetric{{Date: "2026-04-01", CountTraces: 10}},
	}
	page1.Meta.Page = 1
	page1.Meta.Limit = 1
	page1.Meta.TotalItems = 2
	page1.Meta.TotalPages = 2

	page2 := DailyMetricsResponse{
		Data: []DailyMetric{{Date: "2026-04-02", CountTraces: 20}},
	}
	page2.Meta.Page = 2
	page2.Meta.Limit = 1
	page2.Meta.TotalItems = 2
	page2.Meta.TotalPages = 2

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			json.NewEncoder(w).Encode(page1)
		} else {
			json.NewEncoder(w).Encode(page2)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "pk", "sk")
	metrics, err := c.GetDailyMetrics(context.Background(), "", "", "")
	if err != nil {
		t.Fatalf("GetDailyMetrics returned error: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls (one per page), got %d", callCount)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}
	if metrics[0].Date != "2026-04-01" {
		t.Errorf("metrics[0].Date = %q, want 2026-04-01", metrics[0].Date)
	}
	if metrics[1].Date != "2026-04-02" {
		t.Errorf("metrics[1].Date = %q, want 2026-04-02", metrics[1].Date)
	}
}

func TestGetDailyMetrics_SinglePage(t *testing.T) {
	resp := DailyMetricsResponse{
		Data: []DailyMetric{{Date: "2026-04-01", CountTraces: 5}},
	}
	resp.Meta.Page = 1
	resp.Meta.TotalPages = 1

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "pk", "sk")
	metrics, err := c.GetDailyMetrics(context.Background(), "dep-1", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call, got %d", callCount)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
}

func assertParam(t *testing.T, query map[string][]string, key, want string) {
	t.Helper()
	vals, exists := query[key]
	if want == "" {
		if exists {
			t.Errorf("param %q should be absent, got %q", key, vals)
		}
		return
	}
	if !exists {
		t.Errorf("param %q not set, want %q", key, want)
		return
	}
	if vals[0] != want {
		t.Errorf("param %q = %q, want %q", key, vals[0], want)
	}
}
