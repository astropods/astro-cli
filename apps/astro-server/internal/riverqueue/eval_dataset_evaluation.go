package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type EvalDatasetEvaluationArgs struct {
	EvalDatasetID string `json:"eval_dataset_id"`
	TraceID       string `json:"trace_id"`
	EvaluationRef string `json:"evaluation_ref"`
}

func (EvalDatasetEvaluationArgs) Kind() string { return "eval_dataset.evaluation" }

func init() {
	registerJobKind[EvalDatasetEvaluationArgs]()
}

func (EvalDatasetEvaluationArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueEvaluation,
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}

const (
	evaluationTimeout           = 10 * time.Minute
	evaluationPreviousTurnLimit = 3
	evaluationFailureMessage    = "Evaluation failed. Try again."
	evaluationNoResultMessage   = "No evaluator produced a result."
	evaluationQuotaMessage      = "AI usage quota exceeded. Try again after the quota resets or is increased."
	evaluationBillingMessage    = "This account is suspended for a billing issue. Resolve it to run evaluations."
)

var (
	errEvaluationNotConfigured = errors.New("evaluation is not configured")
	errPermanentEvaluation     = errors.New("evaluation cannot proceed")
)

type evaluationDatasetStore interface {
	GetByID(context.Context, string) (*evaldatasetstore.EvalDataset, error)
}

type evaluationRunStore interface {
	StartRun(ctx context.Context, evalDatasetID, traceID, evaluationRef string) (*evalrunstore.Run, error)
	CreateResults(ctx context.Context, runID string, evaluatorKeys []string) error
	CompletedResultKeys(ctx context.Context, runID string) (map[string]bool, error)
	MarkResultInProgress(ctx context.Context, runID, evaluatorKey string) error
	CompleteResult(ctx context.Context, runID string, result evalrunstore.Result) error
	FailResult(ctx context.Context, runID, evaluatorKey, message string) error
	FailPendingResults(ctx context.Context, runID, message string) error
	FinalizeRun(ctx context.Context, runID string, status evalrunstore.Status, errorMessage *string) error
}

type evaluationTraceClient interface {
	GetTrace(context.Context, string) (*langfuse.TraceDetail, error)
	GetObservation(context.Context, string) (*langfuse.Observation, error)
	GetPreviousSessionTraces(context.Context, string, string, string, string, string, int) ([]langfuse.Trace, error)
	GetNextSessionTrace(context.Context, string, string, string, string, string) (*langfuse.Trace, error)
}

type evaluationRunner interface {
	Evaluate(context.Context, string, evaluator.Evaluator, evaluator.Input) (evaluator.Result, error)
}

type evaluationSetResolver interface {
	Set(ctx context.Context, evaluationRef string) ([]evaluator.Evaluator, error)
}

type EvalDatasetEvaluationWorker struct {
	river.WorkerDefaults[EvalDatasetEvaluationArgs]
	datasets       evaluationDatasetStore
	runs           evaluationRunStore
	resolver       evaluationSetResolver
	loadLangfuse   func(context.Context, string) (*langfuse.AccountLangfuse, error)
	newTraceClient func(*langfuse.AccountLangfuse) evaluationTraceClient
	ensureJudgeKey func(context.Context, string) (string, string, error)
	newRunner      func(string) evaluationRunner
	billing        evaluationBillingGate
	log            *logger.Logger
}

func (w *EvalDatasetEvaluationWorker) Timeout(*river.Job[EvalDatasetEvaluationArgs]) time.Duration {
	return evaluationTimeout
}

