package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

// WithEntitlement wraps a handler with an entitlement check. Use this when the
// entitlement check should only apply to a specific route, not an entire group.
func WithEntitlement(log *logger.Logger, omClient *openmeter.Client, enforce bool, handler gin.HandlerFunc, features ...string) gin.HandlerFunc {
	check := RequireEntitlement(log, omClient, enforce, features...)
	return func(c *gin.Context) {
		check(c)
		if c.IsAborted() {
			return
		}
		handler(c)
	}
}

// RequireEntitlement returns a Gin middleware that checks if the account has access
// to the given feature(s) via OpenMeter entitlements. If enforce is false, the check
// is skipped (log-only). If the OpenMeter client is nil, the check is skipped.
//
// The middleware uses the account from context (set by the account resolution middleware)
// and calls GetEntitlementValue with the account ID as the subject key.
func RequireEntitlement(log *logger.Logger, omClient *openmeter.Client, enforce bool, features ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if omClient == nil {
			c.Next()
			return
		}

		acct, ok := GetAccountFromContext(c)
		if !ok {
			// No account in context — let the handler deal with it
			c.Next()
			return
		}

		for _, feature := range features {
			ent, err := omClient.GetEntitlementValue(c.Request.Context(), acct.ID, feature)
			if err != nil {
				log.Warn("Entitlement check failed", "error", err, "account_id", acct.ID, "feature", feature)
				// On error, allow the request through (fail open)
				continue
			}

			if !ent.HasAccess {
				if enforce {
					c.JSON(http.StatusPaymentRequired, gin.H{
						"error":   "entitlement limit reached",
						"feature": feature,
						"usage":   ent.Usage,
						"limit":   ent.Limit,
					})
					c.Abort()
					return
				}
				log.Warn("Entitlement exceeded (not enforcing)",
					"account_id", acct.ID,
					"feature", feature,
					"usage", ent.Usage,
					"limit", ent.Limit,
				)
			}
		}

		c.Next()
	}
}
