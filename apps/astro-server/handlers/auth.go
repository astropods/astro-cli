package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/auth"
	"github.com/postman/astro/apps/astro-server/internal/config"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	log            *logger.Logger
	cfg            *config.Config
	workos         *auth.WorkOSClient
	sessionManager *auth.SessionManager
	jwtValidator   *auth.JWTValidator
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(log *logger.Logger, cfg *config.Config) *AuthHandler {
	workos := auth.NewWorkOSClient(
		cfg.Auth.WorkOSAPIKey,
		cfg.Auth.WorkOSClientID,
		cfg.Auth.RedirectURI,
		cfg.Auth.FrontendURL,
	)

	sessionManager := auth.NewSessionManager(
		cfg.Auth.CookiePassword,
		cfg.Auth.SessionMaxAge,
	)

	// Get JWKS URL for token validation
	jwksURL, _ := workos.GetJWKSURL()

	jwtValidator := auth.NewJWTValidator(
		jwksURL,
		cfg.Auth.JWTIssuer,
		cfg.Auth.WorkOSClientID,
	)

	return &AuthHandler{
		log:            log,
		cfg:            cfg,
		workos:         workos,
		sessionManager: sessionManager,
		jwtValidator:   jwtValidator,
	}
}

// Login initiates the authentication flow by redirecting to WorkOS AuthKit
func (h *AuthHandler) Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate a random state for CSRF protection
		state, err := generateRandomState()
		if err != nil {
			h.log.Error("Failed to generate state", "error", err)
			c.JSON(http.StatusInternalServerError, auth.ErrorResponse{
				Error:       "server_error",
				Description: "Failed to initiate authentication",
			})
			return
		}

		// Store state in a short-lived cookie for validation
		h.setSameSiteMode(c)
		c.SetCookie(
			"auth_state",
			state,
			300, // 5 minutes
			"/",
			h.cfg.Auth.CookieDomain,
			h.cfg.Auth.CookieSecure,
			true, // httpOnly
		)

		// Get the authorization URL
		authURL, err := h.workos.GetAuthorizationURL(state)
		if err != nil {
			h.log.Error("Failed to get authorization URL", "error", err)
			c.JSON(http.StatusInternalServerError, auth.ErrorResponse{
				Error:       "server_error",
				Description: "Failed to initiate authentication",
			})
			return
		}

		// Redirect to WorkOS
		c.Redirect(http.StatusFound, authURL)
	}
}

// Callback handles the OAuth callback from WorkOS
func (h *AuthHandler) Callback() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check for error in callback
		if errCode := c.Query("error"); errCode != "" {
			errDesc := c.Query("error_description")
			h.log.Warn("Authentication error from WorkOS",
				"error", errCode,
				"description", errDesc,
			)
			redirectURL := h.workos.BuildCallbackErrorURL(errCode, errDesc)
			c.Redirect(http.StatusFound, redirectURL)
			return
		}

		// Get the authorization code
		code := c.Query("code")
		if code == "" {
			h.log.Warn("Missing authorization code in callback")
			redirectURL := h.workos.BuildCallbackErrorURL("invalid_request", "Missing authorization code")
			c.Redirect(http.StatusFound, redirectURL)
			return
		}

		// Validate state parameter (CSRF protection)
		state := c.Query("state")
		storedState, err := c.Cookie("auth_state")
		if err != nil || state != storedState {
			h.log.Warn("State mismatch in callback", "received", state, "expected", storedState)
			redirectURL := h.workos.BuildCallbackErrorURL("invalid_state", "State parameter mismatch")
			c.Redirect(http.StatusFound, redirectURL)
			return
		}

		// Clear the state cookie
		h.setSameSiteMode(c)
		c.SetCookie("auth_state", "", -1, "/", h.cfg.Auth.CookieDomain, h.cfg.Auth.CookieSecure, true)

		// Exchange code for tokens
		result, err := h.workos.AuthenticateWithCode(c.Request.Context(), code)
		if err != nil {
			h.log.Error("Failed to authenticate with code", "error", err)
			redirectURL := h.workos.BuildCallbackErrorURL("authentication_failed", "Failed to authenticate")
			c.Redirect(http.StatusFound, redirectURL)
			return
		}

		h.log.Info("User authenticated successfully",
			"user_id", result.User.ID,
			"email", result.User.Email,
			"session_id", result.SessionID,
		)

		// Create session data
		session := h.sessionManager.CreateSession(
			result.SessionID,
			result.User.ID,
			result.OrganizationID,
			result.AccessToken,
			result.RefreshToken,
			3600, // 1 hour default, will be capped by session max age
		)

		sessionData := &auth.SessionData{
			Session: session,
			User:    result.User,
		}

		// Seal and store session in cookie
		sealed, err := h.sessionManager.SealSession(sessionData)
		if err != nil {
			h.log.Error("Failed to seal session", "error", err)
			redirectURL := h.workos.BuildCallbackErrorURL("server_error", "Failed to create session")
			c.Redirect(http.StatusFound, redirectURL)
			return
		}

		// Set the session cookie
		maxAge := int(h.cfg.Auth.CookieMaxAge.Seconds())
		h.setSameSiteMode(c)
		c.SetCookie(
			h.cfg.Auth.CookieName,
			sealed,
			maxAge,
			"/",
			h.cfg.Auth.CookieDomain,
			h.cfg.Auth.CookieSecure,
			true, // httpOnly
		)

		// Redirect to frontend
		c.Redirect(http.StatusFound, h.workos.BuildCallbackSuccessURL())
	}
}

