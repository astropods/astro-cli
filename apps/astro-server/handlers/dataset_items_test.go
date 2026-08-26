package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/evaldataset"
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
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

const validItemOutputs = `[
	{"key":"exposed_pii","value":false},
	{"key":"leaked_credentials","value":false},
	{"key":"disclosed_system_instructions","value":false},
	{"key":"unnecessary_tool_call","value":true},
	{"key":"claim_grounding","value":"grounded"},
	{"key":"user_sentiment","value":"positive"}
]`

type datasetItemUpsert struct {
	called   atomic.Bool
	metadata map[string]any
	status   int
}

type datasetItemDelete struct {
	called atomic.Bool
	id     string
	status int
}

func langfuseItemDeleteHandler(t *testing.T, del *datasetItemDelete) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.HasPrefix(r.URL.Path, "/api/public/dataset-items/") {
			t.Errorf("unexpected upstream call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		del.called.Store(true)
		del.id = strings.TrimPrefix(r.URL.Path, "/api/public/dataset-items/")
		if del.status != 0 {
			w.WriteHeader(del.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": del.id})
	}
}

func deleteDatasetItem(t *testing.T, f *datasetFixture) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/deployments/dep-1/dataset/items/trace-1", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func langfuseItemHandler(t *testing.T, upsert *datasetItemUpsert) http.HandlerFunc {
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
			upsert.called.Store(true)
			var body struct {
				Metadata map[string]any `json:"metadata"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode dataset item body: %v", err)
			}
			upsert.metadata = body.Metadata
			if upsert.status != 0 {
				w.WriteHeader(upsert.status)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "item-1"})
		default:
			t.Errorf("unexpected upstream call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

func postDatasetItem(t *testing.T, f *datasetFixture, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/items",
		bytes.NewBufferString(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func expectItemInsert(f *datasetFixture, runID any, affected int64) {
	f.itemMock.ExpectBegin()
	f.itemMock.ExpectExec("INSERT INTO eval_dataset_items").
		WithArgs("dataset-dep-1", "trace-1", evalpreset.RefDefaultSet, runID, "user-1").
		WillReturnResult(sqlmock.NewResult(0, affected))
	if affected == 0 {
		f.itemMock.ExpectRollback()
		return
	}
	f.itemMock.ExpectExec("INSERT INTO eval_dataset_item_evaluator_outputs").
		WillReturnResult(sqlmock.NewResult(0, 6))
	f.itemMock.ExpectCommit()
}

func putDatasetItemOutputs(t *testing.T, f *datasetFixture, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/deployments/dep-1/dataset/items/trace-1/evaluator-outputs",
		bytes.NewBufferString(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func expectItemLookup(f *datasetFixture, evaluationRef any) {
	rows := sqlmock.NewRows([]string{"evaluation_ref", "source_evaluation_run_id", "added_by_user_id"})
	if evaluationRef != nil {
		rows.AddRow(evaluationRef, "run-1", "user-1")
	}
	f.itemMock.ExpectQuery("FROM eval_dataset_items").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnRows(rows)
}

func expectOutputReplacement(f *datasetFixture) {
	f.itemMock.ExpectBegin()
	f.itemMock.ExpectExec("UPDATE eval_dataset_items").
		WithArgs("dataset-dep-1", "trace-1", "user-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	f.itemMock.ExpectExec("DELETE FROM eval_dataset_item_evaluator_outputs").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnResult(sqlmock.NewResult(0, 6))
	f.itemMock.ExpectExec("INSERT INTO eval_dataset_item_evaluator_outputs").
		WillReturnResult(sqlmock.NewResult(0, 6))
	f.itemMock.ExpectCommit()
}

func expectItemRemoveQueries(f *datasetFixture, outputKeys []string, deleted *sqlmock.Rows) {
	outputs := sqlmock.NewRows([]string{"evaluator_key", "value_json"})
	for _, key := range outputKeys {
		outputs.AddRow(key, []byte(`false`))
	}
	f.itemMock.ExpectBegin()
	f.itemMock.ExpectQuery("FROM eval_dataset_item_evaluator_outputs").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnRows(outputs)
	f.itemMock.ExpectQuery("DELETE FROM eval_dataset_items").
		WithArgs("dataset-dep-1", "trace-1").
		WillReturnRows(deleted)
}

func deletedItemRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"evaluation_ref", "source_evaluation_run_id", "added_by_user_id"})
}

func expectItemRemove(f *datasetFixture, outputKeys []string, evaluationRef string) {
	expectItemRemoveQueries(f, outputKeys, deletedItemRows().AddRow(evaluationRef, "run-1", "user-1"))
	f.itemMock.ExpectCommit()
}

func expectItemRemoveMissing(f *datasetFixture) {
	expectItemRemoveQueries(f, nil, deletedItemRows())
	f.itemMock.ExpectRollback()
}

func expectRunLookup(f *datasetFixture, datasetID, traceID, evaluationRef, status string) {
	f.runMock.ExpectQuery("FROM eval_dataset_evaluation_runs").
		WithArgs("run-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "eval_dataset_id", "trace_id", "evaluation_ref", "status", "error_message"}).
			AddRow("run-1", datasetID, traceID, evaluationRef, status, nil))
}

func TestPostDatasetItem_WithoutARunStoresEveryOutput(t *testing.T) {
	upsert := &datasetItemUpsert{}
	f := setupDatasetRouter(t, true, langfuseItemHandler(t, upsert))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 0, 0, 0)
	expectItemInsert(f, nil, 1)

	rec := postDatasetItem(t, f, `{"trace_id":"trace-1","evaluator_outputs":`+validItemOutputs+`}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvalDatasetID != "dataset-dep-1" || resp.TraceID != "trace-1" {
		t.Errorf("response = %+v, want dataset-dep-1 / trace-1", resp)
	}
	if resp.EvaluationRef != evalpreset.RefDefaultSet {
		t.Errorf("evaluation_ref = %q, want %q", resp.EvaluationRef, evalpreset.RefDefaultSet)
	}
	if !upsert.called.Load() {
		t.Error("expected a Langfuse dataset item upsert")
	}
	if len(upsert.metadata) != 0 {
		t.Errorf("item metadata = %v, want none; evaluator outputs belong to the item store", upsert.metadata)
	}
	if err := f.itemMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet item expectations: %v", err)
	}
}

func TestPostDatasetItem_AcceptsAnyRunStatusForTheTrace(t *testing.T) {
	for _, status := range []string{"queued", "in_progress", "completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			upsert := &datasetItemUpsert{}
			f := setupDatasetRouter(t, true, langfuseItemHandler(t, upsert))
			expectAuthorizedDeployment(f.traceDetailFixture)
			expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 0, 0, 0)
			expectRunLookup(f, "dataset-dep-1", "trace-1", evalpreset.RefDefaultSet, status)
			expectItemInsert(f, "run-1", 1)

			rec := postDatasetItem(t, f,
				`{"trace_id":"trace-1","evaluation_run_id":"run-1","evaluator_outputs":`+validItemOutputs+`}`)

			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
			}
			if err := f.itemMock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet item expectations: %v", err)
			}
			if err := f.runMock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet run expectations: %v", err)
			}
		})
	}
}

