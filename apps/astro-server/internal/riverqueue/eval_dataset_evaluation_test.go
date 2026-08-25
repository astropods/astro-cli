package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

func TestEvalDatasetEvaluationArgsKindIsRegistered(t *testing.T) {
	kind, ok := registeredJobKind[EvalDatasetEvaluationArgs]()
	require.True(t, ok, "kind must be registered so queen can list and trigger it")
	assert.Equal(t, "eval_dataset.evaluation", kind)
}

func TestEvalDatasetEvaluationArgsInsertOpts(t *testing.T) {
	opts := EvalDatasetEvaluationArgs{}.InsertOpts()

	assert.Equal(t, queueEvaluation, opts.Queue)
	assert.Equal(t, 3, opts.MaxAttempts)
	assert.True(t, opts.UniqueOpts.ByArgs)
	assert.ElementsMatch(t, []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStatePending,
		rivertype.JobStateRunning,
		rivertype.JobStateRetryable,
		rivertype.JobStateScheduled,
	}, opts.UniqueOpts.ByState)
}

func TestEvalDatasetEvaluationInsertManyParams(t *testing.T) {
	params := evalDatasetEvaluationInsertManyParams("dataset-1", []string{"trace-1", "trace-2"})
	require.Len(t, params, 2)

	for index, traceID := range []string{"trace-1", "trace-2"} {
		args, ok := params[index].Args.(EvalDatasetEvaluationArgs)
		require.True(t, ok)
		assert.Equal(t, "dataset-1", args.EvalDatasetID)
		assert.Equal(t, traceID, args.TraceID)
	}
}

func TestEvalDatasetEvaluationInsertManyParamsEmpty(t *testing.T) {
	assert.Empty(t, evalDatasetEvaluationInsertManyParams("dataset-1", nil))
}

type fakeEvaluationRunStore struct {
	runID      string
	adopted    evalrunstore.Status
	seeded     []string
	completed  map[string]bool
	inProgress []string
	results    map[string]evalrunstore.Result
	failures   map[string]string
	finalized  evalrunstore.Status
	finalErr   *string
	startErr   error
	unclaimed  bool
	pendingErr string
}

func newFakeRunStore() *fakeEvaluationRunStore {
	return &fakeEvaluationRunStore{
		runID:     "run-1",
		adopted:   evalrunstore.StatusInProgress,
		completed: map[string]bool{},
		results:   map[string]evalrunstore.Result{},
		failures:  map[string]string{},
	}
}

func (f *fakeEvaluationRunStore) StartRun(_ context.Context, _, _, ref string) (*evalrunstore.Run, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	if f.unclaimed {
		return nil, nil
	}
	return &evalrunstore.Run{ID: f.runID, EvaluationRef: ref, Status: f.adopted}, nil
}

func (f *fakeEvaluationRunStore) CreateResults(_ context.Context, _ string, keys []string) error {
	f.seeded = append(f.seeded, keys...)
	return nil
}

func (f *fakeEvaluationRunStore) CompletedResultKeys(context.Context, string) (map[string]bool, error) {
	return f.completed, nil
}

func (f *fakeEvaluationRunStore) MarkResultInProgress(_ context.Context, _, key string) error {
	f.inProgress = append(f.inProgress, key)
	return nil
}

func (f *fakeEvaluationRunStore) CompleteResult(_ context.Context, _ string, result evalrunstore.Result) error {
	f.results[result.EvaluatorKey] = result
	return nil
}

func (f *fakeEvaluationRunStore) FailResult(_ context.Context, _, key, message string) error {
	f.failures[key] = message
	return nil
}

func (f *fakeEvaluationRunStore) FailPendingResults(_ context.Context, _, message string) error {
	f.pendingErr = message
	return nil
}

func (f *fakeEvaluationRunStore) FinalizeRun(_ context.Context, _ string, status evalrunstore.Status, message *string) error {
	f.finalized = status
	f.finalErr = message
	return nil
}

