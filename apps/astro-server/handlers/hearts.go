package handlers

import (
	"net/http"
	"strconv"

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
			log.Error("hearts: toggle heart failed", "error", err, "account", accountName, "agent", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to toggle heart"})
			return
		}

		c.JSON(http.StatusOK, HeartResponse{
			Hearted:    hearted,
			HeartCount: count,
		})
	}
}

const defaultHeartsPageSize = 20
const maxHeartsPageSize = 100

// ListHearted handles GET /api/v1/accounts/:account/hearts (public)
// Returns blueprints hearted by the owner of the given personal account, newest first.
// Query params: cursor (pagination), limit (default 20, max 100)
func ListHearted(log *logger.Logger, hearts *heartstore.Store, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		if acct.Type != "personal" {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		ownerID, err := accountStore.GetFirstMemberUserID(acct.ID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"items": []heartstore.HeartedAgent{}, "next_cursor": nil})
			return
		}

		pageSize := defaultHeartsPageSize
		if limitStr := c.Query("limit"); limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				if n > maxHeartsPageSize {
					n = maxHeartsPageSize
				}
				pageSize = n
			}
		}

		cursor := c.Query("cursor")
		items, nextCursor, err := hearts.ListHearted(c.Request.Context(), ownerID, pageSize, cursor)
		if err != nil {
			log.Error("hearts: list hearted agents failed", "error", err, "account", accountName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list hearted blueprints"})
			return
		}

		if items == nil {
			items = []heartstore.HeartedAgent{}
		}

		resp := gin.H{"items": items}
		if nextCursor != "" {
			resp["next_cursor"] = nextCursor
		}
		c.JSON(http.StatusOK, resp)
	}
}
