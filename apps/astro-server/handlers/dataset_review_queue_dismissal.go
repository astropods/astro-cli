package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldatasetstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldismissalstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

type reviewQueueDismissalWriteStore interface {
	Dismiss(ctx context.Context, evalDatasetID, traceID string) error
	Restore(ctx context.Context, evalDatasetID, traceID string) error
}

// ReviewQueueDismissalResponse reports a trace's dismissal state after the mutation.
type ReviewQueueDismissalResponse struct {
	EvalDatasetID string `json:"eval_dataset_id"`
	TraceID       string `json:"trace_id"`
	Dismissed     bool   `json:"dismissed"`
}

// PostReviewQueueDismissal removes a trace from the review queue without adding
// it to the dataset, leaving Langfuse and the trace's evaluator results untouched.
// Dismissing an already dismissed trace succeeds.
// POST /api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss
func PostReviewQueueDismissal(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	dismissalStore reviewQueueDismissalWriteStore,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		dctx, ok := resolveDeploymentAccess(c, accountStore, deploymentStore)
		if !ok {
			return
		}

		traceID, ok := requireTraceIDParam(c)
		if !ok {
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, dctx.DeploymentID)
		if !ok {
			return
		}

		if err := dismissalStore.Dismiss(c.Request.Context(), ds.ID, traceID); err != nil {
			if errors.Is(err, evaldismissalstore.ErrIsDatasetItem) {
				c.JSON(http.StatusConflict, gin.H{"error": "trace is a dataset item"})
				return
			}
			log.Error("dataset review queue dismissal: dismiss trace failed", "error", err, "deployment_id", dctx.DeploymentID, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to dismiss the trace"})
			return
		}

		c.JSON(http.StatusOK, ReviewQueueDismissalResponse{
			EvalDatasetID: ds.ID,
			TraceID:       traceID,
			Dismissed:     true,
		})
	}
}

// DeleteReviewQueueDismissal returns a dismissed trace to the review queue.
// Restoring an undismissed trace succeeds.
// DELETE /api/v1/deployments/:id/dataset/review-queue/:trace_id/dismiss
func DeleteReviewQueueDismissal(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	datasetStore *evaldatasetstore.Store,
	dismissalStore reviewQueueDismissalWriteStore,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		dctx, ok := resolveDeploymentAccess(c, accountStore, deploymentStore)
		if !ok {
			return
		}

		traceID, ok := requireTraceIDParam(c)
		if !ok {
			return
		}

		ds, ok := loadDataset(c, log, datasetStore, dctx.DeploymentID)
		if !ok {
			return
		}

		if err := dismissalStore.Restore(c.Request.Context(), ds.ID, traceID); err != nil {
			log.Error("dataset review queue dismissal: restore trace failed", "error", err, "deployment_id", dctx.DeploymentID, "trace_id", traceID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restore the trace"})
			return
		}

		c.JSON(http.StatusOK, ReviewQueueDismissalResponse{
			EvalDatasetID: ds.ID,
			TraceID:       traceID,
			Dismissed:     false,
		})
	}
}
