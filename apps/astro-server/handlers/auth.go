package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	avatarpkg "github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	log            *logger.Logger
	cfg            *config.Config
	workos         *auth.WorkOSClient
	sessionManager *auth.SessionManager
	jwtValidator   *auth.JWTValidator
	allowedOrigins map[string]bool
	accountStore   *account.AccountStore
	orgSync        *org.Sync
	avatarStore    *avatarpkg.Store
}

// SetOrgSync sets the org sync service on the auth handler.
// Called after construction since org.Sync depends on the account store
// which is also a dependency of the auth handler.
func (h *AuthHandler) SetOrgSync(sync *org.Sync) {
	h.orgSync = sync
}

// SetAvatarStore sets the avatar store on the auth handler for profile picture ingestion at signup.
func (h *AuthHandler) SetAvatarStore(store *avatarpkg.Store) {
	h.avatarStore = store
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(log *logger.Logger, cfg *config.Config, accountStore *account.AccountStore) *AuthHandler {
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

	// Note: WorkOS access tokens don't include an 'aud' claim, so we pass empty string
	// to skip audience validation. The issuer already includes the client ID for validation.
	jwtValidator := auth.NewJWTValidator(
		jwksURL,
		cfg.Auth.JWTIssuer,
		"", // No audience validation for WorkOS tokens
	)

	// Build allowed origins map for quick lookup
	allowedOrigins := make(map[string]bool)
	for _, origin := range cfg.Security.AllowedOrigins {
		allowedOrigins[origin] = true
	}

	return &AuthHandler{
		log:            log,
		cfg:            cfg,
		workos:         workos,
		sessionManager: sessionManager,
		jwtValidator:   jwtValidator,
		allowedOrigins: allowedOrigins,
		accountStore:   accountStore,
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

		// Store the origin for post-auth redirect (if allowed)
		origin := c.Request.Header.Get("Origin")
		if origin == "" {
			// Fall back to Referer header and extract origin
			if referer := c.Request.Header.Get("Referer"); referer != "" {
				if u, err := url.Parse(referer); err == nil {
					origin = u.Scheme + "://" + u.Host
				}
			}
		}
		if origin != "" && h.isAllowedOrigin(origin) {
			c.SetCookie(
				"auth_origin",
				origin,
				300, // 5 minutes
				"/",
				h.cfg.Auth.CookieDomain,
				h.cfg.Auth.CookieSecure,
				true, // httpOnly
			)
		}

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
		// Get the redirect URL early (before clearing cookies)
		redirectURL := h.getRedirectURL(c)

		// Helper to build error redirect URL
		buildErrorURL := func(errorCode, errorDesc string) string {
			u, _ := url.Parse(redirectURL)
			q := u.Query()
			q.Set("error", errorCode)
			q.Set("error_description", errorDesc)
			u.RawQuery = q.Encode()
			return u.String()
		}

		// Check for error in callback
		if errCode := c.Query("error"); errCode != "" {
			errDesc := c.Query("error_description")
			h.log.Warn("Authentication error from WorkOS",
				"error", errCode,
				"description", errDesc,
			)
			c.Redirect(http.StatusFound, buildErrorURL(errCode, errDesc))
			return
		}

		// Get the authorization code
		code := c.Query("code")
		if code == "" {
			h.log.Warn("Missing authorization code in callback")
			c.Redirect(http.StatusFound, buildErrorURL("invalid_request", "Missing authorization code"))
			return
		}

		// Validate state parameter (CSRF protection)
		state := c.Query("state")
		storedState, err := c.Cookie("auth_state")
		if err != nil || state != storedState {
			h.log.Warn("State mismatch in callback", "received", state, "expected", storedState)
			c.Redirect(http.StatusFound, buildErrorURL("invalid_state", "State parameter mismatch"))
			return
		}

		// Clear the state and origin cookies
		h.setSameSiteMode(c)
		c.SetCookie("auth_state", "", -1, "/", h.cfg.Auth.CookieDomain, h.cfg.Auth.CookieSecure, true)
		c.SetCookie("auth_origin", "", -1, "/", h.cfg.Auth.CookieDomain, h.cfg.Auth.CookieSecure, true)

		// Exchange code for tokens
		result, err := h.workos.AuthenticateWithCode(c.Request.Context(), code)
		if err != nil {
			h.log.Error("Failed to authenticate with code", "error", err)
			c.Redirect(http.StatusFound, buildErrorURL("authentication_failed", "Failed to authenticate"))
			return
		}

		h.log.Info("User authenticated successfully",
			"user_id", result.User.ID,
			"email", result.User.Email,
			"session_id", result.SessionID,
		)

		// Best-effort: sync org memberships from WorkOS to local store
		if h.orgSync != nil {
			if err := h.orgSync.SyncMembershipsForUser(c.Request.Context(), result.User.ID); err != nil {
				h.log.Warn("Failed to sync memberships on login", "error", err, "user_id", result.User.ID)
			}
		}

		// Best-effort: ingest OAuth profile picture into our CDN
		if h.avatarStore != nil && result.User.ProfilePictureURL != "" && h.accountStore != nil {
			userAccounts, acctErr := h.accountStore.GetAccountsForUser(result.User.ID)
			if acctErr == nil {
				for _, a := range userAccounts {
					if a.Type == "personal" && a.AvatarVersion == 0 {
						acctName := a.Name
						acctID := a.ID
						profileURL := result.User.ProfilePictureURL
						go func() {
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							if err := h.avatarStore.Ingest(ctx, acctName, profileURL); err != nil {
								h.log.Warn("Failed to ingest profile picture", "error", err, "account", acctName)
							} else {
								if _, err := h.accountStore.IncrementAvatarVersion(acctID); err != nil {
									h.log.Warn("Failed to increment avatar version after ingestion", "error", err, "account", acctName)
								}
							}
						}()
						break
					}
				}
			}
		}

		// Create session data with role and permissions from JWT claims
		session := h.sessionManager.CreateSession(
			result.SessionID,
			result.User.ID,
			result.OrganizationID,
			result.AccessToken,
			result.RefreshToken,
			3600, // 1 hour default, will be capped by session max age
		)

		claims := auth.ExtractTokenClaims(result.AccessToken)
		session.Role = claims.Role
		session.Permissions = claims.Permissions

		sessionData := &auth.SessionData{
			Session: session,
			User:    result.User,
		}

		// Seal and store session in cookie
		sealed, err := h.sessionManager.SealSession(sessionData)
		if err != nil {
			h.log.Error("Failed to seal session", "error", err)
			c.Redirect(http.StatusFound, buildErrorURL("server_error", "Failed to create session"))
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
		c.Redirect(http.StatusFound, redirectURL)
	}
}

// Logout handles user logout
func (h *AuthHandler) Logout() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Determine redirect URL based on origin
		redirectURL := h.getRedirectURLFromRequest(c)

		// Get session from cookie
		sessionCookie, err := c.Cookie(h.cfg.Auth.CookieName)
		if err != nil {
			// No session, just redirect to frontend
			c.Redirect(http.StatusFound, redirectURL)
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

		// If we have a session ID, redirect to WorkOS logout with return URL
		if sessionID != "" {
			logoutURL, err := h.workos.GetLogoutURLWithReturnTo(sessionID, redirectURL)
			if err != nil {
				h.log.Warn("Failed to get logout URL", "error", err)
				c.Redirect(http.StatusFound, redirectURL)
				return
			}

			h.log.Info("Redirecting to WorkOS logout", "logout_url", logoutURL)
			c.Redirect(http.StatusFound, logoutURL)
			return
		}

		c.Redirect(http.StatusFound, redirectURL)
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

		// Return user info with accounts
		permissions := sessionData.Session.Permissions
		if permissions == nil {
			permissions = []string{}
		}
		c.JSON(http.StatusOK, auth.AuthResponse{
			User:         sessionData.User,
			SessionID:    sessionData.Session.ID,
			Organization: sessionData.Session.OrganizationID,
			Role:         sessionData.Session.Role,
			Permissions:  permissions,
			ExpiresAt:    sessionData.Session.ExpiresAt.Format(time.RFC3339),
			Accounts:     h.fetchAccounts(sessionData.User.ID),
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

		refreshedPermissions := refreshed.Session.Permissions
		if refreshedPermissions == nil {
			refreshedPermissions = []string{}
		}
		c.JSON(http.StatusOK, auth.AuthResponse{
			User:         refreshed.User,
			SessionID:    refreshed.Session.ID,
			Organization: refreshed.Session.OrganizationID,
			Role:         refreshed.Session.Role,
			Permissions:  refreshedPermissions,
			ExpiresAt:    refreshed.Session.ExpiresAt.Format(time.RFC3339),
			Accounts:     h.fetchAccounts(refreshed.User.ID),
		})
	}
}

// SwitchOrgRequest represents the request to switch org context
type SwitchOrgRequest struct {
	OrganizationID string `json:"organization_id" binding:"required"`
}

// SwitchOrg handles POST /auth/switch-org
// Refreshes the session token scoped to a different organization, giving the
// user a new JWT with the correct role and permissions for that org.
func (h *AuthHandler) SwitchOrg() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SwitchOrgRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, auth.ErrorResponse{
				Error:       "invalid_request",
				Description: "organization_id is required",
			})
			return
		}

		// Get current session
		sessionCookie, err := c.Cookie(h.cfg.Auth.CookieName)
		if err != nil {
			c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "unauthorized",
				Description: "No session found",
			})
			return
		}

		sessionData, err := h.sessionManager.UnsealSession(sessionCookie)
		if err != nil {
			c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "session_invalid",
				Description: "Session is invalid",
			})
			return
		}

		// Refresh token with the target organization ID
		result, err := h.workos.AuthenticateWithRefreshTokenForOrg(
			c.Request.Context(),
			sessionData.Session.RefreshToken,
			req.OrganizationID,
		)
		if err != nil {
			h.log.Error("Failed to switch org", "error", err, "org_id", req.OrganizationID)
			c.JSON(http.StatusBadRequest, auth.ErrorResponse{
				Error:       "switch_failed",
				Description: "Failed to switch organization",
			})
			return
		}

		// Build new session with the org-scoped token
		newSession := h.sessionManager.CreateSession(
			sessionData.Session.ID,
			sessionData.Session.UserID,
			req.OrganizationID,
			result.AccessToken,
			result.RefreshToken,
			3600,
		)

		claims := auth.ExtractTokenClaims(result.AccessToken)
		newSession.Role = claims.Role
		newSession.Permissions = claims.Permissions

		newSessionData := &auth.SessionData{
			Session: newSession,
			User:    sessionData.User,
		}

		// Seal and update cookie
		sealed, err := h.sessionManager.SealSession(newSessionData)
		if err != nil {
			h.log.Error("Failed to seal session after org switch", "error", err)
			c.JSON(http.StatusInternalServerError, auth.ErrorResponse{
				Error:       "server_error",
				Description: "Failed to update session",
			})
			return
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

		permissions := newSessionData.Session.Permissions
		if permissions == nil {
			permissions = []string{}
		}
		c.JSON(http.StatusOK, auth.AuthResponse{
			User:         newSessionData.User,
			SessionID:    newSessionData.Session.ID,
			Organization: newSessionData.Session.OrganizationID,
			Role:         newSessionData.Session.Role,
			Permissions:  permissions,
			ExpiresAt:    newSessionData.Session.ExpiresAt.Format(time.RFC3339),
			Accounts:     h.fetchAccounts(newSessionData.User.ID),
		})
	}
}

