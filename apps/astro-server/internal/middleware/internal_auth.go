package middleware

import (
	"net/http"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/deploytoken"
	"github.com/gin-gonic/gin"
)

const deploymentIDKey = "deploy_token_deployment_id"

// RequireDeployToken validates the deploy token (HS256 JWT) sent as a Bearer
// header by the messaging container. On success, the deployment ID is stored
// in the gin context for downstream handlers to read via
// DeploymentIDFromContext.
//
// The token's anyone_adapters claim is consumed by the messaging container at
// startup and is not surfaced through this middleware; the server-side
// authorize endpoint always re-checks the grants table directly.
//
// secret == "" disables verification entirely (local dev convenience). The
// handler still runs, with an empty deployment ID — handlers must validate
// their own preconditions if they care.
func RequireDeployToken(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.Next()
			return
		}

		raw := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(raw, "Bearer ")
		if !ok || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing deploy token"})
			return
		}

		deploymentID, _, err := deploytoken.Verify(token, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid deploy token"})
			return
		}

		c.Set(deploymentIDKey, deploymentID)
		c.Next()
	}
}

// DeploymentIDFromContext returns the deployment ID set by RequireDeployToken.
// Returns empty string when no token was validated (e.g. dev mode with empty
// secret).
func DeploymentIDFromContext(c *gin.Context) string {
	id, _ := c.Get(deploymentIDKey)
	s, _ := id.(string)
	return s
}
