package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalrunstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const (
	maxDatasetEvaluationTraceIDs    = 50
	evaluationEnqueueFailureMessage = "Failed to enqueue evaluation. Try again."
)

// DatasetEvaluationsResponse reports which traces reached queue insertion or
// failed during submission.
type DatasetEvaluationsResponse struct {
	EnqueuedTraceIDs []string `json:"enqueued_trace_ids"`
	FailedTraceIDs   []string `json:"failed_trace_ids"`
}

type DatasetEvaluationStatusResponse struct {
	Queued     int `json:"queued"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
}

type datasetEvaluationStatusStore interface {
	StatusCounts(context.Context, string) (evalrunstore.StatusCounts, error)
}

type datasetEvaluationLangfuseStore interface {
	Get(string) (*langfuse.AccountLangfuse, error)
}

type datasetEvaluationRunStore interface {
	reviewQueueRunStore
	CreateQueuedRuns(
		ctx context.Context,
		evalDatasetID, evaluationRef string,
		traces []evalrunstore.RunTrace,
	) ([]string, error)
	FailQueuedRuns(
		ctx context.Context,
		evalDatasetID, evaluationRef string,
		traceIDs []string,
		message string,
	) error
}

type datasetEvaluationQueue interface {
	InsertEvalDatasetEvaluationJobs(ctx context.Context, evalDatasetID string, traceIDs []string) error
}

// GetDatasetEvaluationStatus returns deployment-wide evaluation run counts
// independently from the filtered review queue.
func GetDatasetEvaluationStatus(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	runStore datasetEvaluationStatusStore,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		dctx, ok := resolveDeploymentAccess(c, accountStore, deploymentStore)
		if !ok {
			return
		}

		dataset, ok := loadDataset(c, log, datasetStore, dctx.DeploymentID)
		if !ok {
			return
		}

		counts, err := runStore.StatusCounts(c.Request.Context(), dataset.ID)
		if err != nil {
			log.Error("dataset evaluations: load evaluation status failed", "error", err, "deployment_id", dctx.DeploymentID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load dataset evaluation status"})
			return
		}
		c.JSON(http.StatusOK, DatasetEvaluationStatusResponse{
			Queued:     counts.Queued,
			InProgress: counts.InProgress,
			Completed:  counts.Completed,
			Failed:     counts.Failed,
		})
	}
}

// PostDatasetEvaluations queues one evaluation run for each of the most recent
// eligible traces. Trace loading and model invocation happen only in the worker.
func PostDatasetEvaluations(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore datasetEvaluationLangfuseStore,
	itemStore reviewQueueScanStore,
	runStore datasetEvaluationRunStore,
	dismissalStore reviewQueueDismissalStore,
	queue datasetEvaluationQueue,
	entCheck EntitlementChecker,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		dctx, ok := resolveDeploymentAccess(c, accountStore, deploymentStore)
		if !ok {
			return
		}
		// A run bills model usage to the account's own gateway key, so it is a
		// consuming action and gated like a deploy. Checked before the queue,
		// since a queued job outlives the request that made it.
		if blockedByBilling(c, entCheck, dctx.Deployment.AccountID) {
			return
		}
		deploymentID := dctx.DeploymentID

		if cfg == nil ||
			cfg.Deployment.LangfuseBaseURL == "" ||
			langfuseStore == nil ||
			itemStore == nil ||
			runStore == nil ||
			dismissalStore == nil ||
			queue == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dataset evaluation is not configured"})
			return
		}

		dataset, ok := loadDataset(c, log, datasetStore, deploymentID)
		if !ok {
			return
		}

		response := DatasetEvaluationsResponse{
			EnqueuedTraceIDs: make([]string, 0, maxDatasetEvaluationTraceIDs),
			FailedTraceIDs:   make([]string, 0),
		}

		credentials, err := langfuseStore.Get(dctx.Deployment.AccountID)
		if err != nil || credentials == nil {
			log.Error("dataset evaluations: load Langfuse credentials failed", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dataset evaluation is not configured"})
			return
		}
		traceClient := langfuse.NewClient(
			cfg.Deployment.LangfuseBaseURL,
			credentials.PublicKey,
			credentials.SecretKey,
		)
		recentQueue, err := scanLangfuseReviewQueuePages(
			c.Request.Context(),
			traceClient,
			itemStore,
			runStore,
			dismissalStore,
			dataset.ID,
			deploymentID,
			maxDatasetEvaluationTraceIDs,
			reviewQueueNotEvaluated,
			newReviewQueueCursor(
				dataset.ID,
				reviewQueueNotEvaluated,
				maxDatasetEvaluationTraceIDs,
			),
		)
		if err != nil {
			if errors.Is(err, errReviewQueueLocalRead) {
				log.Error("dataset evaluations: load evaluation state failed", "error", err, "deployment_id", deploymentID)
				c.JSON(http.StatusInternalServerError, response)
				return
			}
			log.Error("dataset evaluations: load recent traces failed", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load recent traces for evaluations"})
			return
		}

		eligible := make([]evalrunstore.RunTrace, 0, len(recentQueue.Items))
		traceIDs := make([]string, 0, len(recentQueue.Items))
		for _, item := range recentQueue.Items {
			if !evaluationTraceIsEligible(item) {
				continue
			}
			timestamp, err := time.Parse(time.RFC3339Nano, item.Timestamp)
			if err != nil {
				log.Error("dataset evaluations: parse trace timestamp failed, trace skipped",
					"error", err, "deployment_id", deploymentID, "trace_id", item.TraceID)
				response.FailedTraceIDs = append(response.FailedTraceIDs, item.TraceID)
				continue
			}
			eligible = append(eligible, evalrunstore.RunTrace{TraceID: item.TraceID, TraceTimestamp: timestamp})
			traceIDs = append(traceIDs, item.TraceID)
		}
		if len(traceIDs) == 0 {
			c.JSON(http.StatusAccepted, response)
			return
		}
		queuedTraceIDs, err := runStore.CreateQueuedRuns(
			c.Request.Context(),
			dataset.ID,
			activeEvaluationRef,
			eligible,
		)
		if err != nil {
			log.Error("dataset evaluations: record evaluation runs failed", "error", err, "deployment_id", deploymentID)
			response.FailedTraceIDs = append(response.FailedTraceIDs, traceIDs...)
			c.JSON(http.StatusInternalServerError, response)
			return
		}
		if err := queue.InsertEvalDatasetEvaluationJobs(c.Request.Context(), dataset.ID, traceIDs); err != nil {
			log.Error("dataset evaluations: enqueue evaluation jobs failed", "error", err, "deployment_id", deploymentID)
			if failErr := runStore.FailQueuedRuns(
				c.Request.Context(),
				dataset.ID,
				activeEvaluationRef,
				queuedTraceIDs,
				evaluationEnqueueFailureMessage,
			); failErr != nil {
				log.Error("dataset evaluations: mark evaluation runs failed", "error", failErr, "deployment_id", deploymentID)
			}
			response.FailedTraceIDs = append(response.FailedTraceIDs, traceIDs...)
			c.JSON(http.StatusInternalServerError, response)
			return
		}

		response.EnqueuedTraceIDs = append(response.EnqueuedTraceIDs, traceIDs...)
		c.JSON(http.StatusAccepted, response)
	}
}

// evaluationTraceIsEligible skips a trace whose run is already queued or in
// flight. The queue filter has already dropped traces holding a completed run.
func evaluationTraceIsEligible(item DatasetReviewQueueItem) bool {
	return item.Run == nil ||
		item.Run.Status == string(evalrunstore.StatusFailed)
}