type fakeEvaluationTraceClient struct {
	trace          *langfuse.TraceDetail
	traceErr       error
	previous       []langfuse.Trace
	next           *langfuse.Trace
	previousHit    int
	nextHit        int
	observationIDs []string
	observationErr error
}

func (f *fakeEvaluationTraceClient) GetTrace(context.Context, string) (*langfuse.TraceDetail, error) {
	return f.trace, f.traceErr
}

func (f *fakeEvaluationTraceClient) GetObservation(_ context.Context, id string) (*langfuse.Observation, error) {
	if f.observationErr != nil {
		return nil, f.observationErr
	}
	f.observationIDs = append(f.observationIDs, id)
	return &langfuse.Observation{
		ID:     id,
		Input:  "hydrated input for " + id,
		Output: "hydrated output for " + id,
	}, nil
}

func (f *fakeEvaluationTraceClient) GetPreviousSessionTraces(context.Context, string, string, string, string, string, int) ([]langfuse.Trace, error) {
	f.previousHit++
	return f.previous, nil
}

func (f *fakeEvaluationTraceClient) GetNextSessionTrace(context.Context, string, string, string, string, string) (*langfuse.Trace, error) {
	f.nextHit++
	return f.next, nil
}

type fakeEvaluationRunner struct {
	calls   []string
	results map[string]evaluator.Result
	errs    map[string]error
	inputs  map[string]evaluator.Input
}

func (f *fakeEvaluationRunner) Evaluate(_ context.Context, _ string, ev evaluator.Evaluator, input evaluator.Input) (evaluator.Result, error) {
	f.calls = append(f.calls, ev.Key)
	if f.inputs == nil {
		f.inputs = map[string]evaluator.Input{}
	}
	f.inputs[ev.Key] = input
	if err, ok := f.errs[ev.Key]; ok {
		return evaluator.Result{}, err
	}
	if result, ok := f.results[ev.Key]; ok {
		return result, nil
	}
	return evaluator.Result{Value: true, Confidence: 0.9, Explanation: "fine"}, nil
}

func evaluationTraceFixture() *langfuse.TraceDetail {
	trace := &langfuse.TraceDetail{
		Trace: langfuse.Trace{
			ID:        "trace-1",
			Timestamp: "2026-08-19T12:00:00.000Z",
			Input:     "question",
			Output:    "answer",
			SessionID: "session-1",
			Tags:      []string{"deployment:dep-1"},
		},
		UserID: "user-1",
		Observations: []langfuse.Observation{
			{ID: "obs-tool", Type: "tool", Name: "search"},
			{ID: "obs-event", Type: "EVENT", Name: "marker"},
			{ID: "obs-retriever", Type: "retriever", Name: "docs", Level: "ERROR", StatusMessage: "index down"},
		},
		Scores: []langfuse.Score{
			{ID: "s1", Name: "user_feedback", StringValue: "thumbs_down", CreatedAt: "2026-08-19T12:01:00Z"},
		},
	}
	return trace
}

func newEvaluationWorker(
	runs *fakeEvaluationRunStore,
	client *fakeEvaluationTraceClient,
	runner *fakeEvaluationRunner,
) *EvalDatasetEvaluationWorker {
	return &EvalDatasetEvaluationWorker{
		datasets: &fakeEvalJudgeDatasetStore{dataset: &evaldatasetstore.EvalDataset{
			ID: "dataset-1", DeploymentID: "dep-1", AccountID: "account-1",
		}},
		runs: runs,
		loadLangfuse: func(context.Context, string) (*langfuse.AccountLangfuse, error) {
			return &langfuse.AccountLangfuse{PublicKey: "pk", SecretKey: "sk"}, nil
		},
		newTraceClient: func(*langfuse.AccountLangfuse) evaluationTraceClient { return client },
		ensureJudgeKey: func(context.Context, string) (string, string, error) {
			return "judge-key", "https://gateway", nil
		},
		newRunner: func(string) evaluationRunner { return runner },
	}
}

