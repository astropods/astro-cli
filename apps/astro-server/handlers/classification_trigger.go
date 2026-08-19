package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
)

// RunClassificationResponse reports that a pass was enqueued.
type RunClassificationResponse struct {
	Message string `json:"message"`
}

// RunClassification enqueues an immediate pass, bypassing the hourly tick.
// Local-only: in a deployed environment this would be a button that drives load
// onto the shared inference pool. Repeated calls collapse via args uniqueness.
func RunClassification(log *logger.Logger, queue *riverqueue.Queue) gin.HandlerFunc {
	return func(c *gin.Context) {
		if queue == nil {
			c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "job queue not available"})
			return
		}
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "account not resolved"})
			return
		}
		if _, err := queue.Insert(c.Request.Context(), riverqueue.ClassificationAccountArgs{AccountID: acct.ID}, nil); err != nil {
			log.Error("classification: manual trigger failed", "account_id", acct.ID, "error", err)
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue classification"})
			return
		}
		log.Info("classification: manual trigger enqueued", "account_id", acct.ID)
		c.JSON(http.StatusAccepted, RunClassificationResponse{Message: "classification pass enqueued"})
	}
}
