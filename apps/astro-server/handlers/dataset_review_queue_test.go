package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore/judgmentstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

func TestGetDatasetReviewQueue_FiltersJudged(t *testing.T) {
	traces := []langfuse.Trace{
		{
			ID:        "trace-3",
			SessionID: "session-2",
			CreatedAt: "2026-06-01T14:00:00Z",
			Input:     "already judged",
			Output:    "done",
		},
		{
			ID:        "trace-2",
			SessionID: "session-1",
			UserID:    "user_01HXX_bob",
			CreatedAt: "2026-06-01T13:00:00Z",
			Input:     "thanks, this helped",
			Output:    "great",
		},
		{
			ID:        "trace-1",
			SessionID: "session-1",
			CreatedAt: "2026-06-01T12:00:00Z",
			Input:     "how do I deploy?",
			Output:    "run astro deploy",
		},
	}
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, len(traces), "100", "", "*"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectEmptyReviewQueueState(f.judgmentMock, "dataset-dep-1", "trace-3")
	expectPersonalProfilesQuery(f.accountMock, []string{"user_01HXX_bob"}, func(rows *sqlmock.Rows) {
		rows.AddRow("user_01HXX_bob", "bob", "Bob Smith", nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?limit=3", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetReviewQueueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(resp.Items))
	}
	if resp.Items[0].TraceID != "trace-2" {
		t.Fatalf("first item = %+v, want trace-2", resp.Items[0])
	}
	if resp.Items[0].UserID != "user_01HXX_bob" || resp.Items[0].UserDetails == nil ||
		resp.Items[0].UserDetails.Kind != UserDetailsKindAstro ||
		resp.Items[0].UserDetails.DisplayName != "Bob Smith" ||
		resp.Items[0].UserDetails.Username != "bob" {
		t.Fatalf("first item user = %q/%+v, want hydrated Bob profile", resp.Items[0].UserID, resp.Items[0].UserDetails)
	}
	if resp.Items[1].TraceID != "trace-1" {
		t.Fatalf("second item = %+v, want trace-1", resp.Items[1])
	}
	if resp.Items[0].PredictionStatus != reviewQueueStatusNotRequested ||
		resp.Items[1].PredictionStatus != reviewQueueStatusNotRequested {
		t.Fatalf("prediction statuses = %q/%q, want not_requested", resp.Items[0].PredictionStatus, resp.Items[1].PredictionStatus)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestGetDatasetReviewQueue_IncludesJudgedTrace(t *testing.T) {
	traces := []langfuse.Trace{{
		ID:        "trace-1",
		CreatedAt: "2026-06-01T12:00:00Z",
		Input:     "question",
		Output:    "answer",
	}}
	traceHandler := langfuseTracesHandler(t, traces, 1, "1", "", "*")
	f := setupDatasetRouter(t, true, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("filter"); got != `[{"type":"stringOptions","column":"id","operator":"any of","value":["trace-1"]}]` {
			t.Errorf("filter = %q, want prediction trace ID filter", got)
		}
		start, startErr := time.Parse(time.RFC3339Nano, r.URL.Query().Get("fromTimestamp"))
		end, endErr := time.Parse(time.RFC3339Nano, r.URL.Query().Get("toTimestamp"))
		if startErr != nil || endErr != nil || end.Sub(start) != reviewQueueWindow {
			t.Errorf("prediction window = %q to %q, want %s", r.URL.Query().Get("fromTimestamp"), r.URL.Query().Get("toTimestamp"), reviewQueueWindow)
		}
		traceHandler(w, r)
	})
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	traceTimestamp := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	judgmentstoretest.ExpectPredictionTracesWithoutJudgments(
		f.judgmentMock,
		"dataset-dep-1",
		nil,
		reviewQueueDefaultLimit+1,
		judgmentstore.PredictionTrace{TraceID: "trace-1", TraceTimestamp: traceTimestamp},
	)
	now := time.Now().UTC()
	judgmentstoretest.ExpectPredictions(
		f.judgmentMock,
		"dataset-dep-1",
		map[string]judgmentstore.Prediction{
			"trace-1": {
				TraceTimestamp: traceTimestamp,
				VerdictScore:   0.25,
				Confidence:     80,
				Explanation:    "Useful trace.",
				JudgeVersion:   "1",
				CreatedAt:      now,
				UpdatedAt:      now,
				Criteria: []judgmentstore.PredictionCriterion{
					{Dimension: judgmentstore.DimensionAccuracy, Value: 0.5},
					{Dimension: judgmentstore.DimensionTone, Value: 0.75},
				},
			},
		},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?prediction=present",
		nil,
	)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetReviewQueueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 ||
		resp.Items[0].PredictionStatus != "completed" ||
		resp.Items[0].Prediction == nil ||
		resp.Items[0].Prediction.VerdictScore != 0.25 ||
		len(resp.Items[0].Prediction.Criteria) != 2 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestGetDatasetReviewQueue_FiltersTracesWithoutPredictions(t *testing.T) {
	traces := []langfuse.Trace{
		{
			ID:        "trace-unpredicted",
			CreatedAt: "2026-07-27T13:00:00Z",
			Input:     "question without prediction",
		},
		{
			ID:        "trace-predicted",
			CreatedAt: "2026-07-27T12:00:00Z",
			Input:     "question with prediction",
		},
	}
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, 2, "100", "", "*"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	judgmentstoretest.ExpectJudgedTraceIDs(f.judgmentMock, "dataset-dep-1")
	judgmentstoretest.ExpectPredictionRequests(f.judgmentMock, "dataset-dep-1")
	judgmentstoretest.ExpectPredictions(
		f.judgmentMock,
		"dataset-dep-1",
		map[string]judgmentstore.Prediction{
			"trace-predicted": {
				VerdictScore: 0.8,
				Confidence:   90,
				Explanation:  "Useful trace.",
				JudgeVersion: "1",
			},
		},
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?prediction=absent",
		nil,
	)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetReviewQueueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 ||
		resp.Items[0].TraceID != "trace-unpredicted" ||
		resp.Items[0].PredictionStatus != reviewQueueStatusNotRequested ||
		resp.Items[0].Prediction != nil {
		t.Fatalf("response = %+v", resp)
	}
}

func TestReviewQueueCursorRoundTrip(t *testing.T) {
	const endTime = "2026-06-18T20:07:29.702000Z"
	want := reviewQueueCursor{
		Version:       reviewQueueCursorVersion,
		EvalDatasetID: "dataset-1",
		Filter:        "present",
		Limit:         25,
		EndTime:       endTime,
		RawPage:       1,
		RawIndex:      0,
		LocalTime:     "2026-06-17T20:07:29.702Z",
		LocalTrace:    "trace-17",
	}
	raw, err := encodeReviewQueueCursor(want)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeReviewQueueCursor(raw, "dataset-1", reviewQueuePredictionPresent, 25)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != want {
		t.Fatalf("cursor = %+v, want %+v", got, want)
	}
}

func TestGetDatasetReviewQueue_CursorResumesWithinRawPage(t *testing.T) {
	traces := []langfuse.Trace{
		{ID: "trace-1", Input: "question 1", Output: "answer 1"},
		{ID: "trace-2", Input: "question 2", Output: "answer 2"},
	}
	var snapshotStarts []string
	var snapshotEnds []string
	upstream := func(w http.ResponseWriter, r *http.Request) {
		snapshotStarts = append(snapshotStarts, r.URL.Query().Get("fromTimestamp"))
		snapshotEnds = append(snapshotEnds, r.URL.Query().Get("toTimestamp"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": traces,
			"meta": map[string]any{
				"page":       1,
				"limit":      100,
				"totalItems": 2,
				"totalPages": 1,
			},
		})
	}
	f := setupDatasetRouter(t, true, upstream)

	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectEmptyReviewQueueState(f.judgmentMock, "dataset-dep-1")
	firstReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?limit=1",
		nil,
	)
	firstRec := httptest.NewRecorder()
	f.router.ServeHTTP(firstRec, firstReq)

	var first DatasetReviewQueueResponse
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first response = %d %s", firstRec.Code, firstRec.Body.String())
	}
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {
		t.Fatalf("unmarshal first response: %v", err)
	}
	if len(first.Items) != 1 || first.Items[0].TraceID != "trace-1" || first.NextCursor == "" {
		t.Fatalf("first response = %+v", first)
	}

	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectEmptyReviewQueueState(f.judgmentMock, "dataset-dep-1")
	secondReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?limit=1&cursor="+url.QueryEscape(first.NextCursor),
		nil,
	)
	secondRec := httptest.NewRecorder()
	f.router.ServeHTTP(secondRec, secondReq)

	var second DatasetReviewQueueResponse
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second response = %d %s", secondRec.Code, secondRec.Body.String())
	}
	if err := json.Unmarshal(secondRec.Body.Bytes(), &second); err != nil {
		t.Fatalf("unmarshal second response: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].TraceID != "trace-2" || second.NextCursor != "" {
		t.Fatalf("second response = %+v", second)
	}
	if len(snapshotStarts) != 2 || snapshotStarts[0] == "" || snapshotStarts[0] != snapshotStarts[1] {
		t.Fatalf("snapshot starts = %v, want one stable timestamp", snapshotStarts)
	}
	if len(snapshotEnds) != 2 || snapshotEnds[0] == "" || snapshotEnds[0] != snapshotEnds[1] {
		t.Fatalf("snapshot ends = %v, want one stable timestamp", snapshotEnds)
	}
	start, startErr := time.Parse(time.RFC3339Nano, snapshotStarts[0])
	end, endErr := time.Parse(time.RFC3339Nano, snapshotEnds[0])
	if startErr != nil || endErr != nil || end.Sub(start) != reviewQueueWindow {
		t.Fatalf("snapshot window = %q to %q, want %s", snapshotStarts[0], snapshotEnds[0], reviewQueueWindow)
	}
}

func TestGetDatasetReviewQueue_FilterWithoutMatchesSkipsLangfuse(t *testing.T) {
	upstreamCalls := 0
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	judgmentstoretest.ExpectPredictionTracesWithoutJudgments(
		f.judgmentMock,
		"dataset-dep-1",
		nil,
		reviewQueueDefaultLimit+1,
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?prediction=present",
		nil,
	)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	var resp DatasetReviewQueueResponse
	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if upstreamCalls != 0 || len(resp.Items) != 0 || resp.NextCursor != "" {
		t.Fatalf("calls=%d response=%+v", upstreamCalls, resp)
	}
}

func TestGetDatasetReviewQueue_DefaultLimitUsesDefaultPageSize(t *testing.T) {
	traces := []langfuse.Trace{
		{
			ID:        "trace-1",
			SessionID: "session-1",
			CreatedAt: "2026-06-01T12:00:00Z",
			Input:     "how do I deploy?",
			Output:    "run astro deploy",
		},
	}
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, len(traces), "100", "", "*"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectEmptyReviewQueueState(f.judgmentMock, "dataset-dep-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDatasetReviewQueue_InvalidCursor(t *testing.T) {
	f := setupDatasetRouter(t, true, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?cursor=bad", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDatasetReviewQueue_InvalidPredictionFilter(t *testing.T) {
	upstreamCalled := false
	f := setupDatasetRouter(t, true, func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	})
	expectAuthorizedDeployment(f.traceDetailFixture)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?prediction=maybe", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamCalled {
		t.Fatal("Langfuse upstream was called for invalid prediction filter")
	}
}

func TestGetDatasetReviewQueue_DatasetNotFoundDoesNotFetchTraces(t *testing.T) {
	upstreamCalled := false
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetNotFound(f.datasetMock, "dep-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?limit=3", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamCalled {
		t.Fatal("Langfuse upstream was called before confirming dataset exists")
	}
}

func TestNewDatasetReviewQueueItemIncludesFullInputOutput(t *testing.T) {
	output := strings.Repeat("x", 300)
	item := newDatasetReviewQueueItem(
		langfuse.Trace{
			ID:        "trace-1",
			CreatedAt: "2026-06-01T12:00:00Z",
			Input:     map[string]any{"prompt": "hello"},
			Output:    output,
		},
		judgmentstore.PredictionRequest{},
		judgmentstore.Prediction{},
		false,
	)

	input, ok := item.Input.(map[string]any)
	if !ok || input["prompt"] != "hello" {
		t.Fatalf("input = %#v, want full input map", item.Input)
	}
	if item.Output != output {
		t.Fatalf("output was not preserved in full")
	}
}

func TestNewDatasetReviewQueueItemPredictionState(t *testing.T) {
	failure := "Prediction failed. Try again."
	trace := langfuse.Trace{ID: "trace-1", Input: "input", Output: "output"}

	failed := newDatasetReviewQueueItem(
		trace,
		judgmentstore.PredictionRequest{
			Status:       judgmentstore.PredictionRequestFailed,
			ErrorMessage: &failure,
		},
		judgmentstore.Prediction{},
		false,
	)
	if failed.PredictionStatus != "failed" ||
		failed.PredictionError == nil ||
		*failed.PredictionError != failure ||
		failed.Prediction != nil {
		t.Fatalf("failed item = %+v", failed)
	}

	completed := newDatasetReviewQueueItem(
		trace,
		judgmentstore.PredictionRequest{Status: judgmentstore.PredictionRequestInProgress},
		judgmentstore.Prediction{
			VerdictScore: 0.8,
			Confidence:   90,
			Explanation:  "Useful trace.",
			JudgeVersion: "1",
			Criteria: []judgmentstore.PredictionCriterion{{
				Dimension: judgmentstore.DimensionAccuracy,
				Value:     0.75,
			}},
		},
		true,
	)
	if completed.PredictionStatus != "completed" ||
		completed.PredictionError != nil ||
		completed.Prediction == nil ||
		completed.Prediction.VerdictScore != 0.8 ||
		len(completed.Prediction.Criteria) != 1 ||
		completed.Prediction.Criteria[0].DimensionKey != "accuracy" {
		t.Fatalf("completed item = %+v", completed)
	}
}

func langfuseTracesHandler(t *testing.T, traces []langfuse.Trace, totalItems int, wantLimit, wantPage, wantToTimestamp string) http.HandlerFunc {
	t.Helper()
	type meta struct {
		Page       int `json:"page"`
		Limit      int `json:"limit"`
		TotalItems int `json:"totalItems"`
		TotalPages int `json:"totalPages"`
	}
	type resp struct {
		Data []langfuse.Trace `json:"data"`
		Meta meta             `json:"meta"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fields") != "core,io" {
			t.Errorf("fields = %q, want core,io", r.URL.Query().Get("fields"))
		}
		if r.URL.Query().Get("limit") != wantLimit {
			t.Errorf("limit = %q, want %q", r.URL.Query().Get("limit"), wantLimit)
		}
		if r.URL.Query().Get("page") != wantPage {
			t.Errorf("page = %q, want %q", r.URL.Query().Get("page"), wantPage)
		}
		gotToTimestamp := r.URL.Query().Get("toTimestamp")
		if wantToTimestamp == "*" {
			if gotToTimestamp == "" {
				t.Error("toTimestamp should be set")
			} else if _, err := time.Parse(time.RFC3339Nano, gotToTimestamp); err != nil {
				t.Errorf("toTimestamp = %q, want RFC3339 timestamp: %v", gotToTimestamp, err)
			}
		} else if gotToTimestamp != wantToTimestamp {
			t.Errorf("toTimestamp = %q, want %q", gotToTimestamp, wantToTimestamp)
		}
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				t.Errorf("page = %q, want integer", raw)
			} else {
				page = parsed
			}
		}
		limit := len(traces)
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				t.Errorf("limit = %q, want integer", raw)
			} else {
				limit = parsed
			}
		}
		totalPages := 0
		if limit > 0 && totalItems > 0 {
			totalPages = (totalItems + limit - 1) / limit
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp{
			Data: traces,
			Meta: meta{Page: page, Limit: limit, TotalItems: totalItems, TotalPages: totalPages},
		})
	}
}

func expectEmptyReviewQueueState(mock sqlmock.Sqlmock, datasetID string, judgedTraceIDs ...string) {
	judgmentstoretest.ExpectJudgedTraceIDs(mock, datasetID, judgedTraceIDs...)
	judgmentstoretest.ExpectPredictionRequests(mock, datasetID)
	judgmentstoretest.ExpectPredictions(mock, datasetID, nil)
}
