package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

// ---------------------------------------------------------------------------
// Dataset handler fixture
// ---------------------------------------------------------------------------

type datasetFixture struct {
	*traceDetailFixture
	datasetMock  sqlmock.Sqlmock
	judgmentMock sqlmock.Sqlmock
}

func setupDatasetRouter(t *testing.T, withUser bool, upstreamHandler http.HandlerFunc) *datasetFixture {
	t.Helper()
	f, log, cfg, accountStore, deployStore, langfuseStore := newLangfuseFixture(t, withUser, upstreamHandler)

	datasetDB, datasetMock, _ := sqlmock.New()
	t.Cleanup(func() { datasetDB.Close() })
	dsStore := evaldatasetstore.NewStore(datasetDB)

	judgmentDB, judgmentMock, _ := sqlmock.New()
	t.Cleanup(func() { judgmentDB.Close() })
	judgmentStore := judgmentstore.NewStore(judgmentDB)

	f.router.GET("/api/v1/deployments/:id/dataset",
		GetEvalDataset(log, accountStore, deployStore, dsStore))
	f.router.GET("/api/v1/deployments/:id/dataset/items",
		GetEvalDatasetItems(log, cfg, accountStore, deployStore, dsStore, langfuseStore))
	f.router.GET("/api/v1/deployments/:id/dataset/download",
		DownloadEvalDataset(log, cfg, accountStore, deployStore, dsStore, langfuseStore))
	f.router.GET("/api/v1/deployments/:id/dataset/review-queue",
		GetDatasetReviewQueue(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))
	f.router.POST("/api/v1/deployments/:id/dataset/judgments",
		PostDatasetJudgment(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))

	return &datasetFixture{traceDetailFixture: f, datasetMock: datasetMock, judgmentMock: judgmentMock}
}

func expectDatasetRow(mock sqlmock.Sqlmock, deploymentID, datasetName string, itemCount int) {
	expectDatasetRowCounts(mock, deploymentID, datasetName, itemCount, itemCount, 0)
}

func expectDatasetRowCounts(mock sqlmock.Sqlmock, deploymentID, datasetName string, itemCount, goodCount, badCount int) {
	datasetstoretest.ExpectRow(mock, deploymentID, datasetName, itemCount, goodCount, badCount)
}

func expectDatasetNotFound(mock sqlmock.Sqlmock, deploymentID string) {
	datasetstoretest.ExpectMissing(mock, deploymentID)
}

func langfuseDatasetItemsHandler(items []langfuse.DatasetItem, totalPages int) http.HandlerFunc {
	type meta struct {
		Page       int `json:"page"`
		Limit      int `json:"limit"`
		TotalItems int `json:"totalItems"`
		TotalPages int `json:"totalPages"`
	}
	type resp struct {
		Data []langfuse.DatasetItem `json:"data"`
		Meta meta                   `json:"meta"`
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp{
			Data: items,
			Meta: meta{Page: 1, Limit: 50, TotalItems: len(items), TotalPages: totalPages},
		})
	}
}

func langfuseTracesHandler(t *testing.T, traces []langfuse.Trace, totalItems int, wantPage, wantToTimestamp string) http.HandlerFunc {
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
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp{
			Data: traces,
			Meta: meta{Page: 1, Limit: len(traces), TotalItems: totalItems, TotalPages: 1},
		})
	}
}

func langfuseJudgeHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces/trace-1":
			_ = json.NewEncoder(w).Encode(langfuse.TraceDetail{
				Trace: langfuse.Trace{
					ID:        "trace-1",
					Input:     map[string]any{"prompt": "hello"},
					Output:    map[string]any{"answer": "world"},
					Tags:      []string{"deployment:dep-1"},
					CreatedAt: "2026-06-01T12:00:00Z",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/public/dataset-items":
			var body struct {
				ID             string         `json:"id"`
				DatasetName    string         `json:"datasetName"`
				SourceTraceID  string         `json:"sourceTraceId"`
				Input          map[string]any `json:"input"`
				ExpectedOutput map[string]any `json:"expectedOutput"`
				Metadata       map[string]any `json:"metadata"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode dataset item body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body.ID == "" {
				t.Error("dataset item id should be deterministic and non-empty")
			}
			if body.DatasetName != "eval-dep-1" {
				t.Errorf("datasetName = %q, want eval-dep-1", body.DatasetName)
			}
			if body.SourceTraceID != "trace-1" {
				t.Errorf("sourceTraceId = %q, want trace-1", body.SourceTraceID)
			}
			if body.Metadata["judged_by_user_id"] != "user-1" {
				t.Errorf("judged_by_user_id = %v, want user-1", body.Metadata["judged_by_user_id"])
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected Langfuse request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func langfuseJudgeHandlerExpectDelete(t *testing.T, itemID string, deleteCalled *atomic.Bool) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces/trace-1":
			_ = json.NewEncoder(w).Encode(langfuse.TraceDetail{
				Trace: langfuse.Trace{
					ID:        "trace-1",
					Input:     map[string]any{"prompt": "hello"},
					Output:    map[string]any{"answer": "world"},
					Tags:      []string{"deployment:dep-1"},
					CreatedAt: "2026-06-01T12:00:00Z",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/public/dataset-items":
			var body struct {
				ID string `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode dataset item body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body.ID != itemID {
				t.Errorf("dataset item id = %q, want %q", body.ID, itemID)
			}
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/public/dataset-items/"+itemID:
			deleteCalled.Store(true)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected Langfuse request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

type langfuseJudgeOptions struct {
	traceID             string
	tags                []string
	expectDatasetItem   bool
	datasetItemStatus   int
	wantMetadataVerdict float64
	datasetItemCalled   *atomic.Bool
}

func langfuseJudgeHandlerWithOptions(t *testing.T, opts langfuseJudgeOptions) http.HandlerFunc {
	t.Helper()
	if opts.traceID == "" {
		opts.traceID = "trace-1"
	}
	if opts.tags == nil {
		opts.tags = []string{"deployment:dep-1"}
	}
	if opts.datasetItemStatus == 0 {
		opts.datasetItemStatus = http.StatusOK
	}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces/"+opts.traceID:
			_ = json.NewEncoder(w).Encode(langfuse.TraceDetail{
				Trace: langfuse.Trace{
					ID:        opts.traceID,
					Input:     map[string]any{"prompt": "hello"},
					Output:    map[string]any{"answer": "world"},
					Tags:      opts.tags,
					CreatedAt: "2026-06-01T12:00:00Z",
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/public/dataset-items":
			if !opts.expectDatasetItem {
				t.Errorf("unexpected dataset item upsert")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if opts.datasetItemCalled != nil {
				opts.datasetItemCalled.Store(true)
			}
			var body struct {
				ID          string         `json:"id"`
				Metadata    map[string]any `json:"metadata"`
				DatasetName string         `json:"datasetName"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode dataset item body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body.ID == "" {
				t.Error("dataset item id should be deterministic and non-empty")
			}
			if body.DatasetName != "eval-dep-1" {
				t.Errorf("datasetName = %q, want eval-dep-1", body.DatasetName)
			}
			if got, ok := body.Metadata["verdict"].(float64); !ok || got != opts.wantMetadataVerdict {
				t.Errorf("metadata verdict = %v, want %v", body.Metadata["verdict"], opts.wantMetadataVerdict)
			}
			w.WriteHeader(opts.datasetItemStatus)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected Langfuse request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func langfuseMissingTraceHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/public/traces/") {
			http.Error(w, `{"message":"trace not found"}`, http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------------
// GetEvalDataset
// ---------------------------------------------------------------------------

func TestGetEvalDataset_Unauthenticated(t *testing.T) {
	f := setupDatasetRouter(t, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestGetEvalDataset_DeploymentNotFound(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectDeploymentNotFound(f.deployMock, "dep-missing")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-missing/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGetEvalDataset_WrongAccount(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectDeploymentLookup(f.deployMock, "dep-1", "acct-other", "agent", "build-1", "ns-1")
	f.accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-other", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestGetEvalDataset_DatasetNotYetCreated(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetNotFound(f.datasetMock, "dep-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestGetEvalDataset_OK(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "dep-dep-1", 42)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		DatasetName string `json:"dataset_name"`
		ItemCount   int    `json:"item_count"`
		GoodCount   int    `json:"good_count"`
		BadCount    int    `json:"bad_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.DatasetName != "dep-dep-1" {
		t.Errorf("dataset_name = %q, want dep-dep-1", resp.DatasetName)
	}
	if resp.ItemCount != 42 || resp.GoodCount != 42 || resp.BadCount != 0 {
		t.Errorf("counts = item %d / good %d / bad %d, want 42 / 42 / 0",
			resp.ItemCount, resp.GoodCount, resp.BadCount)
	}
}

// ---------------------------------------------------------------------------
// DownloadEvalDataset
// ---------------------------------------------------------------------------

func TestDownloadEvalDataset_Unauthenticated(t *testing.T) {
	f := setupDatasetRouter(t, false, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/download", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestDownloadEvalDataset_DatasetNotFound(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetNotFound(f.datasetMock, "dep-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/download", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestDownloadEvalDataset_OK(t *testing.T) {
	items := []langfuse.DatasetItem{
		{
			ID:                  "di-1",
			Input:               json.RawMessage(`{"q":"hello"}`),
			ExpectedOutput:      json.RawMessage(`{"a":"world"}`),
			Metadata:            json.RawMessage(`{"env":"prod"}`),
			SourceTraceID:       "trace-dl-1",
			SourceObservationID: "obs-dl-1",
			CreatedAt:           "2026-06-01T12:00:00Z",
		},
	}
	f := setupDatasetRouter(t, true, langfuseDatasetItemsHandler(items, 1))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1", 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/download", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="eval-dep-1.zip"` {
		t.Errorf("Content-Disposition = %q, want eval-dep-1.zip attachment", cd)
	}
	if rec.Body.Len() == 0 {
		t.Error("response body should not be empty")
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("open response zip: %v", err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "eval-dep-1.jsonl" {
		t.Fatalf("zip entries = %+v, want eval-dep-1.jsonl", zr.File)
	}
}

func TestDownloadEvalDataset_LangfuseError(t *testing.T) {
	errorUpstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	})
	f := setupDatasetRouter(t, true, errorUpstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "dep-dep-1", 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/download", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	// Streaming: headers are written before Langfuse is queried, so the status
	// is always 200. A Langfuse error closes the connection mid-stream, producing
	// an incomplete (invalid) zip that the client's download manager reports as failed.
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
}

func TestGetEvalDatasetItems_OK(t *testing.T) {
	items := []langfuse.DatasetItem{
		{
			ID:             "di-1",
			Input:          map[string]any{"prompt": "hello"},
			ExpectedOutput: map[string]any{"answer": "world"},
			Metadata:       map[string]any{"verdict": 1},
			SourceTraceID:  "trace-1",
			CreatedAt:      "2026-06-01T12:00:00Z",
		},
	}
	upstream := langfuseDatasetItemsHandler(items, 1)
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?page=2&limit=25", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []struct {
			ID            string         `json:"id"`
			Input         map[string]any `json:"input"`
			SourceTraceID string         `json:"source_trace_id"`
		} `json:"items"`
		TotalItems int `json:"total_items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ID != "di-1" || resp.Items[0].SourceTraceID != "trace-1" {
		t.Fatalf("items = %+v, want one di-1 row", resp.Items)
	}
	if resp.Items[0].Input["prompt"] != "hello" {
		t.Fatalf("input = %+v, want prompt hello", resp.Items[0].Input)
	}
	if resp.TotalItems != 1 {
		t.Fatalf("total_items = %d, want 1", resp.TotalItems)
	}
}

func TestGetDatasetReviewQueue_FiltersJudgedAndAnnotatesSentiment(t *testing.T) {
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
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, len(traces), "", "*"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	f.judgmentMock.ExpectQuery("SELECT trace_id FROM eval_dataset_judgments").
		WithArgs("dataset-dep-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"trace_id"}).AddRow("trace-3"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?limit=3", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp datasetReviewQueueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(resp.Items))
	}
	if resp.Items[0].TraceID != "trace-1" || resp.Items[0].Sentiment != "positive" {
		t.Fatalf("first item = %+v, want trace-1 with positive sentiment", resp.Items[0])
	}
	if resp.Items[1].TraceID != "trace-2" {
		t.Fatalf("second item = %+v, want trace-2", resp.Items[1])
	}
	if _, err := time.Parse(time.RFC3339Nano, resp.EndTime); err != nil {
		t.Fatalf("end_time = %q, want RFC3339 timestamp: %v", resp.EndTime, err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestGetDatasetReviewQueue_UsesOffset(t *testing.T) {
	traces := []langfuse.Trace{
		{
			ID:        "trace-5",
			SessionID: "session-5",
			CreatedAt: "2026-06-01T15:00:00Z",
			Input:     "question 5",
			Output:    "answer 5",
		},
		{
			ID:        "trace-4",
			SessionID: "session-4",
			CreatedAt: "2026-06-01T14:00:00Z",
			Input:     "question 4",
			Output:    "answer 4",
		},
	}
	const endTime = "2026-06-18T20:07:29.702000Z"
	f := setupDatasetRouter(t, true, langfuseTracesHandler(t, traces, 5, "2", endTime))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	f.judgmentMock.ExpectQuery("SELECT trace_id FROM eval_dataset_judgments").
		WithArgs("dataset-dep-1", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"trace_id"}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?offset=2&limit=2&end_time="+endTime, nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp datasetReviewQueueResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.NextOffset != 4 {
		t.Fatalf("next_offset = %d, want 4", resp.NextOffset)
	}
	if resp.EndTime != endTime {
		t.Fatalf("end_time = %q, want %q", resp.EndTime, endTime)
	}
}

func TestGetDatasetReviewQueue_InvalidEndTime(t *testing.T) {
	f := setupDatasetRouter(t, true, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?end_time=bad", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDatasetReviewQueue_RejectsMisalignedOffset(t *testing.T) {
	upstreamCalled := false
	f := setupDatasetRouter(t, true, func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	})
	expectAuthorizedDeployment(f.traceDetailFixture)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?offset=51&limit=50&end_time=2026-06-18T20:07:29.702Z", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamCalled {
		t.Fatal("Langfuse upstream was called for invalid offset")
	}
}

func TestGetDatasetReviewQueue_RequiresEndTimeForNonZeroOffset(t *testing.T) {
	upstreamCalled := false
	f := setupDatasetRouter(t, true, func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	})
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/review-queue?offset=50&limit=50", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamCalled {
		t.Fatal("Langfuse upstream was called without end_time for non-zero offset")
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

func TestAnnotateQueueFiltersNilInput(t *testing.T) {
	items := annotateQueue([]langfuse.Trace{
		{
			ID:        "trace-nil",
			CreatedAt: "2026-06-01T12:00:00Z",
			Input:     nil,
			Output:    "nil input",
		},
		{
			ID:        "trace-valid",
			CreatedAt: "2026-06-01T12:04:00Z",
			Input:     map[string]any{"prompt": "hello"},
			Output:    "valid input",
		},
	}, map[string]bool{})

	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if items[0].TraceID != "trace-valid" {
		t.Fatalf("trace id = %q, want trace-valid", items[0].TraceID)
	}
}

func TestAnnotateQueueIncludesFullInputOutput(t *testing.T) {
	output := strings.Repeat("x", 300)
	items := annotateQueue([]langfuse.Trace{
		{
			ID:        "trace-1",
			CreatedAt: "2026-06-01T12:00:00Z",
			Input:     map[string]any{"prompt": "hello"},
			Output:    output,
		},
	}, map[string]bool{})

	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	input, ok := items[0].Input.(map[string]any)
	if !ok || input["prompt"] != "hello" {
		t.Fatalf("input = %#v, want full input map", items[0].Input)
	}
	if items[0].Output != output {
		t.Fatalf("output was not preserved in full")
	}
}

func TestPostDatasetJudgment_GoodWritesDatasetItemAndReturnsJudgment(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandler(t))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	f.judgmentMock.ExpectExec("INSERT INTO eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1", "good").
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.datasetMock.ExpectExec("UPDATE eval_datasets").
		WithArgs(1, 0, "dataset-dep-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/judgments",
		bytes.NewBufferString(`{"trace_id":"trace-1","verdict":"good"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetJudgmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvalDatasetID != "dataset-dep-1" || resp.TraceID != "trace-1" || resp.Verdict != "good" {
		t.Fatalf("judgment response = %+v, want dataset id / trace id / verdict", resp)
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPostDatasetJudgment_BadWritesDatasetItemAndBumpsBadCount(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		expectDatasetItem:   true,
		wantMetadataVerdict: -1,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	f.judgmentMock.ExpectExec("INSERT INTO eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1", "bad").
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.datasetMock.ExpectExec("UPDATE eval_datasets").
		WithArgs(0, 1, "dataset-dep-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/judgments",
		bytes.NewBufferString(`{"trace_id":"trace-1","verdict":"bad"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetJudgmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvalDatasetID != "dataset-dep-1" || resp.TraceID != "trace-1" || resp.Verdict != "bad" {
		t.Fatalf("judgment response = %+v, want dataset id / trace id / bad verdict", resp)
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPostDatasetJudgment_UnknownRecordsJudgmentWithoutDatasetItem(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	f.judgmentMock.ExpectExec("INSERT INTO eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1", "unknown").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/judgments",
		bytes.NewBufferString(`{"trace_id":"trace-1","verdict":"unknown"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetJudgmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvalDatasetID != "dataset-dep-1" || resp.TraceID != "trace-1" || resp.Verdict != "unknown" {
		t.Fatalf("judgment response = %+v, want dataset id / trace id / unknown verdict", resp)
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPostDatasetJudgment_DuplicateReturnsConflict(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	f.judgmentMock.ExpectExec("INSERT INTO eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1", "good").
		WillReturnResult(sqlmock.NewResult(0, 0))

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/judgments",
		bytes.NewBufferString(`{"trace_id":"trace-1","verdict":"good"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPostDatasetJudgment_WrongDeploymentTagReturnsForbidden(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		tags:              []string{"deployment:other"},
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/judgments",
		bytes.NewBufferString(`{"trace_id":"trace-1","verdict":"good"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostDatasetJudgment_MissingTraceReturnsNotFound(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseMissingTraceHandler())
	expectAuthorizedDeployment(f.traceDetailFixture)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/judgments",
		bytes.NewBufferString(`{"trace_id":"trace-missing","verdict":"good"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostDatasetJudgment_UpsertFailureRollsBackJudgment(t *testing.T) {
	var datasetItemCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		expectDatasetItem:   true,
		datasetItemStatus:   http.StatusInternalServerError,
		wantMetadataVerdict: 1,
		datasetItemCalled:   &datasetItemCalled,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	f.judgmentMock.ExpectExec("INSERT INTO eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1", "good").
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.judgmentMock.ExpectExec("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/judgments",
		bytes.NewBufferString(`{"trace_id":"trace-1","verdict":"good"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if !datasetItemCalled.Load() {
		t.Fatal("expected Langfuse dataset item upsert")
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPostDatasetJudgment_DeletesDatasetItemWhenCountBumpFails(t *testing.T) {
	itemID := hashID("eval-dep-1", "trace-1")
	var deleteCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerExpectDelete(t, itemID, &deleteCalled))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	f.judgmentMock.ExpectExec("INSERT INTO eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1", "good").
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.datasetMock.ExpectExec("UPDATE eval_datasets").
		WithArgs(1, 0, "dataset-dep-1").
		WillReturnError(errors.New("db unavailable"))
	f.judgmentMock.ExpectExec("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/judgments",
		bytes.NewBufferString(`{"trace_id":"trace-1","verdict":"good"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if !deleteCalled.Load() {
		t.Fatal("expected Langfuse dataset item compensation delete")
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}