func TestPostDatasetItem_RejectsAMismatchedRun(t *testing.T) {
	tests := []struct {
		name          string
		datasetID     string
		traceID       string
		evaluationRef string
	}{
		{name: "another trace", datasetID: "dataset-dep-1", traceID: "trace-other", evaluationRef: evalpreset.RefDefaultSet},
		{name: "another dataset", datasetID: "dataset-other", traceID: "trace-1", evaluationRef: evalpreset.RefDefaultSet},
		{name: "another evaluation set", datasetID: "dataset-dep-1", traceID: "trace-1", evaluationRef: "preset/retired"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upsert := &datasetItemUpsert{}
			f := setupDatasetRouter(t, true, langfuseItemHandler(t, upsert))
			expectAuthorizedDeployment(f.traceDetailFixture)
			expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 0, 0, 0)
			expectRunLookup(f, test.datasetID, test.traceID, test.evaluationRef, "completed")

			rec := postDatasetItem(t, f,
				`{"trace_id":"trace-1","evaluation_run_id":"run-1","evaluator_outputs":`+validItemOutputs+`}`)

			if rec.Code != http.StatusConflict {
				t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
			}
			if upsert.called.Load() {
				t.Error("a rejected run must not write a Langfuse item")
			}
		})
	}
}

func TestPostDatasetItem_RejectsAnUnknownRun(t *testing.T) {
	upsert := &datasetItemUpsert{}
	f := setupDatasetRouter(t, true, langfuseItemHandler(t, upsert))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 0, 0, 0)
	f.runMock.ExpectQuery("FROM eval_dataset_evaluation_runs").
		WithArgs("run-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "eval_dataset_id", "trace_id", "evaluation_ref", "status", "error_message"}))

	rec := postDatasetItem(t, f,
		`{"trace_id":"trace-1","evaluation_run_id":"run-1","evaluator_outputs":`+validItemOutputs+`}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostDatasetItem_RejectsADuplicate(t *testing.T) {
	upsert := &datasetItemUpsert{}
	f := setupDatasetRouter(t, true, langfuseItemHandler(t, upsert))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 0, 0, 0)
	expectItemInsert(f, nil, 0)

	rec := postDatasetItem(t, f, `{"trace_id":"trace-1","evaluator_outputs":`+validItemOutputs+`}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if upsert.called.Load() {
		t.Error("the duplicate gate must run before the Langfuse write")
	}
}

func TestPostDatasetItem_RollsBackWhenTheLangfuseWriteFails(t *testing.T) {
	upsert := &datasetItemUpsert{status: http.StatusInternalServerError}
	f := setupDatasetRouter(t, true, langfuseItemHandler(t, upsert))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 0, 0, 0)
	expectItemInsert(f, nil, 1)
	expectItemRemove(f, []string{"exposed_pii"}, evalpreset.RefDefaultSet)

	rec := postDatasetItem(t, f, `{"trace_id":"trace-1","evaluator_outputs":`+validItemOutputs+`}`)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := f.itemMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet item expectations: %v", err)
	}
}