// fetchAccounts returns the accounts for a user, always returning a non-nil slice
func (h *AuthHandler) fetchAccounts(userID string) []auth.AuthAccountResponse {
	accounts := make([]auth.AuthAccountResponse, 0)
	if h.accountStore != nil {
		userAccounts, err := h.accountStore.GetAccountsForUser(userID)
		if err != nil {
			h.log.Warn("Failed to fetch accounts for user", "error", err, "user_id", userID)
		} else {
			for _, a := range userAccounts {
				accounts = append(accounts, auth.AuthAccountResponse{
					ID:                   a.ID,
					Name:                 a.Name,
					Type:                 a.Type,
					WorkOSOrganizationID: a.WorkOSOrganizationID,
					AvatarVersion:        a.AvatarVersion,
				})
			}
		}
	}
	return accounts
}

// refreshSession refreshes the session using the refresh token
func (h *AuthHandler) refreshSession(c *gin.Context, sessionData *auth.SessionData) (*auth.SessionData, error) {
	result, err := h.workos.AuthenticateWithRefreshToken(c.Request.Context(), sessionData.Session.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Run org sync and user fetch concurrently (both are independent HTTP calls)
	var freshUser *auth.User
	var userErr error
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		freshUser, userErr = h.workos.GetUser(c.Request.Context(), sessionData.Session.UserID)
	}()

	// Best-effort: sync org memberships on token refresh
	if h.orgSync != nil {
		if err := h.orgSync.SyncMembershipsForUser(c.Request.Context(), sessionData.Session.UserID); err != nil {
			h.log.Warn("Failed to sync memberships on refresh", "error", err, "user_id", sessionData.Session.UserID)
		}
	}

	wg.Wait()
	if userErr != nil {
		h.log.Warn("Failed to fetch fresh user on refresh, using cached data", "error", userErr)
		freshUser = sessionData.User
	}

	// Update session with new tokens and refreshed claims
	newSession := h.sessionManager.CreateSession(
		sessionData.Session.ID,
		sessionData.Session.UserID,
		sessionData.Session.OrganizationID,
		result.AccessToken,
		result.RefreshToken,
		3600,
	)

	claims := auth.ExtractTokenClaims(result.AccessToken)
	newSession.Role = claims.Role
	newSession.Permissions = claims.Permissions

	newSessionData := &auth.SessionData{
		Session: newSession,
		User:    freshUser,
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

// GetWorkOSClient returns the WorkOS client for use in other handlers
func (h *AuthHandler) GetWorkOSClient() *auth.WorkOSClient {
	return h.workos
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

// isAllowedOrigin checks if an origin is in the allowed origins list
func (h *AuthHandler) isAllowedOrigin(origin string) bool {
	if h.allowedOrigins["*"] {
		return true
	}
	return h.allowedOrigins[origin]
}

// getRedirectURL determines the appropriate redirect URL based on the stored origin cookie
// Falls back to the configured frontend URL if the origin is not allowed
func (h *AuthHandler) getRedirectURL(c *gin.Context) string {
	origin, err := c.Cookie("auth_origin")
	if err == nil && origin != "" && h.isAllowedOrigin(origin) {
		return origin
	}
	return h.cfg.Auth.FrontendURL
}

// getRedirectURLFromRequest determines the redirect URL from query param, headers, or config
// Priority: 1) redirect query param, 2) Origin header, 3) Referer header, 4) configured FrontendURL
func (h *AuthHandler) getRedirectURLFromRequest(c *gin.Context) string {
	// First check for explicit redirect query parameter
	if redirect := c.Query("redirect"); redirect != "" {
		if h.isAllowedOrigin(redirect) {
			return redirect
		}
	}

	// Fall back to Origin header
	origin := c.Request.Header.Get("Origin")
	if origin == "" {
		// Fall back to Referer header
		if referer := c.Request.Header.Get("Referer"); referer != "" {
			if u, err := url.Parse(referer); err == nil {
				origin = u.Scheme + "://" + u.Host
			}
		}
	}
	if origin != "" && h.isAllowedOrigin(origin) {
		return origin
	}
	return h.cfg.Auth.FrontendURL
}
