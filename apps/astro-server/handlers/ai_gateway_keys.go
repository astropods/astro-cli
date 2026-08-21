package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
)

// AIGatewayKeyResponse is returned by POST /accounts/:account/ai-gateway-keys.
// The CLI maps api_key + base_url onto ASTRO_GATEWAY_API_KEY + ASTRO_GATEWAY_URL
// in the local agent container — same names the deployer would inject in prod.
type AIGatewayKeyResponse struct {
	KeyID     string `json:"key_id"`
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url"`
	ExpiresAt string `json:"expires_at"`
}

// IssueAIGatewayDevKey handles POST /api/v1/accounts/:account/ai-gateway-keys.
//
// Returns the existing dev key on the account_ai_gateway row when it has
// enough remaining lifetime (DevKeySafetyMargin); mints a fresh one otherwise,
// persisting the KMS-encrypted plaintext and best-effort revoking the
// predecessor upstream. user_id and team_id on the LiteLLM /key/generate call
// are always the account-id — load-bearing attribution invariant per
// docs/plans/ai-gateway-astro-server.md.
//
// One dev key per account (shared across that account's developers). LiteLLM-
// side metadata.actor_user_id records who minted it for audit on the gateway.
func IssueAIGatewayDevKey(
	log *logger.Logger,
	provisioner *aigateway.Provisioner,
	devStore *aigateway.DevStore,
	vault *envelope.Vault,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if provisioner == nil || devStore == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "AI Gateway is not configured in this environment",
			})
			return
		}
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		ctx := c.Request.Context()
		apiKey, baseURL, expiresAt, err := provisioner.EnsureDevKey(ctx, devStore, vault, acct.ID, user.ID)
		if err != nil {
			log.Error("ai gateway keys: ensure AI Gateway dev key failed", "error", err, "account_id", acct.ID, "user_id", user.ID)
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to issue AI Gateway key"})
			return
		}

		c.JSON(http.StatusOK, AIGatewayKeyResponse{
			APIKey:    apiKey,
			BaseURL:   baseURL,
			ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
		})
	}
}
