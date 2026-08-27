package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	avatarpkg "github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/workos/workos-go/v6/pkg/usermanagement"
	"github.com/workos/workos-go/v6/pkg/workos_errors"
)

// orgSyncer is satisfied by *org.Sync; extracted for unit testing.
type orgSyncer interface {
	SyncMembershipsForUser(ctx context.Context, userID string) error
	GetMembershipRoles(ctx context.Context, userID string) map[string]string
}

// accountGetter is satisfied by *account.AccountStore; extracted for unit testing.
type accountGetter interface {
	GetAccountsForUser(userID string) ([]account.AccountWithRole, error)
	TouchAvatarUpdatedAtByName(name string) (time.Time, error)
}

// orgTokenRefresher is the subset of WorkOSClient used by SwitchOrg.
type orgTokenRefresher interface {
	AuthenticateWithRefreshTokenForOrg(ctx context.Context, refreshToken, organizationID string) (*auth.RefreshResult, error)
}

// memberEmailUpserter is satisfied by *memberemails.Store; extracted for unit testing.
type memberEmailUpserter interface {
	UpsertWorkOS(ctx context.Context, userID, email string, verified bool) error
}

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	log                *logger.Logger
	cfg                *config.Config
	workos             *auth.WorkOSClient
	orgRefresher       orgTokenRefresher
	sessionManager     *auth.SessionManager
	jwtValidator       *auth.JWTValidator
	allowedOrigins     map[string]bool
	accountStore       accountGetter
	membershipResolver auth.MembershipIDResolver
	orgSync            orgSyncer
	avatarStore        *avatarpkg.Store
	memberEmails       memberEmailUpserter
}

// SetOrgSync sets the org sync service on the auth handler.
// Called after construction since org.Sync depends on the account store
// which is also a dependency of the auth handler.
func (h *AuthHandler) SetOrgSync(sync orgSyncer) {
	h.orgSync = sync
}

// SetAvatarStore sets the avatar store on the auth handler for profile picture ingestion at signup.
func (h *AuthHandler) SetAvatarStore(store *avatarpkg.Store) {
	h.avatarStore = store
}

// SetMemberEmails sets the member-email mirror store, used to capture the
// authenticated user's WorkOS email on every login for dev-tool attribution.
func (h *AuthHandler) SetMemberEmails(store memberEmailUpserter) {
	h.memberEmails = store
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
		log:                log,
		cfg:                cfg,
		workos:             workos,
		orgRefresher:       workos,
		sessionManager:     sessionManager,
		jwtValidator:       jwtValidator,
		allowedOrigins:     allowedOrigins,
		accountStore:       accountStore,
		membershipResolver: accountStore,
	}
}

