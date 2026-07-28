package langfuse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	c := NewClient(srv.URL, "pk", "sk")
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

	got, err := NewClient(srv.URL, "pk", "sk").GetNextSessionTrace(
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

func TestGetNextSessionTraceOmitsIncompleteContext(t *testing.T) {
	client := NewClient("http://unused", "pk", "sk")
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

	c := NewClient(srv.URL, "pk", "sk")
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

	c := NewClient(srv.URL, "pk", "sk")
	_, err := c.GetTracesFilteredOrdered(
		context.Background(),
		"dep-1",
		"2026-01-01T00:00:00Z",
		"2026-01-02T00:00:00Z",
		[]TraceFilter{
			{Type: "arrayOptions", Column: "tags", Operator: "all of", Value: []string{"deployment:dep-1"}},
			{Type: "null", Column: "userId", Operator: "is null"},
		},
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

func TestGetQueueTraces_QueryParams(t *testing.T) {
	var gotQuery map[string][]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TracesResponse{})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "pk", "sk")
	_, err := c.GetQueueTraces(context.Background(), "dep-1", "2026-01-02T00:00:00Z", 50, 50)
	if err != nil {
		t.Fatalf("GetQueueTraces returned error: %v", err)
	}

	assertParam(t, gotQuery, "tags", "deployment:dep-1")
	assertParam(t, gotQuery, "fromTimestamp", "")
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

	c := NewClient(srv.URL, "pk", "sk")
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

	c := NewClient(srv.URL, "pk", "sk")
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

	c := NewClient(srv.URL, "pk", "sk")
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

	c := NewClient(srv.URL, "pk", "sk")
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

	c := NewClient(srv.URL, "pk", "sk")
	_, err := c.GetObservation(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention not found", err)
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
