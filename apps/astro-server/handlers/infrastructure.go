package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
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
// Metered compute usage has no data source wired yet, so this returns an empty
// (zero-usage) payload with the resolved account/range so clients render without
// error. The route shape is retained for a future provider-backed reader.
func GetInfrastructureUsage(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		c.JSON(http.StatusOK, InfrastructureUsageResponse{
			AccountID: acct.ID,
			AgentName: c.Param("name"),
			From:      from.Format(time.RFC3339),
			To:        to.Format(time.RFC3339),
		})
	}
}