// Logout handles user logout
func (h *AuthHandler) Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get session from cookie
		sessionCookie, err := c.Cookie(h.cfg.Auth.CookieName)
		if err != nil {
			// No session, just redirect to frontend
			c.Redirect(http.StatusFound, h.cfg.Auth.FrontendURL)
			return
		}

		// Try to unseal and get session ID
		sessionData, err := h.sessionManager.UnsealSession(sessionCookie)
		var sessionID string
		if err == nil && sessionData.Session != nil {
			sessionID = sessionData.Session.ID

			h.log.Info("Processing logout request", "session_id", sessionID)

			// Revoke the session at WorkOS (best effort)
			if err := h.workos.RevokeSession(c.Request.Context(), sessionID); err != nil {
				h.log.Warn("Failed to revoke session at WorkOS", "error", err, "session_id", sessionID)
			}
		}

		// Clear the session cookie
		h.setSameSiteMode(c)
		c.SetCookie(
			h.cfg.Auth.CookieName,
			"",
			-1,
			"/",
			h.cfg.Auth.CookieDomain,
			h.cfg.Auth.CookieSecure,
			true,
		)

		// If we have a session ID, redirect to WorkOS logout
		if sessionID != "" {
			logoutURL, err := h.workos.GetLogoutURL(sessionID)
			if err != nil {
				h.log.Warn("Failed to get logout URL", "error", err)
				c.Redirect(http.StatusFound, h.cfg.Auth.FrontendURL)
				return
			}

			h.log.Info("Redirecting to WorkOS logout", "logout_url", logoutURL)
			c.Redirect(http.StatusFound, logoutURL)
			return
		}

		c.Redirect(http.StatusFound, h.cfg.Auth.FrontendURL)
	}
}

