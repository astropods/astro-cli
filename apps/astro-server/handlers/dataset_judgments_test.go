package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/evaldataset"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore/judgmentstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

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
		expectDatasetItem: true,
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
		expectDatasetItem: true,
		datasetItemStatus: http.StatusInternalServerError,
		datasetItemCalled: &datasetItemCalled,
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
	itemID := evaldataset.ItemID("eval-dep-1", "trace-1")
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
		traceID:           "trace-1",
		expectDatasetItem: true,
		wantEmptyCriteria: true,
		datasetItemCalled: &datasetItemCalled,
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
		traceID:           "trace-1",
		expectDatasetItem: true,
		datasetItemStatus: http.StatusInternalServerError,
		wantEmptyCriteria: true,
		datasetItemCalled: &datasetItemCalled,
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
		traceID:           "trace-1",
		expectDatasetItem: true,
		datasetItemStatus: http.StatusInternalServerError,
		wantEmptyCriteria: true,
		datasetItemCalled: &datasetItemCalled,
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
	itemID := evaldataset.ItemID("eval-dep-1", "trace-1")
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
		traceID:           "trace-1",
		expectDatasetItem: true,
		wantCriteria:      []judgmentCriterion{{DimensionKey: "tone", Value: -1}},
		datasetItemCalled: &datasetItemCalled,
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
		traceID:           "trace-1",
		expectDatasetItem: true,
		wantEmptyCriteria: true,
		datasetItemCalled: &datasetItemCalled,
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
		traceID:           "trace-1",
		expectDatasetItem: true,
		datasetItemStatus: http.StatusInternalServerError,
		wantCriteria:      []judgmentCriterion{{DimensionKey: "tone", Value: -1}},
		datasetItemCalled: &datasetItemCalled,
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
	itemID := evaldataset.ItemID("eval-dep-1", "trace-1")
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
	itemID := evaldataset.ItemID("eval-dep-1", "trace-1")
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
	traceID           string
	tags              []string
	expectDatasetItem bool
	datasetItemStatus int
	wantEmptyCriteria bool
	wantCriteria      []judgmentCriterion
	datasetItemCalled *atomic.Bool
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
