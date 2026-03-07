package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/astropods/astro/apps/astro-registry/internal/logger"
	"github.com/gin-gonic/gin"
)

// Recovery returns a middleware that recovers from panics
func Recovery(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log the stack trace
				log.Error("Panic recovered",
					"error", err,
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
					"stack", string(debug.Stack()),
				)

				// Respond with error
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"errors": []gin.H{
						{"code": "SERVER_ERROR", "message": "Internal server error"},
					},
				})
			}
		}()
		c.Next()
	}
}
