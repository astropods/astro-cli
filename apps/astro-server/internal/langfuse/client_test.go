package langfuse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
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

			c := newV3Client(srv.URL)
			_, err := c.GetTraces(context.Background(), tt.deploymentID, tt.startTime, tt.endTime, tt.limit, tt.offset)
			if err != nil {
				t.Fatalf("GetTraces returned error: %v", err)
			}

			assertParam(t, gotQuery, "tags", tt.wantTags)
			assertParam(t, gotQuery, "page", tt.wantPage)
			assertParam(t, gotQuery, "limit", tt.wantLimit)
			assertParam(t, gotQuery, "fromTimestamp", tt.wantFrom)
			assertParam(t, gotQuery, "toTimestamp", tt.wantTo)
			assertParam(t, gotQuery, "fields", "core,metrics")
			assertParam(t, gotQuery, "orderBy", "")
		})
	}
}

func TestGetSessionTraces_QueryParams(t *testing.T) {
	var gotQuery map[string][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TracesResponse{})
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
	_, err := c.GetSessionTraces(
		context.Background(),
		"dep-1",
		"user-1",
		"conv-1",
		50,
		"timestamp.desc",
	)
	if err != nil {
		t.Fatalf("GetSessionTraces returned error: %v", err)
	}

	assertParam(t, gotQuery, "tags", "deployment:dep-1")
	assertParam(t, gotQuery, "userId", "user-1")
	assertParam(t, gotQuery, "sessionId", "conv-1")
	assertParam(t, gotQuery, "limit", "50")
	assertParam(t, gotQuery, "fields", "core,io")
	assertParam(t, gotQuery, "orderBy", "timestamp.desc")
}

func TestGetNextSessionTrace(t *testing.T) {
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_ = json.NewEncoder(w).Encode(TracesResponse{Data: []Trace{
			{ID: "target", Timestamp: "2026-07-27T12:00:00Z"},
			{ID: "same-time", Timestamp: "2026-07-27T12:00:00Z", Input: "follow-up"},
			{ID: "next", Timestamp: "2026-07-27T12:00:01Z", Input: "follow-up"},
		}})
	}))
	defer srv.Close()

	got, err := newV3Client(srv.URL).GetNextSessionTrace(
		context.Background(), "dep-1", "user-1", "session-1", "target", "2026-07-27T12:00:00Z",
	)
	if err != nil {
		t.Fatalf("GetNextSessionTrace: %v", err)
	}
	if got == nil || got.ID != "same-time" {
		t.Fatalf("GetNextSessionTrace = %+v", got)
	}
	if query.Get("tags") != "deployment:dep-1" ||
		query.Get("userId") != "user-1" ||
		query.Get("sessionId") != "session-1" ||
		query.Get("fromTimestamp") != "2026-07-27T12:00:00Z" ||
		query.Get("orderBy") != "timestamp.asc" ||
		query.Get("fields") != "core,io" ||
		query.Get("limit") != "10" {
		t.Fatalf("query = %v", query)
	}
}

func TestGetPreviousSessionTraces(t *testing.T) {
	var query url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		_ = json.NewEncoder(w).Encode(TracesResponse{Data: []Trace{
			{ID: "target", Timestamp: "2026-07-27T12:00:03Z"},
			{ID: "previous-2", Timestamp: "2026-07-27T12:00:02Z"},
			{ID: "previous-1", Timestamp: "2026-07-27T12:00:01Z"},
		}})
	}))
	defer srv.Close()

	got, err := newV3Client(srv.URL).GetPreviousSessionTraces(
		context.Background(), "dep-1", "user-1", "session-1", "target", "2026-07-27T12:00:03Z", 2,
	)
	if err != nil {
		t.Fatalf("GetPreviousSessionTraces: %v", err)
	}
	if len(got) != 2 || got[0].ID != "previous-1" || got[1].ID != "previous-2" {
		t.Fatalf("GetPreviousSessionTraces = %+v", got)
	}
	if query.Get("tags") != "deployment:dep-1" ||
		query.Get("userId") != "user-1" ||
		query.Get("sessionId") != "session-1" ||
		query.Get("toTimestamp") != "2026-07-27T12:00:03Z" ||
		query.Get("orderBy") != "timestamp.desc" ||
		query.Get("fields") != "core,io" ||
		query.Get("limit") != "3" {
		t.Fatalf("query = %v", query)
	}
}

