package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

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
