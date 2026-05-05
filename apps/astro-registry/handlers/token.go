package handlers

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-registry/internal/auth"
	"github.com/astropods/astro/apps/astro-registry/internal/logger"
	"github.com/gin-gonic/gin"
)

// IdPValidator validates the IdP credential presented at the token endpoint.
// Implemented by *auth.JWTValidator in production.
type IdPValidator interface {
	ValidateToken(ctx context.Context, token string) (*auth.JWTClaims, error)
}

// AccountResolver resolves an account name to its UUID and verifies the user's
// membership in a single query. Implemented by *account.MembershipChecker.
type AccountResolver interface {
	IsMemberWithID(accountName, userID string) (bool, string, error)
}

// TokenSigner mints registry-scope tokens. Implemented by *auth.RegistryTokenSigner.
type TokenSigner interface {
	Issue(subject string, access []auth.ResourceAccess) (string, int, time.Time, error)
}

// TokenHandlerConfig wires the /token endpoint dependencies.
type TokenHandlerConfig struct {
	Logger            *logger.Logger
	WorkOSValidator   IdPValidator
	Signer            TokenSigner
	MembershipChecker AccountResolver
	Service           string // expected service name in query param
}

// TokenResponse is the spec response body for the token endpoint.
// https://distribution.github.io/distribution/spec/auth/token/#token-response-fields
type TokenResponse struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"` // Compat alias for OAuth2 clients.
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}

// Token handles GET /token — exchanges an IdP credential (WorkOS bearer in
// HTTP Basic password slot) for a registry-scope JWT.
func Token(cfg TokenHandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse Basic auth. Username is conventional ("token" from astro-cli);
		// the IdP credential is in the password slot per the Distribution spec.
		_, password, ok := c.Request.BasicAuth()
		if !ok {
			cfg.Logger.Warn("Token request missing Basic auth")
			c.Header("WWW-Authenticate", `Basic realm="astro-registry"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"errors": []gin.H{{
				"code":    "UNAUTHORIZED",
				"message": "Basic auth required",
			}}})
			return
		}

		claims, err := cfg.WorkOSValidator.ValidateToken(c.Request.Context(), password)
		if err != nil {
			cfg.Logger.Warn("Token request failed WorkOS validation", "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"errors": []gin.H{{
				"code":    "UNAUTHORIZED",
				"message": "Invalid IdP credential",
			}}})
			return
		}

		service := c.Query("service")
		if cfg.Service != "" && service != cfg.Service {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"errors": []gin.H{{
				"code":    "DENIED",
				"message": "Unrecognized service",
			}}})
			return
		}

		// scope params: zero or more "repository:<ns>/<image>:<actions>" entries.
		// Spec returns the intersection of requested and authorized actions —
		// never an error for unauthorized scopes.
		requested := c.Request.URL.Query()["scope"]
		granted := make([]auth.ResourceAccess, 0, len(requested))
		for _, raw := range requested {
			parsed, ok := parseScope(raw)
			if !ok {
				cfg.Logger.Warn("Token request had malformed scope", "scope", raw)
				continue
			}
			authorized := authorizeScope(c.Request.Context(), parsed, claims, cfg.MembershipChecker, cfg.Logger)
			if len(authorized.Actions) > 0 {
				granted = append(granted, authorized)
			}
		}

		token, expiresIn, issuedAt, err := cfg.Signer.Issue(claims.Subject, granted)
		if err != nil {
			cfg.Logger.Error("Failed to mint registry token", "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"errors": []gin.H{{
				"code":    "SERVER_ERROR",
				"message": "Failed to mint registry token",
			}}})
			return
		}

		c.JSON(http.StatusOK, TokenResponse{
			Token:       token,
			AccessToken: token,
			ExpiresIn:   expiresIn,
			IssuedAt:    issuedAt,
		})
	}
}

// parseScope parses a "repository:<ns>/<image>:<actions>" entry.
// Returns ok=false on malformed input. Actions are comma-separated.
func parseScope(s string) (auth.ResourceAccess, bool) {
	// Split on ":" — exactly 3 segments. The Distribution spec also allows
	// trailing ":" with empty actions, which we treat as malformed.
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return auth.ResourceAccess{}, false
	}
	actions := strings.Split(parts[2], ",")
	cleaned := actions[:0]
	for _, a := range actions {
		a = strings.TrimSpace(a)
		if a == "pull" || a == "push" || a == "delete" {
			cleaned = append(cleaned, a)
		}
	}
	if len(cleaned) == 0 {
		return auth.ResourceAccess{}, false
	}
	return auth.ResourceAccess{
		Type:    parts[0],
		Name:    parts[1],
		Actions: cleaned,
	}, true
}

// authorizeScope returns the subset of requested actions the user is permitted.
// Empty Actions → caller drops the scope from the response.
func authorizeScope(
	ctx context.Context,
	requested auth.ResourceAccess,
	claims *auth.JWTClaims,
	mc AccountResolver,
	log *logger.Logger,
) auth.ResourceAccess {
	_ = ctx // reserved for future per-request plumbing; current MembershipChecker takes no ctx

	out := auth.ResourceAccess{Type: requested.Type, Name: requested.Name}
	if requested.Type != "repository" {
		return out
	}

	// "<ns>/<image>" — namespace is the account name.
	parts := strings.SplitN(requested.Name, "/", 2)
	if len(parts) < 2 || parts[0] == "" {
		return out
	}
	namespace := parts[0]

	if mc == nil {
		return out
	}
	isMember, accountID, err := mc.IsMemberWithID(namespace, claims.Subject)
	if err != nil {
		if log != nil {
			log.Warn("Membership check failed during token issuance",
				"namespace", namespace, "user_id", claims.Subject, "error", err)
		}
		return out
	}
	if !isMember {
		if log != nil {
			log.Warn("Token request denied — not a member",
				"namespace", namespace, "user_id", claims.Subject)
		}
		return out
	}
	out.AccountID = accountID

	// Map registry actions to internal permissions for org accounts.
	// Personal accounts (no org_id on the JWT) skip the permission check —
	// membership is sufficient.
	hasOrg := claims.OrganizationID != ""

	allowed := make([]string, 0, len(requested.Actions))
	for _, action := range requested.Actions {
		if hasOrg {
			required := registryActionToPermission(action)
			if required != "" && !slices.Contains(claims.Permissions, required) {
				continue
			}
		}
		allowed = append(allowed, action)
	}
	out.Actions = allowed
	return out
}

// registryActionToPermission maps a Distribution action to the internal
// permission required to perform it.
func registryActionToPermission(action string) string {
	switch action {
	case "pull":
		return "agents:read"
	case "push", "delete":
		return "agents:write"
	default:
		return ""
	}
}
