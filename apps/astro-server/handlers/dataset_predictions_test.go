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

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore/datasetstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore/judgmentstoretest"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

type fakeDatasetPredictionLangfuseStore struct {
	credentials *langfuse.AccountLangfuse
	err         error
	calls       int
}

func (f *fakeDatasetPredictionLangfuseStore) GetDecrypted(
	_ context.Context,
	_ envelope.KMSClient,
	_ string,
) (*langfuse.AccountLangfuse, error) {
	f.calls++
	return f.credentials, f.err
}

type fakeDatasetPredictionLangfuseAPI struct {
	calls      int
	fields     string
	limit      string
	offsets    []string
	orderBy    string
	tags       string
	traces     []langfuse.Trace
	statusCode int
}

func (f *fakeDatasetPredictionLangfuseAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

type predictionJobCall struct {
	datasetID string
	traceID   string
}

type fakeDatasetPredictionQueue struct {
	jobs   []predictionJobCall
	failAt int
}

func (f *fakeDatasetPredictionQueue) InsertEvalJudgePredictionJobs(
	_ context.Context,
	evalDatasetID string,
	traceIDs []string,
) error {
	first := len(f.jobs) + 1
	for _, traceID := range traceIDs {
		f.jobs = append(f.jobs, predictionJobCall{datasetID: evalDatasetID, traceID: traceID})
	}
	if f.failAt >= first && f.failAt <= len(f.jobs) {
		return errors.New("insert failed")
	}
	return nil
}

type datasetPredictionsFixture struct {
	*traceDetailFixture
	cfg         *config.Config
	datasetMock sqlmock.Sqlmock
	langfuse    *fakeDatasetPredictionLangfuseAPI
	credentials *fakeDatasetPredictionLangfuseStore
}

func setupDatasetPredictionsRouter(
	t *testing.T,
	withUser bool,
	store datasetPredictionStore,
	queue datasetPredictionQueue,
) *datasetPredictionsFixture {
	t.Helper()

	langfuseAPI := &fakeDatasetPredictionLangfuseAPI{}
	base, log, cfg, accountStore, deploymentStore, _ := newLangfuseFixture(
		t,
		withUser,
		langfuseAPI.ServeHTTP,
	)
	datasetMock, datasetStore := datasetstoretest.NewMock(t)
	credentials := &fakeDatasetPredictionLangfuseStore{
		credentials: &langfuse.AccountLangfuse{
			AccountID: "acct-1",
			PublicKey: "pk",
			SecretKey: "sk",
		},
	}
	base.router.POST(
		"/api/v1/deployments/:id/dataset/predictions",
		PostDatasetPredictions(
			log,
			cfg,
			accountStore,
			deploymentStore,
			datasetStore,
			credentials,
			nil,
			store,
			queue,
		),
	)

	return &datasetPredictionsFixture{
		traceDetailFixture: base,
		cfg:                cfg,
		datasetMock:        datasetMock,
		langfuse:           langfuseAPI,
		credentials:        credentials,
	}
}

func (f *datasetPredictionsFixture) expectAuthorized(member bool) {
	expectDeploymentLookup(f.deployMock, "dep-1", "acct-1", "agent", "build-1", "namespace")
	count := 0
	if member {
		count = 1
	}
	f.accountMock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

func predictionTrace(id string) langfuse.Trace {
	return langfuse.Trace{
		ID:        id,
		Input:     "input",
		Timestamp: "2026-07-29T12:00:00Z",
		Tags:      []string{"deployment:dep-1"},
	}
}

func predictionRequest(t *testing.T, router http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/deployments/dep-1/dataset/predictions",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeDatasetPredictionsResponse(t *testing.T, rec *httptest.ResponseRecorder) DatasetPredictionsResponse {
	t.Helper()
	var response DatasetPredictionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func TestPostDatasetPredictionsQueuesMostRecentEligibleTraces(t *testing.T) {
	store := &judgmentstoretest.FakePredictionStore{
		Judged: map[string]bool{"judged": true},
		Predictions: map[string]judgmentstore.Prediction{
			"predicted": {VerdictScore: 0.5},
		},
	}
	queue := &fakeDatasetPredictionQueue{}
	fixture := setupDatasetPredictionsRouter(t, true, store, queue)
	fixture.langfuse.traces = []langfuse.Trace{
		predictionTrace("judged"),
		predictionTrace("predicted"),
		{ID: "missing-input"},
		predictionTrace("trace-1"),
		predictionTrace("trace-2"),
	}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectLegacyExists(fixture.datasetMock, "dep-1")

	rec := predictionRequest(t, fixture.router)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	response := decodeDatasetPredictionsResponse(t, rec)
	if fmt.Sprint(response.EnqueuedTraceIDs) != "[trace-1 trace-2]" ||
		len(response.FailedTraceIDs) != 0 {
		t.Fatalf("response = %+v", response)
	}
	wantBatch := "[judged predicted missing-input trace-1 trace-2]"
	if fmt.Sprint(store.BatchTraceIDs) != wantBatch ||
		fmt.Sprint(store.PredictionIDs) != wantBatch ||
		fmt.Sprint(store.QueuedTraceIDs) != "[trace-1 trace-2]" {
		t.Fatalf(
			"batch=%v prediction IDs=%v queued=%v",
			store.BatchTraceIDs,
			store.PredictionIDs,
			store.QueuedTraceIDs,
		)
	}
	if fixture.langfuse.calls != 1 ||
		fixture.langfuse.fields != "core,io" ||
		fixture.langfuse.limit != "100" ||
		fixture.langfuse.tags != "deployment:dep-1" ||
		fixture.langfuse.orderBy != "timestamp.desc" {
		t.Fatalf(
			"Langfuse calls=%d fields=%q limit=%q tags=%q orderBy=%q",
			fixture.langfuse.calls,
			fixture.langfuse.fields,
			fixture.langfuse.limit,
			fixture.langfuse.tags,
			fixture.langfuse.orderBy,
		)
	}
	if len(queue.jobs) != 2 ||
		queue.jobs[0] != (predictionJobCall{datasetID: "dataset-dep-1", traceID: "trace-1"}) ||
		queue.jobs[1] != (predictionJobCall{datasetID: "dataset-dep-1", traceID: "trace-2"}) {
		t.Fatalf("jobs = %+v", queue.jobs)
	}
}

func TestPostDatasetPredictionsScansUntilFiftyEligibleTraces(t *testing.T) {
	store := &judgmentstoretest.FakePredictionStore{Judged: map[string]bool{}}
	queue := &fakeDatasetPredictionQueue{}
	fixture := setupDatasetPredictionsRouter(t, true, store, queue)
	for i := 0; i < 120; i++ {
		traceID := fmt.Sprintf("trace-%03d", i)
		fixture.langfuse.traces = append(fixture.langfuse.traces, predictionTrace(traceID))
		if i < 70 {
			store.Judged[traceID] = true
		}
	}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectExists(fixture.datasetMock, "dep-1")

	rec := predictionRequest(t, fixture.router)

	if rec.Code != http.StatusAccepted ||
		len(queue.jobs) != maxDatasetPredictionTraceIDs ||
		fixture.langfuse.calls != 2 ||
		fmt.Sprint(fixture.langfuse.offsets) != "[0 100]" {
		t.Fatalf(
			"response=%d jobs=%d Langfuse calls=%d offsets=%v",
			rec.Code,
			len(queue.jobs),
			fixture.langfuse.calls,
			fixture.langfuse.offsets,
		)
	}
	response := decodeDatasetPredictionsResponse(t, rec)
	if len(response.EnqueuedTraceIDs) != maxDatasetPredictionTraceIDs ||
		response.EnqueuedTraceIDs[0] != "trace-070" ||
		response.EnqueuedTraceIDs[49] != "trace-119" {
		t.Fatalf("response = %+v", response)
	}
}

func TestPostDatasetPredictionsAuthorizationAndConfiguration(t *testing.T) {
	t.Run("authentication", func(t *testing.T) {
		fixture := setupDatasetPredictionsRouter(t, false, &judgmentstoretest.FakePredictionStore{}, &fakeDatasetPredictionQueue{})
		rec := predictionRequest(t, fixture.router)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("membership", func(t *testing.T) {
		fixture := setupDatasetPredictionsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, &fakeDatasetPredictionQueue{})
		fixture.expectAuthorized(false)
		rec := predictionRequest(t, fixture.router)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("Langfuse configuration", func(t *testing.T) {
		fixture := setupDatasetPredictionsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, &fakeDatasetPredictionQueue{})
		fixture.cfg.Deployment.LangfuseBaseURL = ""
		fixture.expectAuthorized(true)
		rec := predictionRequest(t, fixture.router)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})
}

func TestPostDatasetPredictionsMissingDataset(t *testing.T) {
	fixture := setupDatasetPredictionsRouter(t, true, &judgmentstoretest.FakePredictionStore{}, &fakeDatasetPredictionQueue{})
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectMissing(fixture.datasetMock, "dep-1")

	rec := predictionRequest(t, fixture.router)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "dataset not yet created") {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPostDatasetPredictionsIdempotentNoOp(t *testing.T) {
	store := &judgmentstoretest.FakePredictionStore{
		Judged: map[string]bool{"judged": true},
		Predictions: map[string]judgmentstore.Prediction{
			"predicted": {VerdictScore: 0.5},
		},
	}
	queue := &fakeDatasetPredictionQueue{}
	fixture := setupDatasetPredictionsRouter(t, true, store, queue)
	fixture.langfuse.traces = []langfuse.Trace{
		predictionTrace("judged"),
		predictionTrace("predicted"),
	}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectExists(fixture.datasetMock, "dep-1")

	rec := predictionRequest(t, fixture.router)
	if rec.Code != http.StatusAccepted || len(store.QueuedTraceIDs) != 0 || len(queue.jobs) != 0 {
		t.Fatalf("response=%d queued=%v jobs=%v", rec.Code, store.QueuedTraceIDs, queue.jobs)
	}
	response := decodeDatasetPredictionsResponse(t, rec)
	if len(response.EnqueuedTraceIDs) != 0 ||
		len(response.FailedTraceIDs) != 0 {
		t.Fatalf("response = %+v", response)
	}
}

func TestPostDatasetPredictionsStoreReadFailures(t *testing.T) {
	tests := []struct {
		name  string
		store *judgmentstoretest.FakePredictionStore
	}{
		{name: "judgment read", store: &judgmentstoretest.FakePredictionStore{JudgedErr: errors.New("read failed")}},
		{name: "prediction read", store: &judgmentstoretest.FakePredictionStore{PredictionsErr: errors.New("read failed")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := setupDatasetPredictionsRouter(t, true, tt.store, &fakeDatasetPredictionQueue{})
			fixture.langfuse.traces = []langfuse.Trace{predictionTrace("trace-1")}
			fixture.expectAuthorized(true)
			datasetstoretest.ExpectExists(fixture.datasetMock, "dep-1")
			rec := predictionRequest(t, fixture.router)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPostDatasetPredictionsLangfuseFailures(t *testing.T) {
	t.Run("credentials unavailable", func(t *testing.T) {
		fixture := setupDatasetPredictionsRouter(
			t,
			true,
			&judgmentstoretest.FakePredictionStore{},
			&fakeDatasetPredictionQueue{},
		)
		fixture.credentials.credentials = nil
		fixture.expectAuthorized(true)
		datasetstoretest.ExpectExists(fixture.datasetMock, "dep-1")

		rec := predictionRequest(t, fixture.router)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("trace lookup", func(t *testing.T) {
		fixture := setupDatasetPredictionsRouter(
			t,
			true,
			&judgmentstoretest.FakePredictionStore{},
			&fakeDatasetPredictionQueue{},
		)
		fixture.langfuse.statusCode = http.StatusInternalServerError
		fixture.expectAuthorized(true)
		datasetstoretest.ExpectExists(fixture.datasetMock, "dep-1")

		rec := predictionRequest(t, fixture.router)
		if rec.Code != http.StatusBadGateway {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
	})
}

func TestPostDatasetPredictionsBatchEnqueueFailureMarksChangedRequestsFailed(t *testing.T) {
	store := &judgmentstoretest.FakePredictionStore{
		PreservedIDs: map[string]bool{"trace-2": true},
	}
	queue := &fakeDatasetPredictionQueue{failAt: 2}
	fixture := setupDatasetPredictionsRouter(t, true, store, queue)
	fixture.langfuse.traces = []langfuse.Trace{
		predictionTrace("trace-1"),
		predictionTrace("trace-2"),
		predictionTrace("trace-3"),
	}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectExists(fixture.datasetMock, "dep-1")

	rec := predictionRequest(t, fixture.router)
	if rec.Code != http.StatusInternalServerError ||
		fmt.Sprint(store.QueuedTraceIDs) != "[trace-1 trace-2 trace-3]" ||
		len(queue.jobs) != 3 {
		t.Fatalf("response=%d queued=%v jobs=%v", rec.Code, store.QueuedTraceIDs, queue.jobs)
	}
	response := decodeDatasetPredictionsResponse(t, rec)
	if len(response.EnqueuedTraceIDs) != 0 ||
		fmt.Sprint(response.FailedTraceIDs) != "[trace-1 trace-2 trace-3]" {
		t.Fatalf("response = %+v", response)
	}
	if len(store.Updates) != 1 ||
		fmt.Sprint(store.Updates[0].TraceIDs) != "[trace-1 trace-3]" ||
		store.Updates[0].Status != judgmentstore.PredictionRequestFailed ||
		store.Updates[0].ErrorMessage == nil ||
		*store.Updates[0].ErrorMessage != predictionEnqueueFailureMessage {
		t.Fatalf("updates = %+v", store.Updates)
	}
}

func TestPostDatasetPredictionsBatchRequestFailureSkipsEnqueue(t *testing.T) {
	store := &judgmentstoretest.FakePredictionStore{QueueErr: errors.New("write failed")}
	queue := &fakeDatasetPredictionQueue{}
	fixture := setupDatasetPredictionsRouter(t, true, store, queue)
	fixture.langfuse.traces = []langfuse.Trace{
		predictionTrace("trace-1"),
		predictionTrace("trace-2"),
		predictionTrace("trace-3"),
	}
	fixture.expectAuthorized(true)
	datasetstoretest.ExpectExists(fixture.datasetMock, "dep-1")

	rec := predictionRequest(t, fixture.router)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	response := decodeDatasetPredictionsResponse(t, rec)
	if len(response.EnqueuedTraceIDs) != 0 ||
		fmt.Sprint(response.FailedTraceIDs) != "[trace-1 trace-2 trace-3]" ||
		fmt.Sprint(store.QueuedTraceIDs) != "[trace-1 trace-2 trace-3]" ||
		len(queue.jobs) != 0 {
		t.Fatalf(
			"response=%+v queued=%v jobs=%+v",
			response,
			store.QueuedTraceIDs,
			queue.jobs,
		)
	}
}
