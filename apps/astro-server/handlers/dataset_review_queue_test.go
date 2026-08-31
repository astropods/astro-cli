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
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

func TestGetDatasetReviewQueue_FiltersDatasetItems(t *testing.T) {
	traces := []langfuse.Trace{
		{
			ID:        "trace-3",
			SessionID: "session-2",
			CreatedAt: "2026-06-01T14:00:00Z", Timestamp: "2026-06-01T14:00:00Z",
			Input:  "already in the dataset",
			Output: "done",
		},
		{
			ID:        "trace-2",
			SessionID: "session-1",
			UserID:    "user_01HXX_bob",
			CreatedAt: "2026-06-01T13:00:00Z", Timestamp: "2026-06-01T13:00:00Z",
			Input:  "thanks, this helped",
			Output: "great",
		},
		{
			ID:        "trace-1",
			SessionID: "session-1",
			CreatedAt: "2026-06-01T12:00:00Z", Timestamp: "2026-06-01T12:00:00Z",
			Input:  "how do I deploy?",
			Output: "run astro deploy",
		},
	}
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, len(traces), "100", "", "*"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	expectEmptyReviewQueueState(f, "dataset-dep-1", "trace-3")
	expectNoRuns(f.runMock)

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
	if resp.Items[1].TraceID != "trace-1" {
		t.Fatalf("second item = %+v, want trace-1", resp.Items[1])
	}
	if resp.Items[0].Run != nil || resp.Items[1].Run != nil {
		t.Fatalf("runs = %+v/%+v, want none", resp.Items[0].Run, resp.Items[1].Run)
	}
	if err := f.itemMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset item expectations: %v", err)
	}
}

func TestGetDatasetReviewQueue_EvaluatedPagesLocallyThenFetchesTraces(t *testing.T) {
	traces := []langfuse.Trace{{
		ID:        "trace-1",
		CreatedAt: "2026-06-01T12:00:00Z", Timestamp: "2026-06-01T12:00:00Z",
		Input:  "question",
		Output: "answer",
	}}
	traceHandler := langfuseTracesHandler(t, traces, 1, "1", "", "*")
	f := setupDatasetRouter(t, true, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("filter"); got != `[{"type":"stringOptions","column":"id","operator":"any of","value":["trace-1"]}]` {
			t.Errorf("filter = %q, want the locally paged trace IDs", got)
		}
		start, startErr := time.Parse(time.RFC3339Nano, r.URL.Query().Get("fromTimestamp"))
		end, endErr := time.Parse(time.RFC3339Nano, r.URL.Query().Get("toTimestamp"))
		if startErr != nil || endErr != nil || end.Sub(start) != reviewQueueWindow {
			t.Errorf("window = %q to %q, want %s", r.URL.Query().Get("fromTimestamp"), r.URL.Query().Get("toTimestamp"), reviewQueueWindow)
		}
		traceHandler(w, r)
	})
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	expectCompletedRunTraces(f.runMock, evalrunstore.RunTrace{
		TraceID:        "trace-1",
		TraceTimestamp: time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC),
	})
	expectEmptyReviewQueueState(f, "dataset-dep-1")
	expectLatestRuns(f.runMock, map[string]string{"trace-1": "completed"})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?evaluation=evaluated",
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
		resp.Items[0].Run == nil ||
		resp.Items[0].Run.Status != "completed" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestGetDatasetReviewQueue_FiltersDismissedTraces(t *testing.T) {
	traces := []langfuse.Trace{
		{ID: "trace-2", CreatedAt: "2026-06-01T13:00:00Z", Timestamp: "2026-06-01T13:00:00Z", Input: "dismissed earlier"},
		{ID: "trace-1", CreatedAt: "2026-06-01T12:00:00Z", Timestamp: "2026-06-01T12:00:00Z", Input: "how do I deploy?"},
	}
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, len(traces), "100", "", "*"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	expectAddedTraces(f.itemMock, "dataset-dep-1")
	expectDismissedTraces(f.dismissalMock, "dataset-dep-1", "trace-2")
	expectNoRuns(f.runMock)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetReviewQueueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].TraceID != "trace-1" {
		t.Fatalf("items = %+v, want only trace-1", resp.Items)
	}
	if err := f.dismissalMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("dismissal expectations: %v", err)
	}
}

func TestGetDatasetReviewQueue_EvaluatedFiltersDismissedTraces(t *testing.T) {
	traces := []langfuse.Trace{
		{ID: "trace-1", CreatedAt: "2026-07-27T12:00:00Z", Timestamp: "2026-07-27T12:00:00Z", Input: "evaluated then dismissed"},
	}
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, 1, "1", "", "*"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	expectCompletedRunTraces(f.runMock, evalrunstore.RunTrace{
		TraceID:        "trace-1",
		TraceTimestamp: time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC),
	})
	expectAddedTraces(f.itemMock, "dataset-dep-1")
	expectDismissedTraces(f.dismissalMock, "dataset-dep-1", "trace-1")
	expectLatestRuns(f.runMock, map[string]string{"trace-1": "completed"})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?evaluation=evaluated",
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
	if len(resp.Items) != 0 {
		t.Fatalf("items = %+v, want none", resp.Items)
	}
}

