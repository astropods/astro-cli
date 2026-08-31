package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func dismissalRequest(t *testing.T, f *datasetFixture, method, traceID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/deployments/dep-1/dataset/review-queue/"+traceID+"/dismiss", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func decodeDismissalResponse(t *testing.T, rec *httptest.ResponseRecorder) ReviewQueueDismissalResponse {
	t.Helper()
	var resp ReviewQueueDismissalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp
}

func TestPostReviewQueueDismissal_DismissesTheTrace(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectDatasetAuthorization(f, true)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	f.dismissalMock.ExpectQuery("INSERT INTO eval_dataset_dismissed_traces").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"is_item"}).AddRow(false))

	rec := dismissalRequest(t, f, http.MethodPost, "trace-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	resp := decodeDismissalResponse(t, rec)
	if resp.EvalDatasetID != "dataset-dep-1" || resp.TraceID != "trace-1" || !resp.Dismissed {
		t.Fatalf("response = %+v", resp)
	}
	if err := f.dismissalMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("dismissal expectations: %v", err)
	}
}

func TestPostReviewQueueDismissal_RejectsADatasetItem(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectDatasetAuthorization(f, true)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	f.dismissalMock.ExpectQuery("INSERT INTO eval_dataset_dismissed_traces").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"is_item"}).AddRow(true))

	if rec := dismissalRequest(t, f, http.MethodPost, "trace-1"); rec.Code != http.StatusConflict {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPostReviewQueueDismissal_SucceedsWhenAlreadyDismissed(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectDatasetAuthorization(f, true)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	f.dismissalMock.ExpectQuery("INSERT INTO eval_dataset_dismissed_traces").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnRows(sqlmock.NewRows([]string{"is_item"}).AddRow(false))

	if rec := dismissalRequest(t, f, http.MethodPost, "trace-1"); rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPostReviewQueueDismissal_RequiresADataset(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectDatasetAuthorization(f, true)
	expectDatasetNotFound(f.datasetMock, "dep-1")

	if rec := dismissalRequest(t, f, http.MethodPost, "trace-1"); rec.Code != http.StatusNotFound {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPostReviewQueueDismissal_RequiresMembership(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectDatasetAuthorization(f, false)

	if rec := dismissalRequest(t, f, http.MethodPost, "trace-1"); rec.Code != http.StatusForbidden {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteReviewQueueDismissal_RestoresTheTrace(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectDatasetAuthorization(f, true)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	f.dismissalMock.ExpectExec("DELETE FROM eval_dataset_dismissed_traces").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	rec := dismissalRequest(t, f, http.MethodDelete, "trace-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if resp := decodeDismissalResponse(t, rec); resp.Dismissed || resp.TraceID != "trace-1" {
		t.Fatalf("response = %+v", resp)
	}
	if err := f.dismissalMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("dismissal expectations: %v", err)
	}
}

func TestDeleteReviewQueueDismissal_SucceedsWhenNotDismissed(t *testing.T) {
	f := setupDatasetRouter(t, true, nil)
	expectDatasetAuthorization(f, true)
	expectDatasetRow(f.datasetMock, "dep-1", "eval-dep-1")
	f.dismissalMock.ExpectExec("DELETE FROM eval_dataset_dismissed_traces").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if rec := dismissalRequest(t, f, http.MethodDelete, "trace-1"); rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}
