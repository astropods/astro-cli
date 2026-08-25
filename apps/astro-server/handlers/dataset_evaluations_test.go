package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore/judgmentstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

type fakeDatasetEvaluationLangfuseStore struct {
	credentials *langfuse.AccountLangfuse
	err         error
	calls       int
}

func (f *fakeDatasetEvaluationLangfuseStore) Get(_ string) (*langfuse.AccountLangfuse, error) {
	f.calls++
	return f.credentials, f.err
}

type fakeDatasetEvaluationLangfuseAPI struct {
	calls      int
	fields     string
	limit      string
	offsets    []string
	orderBy    string
	tags       string
	traces     []langfuse.Trace
	statusCode int
}

func (f *fakeDatasetEvaluationLangfuseAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls++
	f.fields = r.URL.Query().Get("fields")
	f.limit = r.URL.Query().Get("limit")
	f.orderBy = r.URL.Query().Get("orderBy")
	f.tags = r.URL.Query().Get("tags")
	if f.statusCode != 0 {
		http.Error(w, "langfuse unavailable", f.statusCode)
		return
	}

	limit, _ := strconv.Atoi(f.limit)
	if limit <= 0 {
		limit = len(f.traces)
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit
	f.offsets = append(f.offsets, strconv.Itoa(offset))
	end := offset + limit
	if end > len(f.traces) {
		end = len(f.traces)
	}
	data := []langfuse.Trace{}
	if offset < len(f.traces) {
		data = f.traces[offset:end]
	}
	response := langfuse.TracesResponse{Data: data}
	response.Meta.Page = page
	response.Meta.Limit = limit
	response.Meta.TotalItems = len(f.traces)
	response.Meta.TotalPages = (len(f.traces) + limit - 1) / limit
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

type evaluationJobCall struct {
	datasetID string
	traceID   string
}

type fakeEvaluationRunStore struct {
	runs      map[string]evalrunstore.Run
	completed []evalrunstore.RunTrace
	created   []string
	failed    []string
	runsErr   error
	createErr error
}

func newFakeEvaluationRunStore() *fakeEvaluationRunStore {
	return &fakeEvaluationRunStore{runs: map[string]evalrunstore.Run{}}
}

func (f *fakeEvaluationRunStore) LatestRuns(
	_ context.Context,
	_ string,
	_ []string,
) (map[string]evalrunstore.Run, error) {
	return f.runs, f.runsErr
}

func (f *fakeEvaluationRunStore) TracesWithCompletedRuns(
	_ context.Context,
	_ string,
	_, _ time.Time,
	_ *evalrunstore.RunTrace,
	_ int,
) ([]evalrunstore.RunTrace, error) {
	return f.completed, f.runsErr
}

func (f *fakeEvaluationRunStore) CreateQueuedRuns(
	_ context.Context,
	_, _ string,
	traces []evalrunstore.RunTrace,
) ([]string, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	for _, trace := range traces {
		f.created = append(f.created, trace.TraceID)
	}
	return f.created, nil
}

func (f *fakeEvaluationRunStore) FailQueuedRuns(
	_ context.Context,
	_, _ string,
	traceIDs []string,
	_ string,
) error {
	f.failed = append(f.failed, traceIDs...)
	return nil
}

type fakeDatasetEvaluationQueue struct {
	jobs   []evaluationJobCall
	failAt int
}

func (f *fakeDatasetEvaluationQueue) InsertEvalDatasetEvaluationJobs(
	_ context.Context,
	evalDatasetID string,
	traceIDs []string,
) error {
	first := len(f.jobs) + 1
	for _, traceID := range traceIDs {
		f.jobs = append(f.jobs, evaluationJobCall{datasetID: evalDatasetID, traceID: traceID})
	}
	if f.failAt >= first && f.failAt <= len(f.jobs) {
		return errors.New("insert failed")
	}
	return nil
}

// fixtureEntCheck is nil by default, which skips the gate as the other handler
// fixtures do. A test that exercises gating sets it.
var fixtureEntCheck EntitlementChecker

// blockingEntCheck reports every account as suspended.
type blockingEntCheck struct{ reason string }

func (b blockingEntCheck) Check(context.Context, string) middleware.Decision {
	return middleware.Decision{Blocked: true, Reason: b.reason}
}

type datasetEvaluationsFixture struct {
	*traceDetailFixture
	cfg         *config.Config
	datasetMock sqlmock.Sqlmock
	langfuse    *fakeDatasetEvaluationLangfuseAPI
	credentials *fakeDatasetEvaluationLangfuseStore
}

func setupDatasetEvaluationsRouter(
	t *testing.T,
	withUser bool,
	store reviewQueueScanStore,
	runStore datasetEvaluationRunStore,
	queue datasetEvaluationQueue,
) *datasetEvaluationsFixture {
	t.Helper()

	langfuseAPI := &fakeDatasetEvaluationLangfuseAPI{}
	base, log, cfg, accountStore, deploymentStore, _ := newLangfuseFixture(
		t,
		withUser,
		langfuseAPI.ServeHTTP,
	)
	datasetMock, datasetStore := datasetstoretest.NewMock(t)
	credentials := &fakeDatasetEvaluationLangfuseStore{
		credentials: &langfuse.AccountLangfuse{
			AccountID: "acct-1",
			PublicKey: "pk",
			SecretKey: "sk",
		},
	}
	base.router.POST(
		"/api/v1/deployments/:id/dataset/evaluations",
		PostDatasetEvaluations(
			log,
			cfg,
			accountStore,
			deploymentStore,
			datasetStore,
			credentials,
			store,
			runStore,
			queue,
			fixtureEntCheck,
		),
	)

	return &datasetEvaluationsFixture{
		traceDetailFixture: base,
		cfg:                cfg,
		datasetMock:        datasetMock,
		langfuse:           langfuseAPI,
		credentials:        credentials,
	}
}

func (f *datasetEvaluationsFixture) expectAuthorized(member bool) {
	expectDeploymentLookup(f.deployMock, "dep-1", "acct-1", "agent", "build-1", "namespace")
	count := 0
	if member {
		count = 1
	}
	f.accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func evaluationTrace(id string) langfuse.Trace {
	return langfuse.Trace{
		ID:        id,
		Input:     "input",
		Timestamp: "2026-07-29T12:00:00Z",
		Tags:      []string{"deployment:dep-1"},
	}
}

func evaluationRequest(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/evaluations",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeDatasetEvaluationsResponse(t *testing.T, rec *httptest.ResponseRecorder) DatasetEvaluationsResponse {
	t.Helper()
	var response DatasetEvaluationsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func completedRun(traceID string) evalrunstore.Run {
	return evalrunstore.Run{ID: "run-" + traceID, TraceID: traceID, Status: evalrunstore.StatusCompleted}
}

func TestPostDatasetEvaluationsQueuesMostRecentEligibleTraces(t *testing.T) {
	store := &judgmentstoretest.FakePredictionStore{Judged: map[string]bool{"judged": true}}
	runStore := newFakeEvaluationRunStore()
	runStore.runs = map[string]evalrunstore.Run{"evaluated": completedRun("evaluated")}
	queue := &fakeDatasetEvaluationQueue{}
	fixture := setupDatasetEvaluationsRouter(t, true, store, runStore, queue)
	fixture.langfuse.traces = []langfuse.Trace{
		evaluationTrace("judged"),
		evaluationTrace("evaluated"),
		{ID: "missing-input"},
		evaluationTrace("trace-1"),
		evaluationTrace("trace-2"),
	}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

	rec := evaluationRequest(t, fixture.router)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	response := decodeDatasetEvaluationsResponse(t, rec)
	if fmt.Sprint(response.EnqueuedTraceIDs) != "[trace-1 trace-2]" || len(response.FailedTraceIDs) != 0 {
		t.Fatalf("response = %+v", response)
	}
	if fmt.Sprint(runStore.created) != "[trace-1 trace-2]" {
		t.Fatalf("created runs = %v", runStore.created)
	}
	if len(queue.jobs) != 2 ||
		queue.jobs[0] != (evaluationJobCall{datasetID: "dataset-dep-1", traceID: "trace-1"}) ||
		queue.jobs[1] != (evaluationJobCall{datasetID: "dataset-dep-1", traceID: "trace-2"}) {
		t.Fatalf("jobs = %+v", queue.jobs)
	}
}

func TestPostDatasetEvaluationsSkipsRunsAlreadyInFlight(t *testing.T) {
	for _, status := range []evalrunstore.Status{evalrunstore.StatusQueued, evalrunstore.StatusInProgress} {
		t.Run(string(status), func(t *testing.T) {
			runStore := newFakeEvaluationRunStore()
			runStore.runs = map[string]evalrunstore.Run{
				"trace-1": {ID: "run-1", TraceID: "trace-1", Status: status},
			}
			queue := &fakeDatasetEvaluationQueue{}
			fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, runStore, queue)
			fixture.langfuse.traces = []langfuse.Trace{evaluationTrace("trace-1"), evaluationTrace("trace-2")}
			fixture.expectAuthorized(true)
			datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

			rec := evaluationRequest(t, fixture.router)

			response := decodeDatasetEvaluationsResponse(t, rec)
			if fmt.Sprint(response.EnqueuedTraceIDs) != "[trace-2]" {
				t.Fatalf("response = %+v", response)
			}
		})
	}
}

func TestPostDatasetEvaluationsRetriesAFailedRun(t *testing.T) {
	runStore := newFakeEvaluationRunStore()
	runStore.runs = map[string]evalrunstore.Run{
		"trace-1": {ID: "run-1", TraceID: "trace-1", Status: evalrunstore.StatusFailed},
	}
	queue := &fakeDatasetEvaluationQueue{}
	fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, runStore, queue)
	fixture.langfuse.traces = []langfuse.Trace{evaluationTrace("trace-1")}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

	rec := evaluationRequest(t, fixture.router)

	response := decodeDatasetEvaluationsResponse(t, rec)
	if fmt.Sprint(response.EnqueuedTraceIDs) != "[trace-1]" {
		t.Fatalf("a run that produced nothing stays eligible, got %+v", response)
	}
}

func TestPostDatasetEvaluationsFailsRunsItCouldNotEnqueue(t *testing.T) {
	runStore := newFakeEvaluationRunStore()
	queue := &fakeDatasetEvaluationQueue{failAt: 1}
	fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, runStore, queue)
	fixture.langfuse.traces = []langfuse.Trace{evaluationTrace("trace-1")}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

	rec := evaluationRequest(t, fixture.router)

	response := decodeDatasetEvaluationsResponse(t, rec)
	if rec.Code != http.StatusInternalServerError ||
		fmt.Sprint(response.FailedTraceIDs) != "[trace-1]" {
		t.Fatalf("response = %d %+v", rec.Code, response)
	}
	if fmt.Sprint(runStore.failed) != "[trace-1]" {
		t.Fatalf("a run left queued never settles and blocks the next request, got %v", runStore.failed)
	}
}

func TestPostDatasetEvaluationsScansUntilFiftyEligibleTraces(t *testing.T) {
	queue := &fakeDatasetEvaluationQueue{}
	fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, newFakeEvaluationRunStore(), queue)
	for i := 0; i < 120; i++ {
		fixture.langfuse.traces = append(fixture.langfuse.traces, evaluationTrace(fmt.Sprintf("trace-%d", i)))
	}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

	rec := evaluationRequest(t, fixture.router)

	response := decodeDatasetEvaluationsResponse(t, rec)
	if rec.Code != http.StatusAccepted || len(response.EnqueuedTraceIDs) != maxDatasetEvaluationTraceIDs {
		t.Fatalf("response = %d enqueued=%d", rec.Code, len(response.EnqueuedTraceIDs))
	}
}

