package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaljudge"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

type predictionUpdate struct {
	status  judgmentstore.PredictionRequestStatus
	message *string
}

type fakeEvalJudgePredictionStore struct {
	requests        map[string]judgmentstore.PredictionRequest
	predictions     map[string]judgmentstore.Prediction
	judged          bool
	updates         []predictionUpdate
	deleted         bool
	stored          *judgmentstore.Prediction
	getRequestsErr  error
	getPredsErr     error
	updateErr       error
	failedUpdateErr error
	deleteErr       error
	judgedErr       error
	upsertErr       error
}

func (f *fakeEvalJudgePredictionStore) GetPredictionRequests(context.Context, string, []string) (map[string]judgmentstore.PredictionRequest, error) {
	return f.requests, f.getRequestsErr
}

func (f *fakeEvalJudgePredictionStore) GetPredictions(context.Context, string, []string) (map[string]judgmentstore.Prediction, error) {
	return f.predictions, f.getPredsErr
}

func (f *fakeEvalJudgePredictionStore) UpdatePredictionRequest(
	_ context.Context,
	_, _ string,
	status judgmentstore.PredictionRequestStatus,
	message *string,
) error {
	if status == judgmentstore.PredictionRequestFailed && f.failedUpdateErr != nil {
		return f.failedUpdateErr
	}
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updates = append(f.updates, predictionUpdate{status: status, message: message})
	return nil
}

func (f *fakeEvalJudgePredictionStore) DeletePredictionRequest(context.Context, string, string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = true
	return nil
}

func (f *fakeEvalJudgePredictionStore) IsJudged(context.Context, string, string) (bool, error) {
	if f.judgedErr != nil {
		return false, f.judgedErr
	}
	return f.judged, nil
}

func (f *fakeEvalJudgePredictionStore) UpsertPrediction(_ context.Context, _, _ string, prediction judgmentstore.Prediction) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.stored = &prediction
	return nil
}

type fakeEvalJudgeDatasetStore struct {
	dataset *evaldatasetstore.EvalDataset
	err     error
}

func (f *fakeEvalJudgeDatasetStore) GetByID(context.Context, string) (*evaldatasetstore.EvalDataset, error) {
	return f.dataset, f.err
}

type fakeEvalJudgeTraceClient struct {
	trace       *langfuse.TraceDetail
	next        *langfuse.Trace
	traceErr    error
	nextErr     error
	nextCalls   int
	nextStartAt string
	getTraceCtx context.Context
}

func (f *fakeEvalJudgeTraceClient) GetTrace(ctx context.Context, _ string) (*langfuse.TraceDetail, error) {
	f.getTraceCtx = ctx
	return f.trace, f.traceErr
}

func (f *fakeEvalJudgeTraceClient) GetNextSessionTrace(
	_ context.Context,
	_, _, _, _, targetTimestamp string,
) (*langfuse.Trace, error) {
	f.nextCalls++
	f.nextStartAt = targetTimestamp
	return f.next, f.nextErr
}

type fakeEvalJudgePredictor struct {
	result evaljudge.Result
	err    error
	calls  int
	apiKey string
	input  evaljudge.Input
}

func (f *fakeEvalJudgePredictor) Predict(_ context.Context, apiKey string, input evaljudge.Input) (evaljudge.Result, error) {
	f.calls++
	f.apiKey = apiKey
	f.input = input
	return f.result, f.err
}

