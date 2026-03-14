package handlers

import (
	"database/sql"
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

var validFeatureKeys = map[string]bool{
	"compute":           true,
	"agent_builds":      true,
	"agent_deployments": true,
	"agents":            true,
	"members":           true,
}

// QuotaIncreaseInput is the request body for creating a quota increase request.
type QuotaIncreaseInput struct {
	FeatureKey      string   `json:"feature_key"`
	CurrentUsage    float64  `json:"current_usage"`
	CurrentQuota    *float64 `json:"current_quota,omitempty"`
	RequestedAmount *float64 `json:"requested_amount,omitempty"`
	Reason          string   `json:"reason"`
}

// QuotaIncreaseResponse is the response after creating a quota increase request.
type QuotaIncreaseResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// RequestQuotaIncrease handles POST /api/v1/accounts/:account/quota-increase.
func RequestQuotaIncrease(log *logger.Logger, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		userID, _ := c.Get("user_id")
		uid, _ := userID.(string)

		var input QuotaIncreaseInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		if !validFeatureKeys[input.FeatureKey] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid feature_key"})
			return
		}

		if input.Reason == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
			return
		}

		var id string
		err := db.QueryRowContext(c.Request.Context(),
			`INSERT INTO quota_increase_requests (account_id, feature_key, current_usage, current_quota, requested_amount, reason, requested_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 RETURNING id`,
			acct.ID, input.FeatureKey, input.CurrentUsage, input.CurrentQuota, input.RequestedAmount, input.Reason, uid,
		).Scan(&id)
		if err != nil {
			log.Error("Failed to create quota increase request", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create request"})
			return
		}

		log.Info("Quota increase requested", "request_id", id, "account_id", acct.ID, "feature", input.FeatureKey)
		c.JSON(http.StatusCreated, QuotaIncreaseResponse{ID: id, Status: "pending"})
	}
}

// QuotaIncreaseListItem represents a quota increase request in a list response.
type QuotaIncreaseListItem struct {
	ID              string   `json:"id"`
	FeatureKey      string   `json:"feature_key"`
	CurrentUsage    float64  `json:"current_usage"`
	CurrentQuota    *float64 `json:"current_quota,omitempty"`
	RequestedAmount *float64 `json:"requested_amount,omitempty"`
	Reason          string   `json:"reason"`
	Status          string   `json:"status"`
	CreatedAt       string   `json:"created_at"`
}

// QuotaIncreaseListResponse is the response for listing quota increase requests.
type QuotaIncreaseListResponse struct {
	Requests []QuotaIncreaseListItem `json:"requests"`
}

// ListQuotaIncreaseRequests handles GET /api/v1/accounts/:account/quota-increase.
func ListQuotaIncreaseRequests(log *logger.Logger, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		rows, err := db.QueryContext(c.Request.Context(),
			`SELECT id, feature_key, current_usage, current_quota, requested_amount, reason, status, created_at
			 FROM quota_increase_requests
			 WHERE account_id = $1
			 ORDER BY created_at DESC`,
			acct.ID,
		)
		if err != nil {
			log.Error("Failed to list quota increase requests", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list requests"})
			return
		}
		defer rows.Close() //nolint:errcheck

		var items []QuotaIncreaseListItem
		for rows.Next() {
			var item QuotaIncreaseListItem
			var createdAt sql.NullTime
			if err := rows.Scan(&item.ID, &item.FeatureKey, &item.CurrentUsage, &item.CurrentQuota, &item.RequestedAmount, &item.Reason, &item.Status, &createdAt); err != nil {
				log.Error("Failed to scan quota increase request", "error", err)
				continue
			}
			if createdAt.Valid {
				item.CreatedAt = createdAt.Time.Format("2006-01-02T15:04:05Z")
			}
			items = append(items, item)
		}

		if items == nil {
			items = []QuotaIncreaseListItem{}
		}

		c.JSON(http.StatusOK, QuotaIncreaseListResponse{Requests: items})
	}
}
