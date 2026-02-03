package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-registry/internal/logger"
)

// Logger returns a middleware that logs requests
func Logger(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Get status code
		status := c.Writer.Status()

		// Log level based on status code
		if status >= 500 {
			log.Error("Request completed",
				"status", status,
				"method", c.Request.Method,
				"path", path,
				"query", query,
				"latency", latency.String(),
				"client_ip", c.ClientIP(),
				"error", c.Errors.String(),
			)
		} else if status >= 400 {
			log.Warn("Request completed",
				"status", status,
				"method", c.Request.Method,
				"path", path,
				"query", query,
				"latency", latency.String(),
				"client_ip", c.ClientIP(),
			)
		} else {
			log.Info("Request completed",
				"status", status,
				"method", c.Request.Method,
				"path", path,
				"latency", latency.String(),
			)
		}
	}
}
