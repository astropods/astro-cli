package handlers

import (
	"context"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/experiment"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

type experimentStore interface {
	Enabled(context.Context, string, experiment.Key) (bool, error)
	SetEnabled(context.Context, string, experiment.Key, bool) error
}

type experimentCacheInvalidator interface {
	InvalidateAccount(string)
}

type FineGrainedAccessExperimentResponse struct {
	Experiment string `json:"experiment"`
	Enabled    bool   `json:"enabled"`
}

type UpdateFineGrainedAccessExperimentRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

func GetFineGrainedAccessExperiment(log *logger.Logger, store experimentStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		if acct.Type != "organization" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "fine-grained access is only available for organizations"})
			return
		}

		enabled, err := store.Enabled(c.Request.Context(), acct.ID, experiment.FineGrainedAccess)
		if err != nil {
			log.Error("experiments: read account experiment failed", "error", err, "account_id", acct.ID, "experiment", experiment.FineGrainedAccess)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read experiment"})
			return
		}
		c.JSON(http.StatusOK, FineGrainedAccessExperimentResponse{
			Experiment: string(experiment.FineGrainedAccess),
			Enabled:    enabled,
		})
	}
}

func UpdateFineGrainedAccessExperiment(
	log *logger.Logger,
	store experimentStore,
	auditStore *auditlog.Store,
	cache experimentCacheInvalidator,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		if acct.Type != "organization" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "fine-grained access is only available for organizations"})
			return
		}

		var req UpdateFineGrainedAccessExperimentRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "enabled is required"})
			return
		}
		if err := store.SetEnabled(c.Request.Context(), acct.ID, experiment.FineGrainedAccess, *req.Enabled); err != nil {
			log.Error("experiments: update account experiment failed", "error", err, "account_id", acct.ID, "experiment", experiment.FineGrainedAccess)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update experiment"})
			return
		}
		if cache != nil {
			cache.InvalidateAccount(acct.ID)
		}

		event := auditlog.FromGinContext(c, acct.ID)
		event.Action = auditlog.AccountUpdateExperiment
		event.ResourceType = "account_experiment"
		event.ResourceID = string(experiment.FineGrainedAccess)
		event.Description = "Updated fine-grained access experiment"
		event.Metadata = map[string]any{"enabled": *req.Enabled}
		auditStore.LogAsync(log, event)

		c.JSON(http.StatusOK, FineGrainedAccessExperimentResponse{
			Experiment: string(experiment.FineGrainedAccess),
			Enabled:    *req.Enabled,
		})
	}
}