func newEvalJudgeWorkerFixture() (*EvalJudgePredictionWorker, *fakeEvalJudgePredictionStore, *fakeEvalJudgeTraceClient, *fakeEvalJudgePredictor) {
	store := &fakeEvalJudgePredictionStore{
		requests: map[string]judgmentstore.PredictionRequest{
			"trace-1": {TraceID: "trace-1", Status: judgmentstore.PredictionRequestQueued},
		},
		predictions: map[string]judgmentstore.Prediction{},
	}
	traceClient := &fakeEvalJudgeTraceClient{
		trace: &langfuse.TraceDetail{
			Trace: langfuse.Trace{
				ID:        "trace-1",
				Input:     map[string]any{"question": "hello"},
				Output:    map[string]any{"answer": "hi"},
				SessionID: "session-1",
				UserID:    "user-1",
				Timestamp: "2026-07-27T12:00:00Z",
				CreatedAt: "2026-07-27T12:00:05Z",
				Tags:      []string{"deployment:dep-1"},
			},
			Scores: []langfuse.Score{
				{ID: "old", Name: "user_feedback", StringValue: "thumbs_down", CreatedAt: "2026-07-27T12:01:00Z"},
				{ID: "comment", Name: "user_feedback", StringValue: "comment", CreatedAt: "2026-07-27T12:02:00Z"},
				{ID: "new", Name: "user_feedback", StringValue: "thumbs_up", CreatedAt: "2026-07-27T12:03:00Z"},
			},
		},
		next: &langfuse.Trace{ID: "trace-2", Input: map[string]any{"message": map[string]any{"content": "Thanks, one more thing."}}},
	}
	prediction := judgmentstore.Prediction{
		VerdictScore: 0.8,
		Confidence:   90,
		Explanation:  "Useful and accurate.",
		JudgeVersion: evaljudge.EvalDatasetJudgeVersion,
	}
	predictor := &fakeEvalJudgePredictor{
		result: evaljudge.Result{
			Prediction: prediction,
			Usage:      &aigateway.ChatCompletionUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		},
	}
	worker := &EvalJudgePredictionWorker{
		datasets: &fakeEvalJudgeDatasetStore{dataset: &evaldatasetstore.EvalDataset{
			ID: "dataset-1", DeploymentID: "dep-1", AccountID: "acct-1",
		}},
		predictions: store,
		loadLangfuse: func(context.Context, string) (*langfuse.AccountLangfuse, error) {
			return &langfuse.AccountLangfuse{PublicKey: "pk", SecretKey: "sk"}, nil
		},
		newTraceClient: func(*langfuse.AccountLangfuse) evalJudgeTraceClient {
			return traceClient
		},
		ensureJudgeKey: func(context.Context, string) (string, string, error) {
			return "judge-key", "https://gateway.example", nil
		},
		newPredictor: func(string) evalJudgePredictor {
			return predictor
		},
	}
	return worker, store, traceClient, predictor
}

func predictionJob(attempt int) *river.Job[EvalJudgePredictionArgs] {
	return &river.Job[EvalJudgePredictionArgs]{
		JobRow: &rivertype.JobRow{
			Attempt:     attempt,
			MaxAttempts: 3,
		},
		Args: EvalJudgePredictionArgs{EvalDatasetID: "dataset-1", TraceID: "trace-1"},
	}
}

func TestEvalJudgePredictionArgs(t *testing.T) {
	args := EvalJudgePredictionArgs{EvalDatasetID: "dataset-1", TraceID: "trace-1"}
	if args.Kind() != "eval_dataset.judge_prediction" {
		t.Fatalf("Kind = %q", args.Kind())
	}
	opts := args.InsertOpts()
	if opts.Queue != queueEvalJudge || opts.MaxAttempts != 3 || !opts.UniqueOpts.ByArgs {
		t.Fatalf("InsertOpts = %+v", opts)
	}
	wantStates := map[rivertype.JobState]bool{
		rivertype.JobStateAvailable: true,
		rivertype.JobStatePending:   true,
		rivertype.JobStateRunning:   true,
		rivertype.JobStateRetryable: true,
		rivertype.JobStateScheduled: true,
	}
	if len(opts.UniqueOpts.ByState) != len(wantStates) {
		t.Fatalf("ByState = %v", opts.UniqueOpts.ByState)
	}
	for _, state := range opts.UniqueOpts.ByState {
		if !wantStates[state] {
			t.Fatalf("unexpected unique state %q", state)
		}
	}
	if evalJudgeMaxWorkers != 3 {
		t.Fatalf("eval judge workers = %d", evalJudgeMaxWorkers)
	}
	if kind, ok := registeredJobKind[EvalJudgePredictionArgs](); !ok || kind != args.Kind() {
		t.Fatalf("registered kind = %q, %v", kind, ok)
	}
}

