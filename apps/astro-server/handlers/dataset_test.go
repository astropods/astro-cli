package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/datasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

// ---------------------------------------------------------------------------
// Dataset handler fixture
// ---------------------------------------------------------------------------

type datasetFixture struct {
	*traceDetailFixture
	datasetMock sqlmock.Sqlmock
}

func setupDatasetRouter(t *testing.T, withUser bool, upstreamHandler http.HandlerFunc) *datasetFixture {
	t.Helper()
	f, log, cfg, accountStore, deployStore, langfuseStore := newLangfuseFixture(t, withUser, upstreamHandler)

	datasetDB, datasetMock, _ := sqlmock.New()
	t.Cleanup(func() { datasetDB.Close() })
	dsStore := datasetstore.NewStore(datasetDB)

	f.router.GET("/api/v1/deployments/:id/dataset",
		GetEvalDataset(log, accountStore, deployStore, dsStore))
	f.router.GET("/api/v1/deployments/:id/dataset/download",
		DownloadEvalDataset(log, cfg, accountStore, deployStore, dsStore, langfuseStore))

	return &datasetFixture{traceDetailFixture: f, datasetMock: datasetMock}
}

func expectDatasetRow(mock sqlmock.Sqlmock, deploymentID, datasetName string, itemCount int, lastTraceAt *time.Time) {
	rows := sqlmock.NewRows([]string{
		"deployment_id", "account_id", "langfuse_dataset_name", "item_count", "last_trace_at", "last_sync_attempted_at", "last_synced_at", "created_at", "updated_at",
	}).AddRow(deploymentID, "acct-1", datasetName, itemCount, lastTraceAt, lastTraceAt, lastTraceAt, time.Now(), time.Now())
	mock.ExpectQuery("SELECT .+ FROM eval_datasets").
		WithArgs(deploymentID).
		WillReturnRows(rows)
}

func expectDatasetNotFound(mock sqlmock.Sqlmock, deploymentID string) {
	mock.ExpectQuery("SELECT .+ FROM eval_datasets").
		WithArgs(deploymentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"deployment_id", "account_id", "langfuse_dataset_name", "item_count", "last_trace_at", "last_sync_attempted_at", "last_synced_at", "created_at", "updated_at",
		}))
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
	lastTrace := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "dep-dep-1", 42, &lastTrace)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		DatasetName  string  `json:"dataset_name"`
		LastTraceAt  *string `json:"last_trace_at"`
		LastAttempt  *string `json:"last_sync_attempted_at"`
		LastSyncedAt *string `json:"last_synced_at"`
		ItemCount    int     `json:"item_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.DatasetName != "dep-dep-1" {
		t.Errorf("dataset_name = %q, want dep-dep-1", resp.DatasetName)
	}
	if resp.LastTraceAt == nil {
		t.Error("last_trace_at should not be nil")
	}
	if resp.LastAttempt == nil {
		t.Error("last_sync_attempted_at should not be nil")
	}
	if resp.LastSyncedAt == nil {
		t.Error("last_synced_at should not be nil")
	}
	if resp.ItemCount != 42 {
		t.Errorf("item_count = %d, want 42", resp.ItemCount)
	}
}

func TestGetEvalDataset_NeverSynced(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "dep-dep-1", 0, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		ItemCount   int     `json:"item_count"`
		LastTraceAt *string `json:"last_trace_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ItemCount != 0 {
		t.Errorf("item_count = %d, want 0", resp.ItemCount)
	}
	if resp.LastTraceAt != nil {
		t.Errorf("last_trace_at should be nil when never synced")
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
	expectDatasetRow(f.datasetMock, "dep-1", "dep-dep-1", 1, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset/download", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd == "" {
		t.Error("Content-Disposition should be set")
	}
	if rec.Body.Len() == 0 {
		t.Error("response body should not be empty")
	}
}

func TestDownloadEvalDataset_LangfuseError(t *testing.T) {
	errorUpstream := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
	})
	f := setupDatasetRouter(t, true, errorUpstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRow(f.datasetMock, "dep-1", "dep-dep-1", 0, nil)

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
