package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

// Logger returns a gin middleware for structured logging
func Logger(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Skip logging health check probes
		if path == "/readyz" || path == "/healthz" {
			return
		}

		// Log after processing
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		fields := map[string]interface{}{
			"method":     c.Request.Method,
			"path":       path,
			"query":      query,
			"status":     statusCode,
			"duration":   duration.Milliseconds(),
			"ip":         c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
		}

		if len(c.Errors) > 0 {
			fields["errors"] = c.Errors.String()
		}

		logEntry := log.WithFields(fields)

		switch {
		case statusCode >= 500:
			logEntry.Error("Server error")
		case statusCode >= 400:
			logEntry.Warn("Client error")
		default:
			logEntry.Info("Request completed")
		}
	}
}
