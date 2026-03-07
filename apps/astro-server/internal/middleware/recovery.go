package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

// Recovery returns a middleware that recovers from panics
func Recovery(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic
				log.Error("Panic recovered",
					"error", fmt.Sprintf("%v", err),
					"stack", string(debug.Stack()),
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
				)

				// Return error response
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Internal server error",
				})

				c.Abort()
			}
		}()

		c.Next()
	}
}