func (w *EvalDatasetEvaluationWorker) Work(ctx context.Context, job *river.Job[EvalDatasetEvaluationArgs]) error {
	args := job.Args
	if strings.TrimSpace(args.EvalDatasetID) == "" || strings.TrimSpace(args.TraceID) == "" {
		return river.JobCancel(fmt.Errorf("eval dataset evaluation: dataset and trace IDs are required"))
	}
	if w.datasets == nil || w.runs == nil {
		return river.JobCancel(fmt.Errorf("eval dataset evaluation: %w", errEvaluationNotConfigured))
	}

	dataset, err := w.datasets.GetByID(ctx, args.EvalDatasetID)
	if err != nil {
		return fmt.Errorf("load eval dataset: %w", err)
	}
	if dataset == nil {
		return river.JobCancel(fmt.Errorf("eval dataset %q no longer exists", args.EvalDatasetID))
	}
	if w.loadLangfuse == nil || w.newTraceClient == nil || w.ensureJudgeKey == nil || w.newRunner == nil || w.resolver == nil {
		return river.JobCancel(fmt.Errorf("eval dataset evaluation: %w", errEvaluationNotConfigured))
	}

	evaluationRef := args.EvaluationRef
	set, err := w.resolver.Set(ctx, evaluationRef)
	if err != nil {
		return river.JobCancel(fmt.Errorf("resolve evaluation set %q: %w", evaluationRef, err))
	}

	run, err := w.runs.StartRun(ctx, dataset.ID, args.TraceID, evaluationRef)
	if err != nil {
		return fmt.Errorf("start evaluation run: %w", err)
	}
	if run == nil {
		return river.JobCancel(fmt.Errorf("trace %q has no active evaluation run", args.TraceID))
	}

	// An evaluation bills model usage to the account's gateway key, so a
	// suspended account must not run one. Checked here as well as at enqueue: a
	// job queued while the account was in good standing would otherwise still
	// spend after it was stopped. Failed rather than cancelled, so the run says
	// why instead of disappearing.
	if w.billing != nil && w.billing.Blocked(ctx, dataset.AccountID) {
		return w.failRunWithMessage(ctx, run.ID, evaluationBillingMessage,
			fmt.Errorf("%w: account %s is billing-suspended", errPermanentEvaluation, dataset.AccountID))
	}

	client, err := w.resolveTraceClient(ctx, dataset)
	if err != nil {
		return w.retryOrFailRun(ctx, job, run.ID, err)
	}
	trace, err := loadEvaluationTrace(ctx, client, dataset, args.TraceID)
	if err != nil {
		return w.retryOrFailRun(ctx, job, run.ID, err)
	}

	keys := make([]string, 0, len(set))
	for _, ev := range set {
		keys = append(keys, ev.Key)
	}
	if err := w.runs.CreateResults(ctx, run.ID, keys); err != nil {
		return w.retryOrFailRun(ctx, job, run.ID, fmt.Errorf("create evaluator results: %w", err))
	}

	// Narrow before loading context so a retry only pays for what it re-runs.
	completed, err := w.runs.CompletedResultKeys(ctx, run.ID)
	if err != nil {
		return w.retryOrFailRun(ctx, job, run.ID, fmt.Errorf("load completed evaluator results: %w", err))
	}
	remaining := make([]evaluator.Evaluator, 0, len(set))
	for _, ev := range set {
		if !completed[ev.Key] {
			remaining = append(remaining, ev)
		}
	}

	input, err := buildEvaluationInput(ctx, client, dataset, trace, remaining)
	if err != nil {
		cause := fmt.Errorf("load evaluation context: %w", err)
		if isPermanentLangfuseError(err) {
			return w.failRun(ctx, run.ID, cause)
		}
		return w.retryOrFailRun(ctx, job, run.ID, cause)
	}

	apiKey, gatewayURL, err := w.ensureJudgeKey(ctx, dataset.AccountID)
	if err != nil {
		if errors.Is(err, aigateway.ErrJudgeKeyDecrypt) {
			return w.failRun(ctx, run.ID, fmt.Errorf("ensure judge key: %w", err))
		}
		return w.retryOrFailRun(ctx, job, run.ID, fmt.Errorf("ensure judge key: %w", err))
	}

	return w.runSet(ctx, job, run.ID, w.newRunner(gatewayURL), apiKey, remaining, input, len(completed))
}

// resolveTraceClient loads the account's Langfuse credentials once, so the trace
// read and the context reads share one client.
func (w *EvalDatasetEvaluationWorker) resolveTraceClient(
	ctx context.Context,
	dataset *evaldatasetstore.EvalDataset,
) (evaluationTraceClient, error) {
	credentials, err := w.loadLangfuse(ctx, dataset.AccountID)
	if err != nil {
		return nil, fmt.Errorf("load Langfuse credentials: %w", err)
	}
	if credentials == nil ||
		strings.TrimSpace(credentials.PublicKey) == "" ||
		strings.TrimSpace(credentials.SecretKey) == "" {
		return nil, fmt.Errorf("%w: langfuse is not configured for account %s", errPermanentEvaluation, dataset.AccountID)
	}
	return w.newTraceClient(credentials), nil
}

