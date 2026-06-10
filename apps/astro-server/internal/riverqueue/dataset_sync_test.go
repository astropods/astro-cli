package riverqueue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func TestDatasetSyncArgsInsertOpts(t *testing.T) {
	opts := DatasetSyncArgs{}.InsertOpts()

	if opts.MaxAttempts != 0 {
		t.Fatalf("MaxAttempts = %d, want River default 0", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("UniqueOpts.ByArgs must be true")
	}

	wantStates := map[rivertype.JobState]bool{
		rivertype.JobStateAvailable: true,
		rivertype.JobStatePending:   true,
		rivertype.JobStateRetryable: true,
		rivertype.JobStateRunning:   true,
		rivertype.JobStateScheduled: true,
	}
	if len(opts.UniqueOpts.ByState) != len(wantStates) {
		t.Fatalf("UniqueOpts.ByState length = %d, want %d", len(opts.UniqueOpts.ByState), len(wantStates))
	}
	for _, state := range opts.UniqueOpts.ByState {
		if !wantStates[state] {
			t.Fatalf("unexpected unique state %q", state)
		}
	}
}

func TestDeterministicDatasetItemID(t *testing.T) {
	got := deterministicDatasetItemID("dep-test", "trace-1")
	if got != deterministicDatasetItemID("dep-test", "trace-1") {
		t.Fatal("deterministicDatasetItemID is not stable")
	}

	if !strings.HasPrefix(got, "astro-") {
		t.Fatalf("id = %q, want astro- prefix", got)
	}
	if len(got) != len("astro-")+64 {
		t.Fatalf("id length = %d, want %d", len(got), len("astro-")+64)
	}

	if got == deterministicDatasetItemID("dep-test", "trace-2") {
		t.Fatal("different trace IDs must produce different dataset item IDs")
	}
	if got == deterministicDatasetItemID("dep-other", "trace-1") {
		t.Fatal("different dataset names must produce different dataset item IDs")
	}
	if strings.Contains(got, "trace-1") || strings.Contains(got, "dep-test") {
		t.Fatalf("id = %q, should not embed raw dataset or trace IDs", got)
	}
}

func TestShouldSkipDatasetTrace(t *testing.T) {
	if !shouldSkipDatasetTrace(langfuse.Trace{Input: nil}) {
		t.Fatal("nil input trace should be skipped")
	}
	if shouldSkipDatasetTrace(langfuse.Trace{Input: map[string]any{}}) {
		t.Fatal("empty object input trace should not be skipped")
	}
	if shouldSkipDatasetTrace(langfuse.Trace{Input: ""}) {
		t.Fatal("empty string input trace should not be skipped")
	}
}

func TestDatasetSyncWorker_FinalizesAfterTracePageError(t *testing.T) {
	const depID = "dep-1"
	const accountID = "acct-1"
	const datasetName = "dep-dep-1"

	depMock, depStore := newDeploymentMock(t)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := newDatasetMock(t)
	expectDeploymentByID(depMock, depID, accountID)
	expectLangfuseCreds(lfMock, accountID)
	expectDatasetExists(dsMock, depID)

	traceAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	(*dsMock).ExpectExec("UPDATE eval_datasets").
		WithArgs(1, traceAt, false, depID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var datasetItemWrites int
	var countRefreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces" && r.URL.Query().Get("page") == "":
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{{
					"id":        "trace-1",
					"input":     map[string]any{"prompt": "hello"},
					"output":    map[string]any{"answer": "world"},
					"metadata":  map[string]any{"source": "test"},
					"createdAt": traceAt.Format(time.RFC3339),
				}},
				"meta": map[string]any{
					"page":       1,
					"limit":      50,
					"totalItems": 2,
					"totalPages": 2,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/public/dataset-items":
			datasetItemWrites++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode dataset item body: %v", err)
			}
			if body["id"] == "" {
				t.Fatal("dataset item id must be set")
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces" && r.URL.Query().Get("page") == "2":
			http.Error(w, "trace page failed", http.StatusGatewayTimeout)
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/dataset-items":
			countRefreshes++
			if got := r.URL.Query().Get("datasetName"); got != datasetName {
				t.Fatalf("datasetName = %q, want %q", got, datasetName)
			}
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{},
				"meta": map[string]any{
					"page":       1,
					"limit":      1,
					"totalItems": 1,
					"totalPages": 1,
				},
			})
		default:
			t.Fatalf("unexpected Langfuse request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	worker := &DatasetSyncWorker{
		deploymentStore: depStore,
		datasetStore:    dsStore,
		langfuseStore:   lfStore,
		langfuseBaseURL: srv.URL,
		log:             logger.New("error", "json"),
	}
	err := worker.Work(context.Background(), &river.Job[DatasetSyncArgs]{
		JobRow: &rivertype.JobRow{ID: 1, Attempt: 1, MaxAttempts: 25},
		Args:   DatasetSyncArgs{DeploymentID: depID},
	})
	if err == nil {
		t.Fatal("expected trace page error")
	}
	if datasetItemWrites != 1 {
		t.Fatalf("dataset item writes = %d, want 1", datasetItemWrites)
	}
	if countRefreshes != 1 {
		t.Fatalf("count refreshes = %d, want 1", countRefreshes)
	}
	assertSQLExpectationsMet(t, depMock, lfMock, dsMock)
}

func TestDatasetSyncWorker_DoesNotAdvanceLastTraceAtAfterFailedWrite(t *testing.T) {
	const depID = "dep-write-fail"
	const accountID = "acct-1"
	const datasetName = "dep-dep-write-fail"

	depMock, depStore := newDeploymentMock(t)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := newDatasetMock(t)
	expectDeploymentByID(depMock, depID, accountID)
	expectLangfuseCreds(lfMock, accountID)
	expectDatasetExists(dsMock, depID)

	traceAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	(*dsMock).ExpectExec("UPDATE eval_datasets").
		WithArgs(0, nil, true, depID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var datasetItemWrites int
	var countRefreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces" && r.URL.Query().Get("page") == "":
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{{
					"id":        "trace-fail",
					"input":     map[string]any{"prompt": "hello"},
					"output":    map[string]any{"answer": "world"},
					"metadata":  map[string]any{"source": "test"},
					"createdAt": traceAt.Format(time.RFC3339),
				}},
				"meta": map[string]any{
					"page":       1,
					"limit":      50,
					"totalItems": 1,
					"totalPages": 1,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/public/dataset-items":
			datasetItemWrites++
			http.Error(w, "upsert failed", http.StatusGatewayTimeout)
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/dataset-items":
			countRefreshes++
			if got := r.URL.Query().Get("datasetName"); got != datasetName {
				t.Fatalf("datasetName = %q, want %q", got, datasetName)
			}
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{},
				"meta": map[string]any{
					"page":       1,
					"limit":      1,
					"totalItems": 0,
					"totalPages": 0,
				},
			})
		default:
			t.Fatalf("unexpected Langfuse request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	worker := &DatasetSyncWorker{
		deploymentStore: depStore,
		datasetStore:    dsStore,
		langfuseStore:   lfStore,
		langfuseBaseURL: srv.URL,
		log:             logger.New("error", "json"),
	}
	if err := worker.Work(context.Background(), &river.Job[DatasetSyncArgs]{
		JobRow: &rivertype.JobRow{ID: 1, Attempt: 1, MaxAttempts: 25},
		Args:   DatasetSyncArgs{DeploymentID: depID},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if datasetItemWrites != 1 {
		t.Fatalf("dataset item writes = %d, want 1", datasetItemWrites)
	}
	if countRefreshes != 1 {
		t.Fatalf("count refreshes = %d, want 1", countRefreshes)
	}
	assertSQLExpectationsMet(t, depMock, lfMock, dsMock)
}

func TestDatasetSyncWorker_AdvancesLastTraceAtAfterPermanentWriteFailure(t *testing.T) {
	const depID = "dep-write-bad-request"
	const accountID = "acct-1"
	const datasetName = "dep-dep-write-bad-request"

	depMock, depStore := newDeploymentMock(t)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := newDatasetMock(t)
	expectDeploymentByID(depMock, depID, accountID)
	expectLangfuseCreds(lfMock, accountID)
	expectDatasetExists(dsMock, depID)

	traceAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	(*dsMock).ExpectExec("UPDATE eval_datasets").
		WithArgs(0, traceAt, true, depID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var datasetItemWrites int
	var countRefreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces" && r.URL.Query().Get("page") == "":
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{{
					"id":        "trace-bad-request",
					"input":     map[string]any{"prompt": "hello"},
					"output":    map[string]any{"answer": "world"},
					"metadata":  map[string]any{"source": "test"},
					"createdAt": traceAt.Format(time.RFC3339),
				}},
				"meta": map[string]any{
					"page":       1,
					"limit":      50,
					"totalItems": 1,
					"totalPages": 1,
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/public/dataset-items":
			datasetItemWrites++
			http.Error(w, `{"message":"Dataset item validation failed"}`, http.StatusBadRequest)
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/dataset-items":
			countRefreshes++
			if got := r.URL.Query().Get("datasetName"); got != datasetName {
				t.Fatalf("datasetName = %q, want %q", got, datasetName)
			}
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{},
				"meta": map[string]any{
					"page":       1,
					"limit":      1,
					"totalItems": 0,
					"totalPages": 0,
				},
			})
		default:
			t.Fatalf("unexpected Langfuse request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	worker := &DatasetSyncWorker{
		deploymentStore: depStore,
		datasetStore:    dsStore,
		langfuseStore:   lfStore,
		langfuseBaseURL: srv.URL,
		log:             logger.New("error", "json"),
	}
	if err := worker.Work(context.Background(), &river.Job[DatasetSyncArgs]{
		JobRow: &rivertype.JobRow{ID: 1, Attempt: 1, MaxAttempts: 25},
		Args:   DatasetSyncArgs{DeploymentID: depID},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if datasetItemWrites != 1 {
		t.Fatalf("dataset item writes = %d, want 1", datasetItemWrites)
	}
	if countRefreshes != 1 {
		t.Fatalf("count refreshes = %d, want 1", countRefreshes)
	}
	assertSQLExpectationsMet(t, depMock, lfMock, dsMock)
}

func TestDatasetSyncWorker_AdvancesLastTraceAtAfterSkippedTrace(t *testing.T) {
	const depID = "dep-skip"
	const accountID = "acct-1"
	const datasetName = "dep-dep-skip"

	depMock, depStore := newDeploymentMock(t)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := newDatasetMock(t)
	expectDeploymentByID(depMock, depID, accountID)
	expectLangfuseCreds(lfMock, accountID)
	expectDatasetExists(dsMock, depID)

	traceAt := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	(*dsMock).ExpectExec("UPDATE eval_datasets").
		WithArgs(0, traceAt, true, depID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var countRefreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces" && r.URL.Query().Get("page") == "":
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{{
					"id":        "trace-skip",
					"input":     nil,
					"output":    map[string]any{"answer": "world"},
					"metadata":  map[string]any{"source": "test"},
					"createdAt": traceAt.Format(time.RFC3339),
				}},
				"meta": map[string]any{
					"page":       1,
					"limit":      50,
					"totalItems": 1,
					"totalPages": 1,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/dataset-items":
			countRefreshes++
			if got := r.URL.Query().Get("datasetName"); got != datasetName {
				t.Fatalf("datasetName = %q, want %q", got, datasetName)
			}
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{},
				"meta": map[string]any{
					"page":       1,
					"limit":      1,
					"totalItems": 0,
					"totalPages": 0,
				},
			})
		default:
			t.Fatalf("unexpected Langfuse request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	worker := &DatasetSyncWorker{
		deploymentStore: depStore,
		datasetStore:    dsStore,
		langfuseStore:   lfStore,
		langfuseBaseURL: srv.URL,
		log:             logger.New("error", "json"),
	}
	if err := worker.Work(context.Background(), &river.Job[DatasetSyncArgs]{
		JobRow: &rivertype.JobRow{ID: 1, Attempt: 1, MaxAttempts: 25},
		Args:   DatasetSyncArgs{DeploymentID: depID},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if countRefreshes != 1 {
		t.Fatalf("count refreshes = %d, want 1", countRefreshes)
	}
	assertSQLExpectationsMet(t, depMock, lfMock, dsMock)
}

func TestDatasetSyncWorker_UpdatesItemCountWithoutTraceTimestamp(t *testing.T) {
	const depID = "dep-count"
	const accountID = "acct-1"
	const datasetName = "dep-dep-count"

	depMock, depStore := newDeploymentMock(t)
	lfMock, lfStore := newLangfuseMock(t)
	dsMock, dsStore := newDatasetMock(t)
	expectDeploymentByID(depMock, depID, accountID)
	expectLangfuseCreds(lfMock, accountID)
	expectDatasetExists(dsMock, depID)

	(*dsMock).ExpectExec("UPDATE eval_datasets").
		WithArgs(3, nil, true, depID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	var countRefreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/traces" && r.URL.Query().Get("page") == "":
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{},
				"meta": map[string]any{
					"page":       1,
					"limit":      50,
					"totalItems": 0,
					"totalPages": 0,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/public/dataset-items":
			countRefreshes++
			if got := r.URL.Query().Get("datasetName"); got != datasetName {
				t.Fatalf("datasetName = %q, want %q", got, datasetName)
			}
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{},
				"meta": map[string]any{
					"page":       1,
					"limit":      1,
					"totalItems": 3,
					"totalPages": 1,
				},
			})
		default:
			t.Fatalf("unexpected Langfuse request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(srv.Close)

	worker := &DatasetSyncWorker{
		deploymentStore: depStore,
		datasetStore:    dsStore,
		langfuseStore:   lfStore,
		langfuseBaseURL: srv.URL,
		log:             logger.New("error", "json"),
	}
	if err := worker.Work(context.Background(), &river.Job[DatasetSyncArgs]{
		JobRow: &rivertype.JobRow{ID: 1, Attempt: 1, MaxAttempts: 25},
		Args:   DatasetSyncArgs{DeploymentID: depID},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if countRefreshes != 1 {
		t.Fatalf("count refreshes = %d, want 1", countRefreshes)
	}
	assertSQLExpectationsMet(t, depMock, lfMock, dsMock)
}

func newDeploymentMock(t *testing.T) (*sqlmock.Sqlmock, *deploymentstore.Store) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &mock, deploymentstore.NewStore(db)
}

func expectDeploymentByID(mock *sqlmock.Sqlmock, depID, accountID string) {
	(*mock).ExpectQuery("SELECT .+ FROM deployments").
		WithArgs(depID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
			"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
			"status", "error_message", "error_details", "status_changed_at", "current_revision",
			"deployed_at", "undeployed_at", "avatar_colors",
		}).AddRow(
			depID, accountID, nil, "test-agent", "build-1", "ns", "Test Agent",
			"{}", []byte(nil), nil, nil,
			"active", nil, nil, time.Now(), nil,
			time.Now(), nil, nil,
		))
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func assertSQLExpectationsMet(t *testing.T, mocks ...*sqlmock.Sqlmock) {
	t.Helper()
	for _, mock := range mocks {
		if err := (*mock).ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	}
}
