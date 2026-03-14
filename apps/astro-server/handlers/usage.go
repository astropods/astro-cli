package handlers

import (
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/gin-gonic/gin"
)

// UsageMeter holds the current usage and optional quota for a feature.
type UsageMeter struct {
	Usage float64  `json:"usage"`
	Quota *float64 `json:"quota,omitempty"`
}

// UsageResponse is the response for GET /api/v1/accounts/:account/usage.
type UsageResponse struct {
	AccountID         string     `json:"account_id"`
	PeriodStart       string     `json:"period_start"`
	PeriodEnd         string     `json:"period_end"`
	ComputeUnitHours  UsageMeter `json:"compute_unit_hours"`
	AgentBuilds       UsageMeter `json:"agent_builds"`
	ActiveDeployments UsageMeter `json:"active_deployments"`
	ActiveAgents      UsageMeter `json:"active_agents"`
}

// Entitlement feature keys — must match the feature keys configured in OpenMeter (see integration plan §4).
const (
	featureCompute          = "compute"
	featureAgentBuilds      = "agent_builds"
	featureAgentDeployments = "agent_deployments"
	featureAgents           = "agents"
)

// GetAccountUsage handles GET /api/v1/accounts/:account/usage.
// Fetches all usage and quota data from a single GetCustomerAccess call.
func GetAccountUsage(log *logger.Logger, omClient *openmeter.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		if omClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "usage metering is not configured"})
			return
		}

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		now := time.Now().UTC()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

		resp := UsageResponse{
			AccountID:   acct.ID,
			PeriodStart: from.Format(time.RFC3339),
			PeriodEnd:   now.Format(time.RFC3339),
		}

		access, err := omClient.GetCustomerAccess(c.Request.Context(), acct.ID)
		if err != nil {
			log.Warn("Failed to get customer access", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusOK, resp)
			return
		}

		features := []struct {
			key   string
			meter *UsageMeter
		}{
			{featureCompute, &resp.ComputeUnitHours},
			{featureAgentBuilds, &resp.AgentBuilds},
			{featureAgentDeployments, &resp.ActiveDeployments},
			{featureAgents, &resp.ActiveAgents},
		}
		for _, f := range features {
			if ent, ok := access.Entitlements[f.key]; ok {
				if ent.Usage != nil {
					f.meter.Usage = *ent.Usage
				}
				f.meter.Quota = ent.TotalAvailableGrantAmount
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}
