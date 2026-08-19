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

	// The root filter has to ride in filter=. As a dedicated query param
	// isRootObservation is silently ignored, which lists every child span.
	assertParam(t, gotQuery, "isRootObservation", "")
	if root := findFilter(filters, "isRootObservation"); root == nil {
		t.Error("root filter not set, so every child span is listed as a trace")
	} else if root.Type != "boolean" || root.Operator != "=" || root.Value != true {
		t.Errorf("root filter = %+v, want boolean isRootObservation = true", *root)
	}
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
		{"metrics only omits io", "core,metrics", v4ObservationFields},
		{"io requested adds io", "core,io", v4ObservationFields + ",io"},
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
			{ID: "span-middle", TraceID: "middle", StartTime: "2026-07-27T12:00:01Z"},
			{ID: "span-oldest", TraceID: "oldest", StartTime: "2026-07-27T12:00:00Z"},
			{ID: "span-newest", TraceID: "newest", StartTime: "2026-07-27T12:00:02Z"},
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
		// The listing also totals cost through /v2/metrics; only the
		// observation requests are the cursor walk under test here.
		if strings.Contains(r.URL.Path, "/metrics") {
			_ = json.NewEncoder(w).Encode(MetricsResponse{})
			return
		}
		calls++
		cursors = append(cursors, r.URL.Query().Get("cursor"))
		_ = json.NewEncoder(w).Encode(observationsV2Response{
			Data: []ObservationV2{{ID: fmt.Sprintf("span-%d", calls), TraceID: fmt.Sprintf("obs-%d", calls)}},
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
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{{ID: "span-1", TraceID: "obs-1"}}})
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
	assertParam(t, gotQuery, "fields", v4ObservationFields+",io")
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

func TestV4GetTraces_KeyedByTraceIDWithTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{{
			ID: "root-span-id", TraceID: "trace-1", Name: "root-span", TraceName: "agent-run",
			StartTime: "2026-07-27T12:00:00Z", Tags: []string{"deployment:dep-1"},
		}}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetTraces(context.Background(), "dep-1", "", "", 10, 0)
	if err != nil {
		t.Fatalf("GetTraces: %v", err)
	}
	if len(got.Data) != 1 {
		t.Fatalf("got %d traces, want 1", len(got.Data))
	}
	// The ID has to be the trace ID: callers round-trip it into GetTrace, and
	// traceId is what the v4 filter contract can look a trace up by.
	if got.Data[0].ID != "trace-1" {
		t.Errorf("ID = %q, want trace-1", got.Data[0].ID)
	}
	if got.Data[0].CreatedAt != "2026-07-27T12:00:00Z" {
		t.Errorf("CreatedAt = %q, want the start time", got.Data[0].CreatedAt)
	}
	if !HasDeploymentTag(got.Data[0].Tags, "dep-1") {
		t.Errorf("Tags = %v, want the deployment tag", got.Data[0].Tags)
	}
}