func TestGetNextSessionTraceOmitsIncompleteContext(t *testing.T) {
	client := newV3Client("http://unused")
	got, err := client.GetNextSessionTrace(context.Background(), "dep-1", "", "session-1", "target", "2026-07-27T12:00:00Z")
	if err != nil || got != nil {
		t.Fatalf("GetNextSessionTrace = %+v, %v", got, err)
	}
}

func TestGetUserTracesOrdered_QueryParams(t *testing.T) {
	var gotQuery map[string][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TracesResponse{})
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
	_, err := c.GetUserTracesOrdered(
		context.Background(),
		"dep-1",
		"user-1",
		"2026-01-01T00:00:00Z",
		"2026-01-02T00:00:00Z",
		100,
		100,
		"timestamp.asc",
	)
	if err != nil {
		t.Fatalf("GetUserTracesOrdered returned error: %v", err)
	}

	assertParam(t, gotQuery, "tags", "deployment:dep-1")
	assertParam(t, gotQuery, "userId", "user-1")
	assertParam(t, gotQuery, "fromTimestamp", "2026-01-01T00:00:00Z")
	assertParam(t, gotQuery, "toTimestamp", "2026-01-02T00:00:00Z")
	assertParam(t, gotQuery, "limit", "100")
	assertParam(t, gotQuery, "page", "2")
	assertParam(t, gotQuery, "fields", "core,metrics")
	assertParam(t, gotQuery, "orderBy", "timestamp.asc")
}

func TestGetTracesFilteredOrdered_QueryParams(t *testing.T) {
	var gotQuery map[string][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TracesResponse{})
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
	_, err := c.GetTracesFilteredOrdered(
		context.Background(),
		"dep-1",
		"2026-01-01T00:00:00Z",
		"2026-01-02T00:00:00Z",
		[]TraceFilter{
			{Type: "arrayOptions", Column: "tags", Operator: "all of", Value: []string{"deployment:dep-1"}},
			{Type: "null", Column: "userId", Operator: "is null"},
		},
		"core,metrics",
		100,
		100,
		"timestamp.desc",
	)
	if err != nil {
		t.Fatalf("GetTracesFilteredOrdered returned error: %v", err)
	}

	assertParam(t, gotQuery, "page", "2")
	assertParam(t, gotQuery, "fields", "core,metrics")
	assertParam(t, gotQuery, "orderBy", "timestamp.desc")
	assertParam(t, gotQuery, "filter", `[{"type":"arrayOptions","column":"tags","operator":"all of","value":["deployment:dep-1"]},{"type":"null","column":"userId","operator":"is null"}]`)
}

func TestGetTracesFilteredOrdered_CustomFields(t *testing.T) {
	var gotQuery map[string][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TracesResponse{})
	}))
	defer srv.Close()

	_, err := newV3Client(srv.URL).GetTracesFilteredOrdered(
		context.Background(),
		"dep-1",
		"",
		"",
		[]TraceFilter{{
			Type:     "stringOptions",
			Column:   "id",
			Operator: "any of",
			Value:    []string{"trace-1", "trace-2"},
		}},
		"core",
		2,
		0,
		"",
	)
	if err != nil {
		t.Fatalf("GetTracesFilteredOrdered returned error: %v", err)
	}

	assertParam(t, gotQuery, "limit", "2")
	assertParam(t, gotQuery, "fields", "core")
	assertParam(t, gotQuery, "filter", `[{"type":"stringOptions","column":"id","operator":"any of","value":["trace-1","trace-2"]}]`)
	assertParam(t, gotQuery, "tags", "deployment:dep-1")
}

