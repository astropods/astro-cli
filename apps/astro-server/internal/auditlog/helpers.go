package auditlog

import (
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// FromGinContext builds an Event pre-filled with actor, IP, and User-Agent
// from the current Gin request context. Callers fill in action, resource, etc.
func FromGinContext(c *gin.Context, accountID string) Event {
	e := Event{
		AccountID: accountID,
		ActorType: ActorUser,
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
	}
	if user, ok := middleware.GetUser(c); ok {
		e.ActorID = user.ID
	}
	return e
}

// ForAdmin builds an Event for an admin gRPC action.
func ForAdmin(accountID, adminIdentity string) Event {
	return Event{
		AccountID: accountID,
		ActorID:   "admin:" + adminIdentity,
		ActorType: ActorAdmin,
	}
}
