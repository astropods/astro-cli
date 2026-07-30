package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

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
	datasetPredictionTracePageSize  = 100
	datasetPredictionMaxScanPages   = 3
	predictionEnqueueFailureMessage = "Failed to enqueue prediction. Try again."
)

var errDatasetPredictionLocalRead = errors.New("dataset prediction local read")

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
	JudgedTraceIDs(ctx context.Context, evalDatasetID string, traceIDs []string) (map[string]bool, error)
	GetPredictions(ctx context.Context, evalDatasetID string, traceIDs []string) (map[string]judgmentstore.Prediction, error)
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
		traceIDs, err := selectRecentPredictionTraceIDs(
			c.Request.Context(),
			traceClient,
			predictionStore,
			dataset.ID,
			deploymentID,
		)
		if err != nil {
			if errors.Is(err, errDatasetPredictionLocalRead) {
				log.Error("Failed to load dataset prediction state", "error", err, "deployment_id", deploymentID)
				c.JSON(http.StatusInternalServerError, response)
				return
			}
			log.Error("Failed to load recent traces for predictions", "error", err, "deployment_id", deploymentID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to load recent traces for predictions"})
			return
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

func selectRecentPredictionTraceIDs(
	ctx context.Context,
	client *langfuse.Client,
	predictionStore datasetPredictionStore,
	evalDatasetID, deploymentID string,
) ([]string, error) {
	selected := make([]string, 0, maxDatasetPredictionTraceIDs)
	endTime := time.Now().UTC().Format(time.RFC3339Nano)

	for page := 1; page <= datasetPredictionMaxScanPages; page++ {
		traces, err := client.GetQueueTraces(
			ctx,
			deploymentID,
			endTime,
			datasetPredictionTracePageSize,
			(page-1)*datasetPredictionTracePageSize,
		)
		if err != nil {
			return nil, err
		}
		if len(traces.Data) == 0 {
			break
		}

		traceIDs := make([]string, 0, len(traces.Data))
		for _, trace := range traces.Data {
			traceIDs = append(traceIDs, trace.ID)
		}
		judged, err := predictionStore.JudgedTraceIDs(ctx, evalDatasetID, traceIDs)
		if err != nil {
			return nil, fmt.Errorf("%w: judgments: %w", errDatasetPredictionLocalRead, err)
		}
		predictions, err := predictionStore.GetPredictions(ctx, evalDatasetID, traceIDs)
		if err != nil {
			return nil, fmt.Errorf("%w: predictions: %w", errDatasetPredictionLocalRead, err)
		}

		for _, trace := range traces.Data {
			if trace.Input == nil || judged[trace.ID] {
				continue
			}
			if _, exists := predictions[trace.ID]; exists {
				continue
			}
			selected = append(selected, trace.ID)
			if len(selected) == maxDatasetPredictionTraceIDs {
				return selected, nil
			}
		}
		if !datasetPredictionTracesHaveNextPage(traces, page) {
			break
		}
	}
	return selected, nil
}

func datasetPredictionTracesHaveNextPage(traces *langfuse.TracesResponse, page int) bool {
	if len(traces.Data) == 0 {
		return false
	}
	if traces.Meta.TotalPages > 0 {
		currentPage := traces.Meta.Page
		if currentPage <= 0 {
			currentPage = page
		}
		return currentPage < traces.Meta.TotalPages
	}
	if traces.Meta.TotalItems > 0 {
		return page*datasetPredictionTracePageSize < traces.Meta.TotalItems
	}
	return len(traces.Data) == datasetPredictionTracePageSize
}
