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
// Meters is keyed by the OpenMeter feature key and contains all entitlements
// present in the account's subscription — no hardcoded list.
type UsageResponse struct {
	AccountID   string                `json:"account_id"`
	PeriodStart string                `json:"period_start"`
	PeriodEnd   string                `json:"period_end"`
	Meters      map[string]UsageMeter `json:"meters"`
}

// GetAccountUsage handles GET /api/v1/accounts/:account/usage.
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
			Meters:      map[string]UsageMeter{},
		}

		access, err := omClient.GetCustomerAccess(c.Request.Context(), acct.ID)
		if err != nil {
			log.Warn("Failed to get customer access", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusOK, resp)
			return
		}

		for key, ent := range access.Entitlements {
			m := UsageMeter{}
			if ent.Usage != nil {
				m.Usage = *ent.Usage
			}
			m.Quota = ent.TotalAvailableGrantAmount
			resp.Meters[key] = m
		}

		c.JSON(http.StatusOK, resp)
	}
}