func evaluationJob(attempt int) *river.Job[EvalDatasetEvaluationArgs] {
	return &river.Job[EvalDatasetEvaluationArgs]{
		JobRow: &rivertype.JobRow{
			Attempt:     attempt,
			MaxAttempts: 3,
		},
		Args: EvalDatasetEvaluationArgs{EvalDatasetID: "dataset-1", TraceID: "trace-1"},
	}
}

func TestEvaluationWorkerRunsWholeSetAndFinalizes(t *testing.T) {
	runs := newFakeRunStore()
	runner := &fakeEvaluationRunner{results: map[string]evaluator.Result{
		"user_sentiment":  {Value: "negative", Confidence: 0.8, Explanation: "next message is unhappy"},
		"claim_grounding": {Value: "grounded", Confidence: 0.7, Explanation: "traces to the search result"},
	}}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)

	require.NoError(t, worker.Work(context.Background(), evaluationJob(1)))

	set, err := evalpreset.ResolveSet(evalpreset.RefDefaultSet)
	require.NoError(t, err)
	wantKeys := make([]string, 0, len(set))
	for _, ev := range set {
		wantKeys = append(wantKeys, ev.Key)
	}

	assert.Equal(t, wantKeys, runs.seeded, "one result row per evaluator, in definition order")
	assert.Equal(t, wantKeys, runner.calls, "evaluators run in definition order")
	assert.Len(t, runs.results, len(set))
	assert.Empty(t, runs.failures)
	assert.Equal(t, evalrunstore.StatusCompleted, runs.finalized)
	assert.Nil(t, runs.finalErr)
	assert.Equal(t, "negative", runs.results["user_sentiment"].Value)
}

func TestEvaluationWorkerLoadsOnlyRequestedContext(t *testing.T) {
	runs := newFakeRunStore()
	client := &fakeEvaluationTraceClient{
		trace:    evaluationTraceFixture(),
		previous: []langfuse.Trace{{Input: "earlier", Output: "reply"}},
		next:     &langfuse.Trace{Input: "that is wrong"},
	}
	runner := &fakeEvaluationRunner{}
	require.NoError(t, newEvaluationWorker(runs, client, runner).Work(context.Background(), evaluationJob(1)))

	// user_sentiment is the only preset asking for conversation context, so each
	// loader runs once for the whole set rather than once per evaluator.
	assert.Equal(t, 1, client.previousHit)
	assert.Equal(t, 1, client.nextHit)

	sentiment := runner.inputs["user_sentiment"]
	assert.Len(t, sentiment.PreviousTurns, 1)
	assert.Equal(t, "that is wrong", sentiment.NextUserMessage)
	assert.Equal(t, "thumbs_down", sentiment.UserFeedback)

	grounding := runner.inputs["claim_grounding"]
	require.Len(t, grounding.Steps, 3, "the worker passes every observation through")
	assert.Equal(t, []string{"tool", "event", "retriever"},
		[]string{grounding.Steps[0].Type, grounding.Steps[1].Type, grounding.Steps[2].Type})
	assert.Equal(t, "index down", grounding.Steps[2].Error)

	// claim_grounding declares no step types, so the union covers the whole trace.
	assert.Equal(t, []string{"obs-tool", "obs-event", "obs-retriever"}, client.observationIDs)
	assert.Equal(t, "hydrated output for obs-tool", grounding.Steps[0].Output)
}

func TestEvaluationWorkerStoresPermanentFailureAndContinues(t *testing.T) {
	runs := newFakeRunStore()
	runner := &fakeEvaluationRunner{errs: map[string]error{
		"exposed_pii": fmt.Errorf("bad output: %w", evaluator.ErrInvalidOutput),
	}}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)

	require.NoError(t, worker.Work(context.Background(), evaluationJob(1)))

	assert.Contains(t, runs.failures, "exposed_pii")
	assert.Contains(t, runs.results, "user_sentiment", "siblings still commit")
	assert.Equal(t, evalrunstore.StatusCompleted, runs.finalized,
		"a failed evaluator still leaves the run completed")
}