// Login initiates the authentication flow by redirecting to WorkOS AuthKit
func (h *AuthHandler) Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate a random state for CSRF protection
		state, err := generateRandomState()
		if err != nil {
			h.log.Error("auth: generate state failed", "error", err)
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
			900, // 15 minutes
			"/",
			h.cfg.Auth.CookieDomain,
			h.cfg.Auth.CookieSecure,
			true, // httpOnly
		)

		// Store the redirect URL for post-auth deep linking (if allowed).
		// Priority: explicit redirect query param > Origin header > Referer header.
		if redirect := c.Query("redirect"); redirect != "" {
			// Resolve relative paths against the configured frontend URL
			if strings.HasPrefix(redirect, "/") {
				redirect = h.cfg.Auth.FrontendURL + redirect
			}
			if u, err := url.Parse(redirect); err == nil {
				redirectOrigin := u.Scheme + "://" + u.Host
				if h.isAllowedOrigin(redirectOrigin) {
					c.SetCookie(
						"auth_redirect",
						redirect,
						900, // 15 minutes
						"/",
						h.cfg.Auth.CookieDomain,
						h.cfg.Auth.CookieSecure,
						true, // httpOnly
					)
				}
			}
		} else {
			// Fall back to Origin/Referer headers
			origin := c.Request.Header.Get("Origin")
			if origin == "" {
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
					900, // 15 minutes
					"/",
					h.cfg.Auth.CookieDomain,
					h.cfg.Auth.CookieSecure,
					true, // httpOnly
				)
			}
		}

		// Determine screen hint for WorkOS AuthKit (sign-up vs sign-in)
		var loginOpts auth.AuthorizationURLOpts
		if sh := c.Query("screen_hint"); sh == "sign-up" {
			loginOpts.ScreenHint = usermanagement.SignUp
		}
		if token := c.Query("invitation_token"); token != "" {
			loginOpts.InvitationToken = token
		}

		// Get the authorization URL
		authURL, err := h.workos.GetAuthorizationURL(state, loginOpts)
		if err != nil {
			h.log.Error("auth: get authorization URL failed", "error", err)
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
			h.log.Warn("auth: authentication error from WorkOS",
				"error", errCode,
				"description", errDesc,
			)
			c.Redirect(http.StatusFound, buildErrorURL(errCode, errDesc))
			return
		}

		// Get the authorization code
		code := c.Query("code")
		if code == "" {
			h.log.Warn("auth: missing authorization code in callback")
			c.Redirect(http.StatusFound, buildErrorURL("invalid_request", "Missing authorization code"))
			return
		}

		// Validate state parameter (CSRF protection)
		state := c.Query("state")
		storedState, err := c.Cookie("auth_state")
		if err != nil || state != storedState {
			h.log.Warn("auth: state mismatch in callback", "received", state, "expected", storedState)
			c.Redirect(http.StatusFound, buildErrorURL("invalid_state", "State parameter mismatch"))
			return
		}

		// Clear the temporary auth cookies
		h.setSameSiteMode(c)
		c.SetCookie("auth_state", "", -1, "/", h.cfg.Auth.CookieDomain, h.cfg.Auth.CookieSecure, true)
		c.SetCookie("auth_origin", "", -1, "/", h.cfg.Auth.CookieDomain, h.cfg.Auth.CookieSecure, true)
		c.SetCookie("auth_redirect", "", -1, "/", h.cfg.Auth.CookieDomain, h.cfg.Auth.CookieSecure, true)

		// Exchange code for tokens
		result, err := h.workos.AuthenticateWithCode(c.Request.Context(), code)
		if err != nil {
			h.log.Error("auth: authenticate with code failed", "error", err)
			c.Redirect(http.StatusFound, buildErrorURL("authentication_failed", "Failed to authenticate"))
			return
		}

		h.log.Info("auth: user authenticated successfully",
			"user_id", result.User.ID,
			"email", result.User.Email,
			"session_id", result.SessionID,
		)

		// Best-effort: mirror the member's WorkOS email locally for dev-tool
		// attribution. Runs on every login so the mirror stays fresh without the
		// WorkOS events poller. Never fails the login.
		if h.memberEmails != nil {
			if err := h.memberEmails.UpsertWorkOS(c.Request.Context(), result.User.ID, result.User.Email, result.User.EmailVerified); err != nil {
				h.log.Warn("auth: mirror member email on login failed", "error", err, "user_id", result.User.ID)
			}
		}

		// Best-effort: sync org memberships from WorkOS to local store
		if h.orgSync != nil {
			if err := h.orgSync.SyncMembershipsForUser(c.Request.Context(), result.User.ID); err != nil {
				h.log.Warn("auth: sync memberships on login failed", "error", err, "user_id", result.User.ID)
			}
		}

		// Best-effort: ingest OAuth profile picture into our CDN
		if h.avatarStore != nil && result.User.ProfilePictureURL != "" && h.accountStore != nil {
			userAccounts, acctErr := h.accountStore.GetAccountsForUser(result.User.ID)
			if acctErr == nil {
				for _, a := range userAccounts {
					if a.Type == "personal" {
						acctName := a.Name
						profileURL := result.User.ProfilePictureURL
						avatarStore := h.avatarStore
						go func() {
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							if exists, _ := avatarStore.AvatarExists(ctx, acctName); !exists {
								if err := avatarStore.Ingest(ctx, acctName, profileURL); err != nil {
									h.log.Warn("auth: ingest profile picture failed", "error", err, "account", acctName)
								} else if _, err := h.accountStore.TouchAvatarUpdatedAtByName(acctName); err != nil {
									h.log.Warn("auth: stamp account avatar_updated_at after ingest failed", "error", err, "account", acctName)
								}
							}
						}()
						break
					}
				}
			}
		}

		// Create session data with role and permissions from JWT claims
		accessToken, refreshToken, organizationID := result.AccessToken, result.RefreshToken, result.OrganizationID
		if organizationID == "" {
			if scoped, personalOrgID := h.personalOrgTokens(c.Request.Context(), result.User.ID, result.RefreshToken); scoped != nil {
				accessToken, refreshToken, organizationID = scoped.AccessToken, scoped.RefreshToken, personalOrgID
			}
		}

		session := h.sessionManager.CreateSession(
			result.SessionID,
			result.User.ID,
			organizationID,
			accessToken,
			refreshToken,
			int(h.cfg.Auth.SessionMaxAge.Seconds()),
		)

		claims := auth.ExtractTokenClaims(accessToken)
		session.Role = claims.Role
		session.Permissions = claims.Permissions
		h.populateSessionMembership(session, claims)

		sessionData := &auth.SessionData{
			Session: session,
			User:    result.User,
		}

		// Seal and store session in cookie
		sealed, err := h.sessionManager.SealSession(sessionData)
		if err != nil {
			h.log.Error("auth: seal session failed", "error", err)
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

		// Durable, JS-readable marker the marketing nav reads to show returning
		// users "Log in" instead of "Get started".
		h.setReturningCookie(c)

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

			h.log.Info("auth: processing logout request", "session_id", sessionID)

			// Revoke the session at WorkOS (best effort)
			if err := h.workos.RevokeSession(c.Request.Context(), sessionID); err != nil {
				h.log.Warn("auth: revoke session at WorkOS failed", "error", err, "session_id", sessionID)
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
				h.log.Warn("auth: get logout URL failed", "error", err)
				c.Redirect(http.StatusFound, redirectURL)
				return
			}

			h.log.Info("auth: redirecting to WorkOS logout", "logout_url", logoutURL)
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
			h.log.Debug("auth: unseal session failed", "error", err)

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
				h.log.Debug("auth: refresh session failed", "error", err)

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

		// Existing cookies can predate WorkOSMembershipID. Hydrate and reseal them
		// from the embedded JWT claim so this hot read path never queries the DB.
		if sessionData.Session.OrganizationID != "" && sessionData.Session.WorkOSMembershipID == "" {
			claims := auth.ExtractTokenClaims(sessionData.Session.AccessToken)
			if claims.OrganizationMembershipID != "" {
				sessionData.Session.WorkOSMembershipID = claims.OrganizationMembershipID
				sealed, sealErr := h.sessionManager.SealSession(sessionData)
				if sealErr != nil {
					h.log.Warn("auth: reseal session with WorkOS membership failed", "error", sealErr, "user_id", sessionData.Session.UserID, "org_id", sessionData.Session.OrganizationID)
				} else {
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
				}
			}
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
			Accounts:     h.fetchAccounts(c.Request.Context(), sessionData.User.ID),
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
			h.log.Error("auth: refresh session failed", "error", err)
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
			Accounts:     h.fetchAccounts(c.Request.Context(), refreshed.User.ID),
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
		result, err := h.orgRefresher.AuthenticateWithRefreshTokenForOrg(
			c.Request.Context(),
			sessionData.Session.RefreshToken,
			req.OrganizationID,
		)
		if err != nil {
			h.log.Error("auth: switch org failed", "error", err, "org_id", req.OrganizationID)
			var httpErr workos_errors.HTTPError
			if errors.As(err, &httpErr) && httpErr.ErrorCode == "invalid_grant" {
				c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
					Error:       "session_expired",
					Description: "Session has expired, please log in again",
				})
				return
			}
			c.JSON(http.StatusBadRequest, auth.ErrorResponse{
				Error:       "switch_failed",
				Description: "Failed to switch organization",
			})
			return
		}

		newSession := h.sessionManager.CreateSession(
			sessionData.Session.ID,
			sessionData.Session.UserID,
			req.OrganizationID,
			result.AccessToken,
			result.RefreshToken,
			int(h.cfg.Auth.SessionMaxAge.Seconds()),
		)

		claims := auth.ExtractTokenClaims(result.AccessToken)
		newSession.Role = claims.Role
		newSession.Permissions = claims.Permissions
		h.populateSessionMembership(newSession, claims)

		// A membership id the JWT already carries needs no local repair, and a
		// switch is on the critical path of every org change in the UI: the
		// WorkOS membership listing this sync costs is only worth paying when
		// the id is actually missing.
		if newSession.WorkOSMembershipID == "" && h.orgSync != nil {
			if err := h.orgSync.SyncMembershipsForUser(c.Request.Context(), sessionData.Session.UserID); err != nil {
				h.log.Warn("auth: sync memberships on org switch failed", "error", err, "user_id", sessionData.Session.UserID)
			} else {
				h.populateSessionMembership(newSession, claims)
			}
		}

		newSessionData := &auth.SessionData{
			Session: newSession,
			User:    sessionData.User,
		}

		// Seal and update cookie
		sealed, err := h.sessionManager.SealSession(newSessionData)
		if err != nil {
			h.log.Error("auth: seal session after org switch failed", "error", err)
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
			Accounts:     h.fetchAccounts(c.Request.Context(), newSessionData.User.ID),
		})
	}
}