func TestGetDatasetReviewQueue_NotEvaluatedDropsCompletedRuns(t *testing.T) {
	traces := []langfuse.Trace{
		{ID: "trace-open", CreatedAt: "2026-07-27T13:00:00Z", Timestamp: "2026-07-27T13:00:00Z", Input: "not evaluated yet"},
		{ID: "trace-done", CreatedAt: "2026-07-27T12:00:00Z", Timestamp: "2026-07-27T12:00:00Z", Input: "already evaluated"},
	}
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, 2, "100", "", "*"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	expectEmptyReviewQueueState(f, "dataset-dep-1")
	expectLatestRuns(f.runMock, map[string]string{"trace-done": "completed"})

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?evaluation=not_evaluated",
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
		resp.Items[0].TraceID != "trace-open" ||
		resp.Items[0].Run != nil {
		t.Fatalf("response = %+v", resp)
	}
}

func TestReviewQueueCursorRoundTrip(t *testing.T) {
	const endTime = "2026-06-18T20:07:29.702000Z"
	want := reviewQueueCursor{
		Version:       reviewQueueCursorVersion,
		EvalDatasetID: "dataset-1",
		Filter:        string(reviewQueueEvaluated),
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
	got, err := decodeReviewQueueCursor(raw, "dataset-1", reviewQueueEvaluated, 25)
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
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	expectEmptyReviewQueueState(f, "dataset-dep-1")
	expectNoRuns(f.runMock)
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
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	expectEmptyReviewQueueState(f, "dataset-dep-1")
	expectNoRuns(f.runMock)
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
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	expectCompletedRunTraces(f.runMock)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?evaluation=evaluated",
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
			CreatedAt: "2026-06-01T12:00:00Z", Timestamp: "2026-06-01T12:00:00Z",
			Input:  "how do I deploy?",
			Output: "run astro deploy",
		},
	}
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, len(traces), "100", "", "*"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	expectEmptyReviewQueueState(f, "dataset-dep-1")
	expectNoRuns(f.runMock)

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
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?cursor=bad", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDatasetReviewQueue_InvalidEvaluationFilter(t *testing.T) {
	upstreamCalled := false
	f := setupDatasetRouter(t, true, func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	})
	expectAuthorizedDeployment(f.traceDetailFixture)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?evaluation=maybe", nil)
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

func TestNewDatasetReviewQueueItemIncludesFullInput(t *testing.T) {
	prompt := strings.Repeat("x", 300)
	item := newDatasetReviewQueueItem(
		langfuse.Trace{
			ID:        "trace-1",
			CreatedAt: "2026-06-01T12:00:00Z", Timestamp: "2026-06-01T12:00:00Z",
			Input:  map[string]any{"prompt": prompt},
			Output: "the answer",
		},
		evalrunstore.Run{},
		false,
	)

	input, ok := item.Input.(map[string]any)
	if !ok || input["prompt"] != prompt {
		t.Fatalf("input = %#v, want full input map", item.Input)
	}
	if item.Run != nil {
		t.Fatalf("run = %+v, want none", item.Run)
	}
}

func TestNewDatasetReviewQueueItemRunState(t *testing.T) {
	trace := langfuse.Trace{ID: "trace-1", Input: "input", Output: "output"}

	failed := newDatasetReviewQueueItem(trace, evalrunstore.Run{
		Status:       evalrunstore.StatusFailed,
		ErrorMessage: "No evaluator produced a result.",
	}, true)
	if failed.Run == nil ||
		failed.Run.Status != "failed" ||
		failed.Run.Error == nil ||
		*failed.Run.Error != "No evaluator produced a result." {
		t.Fatalf("failed item = %+v", failed.Run)
	}

	completed := newDatasetReviewQueueItem(trace, evalrunstore.Run{
		Status: evalrunstore.StatusCompleted,
	}, true)
	if completed.Run == nil ||
		completed.Run.Status != "completed" ||
		completed.Run.Error != nil {
		t.Fatalf("completed item = %+v", completed.Run)
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

func expectEmptyReviewQueueState(f *datasetFixture, datasetID string, addedTraceIDs ...string) {
	expectAddedTraces(f.itemMock, datasetID, addedTraceIDs...)
	expectDismissedTraces(f.dismissalMock, datasetID)
}

func expectAddedTraces(mock sqlmock.Sqlmock, datasetID string, traceIDs ...string) {
	mock.ExpectQuery("FROM eval_dataset_items").
		WithArgs(datasetID, sqlmock.AnyArg()).
		WillReturnRows(traceIDRows(traceIDs))
}

func expectDismissedTraces(mock sqlmock.Sqlmock, datasetID string, traceIDs ...string) {
	mock.ExpectQuery("FROM eval_dataset_dismissed_traces").
		WithArgs(datasetID, sqlmock.AnyArg()).
		WillReturnRows(traceIDRows(traceIDs))
}

func traceIDRows(traceIDs []string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"trace_id"})
	for _, traceID := range traceIDs {
		rows.AddRow(traceID)
	}
	return rows
}

func expectLatestRuns(mock sqlmock.Sqlmock, runs map[string]string) {
	rows := sqlmock.NewRows([]string{"trace_id", "id", "evaluation_ref", "status", "error_message"})
	for traceID, status := range runs {
		rows.AddRow(traceID, "run-"+traceID, evalpreset.RefDefaultSet, status, nil)
	}
	mock.ExpectQuery("DISTINCT ON").WillReturnRows(rows)
}

func expectNoRuns(mock sqlmock.Sqlmock) {
	expectLatestRuns(mock, nil)
}

func expectCompletedRunTraces(mock sqlmock.Sqlmock, traces ...evalrunstore.RunTrace) {
	rows := sqlmock.NewRows([]string{"trace_id", "trace_timestamp"})
	for _, trace := range traces {
		rows.AddRow(trace.TraceID, trace.TraceTimestamp)
	}
	mock.ExpectQuery(`DISTINCT ON \(trace_id\) trace_id, trace_timestamp, status`).WillReturnRows(rows)
}
