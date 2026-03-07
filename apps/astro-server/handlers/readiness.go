package handlers

import (
	"net/http"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

// ReadinessCheck returns a handler for readiness checks
func ReadinessCheck(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Add any dependency checks here (database, external services, etc.)
		ready := true

		if ready {
			c.JSON(http.StatusOK, gin.H{
				"status":    "ready",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
		} else {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":    "not ready",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
		}
	}
}
