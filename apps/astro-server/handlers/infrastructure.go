package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/gin-gonic/gin"
)

// InfrastructureUsage holds the usage totals for an infrastructure usage response.
type InfrastructureUsage struct {
	DeploymentCompute float64 `json:"deployment_compute"`
}

// InfrastructureUsageResponse is the response for the infrastructure usage endpoints.
type InfrastructureUsageResponse struct {
	AccountID string              `json:"account_id"`
	AgentName string              `json:"agent_name,omitempty"`
	From      string              `json:"from"`
	To        string              `json:"to"`
	Usage     InfrastructureUsage `json:"usage"`
}

func parseTimeRange(c *gin.Context) (from, to time.Time, err error) {
	now := time.Now().UTC()
	from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to = now
	if raw := c.Query("from"); raw != "" {
		t, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'from' timestamp: must be RFC3339")
		}
		from = t.UTC()
	}
	if raw := c.Query("to"); raw != "" {
		t, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid 'to' timestamp: must be RFC3339")
		}
		to = t.UTC()
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("'from' must be before 'to'")
	}
	return from, to, nil
}

// GetInfrastructureUsage handles infrastructure usage for both account and agent scopes.
// When registered on a route with a :name param it returns total compute for that agent;
// otherwise returns the account total.
func GetInfrastructureUsage(log *logger.Logger, omClient *openmeter.Client) gin.HandlerFunc {
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

		from, to, err := parseTimeRange(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		agentName := c.Param("name")

		params := openmeter.MeterQueryParams{
			Subject: acct.ID,
			From:    from,
			To:      to,
		}
		if agentName != "" {
			params.GroupBy = []string{"agent_name"}
			params.FilterGroupBy = map[string]string{"agent_name": agentName}
		}

		resp := InfrastructureUsageResponse{
			AccountID: acct.ID,
			AgentName: agentName,
			From:      from.Format(time.RFC3339),
			To:        to.Format(time.RFC3339),
		}

		result, err := omClient.QueryMeter(c.Request.Context(), "compute", params)
		if err != nil {
			log.Warn("Failed to query compute meter", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusOK, resp)
			return
		}

		// GroupBy agent_name with a single-value filter produces exactly one row.
		if len(result.Data) > 0 {
			resp.Usage.DeploymentCompute = result.Data[0].Value
		}

		c.JSON(http.StatusOK, resp)
	}
}
