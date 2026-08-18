package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
)

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
		DatasetName    string `json:"dataset_name"`
		ItemCount      int    `json:"item_count"`
		CriteriaCounts []struct {
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
	if resp.ItemCount != 100 {
		t.Errorf("item_count = %d, want 100", resp.ItemCount)
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

// ---------------------------------------------------------------------------
// DownloadEvalDataset
// ---------------------------------------------------------------------------
