package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore/judgmentstoretest"
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
		GetEvalDataset(log, accountStore, deployStore, dsStore, judgmentStore))
	f.router.GET("/api/v1/deployments/:id/dataset/items",
		GetEvalDatasetItems(log, cfg, accountStore, deployStore, dsStore, langfuseStore))
	f.router.GET("/api/v1/deployments/:id/dataset/download",
		DownloadEvalDataset(log, cfg, accountStore, deployStore, dsStore, langfuseStore))
	f.router.GET("/api/v1/deployments/:id/dataset/review-queue",
		GetDatasetReviewQueue(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore, nil))
	f.router.GET("/api/v1/deployments/:id/dataset/predictions/status",
		GetDatasetPredictionStatus(log, accountStore, deployStore, dsStore, judgmentStore))
	f.router.POST("/api/v1/deployments/:id/dataset/judgments",
		PostDatasetJudgment(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))
	f.router.PATCH("/api/v1/deployments/:id/dataset/judgments/:trace_id",
		PatchDatasetJudgment(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))
	f.router.PUT("/api/v1/deployments/:id/dataset/judgments/:trace_id/criteria",
		PutDatasetJudgmentCriteria(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))
	f.router.DELETE("/api/v1/deployments/:id/dataset/judgments/:trace_id",
		DeleteDatasetJudgment(log, cfg, accountStore, deployStore, dsStore, langfuseStore, judgmentStore))

	return &datasetFixture{traceDetailFixture: f, datasetMock: datasetMock, judgmentMock: judgmentMock}
}

func expectDatasetRow(mock sqlmock.Sqlmock, deploymentID, datasetName string, itemCount int) {
	expectDatasetRowCounts(mock, deploymentID, datasetName, itemCount, itemCount, 0)
}

func expectDatasetRowCounts(mock sqlmock.Sqlmock, deploymentID, datasetName string, itemCount, goodCount, badCount int) {
	datasetstoretest.ExpectRow(mock, deploymentID, datasetName, goodCount, badCount)
}

func expectDatasetNotFound(mock sqlmock.Sqlmock, deploymentID string) {
	datasetstoretest.ExpectMissing(mock, deploymentID)
}

type criterionCountRow struct {
	dimension string
	goodCount int
	badCount  int
}