func TestEvalJudgePredictionWorkerSuccess(t *testing.T) {
	worker, store, traceClient, predictor := newEvalJudgeWorkerFixture()

	if err := worker.Work(context.Background(), predictionJob(1)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if predictor.calls != 1 || predictor.apiKey != "judge-key" {
		t.Fatalf("predictor calls=%d key=%q", predictor.calls, predictor.apiKey)
	}
	if predictor.input.TraceID != "trace-1" ||
		predictor.input.NextUserText != "Thanks, one more thing." ||
		predictor.input.ThumbsFeedback != "thumbs_up" ||
		len(predictor.input.PriorExamples) != 0 {
		t.Fatalf("predict input = %+v", predictor.input)
	}
	if traceClient.nextStartAt != "2026-07-27T12:00:00Z" {
		t.Fatalf("next trace start = %q", traceClient.nextStartAt)
	}
	if store.stored == nil || store.stored.VerdictScore != 0.8 {
		t.Fatalf("stored prediction = %+v", store.stored)
	}
	if len(store.updates) != 2 ||
		store.updates[0].status != judgmentstore.PredictionRequestInProgress ||
		store.updates[1].status != judgmentstore.PredictionRequestCompleted {
		t.Fatalf("updates = %+v", store.updates)
	}
}

func TestEvalJudgePredictionWorkerOmitsNextTraceWithoutTimestamp(t *testing.T) {
	worker, _, traceClient, predictor := newEvalJudgeWorkerFixture()
	traceClient.trace.Timestamp = ""

	if err := worker.Work(context.Background(), predictionJob(1)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if traceClient.nextCalls != 0 {
		t.Fatalf("next trace calls = %d", traceClient.nextCalls)
	}
	if predictor.input.NextUserText != "" {
		t.Fatalf("next user text = %q", predictor.input.NextUserText)
	}
}

func TestEvalJudgePredictionWorkerRepairsExistingPrediction(t *testing.T) {
	worker, store, _, predictor := newEvalJudgeWorkerFixture()
	store.predictions["trace-1"] = judgmentstore.Prediction{VerdictScore: 0.5}

	if err := worker.Work(context.Background(), predictionJob(1)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if predictor.calls != 0 || len(store.updates) != 1 ||
		store.updates[0].status != judgmentstore.PredictionRequestCompleted {
		t.Fatalf("calls=%d updates=%+v", predictor.calls, store.updates)
	}
}

func TestEvalJudgePredictionWorkerNoRequest(t *testing.T) {
	worker, store, _, predictor := newEvalJudgeWorkerFixture()
	store.requests = map[string]judgmentstore.PredictionRequest{}

	if err := worker.Work(context.Background(), predictionJob(1)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if predictor.calls != 0 || len(store.updates) != 0 {
		t.Fatalf("calls=%d updates=%+v", predictor.calls, store.updates)
	}
}

func TestEvalJudgePredictionWorkerSkipsJudgedTrace(t *testing.T) {
	worker, store, _, predictor := newEvalJudgeWorkerFixture()
	store.judged = true

	if err := worker.Work(context.Background(), predictionJob(1)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if !store.deleted || predictor.calls != 0 {
		t.Fatalf("deleted=%v calls=%d", store.deleted, predictor.calls)
	}
}

func TestEvalJudgePredictionWorkerPermanentTraceFailures(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*EvalJudgePredictionWorker, *fakeEvalJudgeTraceClient)
	}{
		{
			name: "missing trace",
			mutate: func(_ *EvalJudgePredictionWorker, trace *fakeEvalJudgeTraceClient) {
				trace.traceErr = langfuse.ErrNotFound
			},
		},
		{
			name: "missing input",
			mutate: func(_ *EvalJudgePredictionWorker, trace *fakeEvalJudgeTraceClient) {
				trace.trace.Input = nil
			},
		},
		{
			name: "foreign deployment",
			mutate: func(_ *EvalJudgePredictionWorker, trace *fakeEvalJudgeTraceClient) {
				trace.trace.Tags = []string{"deployment:other"}
			},
		},
		{
			name: "missing credentials",
			mutate: func(worker *EvalJudgePredictionWorker, _ *fakeEvalJudgeTraceClient) {
				worker.loadLangfuse = func(context.Context, string) (*langfuse.AccountLangfuse, error) {
					return nil, nil
				}
			},
		},
		{
			name: "credentials cannot decrypt",
			mutate: func(worker *EvalJudgePredictionWorker, _ *fakeEvalJudgeTraceClient) {
				worker.loadLangfuse = func(context.Context, string) (*langfuse.AccountLangfuse, error) {
					return nil, langfuse.ErrCredentialsDecrypt
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			worker, store, trace, predictor := newEvalJudgeWorkerFixture()
			tt.mutate(worker, trace)

			err := worker.Work(context.Background(), predictionJob(1))
			var cancelErr *river.JobCancelError
			if !errors.As(err, &cancelErr) {
				t.Fatalf("Work error = %v, want JobCancel", err)
			}
			if predictor.calls != 0 || len(store.updates) < 2 {
				t.Fatalf("calls=%d updates=%+v", predictor.calls, store.updates)
			}
			last := store.updates[len(store.updates)-1]
			if last.status != judgmentstore.PredictionRequestFailed ||
				last.message == nil || *last.message != predictionFailureMessage {
				t.Fatalf("last update = %+v", last)
			}
		})
	}
}

func TestEvalJudgePredictionWorkerInvocationFailures(t *testing.T) {
	t.Run("budget is terminal", func(t *testing.T) {
		worker, store, _, predictor := newEvalJudgeWorkerFixture()
		predictor.err = &aigateway.InvocationError{
			StatusCode: 402,
			Message:    "budget exceeded",
		}

		err := worker.Work(context.Background(), predictionJob(1))
		var cancelErr *river.JobCancelError
		if !errors.As(err, &cancelErr) {
			t.Fatalf("Work error = %v, want JobCancel", err)
		}
		last := store.updates[len(store.updates)-1]
		if last.message == nil || *last.message != predictionQuotaMessage {
			t.Fatalf("last update = %+v", last)
		}
	})

	t.Run("generic 429 retries", func(t *testing.T) {
		worker, store, _, predictor := newEvalJudgeWorkerFixture()
		predictor.err = &aigateway.InvocationError{StatusCode: 429, Type: "rate_limited"}

		err := worker.Work(context.Background(), predictionJob(1))
		var cancelErr *river.JobCancelError
		if err == nil || errors.As(err, &cancelErr) {
			t.Fatalf("Work error = %v, want retryable", err)
		}
		if len(store.updates) != 1 || store.updates[0].status != judgmentstore.PredictionRequestInProgress {
			t.Fatalf("updates = %+v", store.updates)
		}
	})

	t.Run("invalid output fails after final attempt", func(t *testing.T) {
		worker, store, _, predictor := newEvalJudgeWorkerFixture()
		predictor.err = fmt.Errorf("wrapped: %w", evaljudge.ErrInvalidOutput)

		if err := worker.Work(context.Background(), predictionJob(3)); err == nil {
			t.Fatal("Work returned nil")
		}
		last := store.updates[len(store.updates)-1]
		if last.status != judgmentstore.PredictionRequestFailed ||
			last.message == nil || *last.message != predictionFailureMessage {
			t.Fatalf("last update = %+v", last)
		}
	})

	t.Run("other 400 is terminal", func(t *testing.T) {
		worker, _, _, predictor := newEvalJudgeWorkerFixture()
		predictor.err = &aigateway.InvocationError{StatusCode: 400, Type: "invalid_request"}

		err := worker.Work(context.Background(), predictionJob(1))
		var cancelErr *river.JobCancelError
		if !errors.As(err, &cancelErr) {
			t.Fatalf("Work error = %v, want JobCancel", err)
		}
	})

	t.Run("judge key decrypt is terminal", func(t *testing.T) {
		worker, _, _, _ := newEvalJudgeWorkerFixture()
		worker.ensureJudgeKey = func(context.Context, string) (string, string, error) {
			return "", "", aigateway.ErrJudgeKeyDecrypt
		}

		err := worker.Work(context.Background(), predictionJob(1))
		var cancelErr *river.JobCancelError
		if !errors.As(err, &cancelErr) {
			t.Fatalf("Work error = %v, want JobCancel", err)
		}
	})
}

func TestEvalJudgePredictionWorkerFailurePersistenceRetries(t *testing.T) {
	worker, store, trace, _ := newEvalJudgeWorkerFixture()
	trace.traceErr = langfuse.ErrNotFound
	store.failedUpdateErr = errors.New("update failed")

	err := worker.Work(context.Background(), predictionJob(1))
	var cancelErr *river.JobCancelError
	if err == nil || errors.As(err, &cancelErr) || !errors.Is(err, store.failedUpdateErr) {
		t.Fatalf("Work error = %v, want retryable persistence error", err)
	}
}

func TestEvalJudgePredictionWorkerTimeout(t *testing.T) {
	worker, _, _, _ := newEvalJudgeWorkerFixture()
	if got := worker.Timeout(predictionJob(1)); got != 2*time.Minute {
		t.Fatalf("Timeout = %v", got)
	}
}

func TestLatestThumbsFeedback(t *testing.T) {
	got := latestThumbsFeedback([]langfuse.Score{
		{ID: "1", Name: "other", StringValue: "thumbs_up", CreatedAt: "2026-01-01T00:00:03Z"},
		{ID: "2", Name: "user_feedback", StringValue: "comment", CreatedAt: "2026-01-01T00:00:04Z"},
		{ID: "3", Name: "user_feedback", StringValue: "thumbs_down", CreatedAt: "2026-01-01T00:00:01Z"},
		{ID: "4", Name: "user_feedback", StringValue: "thumbs_up", CreatedAt: "2026-01-01T00:00:02Z"},
	})
	if got != "thumbs_up" {
		t.Fatalf("latestThumbsFeedback = %q", got)
	}
}