func TestGetQueueTraces_QueryParams(t *testing.T) {
	var gotQuery map[string][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TracesResponse{})
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
	_, err := c.GetQueueTraces(
		context.Background(),
		"dep-1",
		"2025-12-26T00:00:00Z",
		"2026-01-02T00:00:00Z",
		50,
		50,
	)
	if err != nil {
		t.Fatalf("GetQueueTraces returned error: %v", err)
	}

	assertParam(t, gotQuery, "tags", "deployment:dep-1")
	assertParam(t, gotQuery, "fromTimestamp", "2025-12-26T00:00:00Z")
	assertParam(t, gotQuery, "toTimestamp", "2026-01-02T00:00:00Z")
	assertParam(t, gotQuery, "limit", "50")
	assertParam(t, gotQuery, "page", "2")
	assertParam(t, gotQuery, "fields", "core,io")
	assertParam(t, gotQuery, "orderBy", "timestamp.desc")
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

			c := newV3Client(srv.URL)
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
	c.reader = &v3Reader{transport: c.transport}
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

	c := newV3Client(srv.URL)
	_, err := c.GetTraces(context.Background(), "dep", "", "", 50, 0)
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not contain 403", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %q is not APIError", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusForbidden)
	}
}

func TestUpsertDatasetItem_Non2xxStatusIsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"message":"Dataset item validation failed"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "pk", "sk")
	err := c.UpsertDatasetItem(context.Background(), DatasetItemInput{
		DatasetName: "dataset",
		Input:       map[string]any{"prompt": "hello"},
	})
	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %q is not APIError", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
}

func TestDeleteDatasetItem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.EscapedPath() != "/api/public/dataset-items/item%2F1" {
			t.Errorf("path = %q, want escaped dataset item id", r.URL.EscapedPath())
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "pk", "sk")
	if err := c.DeleteDatasetItem(context.Background(), "item/1"); err != nil {
		t.Fatalf("DeleteDatasetItem: %v", err)
	}
}

func TestDeleteDatasetItem_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "pk", "sk")
	err := c.DeleteDatasetItem(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteDatasetItem error = %v, want ErrNotFound", err)
	}
}

