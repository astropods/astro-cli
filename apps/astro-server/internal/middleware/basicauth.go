package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BasicAuthConfig holds configuration for basic auth middleware
type BasicAuthConfig struct {
	Username string
	Password string
	Realm    string
}

// BasicAuth returns a middleware that requires HTTP Basic Authentication
func BasicAuth(cfg BasicAuthConfig) gin.HandlerFunc {
	realm := cfg.Realm
	if realm == "" {
		realm = "Restricted"
	}

	return func(c *gin.Context) {
		user, pass, ok := c.Request.BasicAuth()

		if !ok {
			c.Header("WWW-Authenticate", `Basic realm="`+realm+`"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization required",
			})
			return
		}

		// Constant-time comparison to prevent timing attacks
		userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(cfg.Username)) == 1
		passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(cfg.Password)) == 1

		if !userMatch || !passMatch {
			c.Header("WWW-Authenticate", `Basic realm="`+realm+`"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid credentials",
			})
			return
		}

		c.Next()
	}
}