func allEvaluatorsFail(t *testing.T) map[string]error {
	t.Helper()
	set, err := evalpreset.ResolveSet(evalpreset.RefDefaultSet)
	require.NoError(t, err)
	errs := make(map[string]error, len(set))
	for _, ev := range set {
		errs[ev.Key] = fmt.Errorf("bad output: %w", evaluator.ErrInvalidOutput)
	}
	return errs
}

func TestEvaluationWorkerFailsRunWhenNoEvaluatorProducedAResult(t *testing.T) {
	runs := newFakeRunStore()
	runner := &fakeEvaluationRunner{errs: allEvaluatorsFail(t)}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)

	require.NoError(t, worker.Work(context.Background(), evaluationJob(1)))

	assert.Equal(t, evalrunstore.StatusFailed, runs.finalized,
		"a run that produced no verdict is not a completed run")
	require.NotNil(t, runs.finalErr)
	assert.Equal(t, evaluationNoResultMessage, *runs.finalErr)
}

func TestEvaluationWorkerCompletesRunWhenAnEarlierAttemptProducedAResult(t *testing.T) {
	runs := newFakeRunStore()
	runs.completed = map[string]bool{"exposed_pii": true}
	runner := &fakeEvaluationRunner{errs: allEvaluatorsFail(t)}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)

	require.NoError(t, worker.Work(context.Background(), evaluationJob(2)))

	assert.Equal(t, evalrunstore.StatusCompleted, runs.finalized,
		"a verdict from an earlier attempt still counts")
}

func TestEvaluationWorkerRecordsQuotaMessage(t *testing.T) {
	runs := newFakeRunStore()
	runner := &fakeEvaluationRunner{errs: map[string]error{
		"exposed_pii": &aigateway.InvocationError{StatusCode: http.StatusPaymentRequired},
	}}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)

	require.NoError(t, worker.Work(context.Background(), evaluationJob(1)))
	assert.Equal(t, evaluationQuotaMessage, runs.failures["exposed_pii"])
}

func TestEvaluationWorkerRetriesTransientFailureKeepingSiblings(t *testing.T) {
	runs := newFakeRunStore()
	runner := &fakeEvaluationRunner{errs: map[string]error{
		"exposed_pii": &aigateway.InvocationError{StatusCode: http.StatusTooManyRequests},
	}}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)

	err := worker.Work(context.Background(), evaluationJob(1))
	require.Error(t, err, "a transient failure returns so River retries")

	assert.Empty(t, runs.failures, "nothing is written as failed before the final attempt")
	assert.Contains(t, runs.results, "user_sentiment", "later evaluators still ran this attempt")
	assert.Empty(t, runs.finalized, "the run stays open for the retry")
}

func TestEvaluationWorkerWritesFailureOnFinalAttempt(t *testing.T) {
	runs := newFakeRunStore()
	runner := &fakeEvaluationRunner{errs: map[string]error{
		"exposed_pii": &aigateway.InvocationError{StatusCode: http.StatusTooManyRequests},
	}}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)

	require.NoError(t, worker.Work(context.Background(), evaluationJob(3)))
	assert.Equal(t, evaluationFailureMessage, runs.failures["exposed_pii"])
	assert.Equal(t, evalrunstore.StatusCompleted, runs.finalized)
}

func TestEvaluationWorkerSkipsAlreadyCompletedEvaluators(t *testing.T) {
	runs := newFakeRunStore()
	runs.completed = map[string]bool{"exposed_pii": true, "leaked_credentials": true}
	runner := &fakeEvaluationRunner{}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)

	require.NoError(t, worker.Work(context.Background(), evaluationJob(2)))

	assert.NotContains(t, runner.calls, "exposed_pii")
	assert.NotContains(t, runner.calls, "leaked_credentials")
	assert.Contains(t, runner.calls, "user_sentiment")
}

