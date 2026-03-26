package handlers

import (
	"net/http"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// AuditLogEntryResponse is the JSON shape returned per audit log entry.
type AuditLogEntryResponse struct {
	ID          int64            `json:"id"`
	Actor       AuditLogActor    `json:"actor"`
	Action      string           `json:"action"`
	Resource    AuditLogResource `json:"resource"`
	Description string           `json:"description,omitempty"`
	Metadata    any              `json:"metadata,omitempty"`
	CreatedAt   string           `json:"created_at"`
}

// AuditLogActor identifies who performed the action.
type AuditLogActor struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// AuditLogResource identifies what was acted upon.
type AuditLogResource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// AuditLogListResponse is the paginated response for GET /audit-log.
type AuditLogListResponse struct {
	Entries    []AuditLogEntryResponse `json:"entries"`
	HasMore    bool                    `json:"has_more"`
	NextBefore string                  `json:"next_before,omitempty"`
}

// ListAuditLog handles GET /api/v1/accounts/:account/audit-log
func ListAuditLog(log *logger.Logger, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		limit := auditlog.ParseLimit(c.Query("limit"), 50, 200)

		params := auditlog.QueryParams{
			AccountID:    acct.ID,
			ActorID:      c.Query("actor_id"),
			ResourceType: c.Query("resource_type"),
			ResourceID:   c.Query("resource_id"),
			Action:       c.Query("action"),
			Before:       auditlog.ParseBefore(c.Query("before")),
			Limit:        limit,
		}

		entries, err := auditStore.Query(c.Request.Context(), params)
		if err != nil {
			log.Error("Failed to query audit log", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query audit log"})
			return
		}

		hasMore := len(entries) > limit
		if hasMore {
			entries = entries[:limit]
		}

		resp := AuditLogListResponse{
			Entries: make([]AuditLogEntryResponse, 0, len(entries)),
			HasMore: hasMore,
		}

		for _, e := range entries {
			entry := AuditLogEntryResponse{
				ID: e.ID,
				Actor: AuditLogActor{
					ID:   e.ActorID,
					Type: string(e.ActorType),
				},
				Action: e.Action,
				Resource: AuditLogResource{
					Type: e.ResourceType,
					ID:   e.ResourceID,
					Name: e.ResourceName,
				},
				Description: e.Description,
				CreatedAt:   e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			if e.Metadata != nil {
				entry.Metadata = e.Metadata
			}
			resp.Entries = append(resp.Entries, entry)
		}

		if hasMore && len(entries) > 0 {
			resp.NextBefore = entries[len(entries)-1].CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")
		}

		c.JSON(http.StatusOK, resp)
	}
}
