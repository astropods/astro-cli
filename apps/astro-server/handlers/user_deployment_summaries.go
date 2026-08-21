package handlers

import (
	"net/http"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const maxUserDeploymentSummaries = 100

// ListUserDeploymentSummaries resolves live WorkOS visibility for FGA-enabled organizations before its membership-guarded SQL query and Redis MGET.
func ListUserDeploymentSummaries(
	log *logger.Logger,
	accounts *account.AccountStore,
	deployments *deploymentstore.Store,
	cache k8scache.Cache,
	discovery deploymentVisibilityResolver,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		requested := c.QueryArray("deployment")
		if len(requested) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "at least one deployment is required"})
			return
		}
		if len(requested) > maxUserDeploymentSummaries {
			c.JSON(http.StatusBadRequest, gin.H{"error": "at most 100 deployments are allowed"})
			return
		}
		ids := make([]string, 0, len(requested))
		seen := make(map[string]struct{}, len(requested))
		for _, raw := range requested {
			id := strings.TrimSpace(raw)
			// Deployment IDs are compact strings (normally xxx-xxx-xxx), not UUIDs.
			// Keep validation compatible with historical IDs while bounding the query.
			if id == "" || len(id) > 11 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "deployment is invalid"})
				return
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}

		visibility := authz.DeploymentVisibility{}
		if discovery != nil && discovery.Active() {
			memberships, membershipErr := accounts.GetAccountsForUserContext(c.Request.Context(), user.ID)
			if membershipErr != nil {
				log.Error("user deployment summaries: load memberships for deployment summaries failed", "error", membershipErr, "user_id", user.ID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load deployment summaries"})
				return
			}
			var visibilityErr error
			visibility, visibilityErr = resolveDeploymentVisibility(c.Request.Context(), discovery, user.ID, memberships)
			if visibilityErr != nil {
				writeDeploymentVisibilityError(c, log, visibilityErr)
				return
			}
		}
		visible, err := deployments.ListReadableDeploymentIDsForUser(
			c.Request.Context(),
			user.ID,
			ids,
			visibility.FGAAccountIDs,
			visibility.ReadableDeploymentIDs,
		)
		if err != nil {
			log.Error("user deployment summaries: authorize visible deployment summaries failed", "error", err, "requested_count", len(ids))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load deployment summaries"})
			return
		}
		summaries, cacheErr := deploymentSummariesFromCache(c.Request.Context(), cache, visible)
		if cacheErr != nil {
			log.Warn("user deployment summaries: decode some visible deployment summaries failed", "error", cacheErr)
		}
		c.JSON(http.StatusOK, DeploymentSummariesResponse{Summaries: summaries})
	}
}