// personalOrgTokens re-mints the session scoped to the user's personal
// organization. AuthKit leaves a session unscoped when it does not pick an
// organization itself, and an unscoped session carries no permission claims, so
// the account's own routes would fall back to a membership check.
func (h *AuthHandler) personalOrgTokens(ctx context.Context, userID, refreshToken string) (*auth.RefreshResult, string) {
	if h.orgRefresher == nil || h.accountStore == nil || refreshToken == "" {
		return nil, ""
	}
	accounts, err := h.accountStore.GetAccountsForUser(userID)
	if err != nil {
		h.log.Warn("auth: resolve personal organization failed", "error", err, "user_id", userID)
		return nil, ""
	}
	organizationID := ""
	for _, a := range accounts {
		if a.Type == "personal" && a.WorkOSOrganizationID != "" {
			organizationID = a.WorkOSOrganizationID
			break
		}
	}
	if organizationID == "" {
		return nil, ""
	}
	scoped, err := h.orgRefresher.AuthenticateWithRefreshTokenForOrg(ctx, refreshToken, organizationID)
	if err != nil {
		h.log.Warn("auth: scope session to personal organization failed",
			"error", err, "user_id", userID, "workos_org_id", organizationID)
		return nil, ""
	}
	return scoped, organizationID
}

