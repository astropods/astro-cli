package middleware

import (
	"net/http"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware provides authentication middleware for protecting routes
type AuthMiddleware struct {
	log            *logger.Logger
	cfg            *config.Config
	sessionManager *auth.SessionManager
	jwtValidator   *auth.JWTValidator
}

// NewAuthMiddleware creates a new auth middleware instance
func NewAuthMiddleware(
	log *logger.Logger,
	cfg *config.Config,
	sessionManager *auth.SessionManager,
	jwtValidator *auth.JWTValidator,
) *AuthMiddleware {
	return &AuthMiddleware{
		log:            log,
		cfg:            cfg,
		sessionManager: sessionManager,
		jwtValidator:   jwtValidator,
	}
}

// RequireAuth middleware ensures the request is authenticated.
// It checks for a Bearer token or session cookie.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// First, try to authenticate via Authorization header (for API clients)
		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if m.authenticateWithToken(c, token) {
					c.Next()
					return
				}
			}
		}

		// Then, try to authenticate via session cookie (for web clients)
		sessionCookie, err := c.Cookie(m.cfg.Auth.CookieName)
		if err == nil && sessionCookie != "" {
			if m.authenticateWithCookie(c, sessionCookie) {
				c.Next()
				return
			}
		}

		// No valid authentication found
		c.AbortWithStatusJSON(http.StatusUnauthorized, auth.ErrorResponse{
			Error:       "unauthorized",
			Description: "Authentication required",
		})
	}
}

// OptionalAuth middleware attempts to authenticate the request but doesn't require it
// If authentication succeeds, user info is added to context; otherwise, the request continues
func (m *AuthMiddleware) OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try Authorization header
		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			if strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				m.authenticateWithToken(c, token)
			}
		}

		// Try session cookie if not already authenticated
		if _, exists := c.Get(string(auth.UserContextKey)); !exists {
			sessionCookie, err := c.Cookie(m.cfg.Auth.CookieName)
			if err == nil && sessionCookie != "" {
				m.authenticateWithCookie(c, sessionCookie)
			}
		}

		c.Next()
	}
}

// RequirePermission middleware ensures the user has a specific permission
func (m *AuthMiddleware) RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, exists := c.Get(string(auth.SessionContextKey))
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "unauthorized",
				Description: "Authentication required",
			})
			return
		}

		sessionData, ok := session.(*auth.Session)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, auth.ErrorResponse{
				Error:       "server_error",
				Description: "Invalid session data",
			})
			return
		}

		// Check if user has the required permission
		hasPermission := false
		for _, p := range sessionData.Permissions {
			if p == permission {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.AbortWithStatusJSON(http.StatusForbidden, auth.ErrorResponse{
				Error:       "forbidden",
				Description: "Insufficient permissions",
			})
			return
		}

		c.Next()
	}
}

// RequireRole middleware ensures the user has a specific role
func (m *AuthMiddleware) RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, exists := c.Get(string(auth.SessionContextKey))
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "unauthorized",
				Description: "Authentication required",
			})
			return
		}

		sessionData, ok := session.(*auth.Session)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, auth.ErrorResponse{
				Error:       "server_error",
				Description: "Invalid session data",
			})
			return
		}

		if sessionData.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, auth.ErrorResponse{
				Error:       "forbidden",
				Description: "Insufficient role",
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
		m.log.Warn("Token validation failed", "error", err)
		return false
	}

	// Create session from claims
	session := &auth.Session{
		ID:                 claims.SessionID,
		UserID:             claims.Subject,
		OrganizationID:     claims.OrganizationID,
		WorkOSMembershipID: claims.OrganizationMembershipID,
		Role:               claims.Role,
		Permissions:        claims.Permissions,
		AccessToken:        token,
	}

	// Create minimal user from claims
	user := &auth.User{
		ID: claims.Subject,
	}

	// Set context values
	c.Set(string(auth.UserContextKey), user)
	c.Set(string(auth.SessionContextKey), session)

	return true
}

// authenticateWithCookie validates a session cookie and sets context values
func (m *AuthMiddleware) authenticateWithCookie(c *gin.Context, cookieValue string) bool {
	sessionData, err := m.sessionManager.UnsealSession(cookieValue)
	if err != nil {
		m.log.Debug("Failed to unseal session", "error", err)
		return false
	}

	if !m.sessionManager.IsSessionValid(sessionData.Session) {
		m.log.Debug("Session is not valid")
		return false
	}

	// Set context values
	c.Set(string(auth.UserContextKey), sessionData.User)
	c.Set(string(auth.SessionContextKey), sessionData.Session)

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