// Me returns the current authenticated user
func (h *AuthHandler) Me() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get session from cookie
		sessionCookie, err := c.Cookie(h.cfg.Auth.CookieName)
		if err != nil {
			c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "unauthorized",
				Description: "No session found",
			})
			return
		}

		// Unseal session
		sessionData, err := h.sessionManager.UnsealSession(sessionCookie)
		if err != nil {
			h.log.Debug("Failed to unseal session", "error", err)

			// Clear invalid cookie
			h.setSameSiteMode(c)
			c.SetCookie(
				h.cfg.Auth.CookieName,
				"",
				-1,
				"/",
				h.cfg.Auth.CookieDomain,
				h.cfg.Auth.CookieSecure,
				true,
			)

			c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "session_invalid",
				Description: "Session is invalid or expired",
			})
			return
		}

		// Check if session is still valid
		if !h.sessionManager.IsSessionValid(sessionData.Session) {
			// Try to refresh the session
			refreshed, err := h.refreshSession(c, sessionData)
			if err != nil {
				h.log.Debug("Failed to refresh session", "error", err)

				// Clear expired cookie
				h.setSameSiteMode(c)
				c.SetCookie(
					h.cfg.Auth.CookieName,
					"",
					-1,
					"/",
					h.cfg.Auth.CookieDomain,
					h.cfg.Auth.CookieSecure,
					true,
				)

				c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
					Error:       "session_expired",
					Description: "Session has expired",
				})
				return
			}
			sessionData = refreshed
		}

		// Return user info
		c.JSON(http.StatusOK, auth.AuthResponse{
			User:         sessionData.User,
			SessionID:    sessionData.Session.ID,
			Organization: sessionData.Session.OrganizationID,
			Role:         sessionData.Session.Role,
			ExpiresAt:    sessionData.Session.ExpiresAt.Format(time.RFC3339),
		})
	}
}

// Refresh handles explicit session refresh
func (h *AuthHandler) Refresh() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get session from cookie
		sessionCookie, err := c.Cookie(h.cfg.Auth.CookieName)
		if err != nil {
			c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "unauthorized",
				Description: "No session found",
			})
			return
		}

		// Unseal session
		sessionData, err := h.sessionManager.UnsealSession(sessionCookie)
		if err != nil {
			c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "session_invalid",
				Description: "Session is invalid",
			})
			return
		}

		// Refresh the session
		refreshed, err := h.refreshSession(c, sessionData)
		if err != nil {
			h.log.Error("Failed to refresh session", "error", err)
			c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "refresh_failed",
				Description: "Failed to refresh session",
			})
			return
		}

		c.JSON(http.StatusOK, auth.AuthResponse{
			User:         refreshed.User,
			SessionID:    refreshed.Session.ID,
			Organization: refreshed.Session.OrganizationID,
			Role:         refreshed.Session.Role,
			ExpiresAt:    refreshed.Session.ExpiresAt.Format(time.RFC3339),
		})
	}
}

// refreshSession refreshes the session using the refresh token
func (h *AuthHandler) refreshSession(c *gin.Context, sessionData *auth.SessionData) (*auth.SessionData, error) {
	result, err := h.workos.AuthenticateWithRefreshToken(c.Request.Context(), sessionData.Session.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Update session with new tokens
	newSession := h.sessionManager.CreateSession(
		sessionData.Session.ID,
		sessionData.Session.UserID,
		sessionData.Session.OrganizationID,
		result.AccessToken,
		result.RefreshToken,
		3600,
	)

	newSessionData := &auth.SessionData{
		Session: newSession,
		User:    sessionData.User,
	}

	// Seal and update cookie
	sealed, err := h.sessionManager.SealSession(newSessionData)
	if err != nil {
		return nil, err
	}

	maxAge := int(h.cfg.Auth.CookieMaxAge.Seconds())
	h.setSameSiteMode(c)
	c.SetCookie(
		h.cfg.Auth.CookieName,
		sealed,
		maxAge,
		"/",
		h.cfg.Auth.CookieDomain,
		h.cfg.Auth.CookieSecure,
		true,
	)

	return newSessionData, nil
}

// GetSessionManager returns the session manager for use in middleware
func (h *AuthHandler) GetSessionManager() *auth.SessionManager {
	return h.sessionManager
}

// GetJWTValidator returns the JWT validator for use in middleware
func (h *AuthHandler) GetJWTValidator() *auth.JWTValidator {
	return h.jwtValidator
}

// generateRandomState generates a cryptographically random state string
func generateRandomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// setSameSiteMode configures the SameSite mode on the context based on config
func (h *AuthHandler) setSameSiteMode(c *gin.Context) {
	switch h.cfg.Auth.CookieSameSite {
	case "Strict":
		c.SetSameSite(http.SameSiteStrictMode)
	case "None":
		c.SetSameSite(http.SameSiteNoneMode)
	default: // "Lax" or any other value defaults to Lax
		c.SetSameSite(http.SameSiteLaxMode)
	}
}