func TestPostDatasetEvaluationsAuthorizationAndConfiguration(t *testing.T) {
	t.Run("non-member", func(t *testing.T) {
		fixture := setupDatasetEvaluationsRouter(t, false, &judgmentstoretest.FakePredictionStore{}, newFakeEvaluationRunStore(), &fakeDatasetEvaluationQueue{})
		rec := evaluationRequest(t, fixture.router)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("response = %d", rec.Code)
		}
	})
	t.Run("langfuse not configured", func(t *testing.T) {
		fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, newFakeEvaluationRunStore(), &fakeDatasetEvaluationQueue{})
		fixture.cfg.Deployment.LangfuseBaseURL = ""
		fixture.expectAuthorized(true)
		rec := evaluationRequest(t, fixture.router)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("response = %d", rec.Code)
		}
	})
	t.Run("missing credentials", func(t *testing.T) {
		fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, newFakeEvaluationRunStore(), &fakeDatasetEvaluationQueue{})
		fixture.credentials.credentials = nil
		fixture.expectAuthorized(true)
		datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")
		rec := evaluationRequest(t, fixture.router)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("response = %d", rec.Code)
		}
	})
}

func TestPostDatasetEvaluationsMissingDataset(t *testing.T) {
	fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, newFakeEvaluationRunStore(), &fakeDatasetEvaluationQueue{})
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectMissing(fixture.datasetMock, "dep-1")

	rec := evaluationRequest(t, fixture.router)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
}

