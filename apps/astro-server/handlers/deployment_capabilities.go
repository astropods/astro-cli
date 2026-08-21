package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const deploymentCapabilitiesTimeout = 5 * time.Second

type capabilityEvaluator interface {
	Evaluate(context.Context, authz.Subject, authz.ResourceRef, []authz.Action) (authz.CapabilitySet, error)
}

type DeploymentCapabilitiesResponse struct {
	DeploymentID string          `json:"deployment_id"`
	Mode         string          `json:"mode"`
	Actions      map[string]bool `json:"actions"`
}

func GetDeploymentCapabilities(log *logger.Logger, evaluator capabilityEvaluator) gin.HandlerFunc {
	return func(c *gin.Context) {
		subject, ok := middleware.SubjectFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": ErrorMessageAuthenticationRequired})
			return
		}

		deploymentID := c.Param("id")
		ctx, cancel := context.WithTimeout(c.Request.Context(), deploymentCapabilitiesTimeout)
		defer cancel()
		ctx = authz.WithRequestCache(ctx)
		ctx = authz.WithAuthorizationRoute(ctx, c.FullPath())

		set, err := evaluator.Evaluate(ctx, subject, authz.DeploymentResource(deploymentID), authz.DeploymentActions())
		if err != nil {
			attrs := []any{"error", err, "deployment_id", deploymentID, "user_id", subject.UserID}
			switch {
			case errors.Is(err, authz.ErrResourceNotVisible), errors.Is(err, sql.ErrNoRows):
				log.Debug("deployment capabilities: resource unavailable", attrs...)
				c.JSON(http.StatusNotFound, gin.H{"error": ErrorMessageDeploymentNotFound})
			case errors.Is(err, authz.ErrWorkOSMembershipUnavailable):
				log.Warn("deployment capabilities: identity unavailable", attrs...)
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": ErrorMessageAuthorizationSessionUnavailable})
			default:
				log.Warn("deployment capabilities: check failed", attrs...)
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": ErrorMessageAuthorizationTemporarilyUnavailable})
			}
			return
		}
		if set.Mode == authz.CapabilityModeFGA && !set.Actions[authz.ActionDeploymentRead] {
			log.Debug("deployment capabilities: denied by baseline read", "deployment_id", deploymentID, "user_id", subject.UserID)
			c.JSON(http.StatusNotFound, gin.H{"error": ErrorMessageDeploymentNotFound})
			return
		}

		actions := make(map[string]bool, len(set.Actions))
		for action, allowed := range set.Actions {
			actions[string(action)] = allowed
		}
		c.JSON(http.StatusOK, DeploymentCapabilitiesResponse{
			DeploymentID: deploymentID,
			Mode:         string(set.Mode),
			Actions:      actions,
		})
	}
}
