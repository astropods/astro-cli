package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-registry/internal/auth"
	"github.com/postman/astro/apps/astro-registry/internal/config"
	"github.com/postman/astro/apps/astro-registry/internal/logger"
)

// AuthMiddleware provides authentication middleware for protecting routes
type AuthMiddleware struct {
	log          *logger.Logger
	cfg          *config.Config
	jwtValidator *auth.JWTValidator
}

// NewAuthMiddleware creates a new auth middleware instance
func NewAuthMiddleware(log *logger.Logger, cfg *config.Config) *AuthMiddleware {
	// Note: WorkOS access tokens don't include an 'aud' claim, so we pass empty string
	// to skip audience validation. The issuer already includes the client ID for validation.
	jwtValidator := auth.NewJWTValidator(
		cfg.Auth.JWKSEndpoint,
		cfg.Auth.JWTIssuer,
		"", // No audience validation for WorkOS tokens
	)

	return &AuthMiddleware{
		log:          log,
		cfg:          cfg,
		jwtValidator: jwtValidator,
	}
}

// RequireAuth middleware ensures the request is authenticated via Bearer token.
// For registry operations, we only support Bearer token authentication.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Header("WWW-Authenticate", `Bearer realm="astro-registry"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "unauthorized",
				Description: "Authorization header required",
			})
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.Header("WWW-Authenticate", `Bearer realm="astro-registry"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "unauthorized",
				Description: "Bearer token required",
			})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if !m.authenticateWithToken(c, token) {
			c.Header("WWW-Authenticate", `Bearer realm="astro-registry",error="invalid_token"`)
			c.AbortWithStatusJSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "unauthorized",
				Description: "Invalid or expired token",
			})
			return
		}

		c.Next()
	}
}

// authenticateWithToken validates a Bearer token and sets context values
func (m *AuthMiddleware) authenticateWithToken(c *gin.Context, token string) bool {
	claims, err := m.jwtValidator.ValidateToken(c.Request.Context(), token)
	if err != nil {
		m.log.Debug("Token validation failed", "error", err)
		return false
	}

	// Create session from claims
	session := &auth.Session{
		ID:             claims.SessionID,
		UserID:         claims.Subject,
		OrganizationID: claims.OrganizationID,
		Role:           claims.Role,
		Permissions:    claims.Permissions,
	}

	// Create minimal user from claims
	user := &auth.User{
		ID:             claims.Subject,
		OrganizationID: claims.OrganizationID,
	}

	// Set context values
	c.Set(string(auth.UserContextKey), user)
	c.Set(string(auth.SessionContextKey), session)

	return true
}

// GetUser retrieves the authenticated user from the context
func GetUser(c *gin.Context) (*auth.User, bool) {
	user, exists := c.Get(string(auth.UserContextKey))
	if !exists {
		return nil, false
	}
	u, ok := user.(*auth.User)
	return u, ok
}

// GetSession retrieves the session from the context
func GetSession(c *gin.Context) (*auth.Session, bool) {
	session, exists := c.Get(string(auth.SessionContextKey))
	if !exists {
		return nil, false
	}
	s, ok := session.(*auth.Session)
	return s, ok
}
