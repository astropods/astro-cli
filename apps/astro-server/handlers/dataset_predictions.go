package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

const (
	maxDatasetPredictionTraceIDs    = 50
	predictionEnqueueFailureMessage = "Failed to enqueue prediction. Try again."
)

// DatasetPredictionsResponse reports which traces reached queue insertion or
// failed during submission.
type DatasetPredictionsResponse struct {
	EnqueuedTraceIDs []string `json:"enqueued_trace_ids"`
	FailedTraceIDs   []string `json:"failed_trace_ids"`
}

type datasetPredictionLangfuseStore interface {
	GetDecrypted(context.Context, envelope.KMSClient, string) (*langfuse.AccountLangfuse, error)
}

type datasetPredictionStore interface {
	reviewQueueScanStore
	QueuePredictionRequests(ctx context.Context, evalDatasetID string, traceIDs []string) ([]string, error)
	UpdatePredictionRequests(
		ctx context.Context,
		evalDatasetID string,
		traceIDs []string,
		status judgmentstore.PredictionRequestStatus,
		errorMessage *string,
	) error
}

type datasetPredictionQueue interface {
	InsertEvalJudgePredictionJobs(ctx context.Context, evalDatasetID string, traceIDs []string) error
}

// PostDatasetPredictions queues one independent prediction job for each of the
// most recent eligible traces. Trace loading and model invocation happen only
// in the worker.
func PostDatasetPredictions(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	langfuseStore datasetPredictionLangfuseStore,
	kmsClient envelope.KMSClient,
	predictionStore datasetPredictionStore,
	queue datasetPredictionQueue,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		deploymentID := c.Param("id")
		deployment, err := deploymentStore.GetDeploymentByID(deploymentID)
		if err != nil || deployment == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
			return
		}

		isMember, err := accountStore.IsMember(deployment.AccountID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		if cfg == nil ||
			cfg.Deployment.LangfuseBaseURL == "" ||
			langfuseStore == nil ||
			predictionStore == nil ||
			queue == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dataset prediction generation is not configured"})
			return
		}

		dataset, ok := loadDataset(c, log, datasetStore, deploymentID)
		if !ok {
			return
		}

		response := DatasetPredictionsResponse{
			EnqueuedTraceIDs: make([]string, 0, maxDatasetPredictionTraceIDs),
			FailedTraceIDs:   make([]string, 0),
		}

		credentials, err := langfuseStore.GetDecrypted(c.Request.Context(), kmsClient, deployment.AccountID)
		if err != nil || credentials == nil {
			log.Error("Failed to load Langfuse credentials for predictions", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dataset prediction generation is not configured"})
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
			predictionStore,
			dataset.ID,
			deploymentID,
			maxDatasetPredictionTraceIDs,
			reviewQueuePredictionNone,
			newReviewQueueCursor(
				dataset.ID,
				reviewQueuePredictionNone,
				maxDatasetPredictionTraceIDs,
			),
		)
		if err != nil {
			if errors.Is(err, errReviewQueueLocalRead) {
				log.Error("Failed to load dataset prediction state", "error", err, "deployment_id", deploymentID)
				c.JSON(http.StatusInternalServerError, response)
				return
			}
			log.Error("Failed to load recent traces for predictions", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load recent traces for predictions"})
			return
		}
		traceIDs := make([]string, 0, len(recentQueue.Items))
		for _, item := range recentQueue.Items {
			traceIDs = append(traceIDs, item.TraceID)
		}
		if len(traceIDs) == 0 {
			c.JSON(http.StatusAccepted, response)
			return
		}
		queuedTraceIDs, err := predictionStore.QueuePredictionRequests(c.Request.Context(), dataset.ID, traceIDs)
		if err != nil {
			log.Error("Failed to persist dataset prediction requests", "error", err, "deployment_id", deploymentID)
			response.FailedTraceIDs = append(response.FailedTraceIDs, traceIDs...)
			c.JSON(http.StatusInternalServerError, response)
			return
		}
		if err := queue.InsertEvalJudgePredictionJobs(c.Request.Context(), dataset.ID, traceIDs); err != nil {
			log.Error("Failed to enqueue dataset predictions", "error", err, "deployment_id", deploymentID)
			errorMessage := predictionEnqueueFailureMessage
			if len(queuedTraceIDs) > 0 {
				if updateErr := predictionStore.UpdatePredictionRequests(
					c.Request.Context(),
					dataset.ID,
					queuedTraceIDs,
					judgmentstore.PredictionRequestFailed,
					&errorMessage,
				); updateErr != nil {
					log.Error(
						"Failed to mark dataset prediction requests as failed",
						"error", updateErr,
						"deployment_id", deploymentID,
					)
				}
			}
			response.FailedTraceIDs = append(response.FailedTraceIDs, traceIDs...)
			c.JSON(http.StatusInternalServerError, response)
			return
		}

		response.EnqueuedTraceIDs = append(response.EnqueuedTraceIDs, traceIDs...)
		c.JSON(http.StatusAccepted, response)
	}
}
