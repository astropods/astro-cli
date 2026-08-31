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
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
)

func traceCoreHandler(t *testing.T, userID string, tags ...string) http.HandlerFunc {
	t.Helper()
	if len(tags) == 0 {
		tags = []string{"deployment:dep-1"}
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id":     "trace-1",
			"userId": userID,
			"tags":   tags,
			"input":  "the question",
			"output": "the answer",
		}); err != nil {
			t.Errorf("encode trace: %v", err)
		}
	}
}

func traceEvaluationRequest(router http.Handler, traceID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/review-queue/"+traceID+"/evaluation",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func expectEvaluatorResults(mock sqlmock.Sqlmock, runID string, rows func(*sqlmock.Rows)) {
	result := sqlmock.NewRows([]string{
		"evaluator_key", "status", "value_json", "confidence", "explanation", "error_message",
	})
	rows(result)
	mock.ExpectQuery("FROM eval_dataset_evaluator_results").WithArgs(runID).WillReturnRows(result)
}

func decodeTraceEvaluation(t *testing.T, rec *httptest.ResponseRecorder) DatasetTraceEvaluationResponse {
	t.Helper()
	var response DatasetTraceEvaluationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func TestGetTraceEvaluationReturnsEveryEvaluatorInSetOrder(t *testing.T) {
	f := setupDatasetRouter(t, true, traceCoreHandler(t, "user_01HXX_bob"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
	expectPersonalProfilesQuery(f.accountMock, []string{"user_01HXX_bob"}, func(rows *sqlmock.Rows) {
		rows.AddRow("user_01HXX_bob", "bob", "Bob Smith", nil)
	})
	expectLatestRuns(f.runMock, map[string]string{"trace-1": "completed"})
	expectEvaluatorResults(f.runMock, "run-trace-1", func(rows *sqlmock.Rows) {
		rows.AddRow("exposed_pii", "completed", []byte("false"), 0.92, "no personal data", nil)
		rows.AddRow("user_sentiment", "failed", nil, nil, nil, "Evaluation failed. Try again.")
	})

	rec := traceEvaluationRequest(f.router, "trace-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	response := decodeTraceEvaluation(t, rec)
	if response.TraceID != "trace-1" ||
		response.UserID != "user_01HXX_bob" ||
		response.UserDetails == nil ||
		response.UserDetails.DisplayName != "Bob Smith" {
		t.Fatalf("trace identity = %+v / %+v", response.UserID, response.UserDetails)
	}
	if response.EvaluationRef != evalpreset.RefDefaultSet ||
		response.Run == nil ||
		response.Run.Status != "completed" {
		t.Fatalf("response = %+v", response)
	}
	if response.Input != "the question" || response.Output != "the answer" {
		t.Fatalf("trace io = %v / %v", response.Input, response.Output)
	}
	if len(response.Evaluators) != 2 {
		t.Fatalf("evaluators = %d, want only what the run recorded", len(response.Evaluators))
	}
	first := response.Evaluators[0]
	if first.Key != "exposed_pii" || first.Status != "completed" ||
		first.Value != false || first.Explanation != "no personal data" {
		t.Fatalf("first evaluator = %+v", first)
	}
	if first.Label != "Exposed PII" || first.Type != "llm" ||
		first.Output == nil || first.Output.Type != evaluator.OutputBoolean {
		t.Fatalf("first evaluator display metadata = %+v / %+v", first.Label, first.Output)
	}
	sentiment := response.Evaluators[1]
	if sentiment.Key != "user_sentiment" || sentiment.Status != "failed" || sentiment.Error == nil {
		t.Fatalf("sentiment = %+v", sentiment)
	}
}

func TestGetTraceEvaluationOrdersResultsByTheSetNotTheStore(t *testing.T) {
	f := setupDatasetRouter(t, true, traceCoreHandler(t, ""))
	expectAuthorizedDeployment(f.traceDetailFixture)
	datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
	expectLatestRuns(f.runMock, map[string]string{"trace-1": "completed"})
	// The store returns results by evaluator key, which is not the order the
	// evaluation set defines them in.
	expectEvaluatorResults(f.runMock, "run-trace-1", func(rows *sqlmock.Rows) {
		rows.AddRow("claim_grounding", "completed", []byte(`"grounded"`), 0.9, "", nil)
		rows.AddRow("exposed_pii", "completed", []byte("false"), 0.9, "", nil)
		rows.AddRow("user_sentiment", "completed", []byte(`"positive"`), 0.9, "", nil)
	})

	rec := traceEvaluationRequest(f.router, "trace-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	response := decodeTraceEvaluation(t, rec)
	keys := make([]string, 0, len(response.Evaluators))
	for _, result := range response.Evaluators {
		keys = append(keys, result.Key)
	}
	want := []string{"exposed_pii", "claim_grounding", "user_sentiment"}
	if !slices.Equal(keys, want) {
		t.Fatalf("evaluator order = %v, want %v", keys, want)
	}
}

func TestGetTraceEvaluationReportsARunThatHasRecordedNothingYet(t *testing.T) {
	f := setupDatasetRouter(t, true, traceCoreHandler(t, ""))
	expectAuthorizedDeployment(f.traceDetailFixture)
	datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
	expectLatestRuns(f.runMock, map[string]string{"trace-1": "queued"})
	expectEvaluatorResults(f.runMock, "run-trace-1", func(*sqlmock.Rows) {})

	rec := traceEvaluationRequest(f.router, "trace-1")

	response := decodeTraceEvaluation(t, rec)
	if response.Run == nil || response.Run.Status != "queued" || len(response.Evaluators) != 0 {
		t.Fatalf("response = %+v, want a queued run with nothing recorded", response)
	}
}

func TestGetTraceEvaluationReturnsTheTraceBeforeAnyRun(t *testing.T) {
	f := setupDatasetRouter(t, true, traceCoreHandler(t, ""))
	expectAuthorizedDeployment(f.traceDetailFixture)
	datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
	expectLatestRuns(f.runMock, nil)

	rec := traceEvaluationRequest(f.router, "trace-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	response := decodeTraceEvaluation(t, rec)
	if response.Run != nil || response.EvaluationRef != "" || len(response.Evaluators) != 0 {
		t.Fatalf("response = %+v, want the trace with no run", response)
	}
}

func TestGetTraceEvaluationReturnsResultsForAnUnresolvableRef(t *testing.T) {
	f := setupDatasetRouter(t, true, traceCoreHandler(t, ""))
	expectAuthorizedDeployment(f.traceDetailFixture)
	datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
	f.runMock.ExpectQuery("DISTINCT ON").WillReturnRows(
		sqlmock.NewRows([]string{"trace_id", "id", "evaluation_ref", "status", "error_message"}).
			AddRow("trace-1", "run-trace-1", "preset/retired-set", "completed", nil))
	expectEvaluatorResults(f.runMock, "run-trace-1", func(rows *sqlmock.Rows) {
		rows.AddRow("some_old_evaluator", "completed", []byte("true"), 0.5, "why", nil)
	})

	rec := traceEvaluationRequest(f.router, "trace-1")

	response := decodeTraceEvaluation(t, rec)
	if len(response.Evaluators) != 1 || response.Evaluators[0].Key != "some_old_evaluator" {
		t.Fatalf("evaluators = %+v", response.Evaluators)
	}
	if response.Evaluators[0].Label != "" || response.Evaluators[0].Output != nil {
		t.Fatalf("a set this build cannot resolve carries no display metadata: %+v", response.Evaluators[0])
	}
}

func TestGetTraceEvaluationRejectsATraceFromAnotherDeployment(t *testing.T) {
	f := setupDatasetRouter(t, true, traceCoreHandler(t, "", "deployment:other"))
	expectAuthorizedDeployment(f.traceDetailFixture)
	datasetstoretest.ExpectExists(f.datasetMock, "dep-1")

	if rec := traceEvaluationRequest(f.router, "trace-1"); rec.Code != http.StatusNotFound {
		t.Fatalf("response = %d, want 404 for a trace outside the deployment", rec.Code)
	}
}

func TestGetTraceEvaluationFailures(t *testing.T) {
	t.Run("authentication", func(t *testing.T) {
		f := setupDatasetRouter(t, false, http.NotFound)
		if rec := traceEvaluationRequest(f.router, "trace-1"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("response = %d", rec.Code)
		}
	})

	t.Run("membership", func(t *testing.T) {
		f := setupDatasetRouter(t, true, http.NotFound)
		expectDatasetAuthorization(f, false)
		if rec := traceEvaluationRequest(f.router, "trace-1"); rec.Code != http.StatusForbidden {
			t.Fatalf("response = %d", rec.Code)
		}
	})

	t.Run("missing dataset", func(t *testing.T) {
		f := setupDatasetRouter(t, true, traceCoreHandler(t, ""))
		expectAuthorizedDeployment(f.traceDetailFixture)
		datasetstoretest.ExpectMissing(f.datasetMock, "dep-1")
		if rec := traceEvaluationRequest(f.router, "trace-1"); rec.Code != http.StatusNotFound {
			t.Fatalf("response = %d", rec.Code)
		}
	})

	t.Run("run read failure", func(t *testing.T) {
		f := setupDatasetRouter(t, true, traceCoreHandler(t, ""))
		expectAuthorizedDeployment(f.traceDetailFixture)
		datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
		f.runMock.ExpectQuery("DISTINCT ON").WillReturnError(errors.New("db down"))
		if rec := traceEvaluationRequest(f.router, "trace-1"); rec.Code != http.StatusInternalServerError {
			t.Fatalf("response = %d", rec.Code)
		}
	})
}