// loadEvaluationTrace fetches the trace once: the same response carries its input
// and output, its observations, and its scores.
func loadEvaluationTrace(
	ctx context.Context,
	client evaluationTraceClient,
	dataset *evaldatasetstore.EvalDataset,
	traceID string,
) (*langfuse.TraceDetail, error) {
	trace, err := client.GetTrace(ctx, traceID)
	if err != nil {
		if errors.Is(err, langfuse.ErrNotFound) || isPermanentLangfuseError(err) {
			return nil, fmt.Errorf("%w: load target trace: %w", errPermanentEvaluation, err)
		}
		return nil, fmt.Errorf("load target trace: %w", err)
	}
	if trace == nil || trace.Input == nil {
		return nil, fmt.Errorf("%w: target trace has no input", errPermanentEvaluation)
	}
	if !langfuse.HasDeploymentTag(trace.Tags, dataset.DeploymentID) {
		return nil, fmt.Errorf(
			"%w: target trace does not belong to deployment %s",
			errPermanentEvaluation,
			dataset.DeploymentID,
		)
	}
	return trace, nil
}

// buildEvaluationInput loads only the context the resolved set asks for, so a set
// that wants none costs no extra Langfuse calls.
func buildEvaluationInput(
	ctx context.Context,
	client evaluationTraceClient,
	dataset *evaldatasetstore.EvalDataset,
	trace *langfuse.TraceDetail,
	set []evaluator.Evaluator,
) (evaluator.Input, error) {
	input := evaluator.Input{
		TraceID:     trace.ID,
		TraceInput:  trace.Input,
		TraceOutput: trace.Output,
	}

	wants := evaluator.ContextConfig{}
	for _, ev := range set {
		context := ev.Config.Context
		wants.PreviousTurns = wants.PreviousTurns || context.PreviousTurns
		wants.NextUserMessage = wants.NextUserMessage || context.NextUserMessage
		wants.UserFeedback = wants.UserFeedback || context.UserFeedback
		wants.Steps = wants.Steps || context.Steps
	}

	if wants.PreviousTurns {
		previous, err := client.GetPreviousSessionTraces(
			ctx, dataset.DeploymentID, trace.UserID, trace.SessionID, trace.ID, trace.Timestamp, evaluationPreviousTurnLimit,
		)
		if err != nil {
			return evaluator.Input{}, fmt.Errorf("load previous session traces: %w", err)
		}
		for _, previousTrace := range previous {
			input.PreviousTurns = append(input.PreviousTurns, evaluator.SessionTurn{
				Input:  previousTrace.Input,
				Output: previousTrace.Output,
			})
		}
	}
	if wants.NextUserMessage {
		next, err := client.GetNextSessionTrace(ctx, dataset.DeploymentID, trace.UserID, trace.SessionID, trace.ID, trace.Timestamp)
		if err != nil {
			return evaluator.Input{}, fmt.Errorf("load next session trace: %w", err)
		}
		if next != nil {
			input.NextUserMessage = textFromAny(next.Input)
		}
	}
	if wants.UserFeedback {
		input.UserFeedback = latestThumbsFeedback(trace.Scores)
	}
	if wants.Steps {
		observations, err := hydrateObservationIO(ctx, client, trace.Observations, readableObservations(set, trace.Observations))
		if err != nil {
			return evaluator.Input{}, err
		}
		input.Steps = evaluationSteps(observations)
	}
	return input, nil
}

// readableObservations collects the observations some evaluator will actually
// read. Each one narrows to its declared types and then sees at most
// evaluator.MaxSteps of them, so anything past that window is fetched for
// nothing.
func readableObservations(set []evaluator.Evaluator, observations []langfuse.Observation) map[string]bool {
	readable := make(map[string]bool)
	for _, ev := range set {
		context := ev.Config.Context
		if !context.Steps {
			continue
		}
		declared := make(map[string]bool, len(context.StepTypes))
		for _, stepType := range context.StepTypes {
			declared[normalizeObservationType(stepType)] = true
		}
		matched := 0
		for _, observation := range observations {
			if matched == evaluator.MaxSteps {
				break
			}
			if len(declared) > 0 && !declared[normalizeObservationType(observation.Type)] {
				continue
			}
			matched++
			if observation.ID != "" {
				readable[observation.ID] = true
			}
		}
	}
	return readable
}