func TestEvaluationWorkerLoadsNoContextWhenOnlyOutputEvaluatorsRemain(t *testing.T) {
	runs := newFakeRunStore()
	runs.completed = map[string]bool{
		"unnecessary_tool_call": true,
		"claim_grounding":       true,
		"user_sentiment":        true,
	}
	client := &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}
	worker := newEvaluationWorker(runs, client, &fakeEvaluationRunner{})

	require.NoError(t, worker.Work(context.Background(), evaluationJob(2)))

	assert.Empty(t, client.observationIDs, "a retry does not hydrate steps no remaining evaluator reads")
	assert.Zero(t, client.previousHit)
	assert.Zero(t, client.nextHit)
}

func TestEvaluationWorkerFailsRunOnPermanentContextLoadError(t *testing.T) {
	runs := newFakeRunStore()
	client := &fakeEvaluationTraceClient{
		trace:          evaluationTraceFixture(),
		observationErr: &langfuse.APIError{StatusCode: http.StatusForbidden, Body: "forbidden"},
	}
	worker := newEvaluationWorker(runs, client, &fakeEvaluationRunner{})

	require.Error(t, worker.Work(context.Background(), evaluationJob(1)))

	assert.Equal(t, evalrunstore.StatusFailed, runs.finalized,
		"a permanent Langfuse error must not burn the remaining attempts first")
}

func TestEvaluationWorkerRetriesTransientContextLoadError(t *testing.T) {
	runs := newFakeRunStore()
	client := &fakeEvaluationTraceClient{
		trace:          evaluationTraceFixture(),
		observationErr: &langfuse.APIError{StatusCode: http.StatusBadGateway, Body: "upstream"},
	}
	worker := newEvaluationWorker(runs, client, &fakeEvaluationRunner{})

	require.Error(t, worker.Work(context.Background(), evaluationJob(1)))

	assert.Empty(t, runs.finalized, "the run stays open so the retry can adopt it")
}

func TestEvaluationWorkerRejectsTraceFromAnotherDeployment(t *testing.T) {
	runs := newFakeRunStore()
	trace := evaluationTraceFixture()
	trace.Tags = []string{"deployment:other"}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: trace}, &fakeEvaluationRunner{})

	err := worker.Work(context.Background(), evaluationJob(1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not belong to deployment")
	assert.Empty(t, runs.seeded, "no evaluator runs against a trace we will not evaluate")
	assert.Equal(t, evalrunstore.StatusFailed, runs.finalized, "the recorded run settles instead of staying active")
}

func TestEvaluationWorkerCancelsOnMissingTrace(t *testing.T) {
	runs := newFakeRunStore()
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{traceErr: langfuse.ErrNotFound}, &fakeEvaluationRunner{})

	require.Error(t, worker.Work(context.Background(), evaluationJob(1)))
	assert.Empty(t, runs.seeded)
	assert.Equal(t, evalrunstore.StatusFailed, runs.finalized, "the recorded run settles instead of staying active")
}