// fetchAccounts returns the accounts for a user, always returning a non-nil slice.
// For organization accounts it includes the user's role, fetched from WorkOS.
func (h *AuthHandler) fetchAccounts(ctx context.Context, userID string) []auth.AuthAccountResponse {
	accounts := make([]auth.AuthAccountResponse, 0)

	// Fetch per-org roles from WorkOS so each org card can show the correct badge.
	var orgRoles map[string]string
	if h.orgSync != nil {
		orgRoles = h.orgSync.GetMembershipRoles(ctx, userID)
	}

	if h.accountStore != nil {
		userAccounts, err := h.accountStore.GetAccountsForUser(userID)
		if err != nil {
			h.log.Warn("auth: fetch accounts for user failed", "error", err, "user_id", userID)
		} else {
			for _, a := range userAccounts {
				role := ""
				if orgRoles != nil && a.WorkOSOrganizationID != "" {
					role = orgRoles[a.WorkOSOrganizationID]
				}
				avatarURL := ""
				if h.avatarStore != nil {
					avatarURL = h.avatarStore.AvatarURL(a.Name, a.AvatarUpdatedAt)
				}
				accounts = append(accounts, auth.AuthAccountResponse{
					ID:                   a.ID,
					Name:                 a.Name,
					Type:                 a.Type,
					DisplayName:          a.DisplayName,
					WorkOSOrganizationID: a.WorkOSOrganizationID,
					Role:                 role,
					AvatarURL:            avatarURL,
				})
			}
		}
	}
	return accounts
}