// TestV4Decode_UsesRealResponseKeys pins the field names to Langfuse's
// ObservationSchema. A wrong key decodes silently to a zero value, which
// reaches the UI as a blank cost or an absent model rather than as an error.
func TestV4Decode_UsesRealResponseKeys(t *testing.T) {
	body := `{"data":[{
		"id":"span-1","traceId":"trace-1","type":"GENERATION","name":"llm-call",
		"startTime":"2026-07-27T12:00:00Z","endTime":"2026-07-27T12:00:02Z",
		"level":"DEFAULT","statusMessage":"ok","version":"v9",
		"model":"claude-sonnet","modelParameters":{"temperature":0.2},
		"metadata":{"k":"v"},"tags":["deployment:dep-1"],"traceName":"agent-run","release":"r1",
		"totalCost":0.25,"inputUsage":100,"outputUsage":50,"totalUsage":150,
		"latency":1.5,"input":"hi","output":"there"
	}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetObservation(context.Background(), "span-1")
	if err != nil {
		t.Fatalf("GetObservation: %v", err)
	}
	if got.Model != "claude-sonnet" {
		t.Errorf("Model = %q, want claude-sonnet", got.Model)
	}
	if got.CalculatedTotalCost != 0.25 {
		t.Errorf("CalculatedTotalCost = %v, want 0.25", got.CalculatedTotalCost)
	}
	if got.Usage == nil || got.Usage.Input != 100 || got.Usage.Output != 50 || got.Usage.Total != 150 {
		t.Errorf("Usage = %+v, want 100/50/150", got.Usage)
	}
	if got.Metadata["k"] != "v" {
		t.Errorf("Metadata = %v, want k=v", got.Metadata)
	}
	if got.ModelParameters["temperature"] != 0.2 {
		t.Errorf("ModelParameters = %v, want temperature 0.2", got.ModelParameters)
	}
	if got.Level != "DEFAULT" || got.StatusMessage != "ok" {
		t.Errorf("Level/StatusMessage = %q/%q", got.Level, got.StatusMessage)
	}
}

func TestV4GetTrace_SumsCostAcrossSpans(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/v3/scores" {
			_ = json.NewEncoder(w).Encode(v3ScoresResponse{})
			return
		}
		// The root carries no cost of its own; the generations do.
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{
			{ID: "root", TraceID: "trace-1", StartTime: "2026-07-27T12:00:00Z", Tags: []string{"deployment:dep-1"}},
			{ID: "gen-1", TraceID: "trace-1", ParentObsID: "root", Type: "GENERATION", TotalCost: 0.2},
			{ID: "gen-2", TraceID: "trace-1", ParentObsID: "root", Type: "GENERATION", TotalCost: 0.05},
		}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetTrace(context.Background(), "trace-1")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	// Reading cost off the root alone would report 0.
	if got.TotalCost != 0.25 {
		t.Errorf("TotalCost = %v, want 0.25", got.TotalCost)
	}
}

func TestV4GetTrace_CountsCostOnASingleSpanTrace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/v3/scores" {
			_ = json.NewEncoder(w).Encode(v3ScoresResponse{})
			return
		}
		// One span, no children, and it is the generation that cost money.
		// Excluding the root the way Langfuse's own trace view does would
		// report this trace as free.
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{
			{ID: "only", TraceID: "trace-1", Type: "GENERATION", TotalCost: 0.004},
		}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetTrace(context.Background(), "trace-1")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if got.TotalCost != 0.004 {
		t.Errorf("TotalCost = %v, want 0.004", got.TotalCost)
	}
}

func TestV4GetTraces_TotalsCostPerTrace(t *testing.T) {
	var gotQuery MetricsQuery
	var metricsCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/metrics") {
			metricsCalls++
			if err := json.Unmarshal([]byte(r.URL.Query().Get("query")), &gotQuery); err != nil {
				t.Errorf("unmarshal metrics query: %v", err)
			}
			_ = json.NewEncoder(w).Encode(MetricsResponse{Data: []map[string]any{
				{"traceId": "trace-1", "sum_totalCost": 0.012882},
				{"traceId": "trace-2", "sum_totalCost": 0.05},
			}})
			return
		}
		// Root spans carry no cost of their own.
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{
			{ID: "root-1", TraceID: "trace-1", StartTime: "2026-07-27T12:00:00Z"},
			{ID: "root-2", TraceID: "trace-2", StartTime: "2026-07-27T12:00:01Z"},
			{ID: "root-3", TraceID: "trace-3", StartTime: "2026-07-27T12:00:02Z"},
		}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetTraces(
		context.Background(), "dep-1", "2026-07-27T00:00:00Z", "2026-07-28T00:00:00Z", 10, 0,
	)
	if err != nil {
		t.Fatalf("GetTraces: %v", err)
	}

	// One aggregate call for the whole page, not one per trace.
	if metricsCalls != 1 {
		t.Errorf("metrics calls = %d, want 1", metricsCalls)
	}
	if gotQuery.View != "observations" {
		t.Errorf("view = %q, want observations", gotQuery.View)
	}
	if len(gotQuery.Dimensions) != 1 || gotQuery.Dimensions[0].Field != "traceId" {
		t.Errorf("dimensions = %+v, want traceId", gotQuery.Dimensions)
	}
	// traceId is high cardinality, so v4 rejects the query without both.
	if len(gotQuery.OrderBy) != 1 || gotQuery.Config == nil || gotQuery.Config.RowLimit != 3 {
		t.Errorf("orderBy = %+v, config = %+v", gotQuery.OrderBy, gotQuery.Config)
	}
	// The aggregate must stay scoped to the deployment and to this page.
	var hasTag, hasTraceIDs bool
	for _, fl := range gotQuery.Filters {
		if fl.Column == "tags" {
			hasTag = true
		}
		if fl.Column == "traceId" {
			hasTraceIDs = true
		}
	}
	if !hasTag || !hasTraceIDs {
		t.Errorf("filters = %+v, want tags and traceId", gotQuery.Filters)
	}

	costs := map[string]float64{}
	for _, tr := range got.Data {
		costs[tr.ID] = tr.TotalCost
	}
	if costs["trace-1"] != 0.012882 || costs["trace-2"] != 0.05 {
		t.Errorf("costs = %v, want the summed totals", costs)
	}
	// A trace the aggregate did not report stays at zero rather than borrowing
	// another trace's number.
	if costs["trace-3"] != 0 {
		t.Errorf("trace-3 cost = %v, want 0", costs["trace-3"])
	}
}

func TestV4GetTraces_CostFailureSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/metrics") {
			http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{
			{ID: "root-1", TraceID: "trace-1"},
		}})
	}))
	defer srv.Close()

	// Reporting 0 would read as a free trace, so the failure has to surface.
	_, err := newV4Client(srv.URL).GetTraces(context.Background(), "dep-1", "", "", 10, 0)
	if err == nil {
		t.Fatal("expected an error when the cost aggregate fails")
	}
}

func TestV4GetTraceCore_ReturnsTagsForOwnershipCheck(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{{
			ID: "root-span-id", TraceID: "trace-1", TraceName: "agent-run",
			StartTime: "2026-07-27T12:00:00Z", Tags: []string{"deployment:dep-1"},
		}}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetTraceCore(context.Background(), "trace-1")
	if err != nil {
		t.Fatalf("GetTraceCore: %v", err)
	}
	filters := decodeFilters(t, gotQuery)
	assertFilter(t, filters, "traceId", []string{"trace-1"})
	if findFilter(filters, "isRootObservation") == nil {
		t.Error("root filter not set, so a child span could answer the lookup")
	}
	// This is the ownership check's only input. Without tags it rejects every
	// trace, which reads as a 404 to the caller.
	if !HasDeploymentTag(got.Tags, "dep-1") {
		t.Errorf("Tags = %v, want the deployment tag", got.Tags)
	}
}

func TestV4GetTrace_MergesSpansAndScores(t *testing.T) {
	var obsCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/public/v3/scores" {
			if got := r.URL.Query().Get("traceId"); got != "trace-1" {
				t.Errorf("scores traceId = %q, want trace-1", got)
			}
			_ = json.NewEncoder(w).Encode(v3ScoresResponse{Data: []Score{{ID: "score-1", Name: "quality", Value: 0.9}}})
			return
		}
		obsCalls++
		assertFilter(t, decodeFilters(t, r.URL.Query()), "traceId", []string{"trace-1"})
		// The child is returned first to prove the root is chosen by having no
		// parent, not by being first in the page.
		_ = json.NewEncoder(w).Encode(observationsV2Response{Data: []ObservationV2{
			{ID: "child", TraceID: "trace-1", ParentObsID: "root", Type: "GENERATION", Name: "llm-call"},
			{ID: "root", TraceID: "trace-1", Name: "root-span", TraceName: "agent-run", UserID: "user-1",
				StartTime: "2026-07-27T12:00:00Z", Input: "hi", Output: "there",
				Tags: []string{"deployment:dep-1"}, Release: "v1", Environment: "production"},
		}})
	}))
	defer srv.Close()

	got, err := newV4Client(srv.URL).GetTrace(context.Background(), "trace-1")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	// One call for the whole tree; the root no longer needs its own lookup.
	if obsCalls != 1 {
		t.Errorf("observation calls = %d, want 1", obsCalls)
	}
	if got.ID != "trace-1" || got.UserID != "user-1" || got.Output != "there" {
		t.Errorf("trace = %+v", got.Trace)
	}
	if got.Name != "agent-run" {
		t.Errorf("Name = %q, want the trace name not the span name", got.Name)
	}
	// Callers reject a trace whose tags omit their deployment, and render
	// CreatedAt as the timestamp. Empty values 404 the request or show an
	// invalid date.
	if !HasDeploymentTag(got.Tags, "dep-1") {
		t.Errorf("Tags = %v, want the deployment tag", got.Tags)
	}
	if got.CreatedAt != "2026-07-27T12:00:00Z" {
		t.Errorf("CreatedAt = %q, want the root start time", got.CreatedAt)
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
				t.Errorf("root filter present = %v, want %v", hasRootFilter, tt.wantRootOnly)
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
		{"empty defaults to v3", "", &v3Reader{}},
		{"true opts in", "true", &v4Reader{}},
		{"1 opts in", "1", &v4Reader{}},
		{"TRUE opts in", "TRUE", &v4Reader{}},
		{"false", "false", &v3Reader{}},
		{"0", "0", &v3Reader{}},
		{"unparseable defaults to v3", "yes-please", &v3Reader{}},
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

	t.Run("absent defaults to v3", func(t *testing.T) {
		t.Setenv("LANGFUSE_USE_V4_API", "") // registers the cleanup that restores it
		os.Unsetenv("LANGFUSE_USE_V4_API")
		if got := NewClient("http://unused", "pk", "sk").reader; fmt.Sprintf("%T", got) != "*langfuse.v3Reader" {
			t.Errorf("reader = %T, want *langfuse.v3Reader", got)
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

// Dev-tool traces are scoped by source tag, and environments run either reader,
// so the scope has to survive both encodings — a tags param on v3, a filter
// entry on v4. A deployment-scoped read would exclude every dev-tool trace.
func TestGetDevtoolTraces_ScopesBySourceTag(t *testing.T) {
	capture := func(t *testing.T) (*url.Values, string) {
		t.Helper()
		got := &url.Values{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*got = r.URL.Query()
			_, _ = w.Write([]byte(`{"data":[],"meta":{"page":1,"limit":100,"totalItems":0,"totalPages":0}}`))
		}))
		t.Cleanup(srv.Close)
		return got, srv.URL
	}

	t.Run("v3", func(t *testing.T) {
		t.Setenv("LANGFUSE_USE_V4_API", "false")
		got, base := capture(t)
		c := NewClient(base, "pk", "sk")
		if _, err := c.GetDevtoolTraces(context.Background(), "claude-code", "2026-08-01T00:00:00Z", "2026-08-02T00:00:00Z", 100, 200); err != nil {
			t.Fatalf("GetDevtoolTraces: %v", err)
		}
		for k, want := range map[string]string{
			"tags":          "claude-code",
			"fields":        "core,io",
			"orderBy":       "timestamp.asc",
			"limit":         "100",
			"page":          "3",
			"fromTimestamp": "2026-08-01T00:00:00Z",
			"toTimestamp":   "2026-08-02T00:00:00Z",
		} {
			if v := got.Get(k); v != want {
				t.Errorf("%s = %q, want %q", k, v, want)
			}
		}
		if strings.Contains(got.Get("tags"), "deployment:") {
			t.Errorf("tags = %q, should not be deployment-scoped", got.Get("tags"))
		}
	})

	t.Run("v4", func(t *testing.T) {
		t.Setenv("LANGFUSE_USE_V4_API", "true")
		got, base := capture(t)
		c := NewClient(base, "pk", "sk")
		if _, err := c.GetDevtoolTraces(context.Background(), "claude-code", "2026-08-01T00:00:00Z", "2026-08-02T00:00:00Z", 100, 0); err != nil {
			t.Fatalf("GetDevtoolTraces: %v", err)
		}
		filter := got.Get("filter")
		if !strings.Contains(filter, `"claude-code"`) {
			t.Errorf("filter = %s, want the source tag", filter)
		}
		if strings.Contains(filter, "deployment:") {
			t.Errorf("filter = %s, should not be deployment-scoped", filter)
		}
		if !strings.Contains(got.Get("fields"), "io") {
			t.Errorf("fields = %q, want io so prompt text is returned", got.Get("fields"))
		}
	})
}

func TestGetDevtoolTraces_RequiresSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not issue a request without a source")
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL, "pk", "sk").GetDevtoolTraces(context.Background(), "", "", "", 10, 0); err == nil {
		t.Fatal("empty source should error")
	}
}

// An unscoped trace read would cross deployment boundaries inside the account's
// shared Langfuse project. Both readers are checked: the guard is shared, but
// each builds its scope differently — v3 as a tags param, v4 as a filter entry
// — so a regression could reach one and not the other.
func TestGetTraces_UnscopedQueryRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not issue an unscoped request")
	}))
	defer srv.Close()
	tr := &transport{baseURL: srv.URL, publicKey: "pk", secretKey: "sk", http: srv.Client()}

	readers := map[string]traceReader{
		"v3": &v3Reader{transport: tr},
		"v4": &v4Reader{transport: tr},
	}
	for name, reader := range readers {
		t.Run(name, func(t *testing.T) {
			_, err := reader.getTraces(context.Background(), traceFilter{limit: 10})
			if err == nil || !strings.Contains(err.Error(), "deployment or tag scope") {
				t.Fatalf("err = %v, want scope error", err)
			}
		})
	}
}

// Dev-tool traces have no deployment, so the source tag is their only scope.
func TestTraceScopeTag(t *testing.T) {
	if got, err := traceScopeTag(traceFilter{tag: "claude-code"}); err != nil || got != "claude-code" {
		t.Errorf("tag scope = %q, %v; want claude-code", got, err)
	}
	if got, err := traceScopeTag(traceFilter{deploymentID: "dep-1"}); err != nil || got != "deployment:dep-1" {
		t.Errorf("deployment scope = %q, %v; want deployment:dep-1", got, err)
	}
	// A tag wins so a dev-tool read is never silently widened to a deployment.
	if got, _ := traceScopeTag(traceFilter{tag: "claude-code", deploymentID: "dep-1"}); got != "claude-code" {
		t.Errorf("tag should take precedence, got %q", got)
	}
}