func TestEvaluationWorkerCancelsWithoutARecordedRun(t *testing.T) {
	runs := newFakeRunStore()
	runs.unclaimed = true
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, &fakeEvaluationRunner{})

	err := worker.Work(context.Background(), evaluationJob(1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active evaluation run")
	assert.Empty(t, runs.seeded, "the request records the run, so a worker never invents one")
}

func TestEvaluationWorkerRejectsBlankArgs(t *testing.T) {
	worker := newEvaluationWorker(newFakeRunStore(), &fakeEvaluationTraceClient{}, &fakeEvaluationRunner{})
	job := evaluationJob(1)
	job.Args.TraceID = "  "

	require.Error(t, worker.Work(context.Background(), job))
}

func stepsEvaluator(stepTypes ...string) evaluator.Evaluator {
	return evaluator.Evaluator{Config: evaluator.Config{Context: evaluator.ContextConfig{
		Steps:     true,
		StepTypes: stepTypes,
	}}}
}

func TestReadableObservationsCoversEachEvaluatorsDeclaredTypes(t *testing.T) {
	observations := evaluationTraceFixture().Observations

	readable := readableObservations([]evaluator.Evaluator{stepsEvaluator("tool")}, observations)

	assert.Equal(t, map[string]bool{"obs-tool": true}, readable)
}

func TestReadableObservationsCoversEverythingForAnUndeclaredEvaluator(t *testing.T) {
	observations := evaluationTraceFixture().Observations

	readable := readableObservations([]evaluator.Evaluator{stepsEvaluator()}, observations)

	assert.Equal(t, map[string]bool{"obs-tool": true, "obs-event": true, "obs-retriever": true}, readable)
}

func TestReadableObservationsIgnoresEvaluatorsThatSkipSteps(t *testing.T) {
	noSteps := evaluator.Evaluator{Config: evaluator.Config{Context: evaluator.ContextConfig{
		StepTypes: []string{"tool"},
	}}}

	assert.Empty(t, readableObservations([]evaluator.Evaluator{noSteps}, evaluationTraceFixture().Observations))
}

func TestReadableObservationsMatchesDeclaredTypesCaseInsensitively(t *testing.T) {
	observations := []langfuse.Observation{{ID: "obs-1", Type: "TOOL"}}

	assert.Equal(t, map[string]bool{"obs-1": true},
		readableObservations([]evaluator.Evaluator{stepsEvaluator(" Tool ")}, observations))
}

func TestReadableObservationsStopsAtTheStepCapPerEvaluator(t *testing.T) {
	observations := make([]langfuse.Observation, evaluator.MaxSteps+5)
	for i := range observations {
		observations[i] = langfuse.Observation{ID: fmt.Sprintf("obs-%d", i), Type: "generation"}
	}

	readable := readableObservations([]evaluator.Evaluator{stepsEvaluator()}, observations)

	assert.Len(t, readable, evaluator.MaxSteps, "an evaluator never sees more steps than the payload cap")
	assert.False(t, readable[fmt.Sprintf("obs-%d", evaluator.MaxSteps)])
}

func TestReadableObservationsReachesNarrowedTypesPastTheCap(t *testing.T) {
	observations := make([]langfuse.Observation, 0, evaluator.MaxSteps+2)
	for i := 0; i < evaluator.MaxSteps; i++ {
		observations = append(observations, langfuse.Observation{ID: fmt.Sprintf("obs-%d", i), Type: "generation"})
	}
	observations = append(observations, langfuse.Observation{ID: "obs-late-tool", Type: "tool"})

	readable := readableObservations([]evaluator.Evaluator{stepsEvaluator(), stepsEvaluator("tool")}, observations)

	assert.True(t, readable["obs-late-tool"],
		"a narrowed evaluator reads its own first MaxSteps matches, wherever they sit in the trace")
	assert.Len(t, readable, evaluator.MaxSteps+1)
}

func TestHydrateObservationIOFetchesOnlyReadableObservations(t *testing.T) {
	client := &fakeEvaluationTraceClient{}
	observations := evaluationTraceFixture().Observations

	got, err := hydrateObservationIO(context.Background(), client, observations, map[string]bool{"obs-tool": true})

	require.NoError(t, err)
	assert.Equal(t, []string{"obs-tool"}, client.observationIDs)
	require.Len(t, got, 3, "observations nobody reads still reach the evaluator")
	assert.Equal(t, "hydrated output for obs-tool", got[0].Output)
	assert.Nil(t, got[1].Output, "an unhydrated observation keeps its empty payload")
}

func TestHydrateObservationIOPropagatesFetchFailure(t *testing.T) {
	client := &fakeEvaluationTraceClient{observationErr: errors.New("langfuse unavailable")}

	_, err := hydrateObservationIO(context.Background(), client,
		evaluationTraceFixture().Observations, map[string]bool{"obs-tool": true})

	require.Error(t, err, "an empty step result would make claim grounding wrong, so the run must not proceed")
	assert.Contains(t, err.Error(), "hydrate observation obs-tool")
}

func TestEvaluationStepsNormalizesTypesAndKeepsEveryObservation(t *testing.T) {
	steps := evaluationSteps([]langfuse.Observation{
		{Type: "GENERATION", Name: "llm"},
		{Type: "event", Name: "marker"},
		{Type: "guardrail", Name: "guard"},
		{Type: " Tool ", Name: "search", Level: "error", StatusMessage: ""},
	})

	require.Len(t, steps, 4, "each evaluator narrows by type, so the worker drops nothing")
	assert.Equal(t, []string{"generation", "event", "guardrail", "tool"},
		[]string{steps[0].Type, steps[1].Type, steps[2].Type, steps[3].Type})
	assert.Equal(t, "step failed", steps[3].Error, "an errored step without a message still reads as failed")
}

func TestEvaluationWorkerRetriesJudgeKeyFailureBeforeExhaustion(t *testing.T) {
	runs := newFakeRunStore()
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, &fakeEvaluationRunner{})
	worker.ensureJudgeKey = func(context.Context, string) (string, string, error) {
		return "", "", errors.New("key conflict: concurrent first provision")
	}

	require.Error(t, worker.Work(context.Background(), evaluationJob(1)))
	assert.Empty(t, runs.finalized, "the run stays open so the retry can adopt it")
	assert.Empty(t, runs.pendingErr)
}