func setupItemOutputsRouter(t *testing.T) *datasetFixture {
	t.Helper()
	noUpstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected upstream call %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})
	f := setupDatasetRouter(t, true, noUpstream)
	expectAuthorizedDeployment(f.traceDetailFixture)
	return f
}

func TestPutDatasetItemOutputs_ReplacesEveryOutput(t *testing.T) {
	f := setupItemOutputsRouter(t)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectItemLookup(f, evalpreset.RefDefaultSet)
	expectOutputReplacement(f)

	rec := putDatasetItemOutputs(t, f, `{"values":`+validItemOutputs+`}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetItemOutputsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvalDatasetID != "dataset-dep-1" || resp.TraceID != "trace-1" {
		t.Errorf("response = %+v, want dataset-dep-1 / trace-1", resp)
	}
	if len(resp.EvaluatorOutputs) != 6 {
		t.Errorf("evaluator_outputs len = %d, want 6", len(resp.EvaluatorOutputs))
	}
	if resp.VerifiedByUserID != "user-1" {
		t.Errorf("verified_by_user_id = %q, want user-1", resp.VerifiedByUserID)
	}
	if err := f.itemMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet item expectations: %v", err)
	}
}

func TestPutDatasetItemOutputs_RejectsAnUnknownItem(t *testing.T) {
	f := setupItemOutputsRouter(t)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 0, 0, 0)
	expectItemLookup(f, nil)

	rec := putDatasetItemOutputs(t, f, `{"values":`+validItemOutputs+`}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutDatasetItemOutputs_RejectsARetiredEvaluationSet(t *testing.T) {
	f := setupItemOutputsRouter(t)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectItemLookup(f, "preset/retired")

	rec := putDatasetItemOutputs(t, f, `{"values":`+validItemOutputs+`}`)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutDatasetItemOutputs_RejectsInvalidOutputs(t *testing.T) {
	f := setupItemOutputsRouter(t)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectItemLookup(f, evalpreset.RefDefaultSet)

	rec := putDatasetItemOutputs(t, f, `{"values":[{"key":"exposed_pii","value":"nope"}]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteDatasetItem_RemovesTheItemAndItsLangfuseItem(t *testing.T) {
	del := &datasetItemDelete{}
	f := setupDatasetRouter(t, true, langfuseItemDeleteHandler(t, del))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectItemRemove(f, []string{"exposed_pii", "user_sentiment"}, evalpreset.RefDefaultSet)

	rec := deleteDatasetItem(t, f)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DatasetItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvalDatasetID != "dataset-dep-1" || resp.TraceID != "trace-1" {
		t.Errorf("response = %+v, want dataset-dep-1 / trace-1", resp)
	}
	if resp.EvaluationRef != evalpreset.RefDefaultSet {
		t.Errorf("evaluation_ref = %q, want %q", resp.EvaluationRef, evalpreset.RefDefaultSet)
	}
	if !del.called.Load() {
		t.Error("expected a Langfuse dataset item delete")
	}
	if want := evaldataset.ItemID("eval-dep-1", "trace-1"); del.id != want {
		t.Errorf("deleted item id = %q, want %q", del.id, want)
	}
	if err := f.itemMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet item expectations: %v", err)
	}
}

func TestDeleteDatasetItem_RemovesAnItemOnARetiredEvaluationSet(t *testing.T) {
	del := &datasetItemDelete{}
	f := setupDatasetRouter(t, true, langfuseItemDeleteHandler(t, del))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectItemRemove(f, []string{"exposed_pii"}, "preset/retired")

	rec := deleteDatasetItem(t, f)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !del.called.Load() {
		t.Error("expected a Langfuse dataset item delete")
	}
}

func TestDeleteDatasetItem_DeletesTheLangfuseItemWithoutALocalRow(t *testing.T) {
	del := &datasetItemDelete{}
	f := setupDatasetRouter(t, true, langfuseItemDeleteHandler(t, del))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectItemRemoveMissing(f)

	rec := deleteDatasetItem(t, f)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !del.called.Load() {
		t.Error("expected a Langfuse dataset item delete")
	}
	var resp DatasetItemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.EvaluationRef != "" {
		t.Errorf("evaluation_ref = %q, want empty", resp.EvaluationRef)
	}
}

func TestDeleteDatasetItem_RejectsATraceThatIsNotInTheDataset(t *testing.T) {
	del := &datasetItemDelete{status: http.StatusNotFound}
	f := setupDatasetRouter(t, true, langfuseItemDeleteHandler(t, del))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectItemRemoveMissing(f)

	rec := deleteDatasetItem(t, f)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteDatasetItem_RestoresTheItemWhenTheLangfuseDeleteFails(t *testing.T) {
	del := &datasetItemDelete{status: http.StatusInternalServerError}
	f := setupDatasetRouter(t, true, langfuseItemDeleteHandler(t, del))
	expectAuthorizedDeployment(f.traceDetailFixture)
	expectDatasetRowCounts(f.datasetMock, "dep-1", "eval-dep-1", 1, 1, 0)
	expectItemRemove(f, []string{"exposed_pii"}, evalpreset.RefDefaultSet)
	expectItemInsert(f, "run-1", 1)

	rec := deleteDatasetItem(t, f)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := f.itemMock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet item expectations: %v", err)
	}
}

func TestPostDatasetItem_RejectsInvalidOutputs(t *testing.T) {
	tests := []struct {
		name    string
		outputs string
	}{
		{
			name:    "missing evaluator",
			outputs: `[{"key":"exposed_pii","value":false}]`,
		},
		{
			name: "unknown evaluator",
			outputs: `[
				{"key":"exposed_pii","value":false},
				{"key":"leaked_credentials","value":false},
				{"key":"disclosed_system_instructions","value":false},
				{"key":"unnecessary_tool_call","value":true},
				{"key":"claim_grounding","value":"grounded"},
				{"key":"user_sentiment","value":"positive"},
				{"key":"not_an_evaluator","value":true}
			]`,
		},
		{
			name: "wrong value type",
			outputs: `[
				{"key":"exposed_pii","value":"nope"},
				{"key":"leaked_credentials","value":false},
				{"key":"disclosed_system_instructions","value":false},
				{"key":"unnecessary_tool_call","value":true},
				{"key":"claim_grounding","value":"grounded"},
				{"key":"user_sentiment","value":"positive"}
			]`,
		},
		{
			name: "value outside the enum",
			outputs: `[
				{"key":"exposed_pii","value":false},
				{"key":"leaked_credentials","value":false},
				{"key":"disclosed_system_instructions","value":false},
				{"key":"unnecessary_tool_call","value":true},
				{"key":"claim_grounding","value":"grounded"},
				{"key":"user_sentiment","value":"ecstatic"}
			]`,
		},
		{
			name: "omitted value",
			outputs: `[
				{"key":"exposed_pii"},
				{"key":"leaked_credentials","value":false},
				{"key":"disclosed_system_instructions","value":false},
				{"key":"unnecessary_tool_call","value":true},
				{"key":"claim_grounding","value":"grounded"},
				{"key":"user_sentiment","value":"positive"}
			]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upsert := &datasetItemUpsert{}
			f := setupDatasetRouter(t, true, langfuseItemHandler(t, upsert))
			expectAuthorizedDeployment(f.traceDetailFixture)

			rec := postDatasetItem(t, f, `{"trace_id":"trace-1","evaluator_outputs":`+test.outputs+`}`)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			if upsert.called.Load() {
				t.Error("an invalid request must not write a Langfuse item")
			}
		})
	}
}
