package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
)

type valueCountRow struct {
	evaluatorKey string
	value        string
	count        int
}

func expectItemCount(mock sqlmock.Sqlmock, evalDatasetID string, count int) {
	mock.ExpectQuery("count\\(\\*\\)\\s+FROM eval_dataset_items").
		WithArgs(evalDatasetID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func expectOutputValueCounts(mock sqlmock.Sqlmock, evalDatasetID string, rows ...valueCountRow) {
	dbRows := sqlmock.NewRows([]string{"evaluator_key", "value_json", "count"})
	for _, row := range rows {
		dbRows.AddRow(row.evaluatorKey, []byte(row.value), row.count)
	}
	mock.ExpectQuery("FROM eval_dataset_item_evaluator_outputs").
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

func TestGetEvalDataset_ItemCountError(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "dep-dep-1", 100, 90, 10)
	f.itemMock.ExpectQuery("FROM eval_dataset_items").
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
	if resp.Error != "failed to count dataset items" {
		t.Errorf("error = %q, want failed to count dataset items", resp.Error)
	}
}

func TestGetEvalDataset_OutputValueCountsError(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "dep-dep-1", 100, 90, 10)
	expectItemCount(f.itemMock, datasetstoretest.ID("dep-1"), 40)
	f.itemMock.ExpectQuery("FROM eval_dataset_item_evaluator_outputs").
		WithArgs(datasetstoretest.ID("dep-1")).
		WillReturnError(errors.New("count failed"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetEvalDataset_OK(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "dep-dep-1", 100, 90, 10)
	expectItemCount(f.itemMock, datasetstoretest.ID("dep-1"), 40)
	expectOutputValueCounts(f.itemMock, datasetstoretest.ID("dep-1"),
		valueCountRow{evaluatorKey: "exposed_pii", value: "false", count: 38},
		valueCountRow{evaluatorKey: "exposed_pii", value: "true", count: 2},
		valueCountRow{evaluatorKey: "user_sentiment", value: `"negative"`, count: 9},
		valueCountRow{evaluatorKey: "user_sentiment", value: `"positive"`, count: 31},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp evalDatasetSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.DatasetName != "dep-dep-1" {
		t.Errorf("dataset_name = %q, want dep-dep-1", resp.DatasetName)
	}
	if resp.ItemCount != 40 {
		t.Errorf("item_count = %d, want 40", resp.ItemCount)
	}
	if len(resp.Evaluators) != 2 {
		t.Fatalf("evaluators = %+v, want one entry per evaluator key", resp.Evaluators)
	}
	if resp.Evaluators[0].Key != "exposed_pii" || len(resp.Evaluators[0].Distribution) != 2 {
		t.Fatalf("first evaluator = %+v", resp.Evaluators[0])
	}
	if resp.Evaluators[0].Label != "Exposed PII" {
		t.Errorf("first label = %q, want Exposed PII", resp.Evaluators[0].Label)
	}
	if got := resp.Evaluators[0].Distribution[0]; string(got.Value) != "false" || got.Count != 38 {
		t.Errorf("first value = %+v, want false / 38", got)
	}
	if resp.Evaluators[1].Key != "user_sentiment" || len(resp.Evaluators[1].Distribution) != 2 {
		t.Fatalf("second evaluator = %+v", resp.Evaluators[1])
	}
	if got := resp.Evaluators[1].Distribution[1]; string(got.Value) != `"positive"` || got.Count != 31 {
		t.Errorf("second value = %+v, want positive / 31", got)
	}
}

func TestGetEvalDataset_OrdersEvaluatorsBySetAndKeepsRetiredOnes(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "dep-dep-1", 10, 10, 0)
	expectItemCount(f.itemMock, datasetstoretest.ID("dep-1"), 10)
	expectOutputValueCounts(f.itemMock, datasetstoretest.ID("dep-1"),
		valueCountRow{evaluatorKey: "retired_check", value: "true", count: 1},
		valueCountRow{evaluatorKey: "user_sentiment", value: `"positive"`, count: 4},
		valueCountRow{evaluatorKey: "exposed_pii", value: "false", count: 5},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp evalDatasetSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	keys := make([]string, 0, len(resp.Evaluators))
	labels := make([]string, 0, len(resp.Evaluators))
	for _, evaluator := range resp.Evaluators {
		keys = append(keys, evaluator.Key)
		labels = append(labels, evaluator.Label)
	}
	if want := []string{"exposed_pii", "user_sentiment", "retired_check"}; !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	if want := []string{"Exposed PII", "User sentiment", "retired_check"}; !slices.Equal(labels, want) {
		t.Errorf("labels = %v, want %v", labels, want)
	}
}

func TestGetEvalDataset_EmptyDataset(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "dep-dep-1", 0, 0, 0)
	expectItemCount(f.itemMock, datasetstoretest.ID("dep-1"), 0)
	expectOutputValueCounts(f.itemMock, datasetstoretest.ID("dep-1"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deployments/dep-1/dataset", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp evalDatasetSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ItemCount != 0 || len(resp.Evaluators) != 0 {
		t.Fatalf("response = %+v, want no items and no evaluators", resp)
	}
}

// ---------------------------------------------------------------------------
// DownloadEvalDataset
// ---------------------------------------------------------------------------
