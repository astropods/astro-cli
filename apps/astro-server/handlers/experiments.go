package handlers

import (
	"context"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
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

// experimentDefinition is one switch as the API exposes it. Keyed by URL slug
// rather than by the stored Key so the wire name can stay put if the stored one
// is ever renamed.
type experimentDefinition struct {
	key experiment.Key
	// label names the switch in audit entries and error text.
	label string
	// orgOnly rejects personal accounts, for switches that only mean something
	// among several members.
	orgOnly bool
	// invalidates reports whether flipping this switch has to clear the
	// deployment cache, which reads the flag per request.
	invalidates bool
	// permission is required on top of the route group's org:manage. Empty
	// means org:manage is enough.
	permission string
}

var experimentsBySlug = map[string]experimentDefinition{
	"fine-grained-access": {
		key:         experiment.FineGrainedAccess,
		label:       "fine-grained access",
		orgOnly:     true,
		invalidates: true,
		// Owner-only: this governs deployment privacy, so flipping it can
		// expose every synchronized deployment to every member.
		permission: "org:admin",
	},
	"prompt-classification-stats": {
		key:   experiment.PromptClassificationStats,
		label: "prompt classification statistics",
		// Not organization-only. Fine-grained access is about roles among
		// members, which a single-member account has none of; classification
		// runs off a personal account's own telemetry just the same.
	},
}

type ExperimentResponse struct {
	Experiment string `json:"experiment"`
	Enabled    bool   `json:"enabled"`
}

type UpdateExperimentRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

// resolveExperiment answers the request itself when the slug is unknown or the
// account is the wrong kind, so both handlers share one set of rejections.
func resolveExperiment(c *gin.Context, accountStore *account.AccountStore) (experimentDefinition, string, bool) {
	acct, ok := middleware.GetAccountFromContext(c)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
		return experimentDefinition{}, "", false
	}
	def, known := experimentsBySlug[c.Param("experiment")]
	if !known {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown experiment"})
		return experimentDefinition{}, "", false
	}
	if def.orgOnly && acct.Type != "organization" {
		c.JSON(http.StatusBadRequest, gin.H{"error": def.label + " is only available for organizations"})
		return experimentDefinition{}, "", false
	}
	if def.permission != "" {
		user, ok := middleware.GetUser(c)
		if !ok || !middleware.HasAccountPermission(c, accountStore, acct, user, def.permission) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for " + def.label})
			return experimentDefinition{}, "", false
		}
	}
	return def, acct.ID, true
}

func GetAccountExperiment(log *logger.Logger, store experimentStore, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		def, accountID, ok := resolveExperiment(c, accountStore)
		if !ok {
			return
		}
		enabled, err := store.Enabled(c.Request.Context(), accountID, def.key)
		if err != nil {
			log.Error("experiments: read account experiment failed", "error", err, "account_id", accountID, "experiment", def.key)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read experiment"})
			return
		}
		c.JSON(http.StatusOK, ExperimentResponse{Experiment: string(def.key), Enabled: enabled})
	}
}

func UpdateAccountExperiment(
	log *logger.Logger,
	store experimentStore,
	auditStore *auditlog.Store,
	cache experimentCacheInvalidator,
	accountStore *account.AccountStore,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		def, accountID, ok := resolveExperiment(c, accountStore)
		if !ok {
			return
		}
		var req UpdateExperimentRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "enabled is required"})
			return
		}
		if err := store.SetEnabled(c.Request.Context(), accountID, def.key, *req.Enabled); err != nil {
			log.Error("experiments: update account experiment failed", "error", err, "account_id", accountID, "experiment", def.key)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update experiment"})
			return
		}
		if def.invalidates && cache != nil {
			cache.InvalidateAccount(accountID)
		}

		event := auditlog.FromGinContext(c, accountID)
		event.Action = auditlog.AccountUpdateExperiment
		event.ResourceType = "account_experiment"
		event.ResourceID = string(def.key)
		event.Description = "Updated " + def.label + " experiment"
		event.Metadata = map[string]any{"enabled": *req.Enabled}
		auditStore.LogAsync(log, event)

		c.JSON(http.StatusOK, ExperimentResponse{Experiment: string(def.key), Enabled: *req.Enabled})
	}
}