func TestDoGet_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{not json`))
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
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

	c := newV3Client(srv.URL)
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

	c := newV3Client(srv.URL)
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

func TestGetTrace_TwoParallelRequests(t *testing.T) {
	type call struct{ fields string }
	var mu sync.Mutex
	var calls []call

	traceIO := TraceDetail{}
	traceIO.Input = "hello"
	traceIO.Output = "world"
	traceIO.Scores = []Score{{ID: "s1", Name: "quality", Value: 1}}

	treeSkeleton := TraceDetail{}
	treeSkeleton.Observations = []Observation{{ID: "obs-1", Name: "span"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields := r.URL.Query().Get("fields")
		mu.Lock()
		calls = append(calls, call{fields: fields})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if fields == "core,io,scores,metrics" {
			json.NewEncoder(w).Encode(traceIO)
		} else {
			json.NewEncoder(w).Encode(treeSkeleton)
		}
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
	got, err := c.GetTrace(context.Background(), "trace-abc")
	if err != nil {
		t.Fatalf("GetTrace returned error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(calls))
	}
	fieldsSent := map[string]bool{}
	for _, call := range calls {
		fieldsSent[call.fields] = true
	}
	if !fieldsSent["core,io,scores,metrics"] {
		t.Error("expected a request with fields=core,io,scores,metrics")
	}
	if !fieldsSent["core,observations"] {
		t.Error("expected a request with fields=core,observations")
	}
	if got.Input != "hello" {
		t.Errorf("Input = %v, want hello", got.Input)
	}
	if len(got.Scores) != 1 || got.Scores[0].ID != "s1" {
		t.Errorf("Scores not merged correctly: %+v", got.Scores)
	}
	if len(got.Observations) != 1 || got.Observations[0].ID != "obs-1" {
		t.Errorf("Observations not merged correctly: %+v", got.Observations)
	}
}

func TestGetTrace_ErrNotFoundPreferredOverContextCanceled(t *testing.T) {
	// One branch returns 404 (ErrNotFound); the other is slow and gets cancelled
	// by errgroup when the first fails. GetTrace must surface ErrNotFound so the
	// handler can return 404 rather than 502.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fields := r.URL.Query().Get("fields")
		if fields == "core,io,scores,metrics" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		} else {
			// Simulate a slow response; the context will be cancelled before this
			// goroutine finishes, so it returns context.Canceled instead of a real error.
			<-r.Context().Done()
			http.Error(w, "cancelled", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
	_, err := c.GetTrace(context.Background(), "trace-abc")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetTraceCore_SendsFieldsCore(t *testing.T) {
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TraceDetail{
			Trace: Trace{Tags: []string{"deployment:dep-1"}},
		})
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
	got, err := c.GetTraceCore(context.Background(), "trace-abc")
	if err != nil {
		t.Fatalf("GetTraceCore returned error: %v", err)
	}

	assertParam(t, gotQuery, "fields", "core")
	if len(got.Tags) != 1 || got.Tags[0] != "deployment:dep-1" {
		t.Errorf("Tags = %v, want [deployment:dep-1]", got.Tags)
	}
}

func TestGetObservation_PathAndResponse(t *testing.T) {
	want := Observation{
		ID:                  "obs-1",
		Type:                "GENERATION",
		Name:                "llm-call",
		Latency:             1.5,
		CalculatedTotalCost: 0.002,
		Input:               map[string]any{"prompt": "hello"},
		Output:              map[string]any{"completion": "world"},
	}

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
	got, err := c.GetObservation(context.Background(), "obs-1")
	if err != nil {
		t.Fatalf("GetObservation returned error: %v", err)
	}

	if gotPath != "/api/public/observations/obs-1" {
		t.Errorf("path = %q, want /api/public/observations/obs-1", gotPath)
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.CalculatedTotalCost != want.CalculatedTotalCost {
		t.Errorf("CalculatedTotalCost = %v, want %v", got.CalculatedTotalCost, want.CalculatedTotalCost)
	}
}

func TestGetObservation_404IsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := newV3Client(srv.URL)
	_, err := c.GetObservation(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention not found", err)
	}
}

// ============================================================================
// v4Reader — the v4 read path speaks a different wire contract to v3, so it
// gets its own assertions rather than sharing the v3 cases above.
// ============================================================================

// decodeFilters unpacks the JSON filter= array the v4 endpoints take in place
// of v3's dedicated query params.
func decodeFilters(t *testing.T, query url.Values) []TraceFilter {
	t.Helper()
	raw := query.Get("filter")
	if raw == "" {
		t.Fatal("filter param not set")
	}
	var got []TraceFilter
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal filter %q: %v", raw, err)
	}
	return got
}

func findFilter(filters []TraceFilter, column string) *TraceFilter {
	for i, f := range filters {
		if f.Column == column {
			return &filters[i]
		}
	}
	return nil
}

func assertFilter(t *testing.T, filters []TraceFilter, column string, want []string) {
	t.Helper()
	f := findFilter(filters, column)
	if f == nil {
		t.Errorf("filter on %q not set, want %v", column, want)
		return
	}
	vals, ok := f.Value.([]any)
	if !ok {
		t.Errorf("filter %q value = %#v, want a list", column, f.Value)
		return
	}
	if len(vals) != len(want) {
		t.Errorf("filter %q = %v, want %v", column, vals, want)
		return
	}
	for i, v := range vals {
		if v != want[i] {
			t.Errorf("filter %q[%d] = %v, want %v", column, i, v, want[i])
		}
	}
}

func TestV4GetTraces_UsesFilterArrayNotV3Params(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.Query()
		_ = json.NewEncoder(w).Encode(observationsV2Response{})
	}))
	defer srv.Close()

	_, err := newV4Client(srv.URL).GetUserTracesOrdered(
		context.Background(), "dep-1", "user-1",
		"2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", 25, 0, "",
	)
	if err != nil {
		t.Fatalf("GetUserTracesOrdered: %v", err)
	}

	if gotPath != "/api/public/v2/observations" {
		t.Errorf("path = %q, want /api/public/v2/observations", gotPath)
	}
	filters := decodeFilters(t, gotQuery)
	assertFilter(t, filters, "tags", []string{"deployment:dep-1"})
	assertFilter(t, filters, "userId", []string{"user-1"})

	assertParam(t, gotQuery, "isRootObservation", "true")
	assertParam(t, gotQuery, "fromStartTime", "2026-01-01T00:00:00Z")
	assertParam(t, gotQuery, "toStartTime", "2026-01-02T00:00:00Z")
	assertParam(t, gotQuery, "limit", "25")

	// v2/observations silently ignores these rather than erroring, so sending
	// them instead of the filter array would leak other deployments' traces.
	for _, stale := range []string{"tags", "userId", "fromTimestamp", "toTimestamp", "page"} {
		assertParam(t, gotQuery, stale, "")
	}
}

func TestV4GetTraces_FieldsRequestIOOnlyWhenAsked(t *testing.T) {
	tests := []struct {
		name      string
		v3Fields  string
		wantField string
	}{
		{"metrics only omits io", "core,metrics", "basic"},
		{"io requested adds io", "core,io", "basic,io"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotQuery url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.Query()
				_ = json.NewEncoder(w).Encode(observationsV2Response{})
			}))
			defer srv.Close()

			_, err := newV4Client(srv.URL).GetTracesFilteredOrdered(
				context.Background(), "dep-1", "", "", nil, tt.v3Fields, 10, 0, "",
			)
			if err != nil {
				t.Fatalf("GetTracesFilteredOrdered: %v", err)
			}
			assertParam(t, gotQuery, "fields", tt.wantField)
		})
	}
}

func TestV4GetTraces_SortsClientSide(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{
			{ID: "middle", StartTime: "2026-07-27T12:00:01Z"},
			{ID: "oldest", StartTime: "2026-07-27T12:00:00Z"},
			{ID: "newest", StartTime: "2026-07-27T12:00:02Z"},
		}})
	}))
	defer srv.Close()

	// v2/observations accepts orderBy and ignores it, so the reader has to
	// impose the caller's order itself.
	for _, tt := range []struct {
		orderBy string
		want    []string
	}{
		{"timestamp.desc", []string{"newest", "middle", "oldest"}},
		{"timestamp.asc", []string{"oldest", "middle", "newest"}},
	} {
		t.Run(tt.orderBy, func(t *testing.T) {
			got, err := newV4Client(srv.URL).GetTracesOrdered(
				context.Background(), "dep-1", "", "", 10, 0, tt.orderBy,
			)
			if err != nil {
				t.Fatalf("GetTracesOrdered: %v", err)
			}
			if len(got.Data) != len(tt.want) {
				t.Fatalf("got %d traces, want %d", len(got.Data), len(tt.want))
			}
			for i, id := range tt.want {
				if got.Data[i].ID != id {
					t.Errorf("Data[%d].ID = %q, want %q", i, got.Data[i].ID, id)
				}
			}
		})
	}
}

func TestV4GetTraces_WalksCursorForOffset(t *testing.T) {
	var cursors []string
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		cursors = append(cursors, r.URL.Query().Get("cursor"))
		_ = json.NewEncoder(w).Encode(observationsV2Response{
			Data: []ObservationV2{{ID: fmt.Sprintf("obs-%d", calls)}},
			Meta: struct {
				Cursor string `json:"cursor"`
			}{Cursor: fmt.Sprintf("cursor-%d", calls)},
		})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetTraces(context.Background(), "dep-1", "", "", 10, 20)
	if err != nil {
		t.Fatalf("GetTraces: %v", err)
	}
	// offset 20 / limit 10 = two walk requests, then the real page.
	if calls != 3 {
		t.Errorf("HTTP calls = %d, want 3", calls)
	}
	if want := []string{"", "cursor-1", "cursor-2"}; !equalStrings(cursors, want) {
		t.Errorf("cursors = %v, want %v", cursors, want)
	}
	if len(got.Data) != 1 || got.Data[0].ID != "obs-3" {
		t.Errorf("Data = %+v, want the third page", got.Data)
	}
}

func TestV4GetTraces_OffsetPastEndReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{{ID: "obs-1"}}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetTraces(context.Background(), "dep-1", "", "", 10, 100)
	if err != nil {
		t.Fatalf("GetTraces: %v", err)
	}
	if len(got.Data) != 0 {
		t.Errorf("Data = %+v, want empty", got.Data)
	}
}

func TestV4GetObservation_FiltersByIDAndRequestsIO(t *testing.T) {
	var gotPath string
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.Query()
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{{
			ID: "obs-1", TraceID: "trace-1", Type: "GENERATION", Name: "llm-call",
			Latency: 1.5, Input: "hello", Output: "world",
		}}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetObservation(context.Background(), "obs-1")
	if err != nil {
		t.Fatalf("GetObservation: %v", err)
	}

	if gotPath != "/api/public/v2/observations" {
		t.Errorf("path = %q, want /api/public/v2/observations", gotPath)
	}
	assertFilter(t, decodeFilters(t, gotQuery), "id", []string{"obs-1"})
	assertParam(t, gotQuery, "fields", "basic,io")
	if got.ID != "obs-1" || got.TraceID != "trace-1" || got.Output != "world" {
		t.Errorf("Observation = %+v", got)
	}
}

func TestV4EmptyDataIsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(observationsV2Response{})
	}))
	defer srv.Close()

	c := newV4Client(srv.URL)
	// v2/observations answers a miss with 200 and an empty list, not 404, so
	// each read has to map that to ErrNotFound itself.
	tests := map[string]func() error{
		"GetObservation": func() error { _, err := c.GetObservation(context.Background(), "missing"); return err },
		"GetTraceCore":   func() error { _, err := c.GetTraceCore(context.Background(), "missing"); return err },
		"GetTrace":       func() error { _, err := c.GetTrace(context.Background(), "missing"); return err },
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestV4GetTrace_MergesSpansAndScores(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/v3/scores" {
			if got := r.URL.Query().Get("traceId"); got != "trace-1" {
				t.Errorf("scores traceId = %q, want trace-1", got)
			}
			_ = json.NewEncoder(w).Encode(v3ScoresResponse{Data: []Score{{ID: "score-1", Name: "quality", Value: 0.9}}})
			return
		}
		filters := decodeFilters(t, r.URL.Query())
		if findFilter(filters, "traceId") != nil {
			_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{
				{ID: "root", TraceID: "trace-1", Name: "agent-run"},
				{ID: "child", TraceID: "trace-1", ParentObsID: "root", Type: "GENERATION", Name: "llm-call"},
			}})
			return
		}
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{
			{ID: "root", TraceID: "trace-1", Name: "agent-run", UserID: "user-1", Input: "hi", Output: "there"},
		}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetTrace(context.Background(), "root")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if got.ID != "root" || got.UserID != "user-1" || got.Output != "there" {
		t.Errorf("trace = %+v", got.Trace)
	}
	// The root is the trace itself, so it must not also appear as its own span.
	if len(got.Observations) != 1 || got.Observations[0].ID != "child" {
		t.Errorf("Observations = %+v, want only the child span", got.Observations)
	}
	if len(got.Scores) != 1 || got.Scores[0].Name != "quality" {
		t.Errorf("Scores = %+v", got.Scores)
	}
}

func TestV4GetMetrics_RewritesTracesView(t *testing.T) {
	tests := []struct {
		name         string
		metrics      []MetricsQueryField
		wantRootOnly bool
	}{
		{
			name:         "count-family measures scope to root observations",
			metrics:      []MetricsQueryField{{Measure: "count", Aggregation: "count"}},
			wantRootOnly: true,
		},
		{
			// Root-scoping zeroes these out: usage lives on child spans.
			name:         "cost measures stay unfiltered",
			metrics:      []MetricsQueryField{{Measure: "totalCost", Aggregation: "sum"}},
			wantRootOnly: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			var gotQuery url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery = r.URL.Path, r.URL.Query()
				_ = json.NewEncoder(w).Encode(MetricsResponse{})
			}))
			defer srv.Close()

			_, err := newV4Client(srv.URL).GetMetrics(context.Background(), MetricsQuery{
				View: "traces", Metrics: tt.metrics,
			})
			if err != nil {
				t.Fatalf("GetMetrics: %v", err)
			}

			if gotPath != "/api/public/v2/metrics" {
				t.Errorf("path = %q, want /api/public/v2/metrics", gotPath)
			}
			var sent MetricsQuery
			if err := json.Unmarshal([]byte(gotQuery.Get("query")), &sent); err != nil {
				t.Fatalf("unmarshal query: %v", err)
			}
			// v4 removed view:"traces" entirely.
			if sent.View != "observations" {
				t.Errorf("view = %q, want observations", sent.View)
			}
			var hasRootFilter bool
			for _, f := range sent.Filters {
				if f.Column == "isRootObservation" {
					hasRootFilter = true
				}
			}
			if hasRootFilter != tt.wantRootOnly {
				t.Errorf("isRootObservation filter present = %v, want %v", hasRootFilter, tt.wantRootOnly)
			}
		})
	}
}

func TestV4GetMetrics_AutoFillsHighCardinalityGrouping(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(MetricsResponse{})
	}))
	defer srv.Close()

	_, err := newV4Client(srv.URL).GetMetrics(context.Background(), MetricsQuery{
		View:       "observations",
		Metrics:    []MetricsQueryField{{Measure: "totalCost", Aggregation: "sum"}},
		Dimensions: []MetricsDimension{{Field: "userId"}},
	})
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}

	var sent MetricsQuery
	if err := json.Unmarshal([]byte(gotQuery.Get("query")), &sent); err != nil {
		t.Fatalf("unmarshal query: %v", err)
	}
	// v4 rejects a userId grouping unless both are supplied.
	if len(sent.OrderBy) != 1 || sent.OrderBy[0].Field != "sum_totalCost" || sent.OrderBy[0].Direction != "desc" {
		t.Errorf("OrderBy = %+v, want sum_totalCost desc", sent.OrderBy)
	}
	if sent.Config == nil || sent.Config.RowLimit != defaultHighCardinalityRowLimit {
		t.Errorf("Config = %+v, want row_limit %d", sent.Config, defaultHighCardinalityRowLimit)
	}
}

func TestV4GetMetrics_KeepsCallerOrderAndLimit(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(MetricsResponse{})
	}))
	defer srv.Close()

	_, err := newV4Client(srv.URL).GetMetrics(context.Background(), MetricsQuery{
		View:       "observations",
		Metrics:    []MetricsQueryField{{Measure: "totalCost", Aggregation: "sum"}},
		Dimensions: []MetricsDimension{{Field: "userId"}},
		OrderBy:    []MetricsOrderBy{{Field: "sum_totalCost", Direction: "asc"}},
		Config:     &MetricsConfig{RowLimit: 10},
	})
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}

	var sent MetricsQuery
	if err := json.Unmarshal([]byte(gotQuery.Get("query")), &sent); err != nil {
		t.Fatalf("unmarshal query: %v", err)
	}
	if sent.OrderBy[0].Direction != "asc" || sent.Config.RowLimit != 10 {
		t.Errorf("caller order/limit overridden: OrderBy = %+v, Config = %+v", sent.OrderBy, sent.Config)
	}
}

func TestV4GetDailyMetrics_MergesCountAndUsageByDate(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()

		var sent MetricsQuery
		if err := json.Unmarshal([]byte(r.URL.Query().Get("query")), &sent); err != nil {
			t.Errorf("unmarshal query: %v", err)
			return
		}
		if len(sent.Dimensions) == 0 {
			_ = json.NewEncoder(w).Encode(MetricsResponse{Data: []map[string]any{
				{"time_dimension": "2026-03-02", "count_count": float64(5)},
				{"time_dimension": "2026-03-01", "count_count": float64(2)},
			}})
			return
		}
		_ = json.NewEncoder(w).Encode(MetricsResponse{Data: []map[string]any{
			{"time_dimension": "2026-03-01", "providedModelName": "gpt-4o-mini",
				"sum_inputTokens": float64(100), "sum_outputTokens": float64(50),
				"sum_totalTokens": float64(150), "sum_totalCost": 0.25},
		}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetDailyMetrics(context.Background(), "dep-1", "2026-03-01T00:00:00Z", "2026-03-03T00:00:00Z")
	if err != nil {
		t.Fatalf("GetDailyMetrics: %v", err)
	}

	if len(paths) != 2 {
		t.Errorf("HTTP calls = %d, want 2", len(paths))
	}
	if len(got) != 2 {
		t.Fatalf("got %d days, want 2: %+v", len(got), got)
	}
	if got[0].Date != "2026-03-01" || got[1].Date != "2026-03-02" {
		t.Errorf("dates = %q, %q, want ascending", got[0].Date, got[1].Date)
	}
	if got[0].CountTraces != 2 || got[0].TotalCost != 0.25 {
		t.Errorf("day one = %+v", got[0])
	}
	if got[0].InputTokens() != 100 || got[0].OutputTokens() != 50 {
		t.Errorf("day one tokens = %d in, %d out", got[0].InputTokens(), got[0].OutputTokens())
	}
	if got[1].CountTraces != 5 || len(got[1].Usage) != 0 {
		t.Errorf("day two = %+v, want count only", got[1])
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// newV3Client and newV4Client pin the read path under test. NewClient selects
// its reader from LANGFUSE_USE_V4_API, and the two readers speak deliberately
// different wire contracts, so a query-shape assertion reached through
// NewClient would assert one version's shape against whichever reader the
// ambient environment happened to select.
func newV3Client(baseURL string) *Client {
	c := NewClient(baseURL, "pk", "sk")
	c.reader = &v3Reader{transport: c.transport}
	return c
}

func newV4Client(baseURL string) *Client {
	c := NewClient(baseURL, "pk", "sk")
	c.reader = &v4Reader{transport: c.transport}
	return c
}

func TestNewClient_SelectsReaderFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want traceReader
	}{
		{"empty defaults to v4", "", &v4Reader{}},
		{"true", "true", &v4Reader{}},
		{"1", "1", &v4Reader{}},
		{"false opts out", "false", &v3Reader{}},
		{"0 opts out", "0", &v3Reader{}},
		{"FALSE opts out", "FALSE", &v3Reader{}},
		// A legacy-mode environment opts out explicitly, so an unparseable
		// value must not be what silently decides the read path for it.
		{"unparseable defaults to v4", "yes-please", &v4Reader{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LANGFUSE_USE_V4_API", tt.env)
			got := NewClient("http://unused", "pk", "sk").reader
			if fmt.Sprintf("%T", got) != fmt.Sprintf("%T", tt.want) {
				t.Errorf("reader = %T, want %T", got, tt.want)
			}
		})
	}

	t.Run("absent defaults to v4", func(t *testing.T) {
		t.Setenv("LANGFUSE_USE_V4_API", "") // registers the cleanup that restores it
		os.Unsetenv("LANGFUSE_USE_V4_API")
		if got := NewClient("http://unused", "pk", "sk").reader; fmt.Sprintf("%T", got) != "*langfuse.v4Reader" {
			t.Errorf("reader = %T, want *langfuse.v4Reader", got)
		}
	})
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