func TestEvaluationWorkerFinalizesRunWhenJudgeKeyAttemptsAreSpent(t *testing.T) {
	runs := newFakeRunStore()
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, &fakeEvaluationRunner{})
	worker.ensureJudgeKey = func(context.Context, string) (string, string, error) {
		return "", "", errors.New("gateway unreachable")
	}

	require.Error(t, worker.Work(context.Background(), evaluationJob(3)))
	assert.Equal(t, evalrunstore.StatusFailed, runs.finalized,
		"an exhausted job must not leave the run in progress forever")
	assert.Equal(t, evaluationFailureMessage, runs.pendingErr,
		"queued results are closed out so none still reads as pending")
}

func TestEvaluationWorkerFailsRunOnUndecryptableJudgeKey(t *testing.T) {
	runs := newFakeRunStore()
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, &fakeEvaluationRunner{})
	worker.ensureJudgeKey = func(context.Context, string) (string, string, error) {
		return "", "", fmt.Errorf("wrap: %w", aigateway.ErrJudgeKeyDecrypt)
	}

	require.Error(t, worker.Work(context.Background(), evaluationJob(1)))
	assert.Equal(t, evalrunstore.StatusFailed, runs.finalized, "an undecryptable key will not fix itself")
	assert.Equal(t, evaluationFailureMessage, runs.pendingErr)
}

// The queue outlives the request that filled it, so an account suspended after
// enqueue would still spend on its gateway key when the job ran. Gating only the
// handler leaves that window open.
func TestEvaluationWorkerRefusesASuspendedAccount(t *testing.T) {
	runs := newFakeRunStore()
	runner := &fakeEvaluationRunner{}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)
	gate := &fakeEvalJudgeBillingGate{blocked: true}
	worker.billing = gate

	err := worker.Work(context.Background(), evaluationJob(1))
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr, "a suspended account is permanent, so the job must not retry")

	assert.Empty(t, runner.calls, "a suspended account must not reach the gateway")
	assert.Equal(t, "account-1", gate.accountID, "the gate is asked about the dataset's account")
	assert.Equal(t, evalrunstore.StatusFailed, runs.finalized)
	require.NotNil(t, runs.finalErr)
	assert.Equal(t, evaluationBillingMessage, *runs.finalErr,
		"a generic failure hides why the run stopped, so the owner cannot act on it")
}

// An account in good standing is unaffected.
func TestEvaluationWorkerRunsWhenNotSuspended(t *testing.T) {
	runs := newFakeRunStore()
	runner := &fakeEvaluationRunner{results: map[string]evaluator.Result{
		"user_sentiment": {Value: "negative", Confidence: 0.8, Explanation: "next message is unhappy"},
	}}
	worker := newEvaluationWorker(runs, &fakeEvaluationTraceClient{trace: evaluationTraceFixture()}, runner)
	worker.billing = &fakeEvalJudgeBillingGate{blocked: false}

	require.NoError(t, worker.Work(context.Background(), evaluationJob(1)))
	assert.Equal(t, evalrunstore.StatusCompleted, runs.finalized)
}
