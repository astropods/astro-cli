package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/watcher"
)

type watcherResponse struct {
	UserID string `json:"user_id"`
	Name   string `json:"name,omitempty"`
	Reason string `json:"reason,omitempty"`
	Muted  bool   `json:"muted"`
}

// ListDeploymentWatchers returns everyone subscribed to a deployment's alerts,
// muted members included, so a caller can see their own state in context.
func ListDeploymentWatchers(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, watchers *watcher.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, exists := middleware.GetUser(c); !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		rows, err := watchers.List(c.Request.Context(), dep.ID)
		if err != nil {
			log.Error("Failed to list deployment watchers", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list watchers"})
			return
		}

		out := make([]watcherResponse, 0, len(rows))
		ids := make([]string, 0, len(rows))
		for _, w := range rows {
			ids = append(ids, w.UserID)
		}
		// Best-effort: an unresolvable name renders as the bare id rather than
		// failing a read that is otherwise complete.
		names, err := accountStore.DisplayNamesForUsers(ids)
		if err != nil {
			log.Warn("Failed to resolve watcher display names", "error", err, "deployment_id", dep.ID)
			names = map[string]string{}
		}
		for _, w := range rows {
			out = append(out, watcherResponse{UserID: w.UserID, Name: names[w.UserID], Reason: w.Reason, Muted: w.Muted})
		}

		c.JSON(http.StatusOK, gin.H{"watchers": out})
	}
}

// WatchDeployment subscribes the caller to a deployment's alerts, clearing a
// previous opt-out. Explicit because implicit enrollment only happens when you
// act on the deployment, and a member may want alerts without touching it.
func WatchDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, watchers *watcher.Store) gin.HandlerFunc {
	return setWatchState(log, accountStore, deployStore, watchers, false)
}

// UnwatchDeployment opts the caller out of a deployment's alerts. This is a
// sticky mute rather than a row delete: deleting would let the member's next
// deploy silently resubscribe them to alerts they just turned off.
func UnwatchDeployment(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, watchers *watcher.Store) gin.HandlerFunc {
	return setWatchState(log, accountStore, deployStore, watchers, true)
}

func setWatchState(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, watchers *watcher.Store, muted bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		if err := watchers.SetMuted(c.Request.Context(), dep.AccountID, dep.ID, user.ID, muted); err != nil {
			log.Error("Failed to update watch state", "error", err, "deployment_id", dep.ID, "user_id", user.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update watch state"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"deployment_id": dep.ID, "muted": muted})
	}
}