func TestPostDatasetEvaluationsIsANoOpWhenEverythingIsEvaluated(t *testing.T) {
	runStore := newFakeEvaluationRunStore()
	runStore.runs = map[string]evalrunstore.Run{"trace-1": completedRun("trace-1")}
	queue := &fakeDatasetEvaluationQueue{}
	fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, runStore, queue)
	fixture.langfuse.traces = []langfuse.Trace{evaluationTrace("trace-1")}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

	rec := evaluationRequest(t, fixture.router)

	if rec.Code != http.StatusAccepted || len(runStore.created) != 0 || len(queue.jobs) != 0 {
		t.Fatalf("response=%d created=%v jobs=%v", rec.Code, runStore.created, queue.jobs)
	}
}

func TestPostDatasetEvaluationsStoreReadFailures(t *testing.T) {
	t.Run("judgments", func(t *testing.T) {
		store := &judgmentstoretest.FakePredictionStore{JudgedErr: errors.New("db down")}
		fixture := setupDatasetEvaluationsRouter(t, true, store, newFakeEvaluationRunStore(), &fakeDatasetEvaluationQueue{})
		fixture.langfuse.traces = []langfuse.Trace{evaluationTrace("trace-1")}
		fixture.expectAuthorized(true)
		datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

		if rec := evaluationRequest(t, fixture.router); rec.Code != http.StatusInternalServerError {
			t.Fatalf("response = %d", rec.Code)
		}
	})
	t.Run("runs", func(t *testing.T) {
		runStore := newFakeEvaluationRunStore()
		runStore.runsErr = errors.New("db down")
		fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, runStore, &fakeDatasetEvaluationQueue{})
		fixture.langfuse.traces = []langfuse.Trace{evaluationTrace("trace-1")}
		fixture.expectAuthorized(true)
		datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

		if rec := evaluationRequest(t, fixture.router); rec.Code != http.StatusInternalServerError {
			t.Fatalf("response = %d", rec.Code)
		}
	})
}