func hydrateObservationIO(
	ctx context.Context,
	client evaluationTraceClient,
	observations []langfuse.Observation,
	readable map[string]bool,
) ([]langfuse.Observation, error) {
	out := make([]langfuse.Observation, 0, len(observations))
	for _, observation := range observations {
		if !readable[observation.ID] {
			out = append(out, observation)
			continue
		}
		full, err := client.GetObservation(ctx, observation.ID)
		if err != nil {
			return nil, fmt.Errorf("hydrate observation %s: %w", observation.ID, err)
		}
		if full != nil {
			observation.Input = full.Input
			observation.Output = full.Output
		}
		out = append(out, observation)
	}
	return out, nil
}

func normalizeObservationType(observationType string) string {
	return strings.ToLower(strings.TrimSpace(observationType))
}

func evaluationSteps(observations []langfuse.Observation) []evaluator.Step {
	steps := make([]evaluator.Step, 0, len(observations))
	for _, observation := range observations {
		step := evaluator.Step{
			Name:   observation.Name,
			Type:   normalizeObservationType(observation.Type),
			Input:  observation.Input,
			Output: observation.Output,
		}
		if strings.EqualFold(observation.Level, "ERROR") {
			step.Error = observation.StatusMessage
			if step.Error == "" {
				step.Error = "step failed"
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// runSet evaluates the set in definition order, skipping evaluators a prior
// attempt already completed. A permanent failure is stored and the loop
// continues; a transient one still lets the remaining evaluators run before the
// job returns for retry, so each attempt makes as much progress as it can.
func (w *EvalDatasetEvaluationWorker) runSet(
	ctx context.Context,
	job *river.Job[EvalDatasetEvaluationArgs],
	runID string,
	runner evaluationRunner,
	apiKey string,
	set []evaluator.Evaluator,
	input evaluator.Input,
	completedBefore int,
) error {
	finalAttempt := job.Attempt >= job.MaxAttempts
	completed := completedBefore
	var retryable []error

	for _, ev := range set {
		if err := w.runs.MarkResultInProgress(ctx, runID, ev.Key); err != nil {
			return w.retryOrFailRun(ctx, job, runID, fmt.Errorf("mark evaluator %q in progress: %w", ev.Key, err))
		}

		result, err := runner.Evaluate(ctx, apiKey, ev, input)
		if err == nil {
			if err := w.runs.CompleteResult(ctx, runID, evalrunstore.Result{
				EvaluatorKey: ev.Key,
				Value:        result.Value,
				Confidence:   result.Confidence,
				Explanation:  result.Explanation,
			}); err != nil {
				return w.retryOrFailRun(ctx, job, runID, fmt.Errorf("store evaluator %q result: %w", ev.Key, err))
			}
			completed++
			continue
		}

		message := evaluationFailureMessage
		switch {
		case isBudgetExceeded(err):
			message = evaluationQuotaMessage
		case isPermanentInvocationError(err), errors.Is(err, evaluator.ErrInvalidOutput), errors.Is(err, evaluator.ErrInvalidDefinition):
		default:
			if !finalAttempt {
				retryable = append(retryable, fmt.Errorf("evaluator %q: %w", ev.Key, err))
				continue
			}
		}
		if storeErr := w.runs.FailResult(ctx, runID, ev.Key, message); storeErr != nil {
			return w.retryOrFailRun(ctx, job, runID, fmt.Errorf("store evaluator %q failure: %w", ev.Key, storeErr))
		}
		w.logEvaluatorFailure(job, ev.Key, err)
	}

	if len(retryable) > 0 {
		return errors.Join(retryable...)
	}
	status, message := evalrunstore.StatusCompleted, (*string)(nil)
	if completed == 0 {
		noResult := evaluationNoResultMessage
		status, message = evalrunstore.StatusFailed, &noResult
	}
	if err := w.runs.FinalizeRun(ctx, runID, status, message); err != nil {
		return fmt.Errorf("finalize evaluation run: %w", err)
	}
	return nil
}

// retryOrFailRun leaves retry scheduling to River and finalizes the run only
// once the attempts are spent, so an exhausted job never leaves the run
// in progress forever.
func (w *EvalDatasetEvaluationWorker) retryOrFailRun(
	ctx context.Context,
	job *river.Job[EvalDatasetEvaluationArgs],
	runID string,
	cause error,
) error {
	if job.Attempt < job.MaxAttempts && !errors.Is(cause, errPermanentEvaluation) {
		return cause
	}
	return w.failRun(ctx, runID, cause)
}

// failRun records a trace-level failure that stopped evaluators from running at
// all, which is the only case a run itself ends as failed.
func (w *EvalDatasetEvaluationWorker) failRun(ctx context.Context, runID string, cause error) error {
	return w.failRunWithMessage(ctx, runID, evaluationFailureMessage, cause)
}

// failRunWithMessage is failRun with the reason the caller wants recorded, so a
// run stopped for billing does not report a generic evaluation failure.
func (w *EvalDatasetEvaluationWorker) failRunWithMessage(ctx context.Context, runID, message string, cause error) error {
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := w.runs.FailPendingResults(finalizeCtx, runID, message); err != nil {
		return errors.Join(cause, fmt.Errorf("fail pending results: %w", err))
	}
	if err := w.runs.FinalizeRun(finalizeCtx, runID, evalrunstore.StatusFailed, &message); err != nil {
		return errors.Join(cause, fmt.Errorf("finalize failed run: %w", err))
	}
	return river.JobCancel(cause)
}

func (w *EvalDatasetEvaluationWorker) logEvaluatorFailure(job *river.Job[EvalDatasetEvaluationArgs], key string, err error) {
	if w.log == nil {
		return
	}
	w.log.Error("eval dataset evaluation: evaluator failed",
		"eval_dataset_id", job.Args.EvalDatasetID,
		"trace_id", job.Args.TraceID,
		"evaluator_key", key,
		"attempt", job.Attempt,
		"max_attempts", job.MaxAttempts,
		"error", err,
	)
}

type evaluationBillingGate interface {
	Blocked(ctx context.Context, accountID string) bool
}

type evaluationStatusGate struct {
	status  *billing.StatusStore
	enforce bool
	log     *logger.Logger
}

func (g evaluationStatusGate) Blocked(ctx context.Context, accountID string) bool {
	if g.status == nil || accountID == "" {
		return false
	}
	rec, err := g.status.Record(ctx, accountID)
	if err != nil {
		g.log.Warn("eval dataset evaluation: billing status read failed, allowing", "account_id", accountID, "error", err)
		return false
	}
	if rec.Status != billing.StatusSuspended {
		return false
	}
	if !g.enforce {
		g.log.Info("eval dataset evaluation: billing gate observe, would block", "account_id", accountID)
		return false
	}
	return true
}

func textFromAny(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case map[string]any:
		for _, key := range []string{"content", "text", "message", "input"} {
			if val, ok := x[key]; ok {
				if s := textFromAny(val); s != "" {
					return s
				}
			}
		}
		return ""
	case []any:
		for i := len(x) - 1; i >= 0; i-- {
			if s := textFromAny(x[i]); s != "" {
				return s
			}
		}
		return ""
	default:
		return ""
	}
}

func latestThumbsFeedback(scores []langfuse.Score) string {
	var latestValue, latestAt, latestID string
	for _, score := range scores {
		if score.Name != "user_feedback" ||
			(score.StringValue != "thumbs_up" && score.StringValue != "thumbs_down") {
			continue
		}
		if score.CreatedAt > latestAt || (score.CreatedAt == latestAt && score.ID > latestID) {
			latestValue = score.StringValue
			latestAt = score.CreatedAt
			latestID = score.ID
		}
	}
	return latestValue
}

func isBudgetExceeded(err error) bool {
	var invocationErr *aigateway.InvocationError
	return errors.As(err, &invocationErr) && invocationErr.StatusCode == http.StatusPaymentRequired
}

func isPermanentInvocationError(err error) bool {
	var invocationErr *aigateway.InvocationError
	return errors.As(err, &invocationErr) && isPermanentHTTPStatus(invocationErr.StatusCode)
}

func isPermanentLangfuseError(err error) bool {
	var apiErr *langfuse.APIError
	return errors.As(err, &apiErr) && isPermanentHTTPStatus(apiErr.StatusCode)
}

func isPermanentHTTPStatus(code int) bool {
	return code >= http.StatusBadRequest &&
		code < http.StatusInternalServerError &&
		code != http.StatusRequestTimeout &&
		code != http.StatusTooManyRequests
}
