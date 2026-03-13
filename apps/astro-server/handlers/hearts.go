package handlers

import (
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// HeartResponse is returned by the heart toggle endpoint.
type HeartResponse struct {
	Hearted    bool `json:"hearted"`
	HeartCount int  `json:"heart_count"`
}

// ToggleHeart handles POST /api/v1/agents/:account/:name/heart
// Atomically toggles the heart state for the authenticated user.
func ToggleHeart(log *logger.Logger, hearts *heartstore.Store, accountStore *account.AccountStore) gin.HandlerFunc {
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

		hearted, count, err := hearts.Toggle(c.Request.Context(), acct.ID, agentName, user.ID)
		if err != nil {
			log.Error("Failed to toggle heart", "error", err, "account", accountName, "agent", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to toggle heart"})
			return
		}

		c.JSON(http.StatusOK, HeartResponse{
			Hearted:    hearted,
			HeartCount: count,
		})
	}
}