func TestPostDatasetEvaluationsLangfuseFailures(t *testing.T) {
	fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, newFakeEvaluationRunStore(), &fakeDatasetEvaluationQueue{})
	fixture.langfuse.statusCode = http.StatusBadGateway
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

	if rec := evaluationRequest(t, fixture.router); rec.Code != http.StatusBadGateway {
		t.Fatalf("response = %d", rec.Code)
	}
}

func TestPostDatasetEvaluationsReportsEveryTraceWhenRecordingFails(t *testing.T) {
	runStore := newFakeEvaluationRunStore()
	runStore.createErr = errors.New("write failed")
	queue := &fakeDatasetEvaluationQueue{}
	fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, runStore, queue)
	fixture.langfuse.traces = []langfuse.Trace{evaluationTrace("trace-1")}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

	rec := evaluationRequest(t, fixture.router)

	response := decodeDatasetEvaluationsResponse(t, rec)
	if rec.Code != http.StatusInternalServerError ||
		fmt.Sprint(response.FailedTraceIDs) != "[trace-1]" ||
		len(queue.jobs) != 0 {
		t.Fatalf("response=%d %+v jobs=%v", rec.Code, response, queue.jobs)
	}
}

func TestPostDatasetEvaluationsEnqueueFailureReportsEveryTrace(t *testing.T) {
	queue := &fakeDatasetEvaluationQueue{failAt: 1}
	fixture := setupDatasetEvaluationsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, newFakeEvaluationRunStore(), queue)
	fixture.langfuse.traces = []langfuse.Trace{evaluationTrace("trace-1"), evaluationTrace("trace-2")}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

	rec := evaluationRequest(t, fixture.router)

	response := decodeDatasetEvaluationsResponse(t, rec)
	if rec.Code != http.StatusInternalServerError ||
		fmt.Sprint(response.FailedTraceIDs) != "[trace-1 trace-2]" ||
		len(response.EnqueuedTraceIDs) != 0 {
		t.Fatalf("response=%d %+v", rec.Code, response)
	}
}