// refreshSession refreshes the session using the refresh token
func (h *AuthHandler) refreshSession(c *gin.Context, sessionData *auth.SessionData) (*auth.SessionData, error) {
	// Ask for the organization the session already records, so its permission
	// claims cannot come back for a different scope than the session reports.
	result, err := h.orgRefresher.AuthenticateWithRefreshTokenForOrg(
		c.Request.Context(),
		sessionData.Session.RefreshToken,
		sessionData.Session.OrganizationID,
	)
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
			h.log.Warn("auth: sync memberships on refresh failed", "error", err, "user_id", sessionData.Session.UserID)
		}
	}

	wg.Wait()
	if userErr != nil {
		h.log.Warn("auth: fetch fresh user on refresh, using cached data failed", "error", userErr)
		freshUser = sessionData.User
	}

	// Update session with new tokens and refreshed claims
	newSession := h.sessionManager.CreateSession(
		sessionData.Session.ID,
		sessionData.Session.UserID,
		sessionData.Session.OrganizationID,
		result.AccessToken,
		result.RefreshToken,
		int(h.cfg.Auth.SessionMaxAge.Seconds()),
	)

	claims := auth.ExtractTokenClaims(result.AccessToken)
	newSession.Role = claims.Role
	newSession.Permissions = claims.Permissions
	h.populateSessionMembership(newSession, claims)

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

	// Keep the returning-user marker fresh on refresh so existing sessions
	// (pre-dating this feature) also get it without re-logging in.
	h.setReturningCookie(c)

	return newSessionData, nil
}

// setReturningCookie writes a durable, JS-readable "returning user" marker that
// the marketing nav reads to show "Log in" instead of "Get started". It uses the
// same domain and Secure setting as the session cookie so it is shared across
// *.astropods.com, but is NOT HttpOnly (the marketing site reads it in JS) and is
// deliberately never cleared on logout — the semantic is "has ever logged in".
func (h *AuthHandler) setReturningCookie(c *gin.Context) {
	const oneYearSeconds = 365 * 24 * 60 * 60
	c.SetCookie("astro_returning", "1", oneYearSeconds, "/", h.cfg.Auth.CookieDomain, h.cfg.Auth.CookieSecure, false)
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

// getRedirectURL determines the appropriate redirect URL after authentication.
// Priority: auth_redirect cookie (full URL for deep linking) > auth_origin cookie > configured frontend URL.
func (h *AuthHandler) getRedirectURL(c *gin.Context) string {
	// Check for full redirect URL (deep linking from /login?redirect=...)
	if redirect, err := c.Cookie("auth_redirect"); err == nil && redirect != "" {
		if u, err := url.Parse(redirect); err == nil {
			origin := u.Scheme + "://" + u.Host
			if h.isAllowedOrigin(origin) {
				return redirect
			}
		}
	}
	// Fall back to origin-only cookie
	if origin, err := c.Cookie("auth_origin"); err == nil && origin != "" && h.isAllowedOrigin(origin) {
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

func (h *AuthHandler) populateSessionMembership(session *auth.Session, claims auth.TokenClaims) {
	membershipID, err := auth.ResolveWorkOSMembershipID(
		h.membershipResolver,
		session.UserID,
		session.OrganizationID,
		claims.OrganizationMembershipID,
	)
	if err != nil {
		if errors.Is(err, auth.ErrWorkOSMembershipIDNotFound) || errors.Is(err, account.ErrAccountNotFound) {
			h.log.Debug("auth: resolve WorkOS membership for session failed", "error", err, "user_id", session.UserID, "org_id", session.OrganizationID)
		} else {
			h.log.Warn("auth: resolve WorkOS membership for session failed", "error", err, "user_id", session.UserID, "org_id", session.OrganizationID)
		}
		return
	}
	session.WorkOSMembershipID = membershipID
}