func expectCriterionCounts(mock sqlmock.Sqlmock, evalDatasetID string, rows ...criterionCountRow) {
	dbRows := sqlmock.NewRows([]string{"dimension_key", "good_count", "bad_count"})
	for _, row := range rows {
		dbRows.AddRow(row.dimension, row.goodCount, row.badCount)
	}
	mock.ExpectQuery("FROM eval_dataset_judgment_reasons").
		WithArgs(evalDatasetID).
		WillReturnRows(dbRows)
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

func langfuseDatasetItemsPagesHandler(t *testing.T, pages [][]langfuse.DatasetItem) http.HandlerFunc {
	t.Helper()
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
	totalItems := 0
	for _, pageItems := range pages {
		totalItems += len(pageItems)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				page = parsed
			}
		}
		if got := r.URL.Query().Get("limit"); got != "100" {
			t.Errorf("limit = %q, want 100", got)
		}
		var data []langfuse.DatasetItem
		if page > 0 && page <= len(pages) {
			data = pages[page-1]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp{
			Data: data,
			Meta: meta{Page: page, Limit: 100, TotalItems: totalItems, TotalPages: len(pages)},
		})
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

func expectPredictionStatusCounts(
	mock sqlmock.Sqlmock,
	datasetID string,
	queued, inProgress, completed, failed int,
) {
	mock.ExpectQuery("WITH prediction_states AS").
		WithArgs(datasetID).
		WillReturnRows(sqlmock.NewRows([]string{
			"queued", "in_progress", "completed", "failed",
		}).AddRow(queued, inProgress, completed, failed))
}

func expectDatasetAuthorization(f *datasetFixture, member bool) {
	expectDeploymentLookup(f.deployMock, "dep-1", "acct-1", "sasbot", "build-1", "ns-1")
	count := 0
	if member {
		count = 1
	}
	f.accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func datasetPredictionStatusRequest(router http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/predictions/status",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGetDatasetPredictionStatus(t *testing.T) {
	f := setupDatasetRouter(t, true, http.NotFound)
	expectDatasetAuthorization(f, true)
	datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
	expectPredictionStatusCounts(f.judgmentMock, "dataset-dep-1", 2, 1, 3, 4)

	rec := datasetPredictionStatusRequest(f.router)

	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	var response DatasetPredictionStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response != (DatasetPredictionStatusResponse{
		Queued: 2, InProgress: 1, Completed: 3, Failed: 4,
	}) {
		t.Fatalf("response = %+v", response)
	}
}

func TestGetDatasetPredictionStatusFailures(t *testing.T) {
	t.Run("authentication", func(t *testing.T) {
		f := setupDatasetRouter(t, false, http.NotFound)
		rec := datasetPredictionStatusRequest(f.router)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("deployment", func(t *testing.T) {
		f := setupDatasetRouter(t, true, http.NotFound)
		expectDeploymentNotFound(f.deployMock, "dep-1")
		rec := datasetPredictionStatusRequest(f.router)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("membership", func(t *testing.T) {
		f := setupDatasetRouter(t, true, http.NotFound)
		expectDatasetAuthorization(f, false)
		rec := datasetPredictionStatusRequest(f.router)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("dataset", func(t *testing.T) {
		f := setupDatasetRouter(t, true, http.NotFound)
		expectDatasetAuthorization(f, true)
		datasetstoretest.ExpectMissing(f.datasetMock, "dep-1")
		rec := datasetPredictionStatusRequest(f.router)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("status store", func(t *testing.T) {
		f := setupDatasetRouter(t, true, http.NotFound)
		expectDatasetAuthorization(f, true)
		datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
		f.judgmentMock.ExpectQuery("WITH prediction_states AS").
			WithArgs("dataset-dep-1").
			WillReturnError(errors.New("read failed"))
		rec := datasetPredictionStatusRequest(f.router)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})
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

func langfuseJudgeHandlerExpectDeleteStatus(t *testing.T, itemID string, status int, deleteCalled *atomic.Bool) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/api/public/dataset-items/"+itemID:
			deleteCalled.Store(true)
			w.WriteHeader(status)
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
	wantEmptyCriteria   bool
	wantCriteria        []judgmentCriterion
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
			if opts.wantEmptyCriteria {
				if crit, ok := body.Metadata["judgment_criteria"].([]any); !ok {
					t.Errorf("metadata judgment_criteria = %v, want empty array", body.Metadata["judgment_criteria"])
				} else if len(crit) != 0 {
					t.Errorf("metadata judgment_criteria = %v, want empty", crit)
				}
			}
			if opts.wantCriteria != nil {
				crit, ok := body.Metadata["judgment_criteria"].([]any)
				if !ok || len(crit) != len(opts.wantCriteria) {
					t.Errorf("metadata judgment_criteria = %v, want %d items", body.Metadata["judgment_criteria"], len(opts.wantCriteria))
				} else {
					for i, want := range opts.wantCriteria {
						m, _ := crit[i].(map[string]any)
						if m["dimension_key"] != want.DimensionKey || m["value"] != want.Value {
							t.Errorf("judgment_criteria[%d] = %v, want %+v", i, crit[i], want)
						}
					}
				}
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

func TestGetEvalDataset_CriterionCountsError(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "dep-dep-1", 100, 90, 10)
	f.judgmentMock.ExpectQuery("FROM eval_dataset_judgment_reasons").
		WithArgs(datasetstoretest.ID("dep-1")).
		WillReturnError(errors.New("count failed"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != "failed to get dataset criteria counts" {
		t.Errorf("error = %q, want failed to get dataset criteria counts", resp.Error)
	}
}

func TestGetEvalDataset_OK(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	// 90 good / 10 bad → score 0.9 → grade A.
	expectDatasetRowCounts(f.datasetMock, "dep-1", "dep-dep-1", 100, 90, 10)
	expectCriterionCounts(f.judgmentMock, datasetstoretest.ID("dep-1"),
		criterionCountRow{dimension: "accuracy", goodCount: 12, badCount: 2},
		criterionCountRow{dimension: "tone", goodCount: 4, badCount: 1},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		DatasetName       string  `json:"dataset_name"`
		ItemCount         int     `json:"item_count"`
		GoodCount         int     `json:"good_count"`
		BadCount          int     `json:"bad_count"`
		Grade             string  `json:"grade"`
		NextGrade         string  `json:"next_grade"`
		NextGradeProgress float64 `json:"next_grade_progress"`
		CasesToNextGrade  *int    `json:"cases_to_next_grade"`
		CriteriaCounts    []struct {
			DimensionKey string `json:"dimension_key"`
			GoodCount    int    `json:"good_count"`
			BadCount     int    `json:"bad_count"`
		} `json:"criteria_counts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.DatasetName != "dep-dep-1" {
		t.Errorf("dataset_name = %q, want dep-dep-1", resp.DatasetName)
	}
	if resp.ItemCount != 100 || resp.GoodCount != 90 || resp.BadCount != 10 {
		t.Errorf("counts = item %d / good %d / bad %d, want 100 / 90 / 10",
			resp.ItemCount, resp.GoodCount, resp.BadCount)
	}
	if resp.Grade != "A" {
		t.Errorf("grade = %q, want A", resp.Grade)
	}
	if resp.NextGrade != "" {
		t.Errorf("next_grade = %q, want empty (already at A)", resp.NextGrade)
	}
	if resp.NextGradeProgress != 1 {
		t.Errorf("next_grade_progress = %f, want 1", resp.NextGradeProgress)
	}
	if resp.CasesToNextGrade != nil {
		t.Errorf("cases_to_next_grade = %v, want nil", *resp.CasesToNextGrade)
	}
	if len(resp.CriteriaCounts) != len(judgmentstore.CriterionDimensions) {
		t.Fatalf("criteria_counts len = %d, want %d", len(resp.CriteriaCounts), len(judgmentstore.CriterionDimensions))
	}
	criteriaByDimension := make(map[string]struct {
		goodCount int
		badCount  int
	}, len(resp.CriteriaCounts))
	for _, count := range resp.CriteriaCounts {
		criteriaByDimension[count.DimensionKey] = struct {
			goodCount int
			badCount  int
		}{goodCount: count.GoodCount, badCount: count.BadCount}
	}
	if got := criteriaByDimension["accuracy"]; got.goodCount != 12 || got.badCount != 2 {
		t.Errorf("accuracy criteria counts = good %d / bad %d, want 12 / 2", got.goodCount, got.badCount)
	}
	if got := criteriaByDimension["completeness"]; got.goodCount != 0 || got.badCount != 0 {
		t.Errorf("completeness criteria counts = good %d / bad %d, want 0 / 0", got.goodCount, got.badCount)
	}
	if got := criteriaByDimension["tone"]; got.goodCount != 4 || got.badCount != 1 {
		t.Errorf("tone criteria counts = good %d / bad %d, want 4 / 1", got.goodCount, got.badCount)
	}
}

func TestGetEvalDataset_BelowA(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	// All good / no bad → fcm caps at 0.55, score ≈ 0.55 → grade F, next D.
	expectDatasetRowCounts(f.datasetMock, "dep-1", "dep-dep-1", 100, 100, 0)
	expectCriterionCounts(f.judgmentMock, datasetstoretest.ID("dep-1"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Grade             string  `json:"grade"`
		NextGrade         string  `json:"next_grade"`
		NextGradeProgress float64 `json:"next_grade_progress"`
		CasesToNextGrade  *int    `json:"cases_to_next_grade"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Grade != "F" {
		t.Errorf("grade = %q, want F", resp.Grade)
	}
	if resp.NextGrade != "D" {
		t.Errorf("next_grade = %q, want D", resp.NextGrade)
	}
	if resp.NextGradeProgress <= 0 || resp.NextGradeProgress >= 1 {
		t.Errorf("next_grade_progress = %f, want strictly between 0 and 1", resp.NextGradeProgress)
	}
	if resp.CasesToNextGrade == nil || *resp.CasesToNextGrade != 2 {
		t.Fatalf("cases_to_next_grade = %v, want 2", resp.CasesToNextGrade)
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

func TestGetEvalDatasetItems_FilterByVerdictScansLangfusePages(t *testing.T) {
	upstream := langfuseDatasetItemsPagesHandler(t, [][]langfuse.DatasetItem{
		{
			{
				ID:             "good-1",
				Input:          "good input",
				ExpectedOutput: "good output",
				Metadata:       map[string]any{"verdict": 1},
				SourceTraceID:  "trace-good",
				CreatedAt:      "2026-06-01T12:00:00Z",
			},
		},
		{
			{
				ID:             "bad-1",
				Input:          "bad input 1",
				ExpectedOutput: "bad output 1",
				Metadata:       map[string]any{"verdict": -1},
				SourceTraceID:  "trace-bad-1",
				CreatedAt:      "2026-06-01T12:01:00Z",
			},
			{
				ID:             "bad-2",
				Input:          "bad input 2",
				ExpectedOutput: "bad output 2",
				Metadata:       map[string]any{"verdict": -1},
				SourceTraceID:  "trace-bad-2",
				CreatedAt:      "2026-06-01T12:02:00Z",
			},
		},
	})
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 3, 1, 2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?limit=1&verdict=bad", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []struct {
			ID            string `json:"id"`
			SourceTraceID string `json:"source_trace_id"`
		} `json:"items"`
		Page       int    `json:"page"`
		Limit      int    `json:"limit"`
		TotalItems int    `json:"total_items"`
		TotalPages int    `json:"total_pages"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ID != "bad-1" || resp.Items[0].SourceTraceID != "trace-bad-1" {
		t.Fatalf("items = %+v, want first bad row", resp.Items)
	}
	if resp.Page != 1 || resp.Limit != 1 || resp.TotalItems != 2 || resp.TotalPages != 2 || resp.NextCursor == "" {
		t.Fatalf("pagination = page %d limit %d total_items %d total_pages %d next_cursor %q, want 1/1/2/2 with cursor",
			resp.Page, resp.Limit, resp.TotalItems, resp.TotalPages, resp.NextCursor)
	}

	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 3, 1, 2)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?limit=1&verdict=bad&cursor="+url.QueryEscape(resp.NextCursor), nil)
	rec = httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp = struct {
		Items []struct {
			ID            string `json:"id"`
			SourceTraceID string `json:"source_trace_id"`
		} `json:"items"`
		Page       int    `json:"page"`
		Limit      int    `json:"limit"`
		TotalItems int    `json:"total_items"`
		TotalPages int    `json:"total_pages"`
		NextCursor string `json:"next_cursor"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal cursor page: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ID != "bad-2" || resp.Items[0].SourceTraceID != "trace-bad-2" {
		t.Fatalf("cursor items = %+v, want second bad row", resp.Items)
	}
	if resp.Page != 2 || resp.NextCursor != "" {
		t.Fatalf("cursor page = %d next_cursor = %q, want page 2 with no cursor", resp.Page, resp.NextCursor)
	}
}

func TestGetEvalDatasetItems_FilterByVerdictStopsAtScanLimit(t *testing.T) {
	var calls atomic.Int32
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				page = parsed
			}
		}
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
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp{
			Data: []langfuse.DatasetItem{
				{
					ID:             "good-only",
					Input:          "good input",
					ExpectedOutput: "good output",
					Metadata:       map[string]any{"verdict": 1},
					SourceTraceID:  "trace-good",
					CreatedAt:      "2026-06-01T12:00:00Z",
				},
			},
			Meta: meta{Page: page, Limit: 100, TotalItems: 1, TotalPages: 0},
		})
	})
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 0, 2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?limit=1&verdict=bad", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"next_cursor"`
		TotalItems int              `json:"total_items"`
		TotalPages int              `json:"total_pages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 0 || resp.NextCursor != "" {
		t.Fatalf("response = %+v, want empty partial response with no cursor", resp)
	}
	if resp.TotalItems != 0 || resp.TotalPages != 0 {
		t.Fatalf("pagination = total_items %d total_pages %d, want clamped 0/0", resp.TotalItems, resp.TotalPages)
	}
	if got, want := calls.Load(), int32(3); got != want {
		t.Fatalf("upstream calls = %d, want safety limit %d", got, want)
	}
}

func TestGetEvalDatasetItems_CursorWithoutVerdictRejects(t *testing.T) {
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Langfuse upstream should not be called")
		w.WriteHeader(http.StatusInternalServerError)
	}
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?cursor=abc", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cursor requires verdict") {
		t.Errorf("body = %q, want cursor requires verdict", rec.Body.String())
	}
}

func TestGetEvalDatasetItems_VerdictCursorAndPageRejects(t *testing.T) {
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Langfuse upstream should not be called")
		w.WriteHeader(http.StatusInternalServerError)
	}
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?verdict=good&cursor=abc&page=2", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "page cannot be used with cursor") {
		t.Errorf("body = %q, want page cannot be used with cursor", rec.Body.String())
	}
}

func TestGetEvalDatasetItems_VerdictPageWithoutCursorRejects(t *testing.T) {
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Langfuse upstream should not be called")
		w.WriteHeader(http.StatusInternalServerError)
	}
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?verdict=good&page=2", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "filtered dataset items use cursor pagination") {
		t.Errorf("body = %q, want cursor pagination required", rec.Body.String())
	}
}

func TestGetEvalDatasetItems_UnknownVerdictRejects(t *testing.T) {
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Langfuse upstream should not be called")
		w.WriteHeader(http.StatusInternalServerError)
	}
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?verdict=maybe", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "verdict must be good or bad") {
		t.Errorf("body = %q, want verdict must be good or bad", rec.Body.String())
	}
}

func TestGetEvalDatasetItems_MalformedCursorRejects(t *testing.T) {
	upstream := func(w http.ResponseWriter, _ *http.Request) {
		t.Error("Langfuse upstream should not be called")
		w.WriteHeader(http.StatusInternalServerError)
	}
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

	// "YWJjZA" decodes to "abcd" — valid base64 but not valid cursor JSON.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?verdict=good&cursor=YWJjZA", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid cursor") {
		t.Errorf("body = %q, want invalid cursor", rec.Body.String())
	}
}

func TestGetEvalDatasetItems_MismatchedCursorRejects(t *testing.T) {
	baseCursor := evalDatasetItemsCursor{
		Version:     evalDatasetItemsCursorVersion,
		DatasetName: "eval-dep-1",
		Verdict:     "good",
		Limit:       50,
		RawPage:     1,
		RawIndex:    0,
		Matched:     0,
	}

	mutators := []struct {
		name   string
		mutate func(*evalDatasetItemsCursor)
	}{
		{"wrong dataset", func(c *evalDatasetItemsCursor) { c.DatasetName = "eval-other" }},
		{"wrong verdict", func(c *evalDatasetItemsCursor) { c.Verdict = "bad" }},
		{"wrong limit", func(c *evalDatasetItemsCursor) { c.Limit = 25 }},
	}

	for _, m := range mutators {
		t.Run(m.name, func(t *testing.T) {
			cursor := baseCursor
			m.mutate(&cursor)
			encoded, err := encodeEvalDatasetItemsCursor(cursor)
			if err != nil {
				t.Fatalf("encode cursor: %v", err)
			}

			upstream := func(w http.ResponseWriter, _ *http.Request) {
				t.Error("Langfuse upstream should not be called")
				w.WriteHeader(http.StatusInternalServerError)
			}
			f := setupDatasetRouter(t, true, upstream)
			expectAuthorizedDeployment(f.traceDetailFixture)
			expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)

			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/deployments/dep-1/dataset/items?verdict=good&cursor="+url.QueryEscape(encoded), nil)
			rec := httptest.NewRecorder()
			f.router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "invalid cursor") {
				t.Errorf("body = %q, want invalid cursor", rec.Body.String())
			}
		})
	}
}

func TestGetEvalDatasetItems_FilterByVerdictGoodHappyPath(t *testing.T) {
	upstream := langfuseDatasetItemsPagesHandler(t, [][]langfuse.DatasetItem{
		{
			{
				ID:             "good-1",
				Input:          "good input 1",
				ExpectedOutput: "good output 1",
				Metadata:       map[string]any{"verdict": 1},
				SourceTraceID:  "trace-good-1",
				CreatedAt:      "2026-06-01T12:00:00Z",
			},
			{
				ID:             "bad-1",
				Input:          "bad",
				ExpectedOutput: "bad",
				Metadata:       map[string]any{"verdict": -1},
				SourceTraceID:  "trace-bad",
				CreatedAt:      "2026-06-01T12:01:00Z",
			},
			{
				ID:             "good-2",
				Input:          "good input 2",
				ExpectedOutput: "good output 2",
				Metadata:       map[string]any{"verdict": 1},
				SourceTraceID:  "trace-good-2",
				CreatedAt:      "2026-06-01T12:02:00Z",
			},
		},
	})
	f := setupDatasetRouter(t, true, upstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 3, 2, 1)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/items?verdict=good", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []struct {
			ID            string `json:"id"`
			SourceTraceID string `json:"source_trace_id"`
		} `json:"items"`
		Page       int    `json:"page"`
		Limit      int    `json:"limit"`
		TotalItems int    `json:"total_items"`
		TotalPages int    `json:"total_pages"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items = %+v, want two good rows", resp.Items)
	}
	if resp.Items[0].ID != "good-1" || resp.Items[0].SourceTraceID != "trace-good-1" {
		t.Errorf("first item = %+v, want good-1/trace-good-1", resp.Items[0])
	}
	if resp.Items[1].ID != "good-2" || resp.Items[1].SourceTraceID != "trace-good-2" {
		t.Errorf("second item = %+v, want good-2/trace-good-2", resp.Items[1])
	}
	if resp.Page != 1 || resp.TotalItems != 2 || resp.TotalPages != 1 || resp.NextCursor != "" {
		t.Errorf("pagination = page %d total_items %d total_pages %d next_cursor %q, want 1/2/1/empty",
			resp.Page, resp.TotalItems, resp.TotalPages, resp.NextCursor)
	}
}

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

func TestGetDatasetReviewQueue_IncludesMatchingPrediction(t *testing.T) {
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
		traceHandler(w, r)
	})
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	traceTimestamp := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	judgmentstoretest.ExpectPredictionTracesByVerdict(
		f.judgmentMock,
		"dataset-dep-1",
		judgmentstore.VerdictGood,
		nil,
		reviewQueueDefaultLimit+1,
		judgmentstore.PredictionTrace{TraceID: "trace-1", TraceTimestamp: traceTimestamp},
	)
	judgmentstoretest.ExpectJudgedTraceIDs(f.judgmentMock, "dataset-dep-1")
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
		"/api/v1/deployments/dep-1/dataset/review-queue?prediction=good",
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
		"/api/v1/deployments/dep-1/dataset/review-queue?prediction=none",
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
		Version:          reviewQueueCursorVersion,
		EvalDatasetID:    "dataset-1",
		PredictionFilter: "bad",
		Limit:            25,
		EndTime:          endTime,
		RawPage:          1,
		RawIndex:         0,
		PredictionTime:   "2026-06-17T20:07:29.702Z",
		PredictionTrace:  "trace-17",
	}
	raw, err := encodeReviewQueueCursor(want)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeReviewQueueCursor(raw, "dataset-1", reviewQueuePredictionBad, 25)
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
	var snapshotTimes []string
	upstream := func(w http.ResponseWriter, r *http.Request) {
		snapshotTimes = append(snapshotTimes, r.URL.Query().Get("toTimestamp"))
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
	if len(snapshotTimes) != 2 || snapshotTimes[0] == "" || snapshotTimes[0] != snapshotTimes[1] {
		t.Fatalf("snapshot times = %v, want one stable timestamp", snapshotTimes)
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
	judgmentstoretest.ExpectPredictionTracesByVerdict(
		f.judgmentMock,
		"dataset-dep-1",
		judgmentstore.VerdictGood,
		nil,
		reviewQueueDefaultLimit+1,
	)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue?prediction=good",
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

func TestReviewQueuePredictionMatches(t *testing.T) {
	tests := []struct {
		name       string
		filter     reviewQueuePredictionFilter
		prediction bool
		want       bool
	}{
		{name: "unfiltered without prediction", want: true},
		{name: "verdict filter passes through", filter: reviewQueuePredictionGood, prediction: true, want: true},
		{name: "none", filter: reviewQueuePredictionNone, want: true},
		{name: "none excludes prediction", filter: reviewQueuePredictionNone, prediction: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reviewQueuePredictionMatches(tt.filter, tt.prediction)
			if got != tt.want {
				t.Fatalf("match = %v, want %v", got, tt.want)
			}
		})
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

func TestPatchDatasetJudgment_GoodToBadUpdatesItemAndCounts(t *testing.T) {
	var datasetItemCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:             "trace-1",
		expectDatasetItem:   true,
		wantMetadataVerdict: -1,
		wantEmptyCriteria:   true,
		datasetItemCalled:   &datasetItemCalled,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectSetVerdict(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictBad, judgmentstore.VerdictGood, nil, nil)
	f.datasetMock.ExpectExec("UPDATE eval_datasets").
		WithArgs(-1, 1, "dataset-dep-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-1",
		strings.NewReader(`{"verdict":"bad"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !datasetItemCalled.Load() {
		t.Fatal("expected Langfuse dataset item upsert")
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

func TestPatchDatasetJudgment_RestoresJudgmentWhenDatasetItemUpsertFails(t *testing.T) {
	var datasetItemCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:             "trace-1",
		expectDatasetItem:   true,
		datasetItemStatus:   http.StatusInternalServerError,
		wantMetadataVerdict: -1,
		wantEmptyCriteria:   true,
		datasetItemCalled:   &datasetItemCalled,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectSetVerdict(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictBad, judgmentstore.VerdictGood, nil, nil)
	judgmentstoretest.ExpectSetVerdict(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictGood, judgmentstore.VerdictBad, nil, nil)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-1",
		strings.NewReader(`{"verdict":"bad"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if !datasetItemCalled.Load() {
		t.Fatal("expected Langfuse dataset item upsert attempt")
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPatchDatasetJudgment_RestoresReasonsWhenDatasetItemUpsertFails(t *testing.T) {
	var datasetItemCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:             "trace-1",
		expectDatasetItem:   true,
		datasetItemStatus:   http.StatusInternalServerError,
		wantMetadataVerdict: -1,
		wantEmptyCriteria:   true,
		datasetItemCalled:   &datasetItemCalled,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	accuracy := judgmentstore.Reason{Dimension: judgmentstore.DimensionAccuracy, Value: 1}
	judgmentstoretest.ExpectSetVerdict(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictBad, judgmentstore.VerdictGood, []judgmentstore.Reason{accuracy}, nil)
	judgmentstoretest.ExpectSetVerdict(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictGood, judgmentstore.VerdictBad, nil, []judgmentstore.Reason{accuracy})

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-1",
		strings.NewReader(`{"verdict":"bad"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if !datasetItemCalled.Load() {
		t.Fatal("expected Langfuse dataset item upsert attempt")
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPatchDatasetJudgment_GoodToUnknownDeletesItemAndDecrementsCount(t *testing.T) {
	itemID := hashID("eval-dep-1", "trace-1")
	var deleteCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerExpectDelete(t, itemID, &deleteCalled))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectSetVerdict(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictUnknown, judgmentstore.VerdictGood, nil, nil)
	f.datasetMock.ExpectExec("UPDATE eval_datasets").
		WithArgs(-1, 0, "dataset-dep-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-1",
		strings.NewReader(`{"verdict":"unknown"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !deleteCalled.Load() {
		t.Fatal("expected Langfuse dataset item delete")
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

func TestPatchDatasetJudgment_SameVerdictIsNoOp(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:           "trace-1",
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectSetVerdict(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictBad, judgmentstore.VerdictBad, nil, nil)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-1",
		strings.NewReader(`{"verdict":"bad"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
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

func TestPatchDatasetJudgment_MissingJudgmentReturnsNotFound(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:           "trace-missing",
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectSetVerdictMissing(f.judgmentMock, "dataset-dep-1", "trace-missing", judgmentstore.VerdictBad)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-missing",
		strings.NewReader(`{"verdict":"bad"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPatchDatasetJudgment_CountFailureRestoresCriteriaToLangfuse(t *testing.T) {
	var upserts []map[string]any
	f := setupDatasetRouter(t, true, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces/trace-1":
			_ = json.NewEncoder(w).Encode(langfuse.TraceDetail{Trace: langfuse.Trace{
				ID: "trace-1", Input: map[string]any{"prompt": "x"}, Output: map[string]any{"answer": "y"},
				Tags: []string{"deployment:dep-1"}, CreatedAt: "2026-06-01T12:00:00Z",
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/public/dataset-items":
			var body struct {
				Metadata map[string]any `json:"metadata"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			upserts = append(upserts, body.Metadata)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected Langfuse request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	})
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	accuracy := judgmentstore.Reason{Dimension: judgmentstore.DimensionAccuracy, Value: 1}
	judgmentstoretest.ExpectSetVerdict(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictBad, judgmentstore.VerdictGood, []judgmentstore.Reason{accuracy}, nil)
	f.datasetMock.ExpectExec("UPDATE eval_datasets").
		WithArgs(-1, 1, "dataset-dep-1").
		WillReturnError(errors.New("boom"))
	judgmentstoretest.ExpectSetVerdict(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictGood, judgmentstore.VerdictBad, nil, []judgmentstore.Reason{accuracy})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/deployments/dep-1/dataset/judgments/trace-1", strings.NewReader(`{"verdict":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(upserts) != 2 {
		t.Fatalf("expected 2 Langfuse upserts (forward + compensation), got %d", len(upserts))
	}
	crit, ok := upserts[1]["judgment_criteria"].([]any)
	if !ok || len(crit) != 1 {
		t.Fatalf("compensation judgment_criteria = %v, want 1 item", upserts[1]["judgment_criteria"])
	}
	m, _ := crit[0].(map[string]any)
	if m["dimension_key"] != "accuracy" || m["value"] != 1.0 {
		t.Fatalf("compensation criteria = %v, want accuracy/1", crit[0])
	}
}

func TestPutDatasetJudgmentCriteria_ReplacesCriteria(t *testing.T) {
	var datasetItemCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:             "trace-1",
		expectDatasetItem:   true,
		wantMetadataVerdict: -1,
		wantCriteria:        []judgmentCriterion{{DimensionKey: "tone", Value: -1}},
		datasetItemCalled:   &datasetItemCalled,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectReplaceReasons(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictBad,
		[]judgmentstore.Reason{{Dimension: judgmentstore.DimensionAccuracy, Value: -1}},
		[]judgmentstore.Reason{{Dimension: judgmentstore.DimensionTone, Value: -1}})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep-1/dataset/judgments/trace-1/criteria", strings.NewReader(`{"criteria":[{"dimension_key":"tone","value":-1}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !datasetItemCalled.Load() {
		t.Fatal("expected Langfuse dataset item upsert")
	}
	var resp DatasetJudgmentCriteriaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Verdict != "bad" || len(resp.Criteria) != 1 || resp.Criteria[0] != (judgmentCriterion{DimensionKey: "tone", Value: -1}) {
		t.Fatalf("response = %+v, want bad + tone/-1", resp)
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPutDatasetJudgmentCriteria_EmptyClears(t *testing.T) {
	var datasetItemCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:             "trace-1",
		expectDatasetItem:   true,
		wantMetadataVerdict: -1,
		wantEmptyCriteria:   true,
		datasetItemCalled:   &datasetItemCalled,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectReplaceReasons(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictBad,
		[]judgmentstore.Reason{{Dimension: judgmentstore.DimensionAccuracy, Value: -1}}, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep-1/dataset/judgments/trace-1/criteria", strings.NewReader(`{"criteria":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !datasetItemCalled.Load() {
		t.Fatal("expected Langfuse dataset item upsert")
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPutDatasetJudgmentCriteria_UnknownVerdictReturns409(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:           "trace-1",
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectReplaceReasonsUnknown(f.judgmentMock, "dataset-dep-1", "trace-1")

	req := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep-1/dataset/judgments/trace-1/criteria", strings.NewReader(`{"criteria":[{"dimension_key":"tone","value":-1}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPutDatasetJudgmentCriteria_InvalidCriterionReturns400(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:           "trace-1",
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep-1/dataset/judgments/trace-1/criteria", strings.NewReader(`{"criteria":[{"dimension_key":"nonsense","value":-1}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutDatasetJudgmentCriteria_DuplicateCriterionReturns400(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:           "trace-1",
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep-1/dataset/judgments/trace-1/criteria", strings.NewReader(`{"criteria":[{"dimension_key":"tone","value":-1},{"dimension_key":"tone","value":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutDatasetJudgmentCriteria_MissingValueReturns400(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:           "trace-1",
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep-1/dataset/judgments/trace-1/criteria", strings.NewReader(`{"criteria":[{"dimension_key":"tone"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutDatasetJudgmentCriteria_ValueOutOfRangeReturns400(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:           "trace-1",
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep-1/dataset/judgments/trace-1/criteria", strings.NewReader(`{"criteria":[{"dimension_key":"tone","value":2}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutDatasetJudgmentCriteria_MissingReturns404(t *testing.T) {
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:           "trace-1",
		expectDatasetItem: false,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectReplaceReasonsMissing(f.judgmentMock, "dataset-dep-1", "trace-1")

	req := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep-1/dataset/judgments/trace-1/criteria", strings.NewReader(`{"criteria":[{"dimension_key":"tone","value":-1}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestPutDatasetJudgmentCriteria_RestoresReasonsWhenUpsertFails(t *testing.T) {
	var datasetItemCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerWithOptions(t, langfuseJudgeOptions{
		traceID:             "trace-1",
		expectDatasetItem:   true,
		datasetItemStatus:   http.StatusInternalServerError,
		wantMetadataVerdict: -1,
		wantCriteria:        []judgmentCriterion{{DimensionKey: "tone", Value: -1}},
		datasetItemCalled:   &datasetItemCalled,
	}))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	judgmentstoretest.ExpectReplaceReasons(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictBad,
		[]judgmentstore.Reason{{Dimension: judgmentstore.DimensionAccuracy, Value: -1}},
		[]judgmentstore.Reason{{Dimension: judgmentstore.DimensionTone, Value: -1}})
	judgmentstoretest.ExpectReplaceReasons(f.judgmentMock, "dataset-dep-1", "trace-1", judgmentstore.VerdictBad,
		[]judgmentstore.Reason{{Dimension: judgmentstore.DimensionTone, Value: -1}},
		[]judgmentstore.Reason{{Dimension: judgmentstore.DimensionAccuracy, Value: -1}})

	req := httptest.NewRequest(http.MethodPut, "/api/v1/deployments/dep-1/dataset/judgments/trace-1/criteria", strings.NewReader(`{"criteria":[{"dimension_key":"tone","value":-1}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if !datasetItemCalled.Load() {
		t.Fatal("expected Langfuse dataset item upsert attempt")
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestDeleteDatasetJudgment_GoodDeletesDatasetItemAndDecrementsCount(t *testing.T) {
	itemID := hashID("eval-dep-1", "trace-1")
	var deleteCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerExpectDelete(t, itemID, &deleteCalled))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	f.judgmentMock.ExpectQuery("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("good"))
	f.datasetMock.ExpectExec("UPDATE eval_datasets").
		WithArgs(-1, 0, "dataset-dep-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-1",
		nil,
	)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !deleteCalled.Load() {
		t.Fatal("expected Langfuse dataset item delete")
	}
	var resp DatasetJudgmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvalDatasetID != "dataset-dep-1" || resp.TraceID != "trace-1" || resp.Verdict != "good" {
		t.Fatalf("judgment response = %+v, want dataset id / trace id / good verdict", resp)
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestDeleteDatasetJudgment_RestoresCountsAndJudgmentWhenDatasetItemDeleteFails(t *testing.T) {
	itemID := hashID("eval-dep-1", "trace-1")
	var deleteCalled atomic.Bool
	f := setupDatasetRouter(t, true, langfuseJudgeHandlerExpectDeleteStatus(t, itemID, http.StatusInternalServerError, &deleteCalled))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	f.judgmentMock.ExpectQuery("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("good"))
	f.datasetMock.ExpectExec("UPDATE eval_datasets").
		WithArgs(-1, 0, "dataset-dep-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.datasetMock.ExpectExec("UPDATE eval_datasets").
		WithArgs(1, 0, "dataset-dep-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.judgmentMock.ExpectExec("INSERT INTO eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-1", "good").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-1",
		nil,
	)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if !deleteCalled.Load() {
		t.Fatal("expected Langfuse dataset item delete attempt")
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestDeleteDatasetJudgment_UnknownOnlyDeletesLocalJudgment(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	f.judgmentMock.ExpectQuery("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-unknown").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}).AddRow("unknown"))

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-unknown",
		nil,
	)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetJudgmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvalDatasetID != "dataset-dep-1" || resp.TraceID != "trace-unknown" || resp.Verdict != "unknown" {
		t.Fatalf("judgment response = %+v, want unknown verdict", resp)
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}

func TestDeleteDatasetJudgment_MissingJudgmentReturnsNotFound(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 2, 1, 1)
	f.judgmentMock.ExpectQuery("DELETE FROM eval_dataset_judgments").
		WithArgs("dataset-dep-1", "trace-missing").
		WillReturnRows(sqlmock.NewRows([]string{"verdict"}))

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/deployments/dep-1/dataset/judgments/trace-missing",
		nil,
	)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := f.datasetMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet dataset expectations: %v", err)
	}
	if err := f.judgmentMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet judgment expectations: %v", err)
	}
}
