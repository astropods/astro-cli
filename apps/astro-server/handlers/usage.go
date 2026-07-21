package handlers

import (
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	"github.com/gin-gonic/gin"
)

// UsageMeter holds the current usage and optional quota for a feature.
type UsageMeter struct {
	Usage float64  `json:"usage"`
	Quota *float64 `json:"quota,omitempty"`
}

// UsageResponse is the response for GET /api/v1/accounts/:account/usage.
// Meters is keyed by feature key and reports DB-backed resource counts. Metered
// consumption (compute, knowledge storage) is not reported: it has no data
// source wired yet and the billing provider's usage readback is not yet wired.
type UsageResponse struct {
	AccountID   string                `json:"account_id"`
	PeriodStart string                `json:"period_start"`
	PeriodEnd   string                `json:"period_end"`
	Meters      map[string]UsageMeter `json:"meters"`
}

// GetAccountUsage handles GET /api/v1/accounts/:account/usage. Meters are the
// resource counts from the quota DB (authoritative).
func GetAccountUsage(log *logger.Logger, quotaChecker quota.Reporter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if quotaChecker == nil {
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

		// Resource counts from the quota DB (authoritative). A limit of -1
		// (unlimited) is reported with no quota bar.
		report, err := quotaChecker.Report(c.Request.Context(), acct.ID, quota.AllResources...)
		if err != nil {
			log.Warn("Failed to load quota usage", "error", err, "account_id", acct.ID)
		} else {
			for resource, u := range report {
				m := UsageMeter{Usage: float64(u.Used)}
				if u.Limit >= 0 {
					q := float64(u.Limit)
					m.Quota = &q
				}
				resp.Meters[resource] = m
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}