func expectEvaluationStatusCounts(
	mock sqlmock.Sqlmock,
	datasetID string,
	queued, inProgress, completed, failed int,
) {
	mock.ExpectQuery("SELECT status, COUNT").
		WithArgs(datasetID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}).
			AddRow("queued", queued).
			AddRow("in_progress", inProgress).
			AddRow("completed", completed).
			AddRow("failed", failed))
}

func datasetEvaluationStatusRequest(router http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/deployments/dep-1/dataset/evaluations/status",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGetDatasetEvaluationStatus(t *testing.T) {
	f := setupDatasetRouter(t, true, http.NotFound)
	expectDatasetAuthorization(f, true)
	datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
	expectEvaluationStatusCounts(f.runMock, "dataset-dep-1", 2, 1, 3, 4)

	rec := datasetEvaluationStatusRequest(f.router)

	if rec.Code != http.StatusOK {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	var response DatasetEvaluationStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response != (DatasetEvaluationStatusResponse{
		Queued: 2, InProgress: 1, Completed: 3, Failed: 4,
	}) {
		t.Fatalf("response = %+v", response)
	}
}

func TestGetDatasetEvaluationStatusFailures(t *testing.T) {
	t.Run("authentication", func(t *testing.T) {
		f := setupDatasetRouter(t, false, http.NotFound)
		rec := datasetEvaluationStatusRequest(f.router)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("deployment", func(t *testing.T) {
		f := setupDatasetRouter(t, true, http.NotFound)
		expectDeploymentNotFound(f.deployMock, "dep-1")
		rec := datasetEvaluationStatusRequest(f.router)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("membership", func(t *testing.T) {
		f := setupDatasetRouter(t, true, http.NotFound)
		expectDatasetAuthorization(f, false)
		rec := datasetEvaluationStatusRequest(f.router)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("dataset", func(t *testing.T) {
		f := setupDatasetRouter(t, true, http.NotFound)
		expectDatasetAuthorization(f, true)
		datasetstoretest.ExpectMissing(f.datasetMock, "dep-1")
		rec := datasetEvaluationStatusRequest(f.router)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("status store", func(t *testing.T) {
		f := setupDatasetRouter(t, true, http.NotFound)
		expectDatasetAuthorization(f, true)
		datasetstoretest.ExpectExists(f.datasetMock, "dep-1")
		f.runMock.ExpectQuery("SELECT status, COUNT").
			WithArgs("dataset-dep-1").
			WillReturnError(errors.New("read failed"))
		rec := datasetEvaluationStatusRequest(f.router)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})
}

// An evaluation run bills model usage to the account's own gateway key, so a
// suspended account must not be able to queue one. Nothing downstream would stop
// it: the worker holds the key and the gateway accepts it.
func TestPostDatasetEvaluationsRefusesASuspendedAccount(t *testing.T) {
	fixtureEntCheck = blockingEntCheck{reason: "credits_exhausted"}
	t.Cleanup(func() { fixtureEntCheck = nil })

	store := &judgmentstoretest.FakePredictionStore{}
	runStore := newFakeEvaluationRunStore()
	queue := &fakeDatasetEvaluationQueue{}
	fixture := setupDatasetEvaluationsRouter(t, true, store, runStore, queue)
	fixture.expectAuthorized(true)

	rec := evaluationRequest(t, fixture.router)

	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("response = %d %q, want 402", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "credits_exhausted") {
		t.Errorf("body = %q, want the gating reason", rec.Body.String())
	}
	if len(queue.jobs) != 0 {
		t.Errorf("enqueued %d jobs, want none", len(queue.jobs))
	}
}
