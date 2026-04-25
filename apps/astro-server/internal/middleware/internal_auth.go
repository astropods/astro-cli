package middleware

import (
	"net/http"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/deploytoken"
	"github.com/gin-gonic/gin"
)

const (
	deploymentIDKey = "deploy_token_deployment_id"
	accountIDKey    = "deploy_token_account_id"
)

// RequireDeployToken validates an ASTRO_DEPLOY_TOKEN JWT sent as Bearer token.
// On success it sets the deployment ID in the context; handlers retrieve it with DeploymentIDFromContext.
// When secret is empty (local dev), all requests are allowed through with an empty deployment ID.
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

		deploymentID, accountID, err := deploytoken.Verify(token, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid deploy token"})
			return
		}

		c.Set(deploymentIDKey, deploymentID)
		c.Set(accountIDKey, accountID)
		c.Next()
	}
}

// DeploymentIDFromContext returns the deployment ID set by RequireDeployToken.
func DeploymentIDFromContext(c *gin.Context) string {
	id, _ := c.Get(deploymentIDKey)
	s, _ := id.(string)
	return s
}

// AccountIDFromContext returns the account ID set by RequireDeployToken.
func AccountIDFromContext(c *gin.Context) string {
	id, _ := c.Get(accountIDKey)
	s, _ := id.(string)
	return s
}
