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
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaljudge"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const (
	evalJudgePredictionTimeout = 2 * time.Minute
	predictionFailureMessage   = "Prediction generation failed. Try again."
	predictionQuotaMessage     = "AI usage quota exceeded. Try again after the quota resets or is increased."
)

var errEvalJudgeNotConfigured = errors.New("eval judge is not configured")

// EvalJudgePredictionArgs identifies one prediction target. Trace content and
// credentials are resolved by the worker at execution time.
type EvalJudgePredictionArgs struct {
	EvalDatasetID string `json:"eval_dataset_id"`
	TraceID       string `json:"trace_id"`
}

func (EvalJudgePredictionArgs) Kind() string { return "eval_dataset.judge_prediction" }

func init() {
	registerJobKind[EvalJudgePredictionArgs]()
}

func (EvalJudgePredictionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueEvalJudge,
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

type evalJudgeDatasetStore interface {
	GetByID(context.Context, string) (*evaldatasetstore.EvalDataset, error)
}

type evalJudgePredictionStore interface {
	GetPredictionRequests(context.Context, string, []string) (map[string]judgmentstore.PredictionRequest, error)
	GetPredictions(context.Context, string, []string) (map[string]judgmentstore.Prediction, error)
	UpdatePredictionRequest(context.Context, string, string, judgmentstore.PredictionRequestStatus, *string) error
	DeletePredictionRequest(context.Context, string, string) error
	IsJudged(context.Context, string, string) (bool, error)
	UpsertPrediction(context.Context, string, string, judgmentstore.Prediction) error
}

type evalJudgeTraceClient interface {
	GetTrace(context.Context, string) (*langfuse.TraceDetail, error)
	GetNextSessionTrace(context.Context, string, string, string, string, string) (*langfuse.Trace, error)
}

type evalJudgePredictor interface {
	Predict(context.Context, string, evaljudge.Input) (evaljudge.Result, error)
}

type EvalJudgePredictionWorker struct {
	river.WorkerDefaults[EvalJudgePredictionArgs]
	datasets       evalJudgeDatasetStore
	predictions    evalJudgePredictionStore
	loadLangfuse   func(context.Context, string) (*langfuse.AccountLangfuse, error)
	newTraceClient func(*langfuse.AccountLangfuse) evalJudgeTraceClient
	ensureJudgeKey func(context.Context, string) (string, string, error)
	newPredictor   func(string) evalJudgePredictor
	log            *logger.Logger
}

func (w *EvalJudgePredictionWorker) Timeout(*river.Job[EvalJudgePredictionArgs]) time.Duration {
	return evalJudgePredictionTimeout
}

func (w *EvalJudgePredictionWorker) Work(ctx context.Context, job *river.Job[EvalJudgePredictionArgs]) error {
	args := job.Args
	if strings.TrimSpace(args.EvalDatasetID) == "" || strings.TrimSpace(args.TraceID) == "" {
		return w.cancelWithoutFailureState(
			job,
			fmt.Errorf("eval judge prediction: dataset and trace IDs are required"),
		)
	}
	if w.datasets == nil || w.predictions == nil {
		return w.cancelWithoutFailureState(
			job,
			fmt.Errorf("eval judge prediction: %w", errEvalJudgeNotConfigured),
		)
	}

	requests, err := w.predictions.GetPredictionRequests(ctx, args.EvalDatasetID, []string{args.TraceID})
	if err != nil {
		return w.retryOrRecordFailure(job, fmt.Errorf("load prediction request: %w", err))
	}
	// The request may have been deleted after this River job was enqueued.
	if _, ok := requests[args.TraceID]; !ok {
		return nil
	}

	predictions, err := w.predictions.GetPredictions(ctx, args.EvalDatasetID, []string{args.TraceID})
	if err != nil {
		return w.retryOrRecordFailure(job, fmt.Errorf("load existing prediction: %w", err))
	}
	// A prior attempt may have stored the result before crashing. Treat the
	// prediction as authoritative and repair only the request status.
	if _, ok := predictions[args.TraceID]; ok {
		if err := w.predictions.UpdatePredictionRequest(
			ctx,
			args.EvalDatasetID,
			args.TraceID,
			judgmentstore.PredictionRequestCompleted,
			nil,
		); err != nil {
			return w.retryOrRecordFailure(job, fmt.Errorf("repair completed prediction request: %w", err))
		}
		return nil
	}

	judged, err := w.predictions.IsJudged(ctx, args.EvalDatasetID, args.TraceID)
	if err != nil {
		return w.retryOrRecordFailure(job, fmt.Errorf("check target judgment: %w", err))
	}
	// Human judgment makes the queued prediction obsolete.
	if judged {
		if err := w.predictions.DeletePredictionRequest(ctx, args.EvalDatasetID, args.TraceID); err != nil {
			return w.retryOrRecordFailure(job, fmt.Errorf("delete obsolete prediction request: %w", err))
		}
		return nil
	}

	if err := w.predictions.UpdatePredictionRequest(
		ctx,
		args.EvalDatasetID,
		args.TraceID,
		judgmentstore.PredictionRequestInProgress,
		nil,
	); err != nil {
		return w.retryOrRecordFailure(job, fmt.Errorf("mark prediction in progress: %w", err))
	}

	dataset, err := w.datasets.GetByID(ctx, args.EvalDatasetID)
	if err != nil {
		return w.retryOrRecordFailure(job, fmt.Errorf("load eval dataset: %w", err))
	}
	if dataset == nil {
		return w.failPermanent(job, fmt.Errorf("eval dataset %q no longer exists", args.EvalDatasetID), predictionFailureMessage)
	}
	// Keep the worker registered without configuration so old jobs become
	// durable failures instead of unknown River job kinds.
	if w.loadLangfuse == nil || w.newTraceClient == nil || w.ensureJudgeKey == nil || w.newPredictor == nil {
		return w.failPermanent(job, errEvalJudgeNotConfigured, predictionFailureMessage)
	}

	// Load existing account credentials only; prediction work never provisions
	// a new Langfuse project.
	credentials, err := w.loadLangfuse(ctx, dataset.AccountID)
	if err != nil {
		if errors.Is(err, langfuse.ErrCredentialsDecrypt) {
			return w.failPermanent(job, fmt.Errorf("load Langfuse credentials: %w", err), predictionFailureMessage)
		}
		return w.retryOrRecordFailure(job, fmt.Errorf("load Langfuse credentials: %w", err))
	}
	if credentials == nil {
		return w.failPermanent(job, fmt.Errorf("langfuse is not configured for account %s", dataset.AccountID), predictionFailureMessage)
	}
	if strings.TrimSpace(credentials.PublicKey) == "" || strings.TrimSpace(credentials.SecretKey) == "" {
		return w.failPermanent(job, fmt.Errorf("langfuse credentials are incomplete for account %s", dataset.AccountID), predictionFailureMessage)
	}
	traceClient := w.newTraceClient(credentials)

	trace, err := traceClient.GetTrace(ctx, args.TraceID)
	if err != nil {
		if errors.Is(err, langfuse.ErrNotFound) || isPermanentLangfuseError(err) {
			return w.failPermanent(job, fmt.Errorf("load target trace: %w", err), predictionFailureMessage)
		}
		return w.retryOrRecordFailure(job, fmt.Errorf("load target trace: %w", err))
	}
	if trace == nil || trace.Input == nil {
		return w.failPermanent(job, fmt.Errorf("target trace has no input"), predictionFailureMessage)
	}
	// Do not judge a trace supplied from another deployment.
	if !langfuse.HasDeploymentTag(trace.Tags, dataset.DeploymentID) {
		return w.failPermanent(job, fmt.Errorf("target trace does not belong to deployment %s", dataset.DeploymentID), predictionFailureMessage)
	}
	traceTimestamp, err := time.Parse(time.RFC3339Nano, trace.Timestamp)
	if err != nil {
		return w.failPermanent(job, fmt.Errorf("target trace has invalid timestamp %q", trace.Timestamp), predictionFailureMessage)
	}

	// A later message is optional reaction context, not a requirement for a
	// prediction.
	nextUserText := ""
	nextTrace, err := traceClient.GetNextSessionTrace(
		ctx,
		dataset.DeploymentID,
		trace.UserID,
		trace.SessionID,
		trace.ID,
		trace.Timestamp,
	)
	if err != nil {
		if isPermanentLangfuseError(err) {
			return w.failPermanent(job, fmt.Errorf("load next session trace: %w", err), predictionFailureMessage)
		}
		return w.retryOrRecordFailure(job, fmt.Errorf("load next session trace: %w", err))
	}
	if nextTrace != nil {
		nextUserText = textFromAny(nextTrace.Input)
	}

	apiKey, gatewayURL, err := w.ensureJudgeKey(ctx, dataset.AccountID)
	if err != nil {
		if errors.Is(err, aigateway.ErrJudgeKeyDecrypt) {
			return w.failPermanent(job, fmt.Errorf("ensure judge key: %w", err), predictionFailureMessage)
		}
		// A deterministic-key conflict can be a concurrent first provision:
		// another job may have minted the key but not saved its local row yet.
		return w.retryOrRecordFailure(job, fmt.Errorf("ensure judge key: %w", err))
	}

	predictor := w.newPredictor(gatewayURL)
	result, err := predictor.Predict(ctx, apiKey, evaljudge.Input{
		TraceID:        trace.ID,
		TraceInput:     trace.Input,
		TraceOutput:    trace.Output,
		NextUserText:   nextUserText,
		ThumbsFeedback: latestThumbsFeedback(trace.Scores),
	})
	if err != nil {
		// Quota exhaustion is terminal for this request; ordinary throttling and
		// transient gateway failures remain retryable.
		if isBudgetExceeded(err) {
			return w.failPermanent(job, fmt.Errorf("invoke eval judge: %w", err), predictionQuotaMessage)
		}
		if isPermanentInvocationError(err) {
			return w.failPermanent(job, fmt.Errorf("invoke eval judge: %w", err), predictionFailureMessage)
		}
		return w.retryOrRecordFailure(job, fmt.Errorf("invoke eval judge: %w", err))
	}

	// Store the result before completing the request so retries can recover a
	// crash between these two writes without invoking the model again.
	result.Prediction.TraceTimestamp = traceTimestamp
	if err := w.predictions.UpsertPrediction(ctx, args.EvalDatasetID, args.TraceID, result.Prediction); err != nil {
		return w.retryOrRecordFailure(job, fmt.Errorf("store prediction: %w", err))
	}
	if err := w.predictions.UpdatePredictionRequest(
		ctx,
		args.EvalDatasetID,
		args.TraceID,
		judgmentstore.PredictionRequestCompleted,
		nil,
	); err != nil {
		return w.retryOrRecordFailure(job, fmt.Errorf("mark prediction completed: %w", err))
	}

	if w.log != nil {
		logArgs := []any{
			"eval_dataset_id", args.EvalDatasetID,
			"trace_id", args.TraceID,
		}
		if result.Usage != nil {
			logArgs = append(logArgs,
				"prompt_tokens", result.Usage.PromptTokens,
				"completion_tokens", result.Usage.CompletionTokens,
				"total_tokens", result.Usage.TotalTokens,
			)
		}
		w.log.Info("eval judge prediction completed", logArgs...)
	}
	return nil
}

// textFromAny extracts representative user text from common structured trace
// input shapes. Strings pass through, maps prefer content-like fields, and
// arrays are searched newest-first.
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

// isPermanentHTTPStatus reports whether an HTTP status should end a job rather
// than retry it: client errors, except the transient timeout/throttle codes.
func isPermanentHTTPStatus(code int) bool {
	return code >= http.StatusBadRequest &&
		code < http.StatusInternalServerError &&
		code != http.StatusRequestTimeout &&
		code != http.StatusTooManyRequests
}

// retryOrRecordFailure leaves retry scheduling to River and records durable
// failure state only when the job has exhausted its attempts.
func (w *EvalJudgePredictionWorker) retryOrRecordFailure(job *river.Job[EvalJudgePredictionArgs], err error) error {
	if job.Attempt < job.MaxAttempts {
		w.logPredictionFailure("eval judge prediction will retry", job, err,
			"disposition", "retry",
		)
		return err
	}
	if statusErr := w.storeFailure(job.Args, predictionFailureMessage); statusErr != nil {
		combinedErr := errors.Join(err, statusErr)
		w.logPredictionFailure("eval judge prediction failed and failure status could not be recorded", job, combinedErr,
			"disposition", "final_attempt",
		)
		return combinedErr
	}
	w.logPredictionFailure("eval judge prediction failed after final attempt", job, err,
		"disposition", "final_attempt",
	)
	return err
}

func (w *EvalJudgePredictionWorker) failPermanent(
	job *river.Job[EvalJudgePredictionArgs],
	err error,
	message string,
) error {
	if statusErr := w.storeFailure(job.Args, message); statusErr != nil {
		// Do not cancel the River job until durable failure state is visible.
		combinedErr := errors.Join(err, statusErr)
		w.logPredictionFailure("eval judge prediction failed permanently and failure status could not be recorded", job, combinedErr,
			"disposition", "retry_status_write",
		)
		return combinedErr
	}
	w.logPredictionFailure("eval judge prediction failed permanently", job, err,
		"disposition", "cancel",
	)
	return river.JobCancel(err)
}

func (w *EvalJudgePredictionWorker) cancelWithoutFailureState(
	job *river.Job[EvalJudgePredictionArgs],
	err error,
) error {
	w.logPredictionFailure("eval judge prediction job rejected", job, err,
		"disposition", "cancel",
	)
	return river.JobCancel(err)
}

func (w *EvalJudgePredictionWorker) logPredictionFailure(
	message string,
	job *river.Job[EvalJudgePredictionArgs],
	err error,
	extra ...any,
) {
	if w.log == nil {
		return
	}
	logArgs := []any{
		"eval_dataset_id", job.Args.EvalDatasetID,
		"trace_id", job.Args.TraceID,
		"attempt", job.Attempt,
		"max_attempts", job.MaxAttempts,
		"error", err,
	}
	w.log.Error(message, append(logArgs, extra...)...)
}

func (w *EvalJudgePredictionWorker) storeFailure(args EvalJudgePredictionArgs, message string) error {
	updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.predictions.UpdatePredictionRequest(
		updateCtx,
		args.EvalDatasetID,
		args.TraceID,
		judgmentstore.PredictionRequestFailed,
		&message,
	); err != nil {
		return fmt.Errorf("store prediction failure: %w", err)
	}
	return nil
}
