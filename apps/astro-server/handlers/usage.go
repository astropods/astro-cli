package handlers

import (
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/gin-gonic/gin"
)

// UsageMeter holds the current value and optional entitlement limit for a meter.
type UsageMeter struct {
	Value float64 `json:"value"`
	Limit *int64  `json:"limit,omitempty"`
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

// entitlement feature keys — these must match what's configured in OpenMeter.
const (
	featureComputeUsage      = "compute_usage"
	featureAgentBuilds       = "agent_builds"
	featureActiveDeployments = "active_deployments"
	featureActiveAgents      = "active_agents"
)

// GetAccountUsage handles GET /api/v1/accounts/:account/usage
// Queries OpenMeter for the account's usage over the current billing period (calendar month)
// and fetches entitlement limits for each feature.
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

		// Default to current calendar month; allow override via query params
		now := time.Now().UTC()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		to := now

		if fromStr := c.Query("from"); fromStr != "" {
			if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
				from = t
			}
		}
		if toStr := c.Query("to"); toStr != "" {
			if t, err := time.Parse(time.RFC3339, toStr); err == nil {
				to = t
			}
		}

		ctx := c.Request.Context()
		subject := acct.ID

		resp := UsageResponse{
			AccountID:   acct.ID,
			PeriodStart: from.Format(time.RFC3339),
			PeriodEnd:   to.Format(time.RFC3339),
		}

		// Query each meter — failures are non-fatal, we return zeros
		if result, err := omClient.QueryMeter(ctx, "compute_usage", subject, from, to, ""); err != nil {
			log.Warn("Failed to query compute_usage meter", "error", err, "account_id", acct.ID)
		} else if len(result.Data) > 0 {
			resp.ComputeUnitHours.Value = result.Data[0].Value
		}

		if result, err := omClient.QueryMeter(ctx, "agent_build", subject, from, to, ""); err != nil {
			log.Warn("Failed to query agent_build meter", "error", err, "account_id", acct.ID)
		} else if len(result.Data) > 0 {
			resp.AgentBuilds.Value = result.Data[0].Value
		}

		if result, err := omClient.QueryMeter(ctx, "active_deployments", subject, from, to, ""); err != nil {
			log.Warn("Failed to query active_deployments meter", "error", err, "account_id", acct.ID)
		} else if len(result.Data) > 0 {
			resp.ActiveDeployments.Value = result.Data[0].Value
		}

		if result, err := omClient.QueryMeter(ctx, "active_agents", subject, from, to, ""); err != nil {
			log.Warn("Failed to query active_agents meter", "error", err, "account_id", acct.ID)
		} else if len(result.Data) > 0 {
			resp.ActiveAgents.Value = result.Data[0].Value
		}

		// Fetch entitlement limits — non-fatal, limits are omitted if unavailable
		features := []struct {
			key   string
			meter *UsageMeter
		}{
			{featureComputeUsage, &resp.ComputeUnitHours},
			{featureAgentBuilds, &resp.AgentBuilds},
			{featureActiveDeployments, &resp.ActiveDeployments},
			{featureActiveAgents, &resp.ActiveAgents},
		}
		for _, f := range features {
			ent, err := omClient.GetEntitlementValue(ctx, subject, f.key)
			if err != nil {
				log.Debug("Entitlement not available", "feature", f.key, "error", err, "account_id", acct.ID)
				continue
			}
			f.meter.Limit = ent.Limit
		}

		c.JSON(http.StatusOK, resp)
	}
}
