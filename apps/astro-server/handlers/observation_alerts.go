package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/observation"
)

// DeploymentAlertItem is one configured observation alert and its current state
// for a deployment.
type DeploymentAlertItem struct {
	Name        string  `json:"name"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Severity    string  `json:"severity"`    // "warning" | "error"
	State       string  `json:"state"`       // "ok" | "pending" | "firing"
	ActiveSince *string `json:"activeSince"` // RFC3339; set while pending/firing
}

// DeploymentAlertsResponse is the active observation alert catalog plus each
// alert's current state for one deployment.
type DeploymentAlertsResponse struct {
	Alerts []DeploymentAlertItem `json:"alerts"`
}

// GetDeploymentAlerts lists every active observation condition and its
// current state for a deployment: "ok" (not breaching), "pending" (breaching
// but the sustained `for` window hasn't elapsed, so no alert sent), or "firing"
// (alert emitted). The evaluator keys firing state by the latest deployment id
// for the namespace, so state is read under that id.
func GetDeploymentAlerts(log *logger.Logger, accountStore *account.AccountStore, deployStore *deploymentstore.Store, alertStore *observation.Store) gin.HandlerFunc {
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

		// The tab scopes alerts to the workload whose pod panel is open (the
		// workload's component, e.g. "agent", "model-x"); firing state is tracked
		// per workload.
		workload := c.Query("workload")

		// Firing state is keyed by the latest deployment id for the namespace
		// (what the evaluator writes); fall back to the viewed deployment.
		keyID := dep.ID
		if latest, lerr := deployStore.GetLatestDeploymentByNamespace(dep.Namespace); lerr == nil && latest != nil {
			keyID = latest.ID
		}

		state := map[string]observation.State{}
		if alertStore != nil {
			state, err = alertStore.ForDeploymentWorkload(c.Request.Context(), keyID, workload)
			if err != nil {
				log.Error("observation alerts: read alert state failed", "error", err, "deployment_id", keyID, "workload", workload)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read deployment alerts"})
				return
			}
		}

		active := observation.ActiveConditions()
		resp := DeploymentAlertsResponse{Alerts: make([]DeploymentAlertItem, 0, len(active))}
		for _, cond := range active {
			item := DeploymentAlertItem{
				Name:        cond.Name,
				Title:       cond.Title,
				Description: cond.Description,
				Severity:    cond.Severity.String(),
				State:       "ok",
			}
			if st, ok := state[cond.Name]; ok {
				since := st.ActiveSince.UTC().Format(time.RFC3339)
				item.ActiveSince = &since
				if st.Notified {
					item.State = "firing"
				} else {
					item.State = "pending"
				}
			}
			resp.Alerts = append(resp.Alerts, item)
		}

		c.JSON(http.StatusOK, resp)
	}
}
