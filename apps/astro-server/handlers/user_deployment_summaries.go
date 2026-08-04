package handlers

import (
	"net/http"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const maxUserDeploymentSummaries = 100

// ListUserDeploymentSummaries returns cached observability only for the
// requested deployment cards that the current user can read. It performs one
// membership-guarded SQL query and one Redis MGET; it never calls Langfuse.
func ListUserDeploymentSummaries(
	log *logger.Logger,
	deployments *deploymentstore.Store,
	cache k8scache.Cache,
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

		visible, err := deployments.ListVisibleDeploymentIDsForUser(c.Request.Context(), user.ID, ids)
		if err != nil {
			log.Error("Failed to authorize visible deployment summaries", "error", err, "requested_count", len(ids))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load deployment summaries"})
			return
		}
		summaries, cacheErr := deploymentSummariesFromCache(c.Request.Context(), cache, visible)
		if cacheErr != nil {
			log.Warn("Failed to decode some visible deployment summaries", "error", cacheErr)
		}
		c.JSON(http.StatusOK, DeploymentSummariesResponse{Summaries: summaries})
	}
}
