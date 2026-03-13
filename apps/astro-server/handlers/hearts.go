package handlers

import (
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// HeartResponse is returned by heart/unheart toggle endpoints.
type HeartResponse struct {
	Hearted    bool `json:"hearted"`
	HeartCount int  `json:"heart_count"`
}

// HeartAgent handles PUT /api/v1/agents/:account/:name/heart
// Idempotent — PUT to heart.
func HeartAgent(log *logger.Logger, hearts *heartstore.Store, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		agentName := c.Param("name")

		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		if _, err := hearts.Heart(c.Request.Context(), acct.ID, agentName, user.ID); err != nil {
			log.Error("Failed to heart agent", "error", err, "account", accountName, "agent", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to heart agent"})
			return
		}

		count, _ := hearts.Count(c.Request.Context(), acct.ID, agentName)

		c.JSON(http.StatusOK, HeartResponse{
			Hearted:    true,
			HeartCount: count,
		})
	}
}

// UnheartAgent handles DELETE /api/v1/agents/:account/:name/heart
func UnheartAgent(log *logger.Logger, hearts *heartstore.Store, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		agentName := c.Param("name")

		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		if _, err := hearts.Unheart(c.Request.Context(), acct.ID, agentName, user.ID); err != nil {
			log.Error("Failed to unheart agent", "error", err, "account", accountName, "agent", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unheart agent"})
			return
		}

		count, _ := hearts.Count(c.Request.Context(), acct.ID, agentName)

		c.JSON(http.StatusOK, HeartResponse{
			Hearted:    false,
			HeartCount: count,
		})
	}
}
