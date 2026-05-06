package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	slackclient "github.com/astropods/astro/apps/astro-server/internal/slack"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// SlackProvider is the WorkOS Pipes provider slug for Slack OAuth.
// Centralized so the link, callback, and disconnect handlers don't drift.
const SlackProvider = "slack"

// SlackHandlerConfig carries the URL-shape values the handlers need.
// Mirrors GitHubHandlerConfig — same conventions, same naming.
type SlackHandlerConfig struct {
	// WebhookBaseURL is the public API base URL (e.g. https://astropods.com).
	WebhookBaseURL string
	// FrontendURL is the web app's base URL for browser redirects after the OAuth round-trip.
	FrontendURL string
}

// SlackConnectResponse mirrors GitHubConnectResponse — a single redirect_url
// the frontend navigates to in order to start the WorkOS Pipes OAuth flow.
type SlackConnectResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// SlackConnectRequest is the body for the connect endpoint. redirect_to
// controls where the browser lands after the callback completes.
type SlackConnectRequest struct {
	RedirectTo string `json:"redirect_to"`
}

// SlackWorkspace is one linked Slack workspace surfaced in the status
// response. A single WorkOS user can hold multiple, one per team_id.
type SlackWorkspace struct {
	TeamID        string `json:"team_id"`
	SlackUserID   string `json:"slack_user_id"`
	Team          string `json:"team,omitempty"`
	TeamDomain    string `json:"team_domain,omitempty"`
	SlackUsername string `json:"slack_username,omitempty"`
}

// SlackStatusResponse lists every Slack workspace the current user has
// linked. Empty list = not connected; the frontend renders an "Add
// workspace" affordance unconditionally so the user can link more.
type SlackStatusResponse struct {
	Workspaces []SlackWorkspace `json:"workspaces"`
}

// SlackAccountConnect handles POST /api/v1/accounts/:account/slack/connect.
// Always returns a fresh Pipes authorization URL — there is no
// already-connected short-circuit because each click is intended to add a
// new workspace, not reuse an existing token. Pipes scopes by (user, org)
// so any prior connection is revoked by the callback after the mapping is
// captured, freeing the slot for the next link.
func SlackAccountConnect(log *logger.Logger, pipesClient *pipes.Client, cfg SlackHandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		var req SlackConnectRequest
		_ = c.ShouldBindJSON(&req) // body is optional — empty redirect_to defaults below

		callbackURL := fmt.Sprintf("%s/api/v1/accounts/%s/slack/callback?redirect_to=%s",
			cfg.WebhookBaseURL, acct.Name, url.QueryEscape(req.RedirectTo))

		authURL, err := pipesClient.GetAuthorizationURL(c.Request.Context(), pipes.GetAuthorizationURLInput{
			Provider:       SlackProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
			ReturnTo:       callbackURL,
		})
		if err != nil {
			log.Error("slack: pipes get authorization URL", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate Slack authorization URL"})
			return
		}

		c.JSON(http.StatusOK, SlackConnectResponse{RedirectURL: authURL})
	}
}

// SlackAccountCallback handles GET /api/v1/accounts/:account/slack/callback.
// WorkOS redirects here after the Pipes OAuth round-trip. We fetch the
// freshly-issued Slack token, call auth.test to resolve (team_id,
// slack_user_id), upsert the mapping, then send the browser back to the
// frontend with ?slack_connected=true.
//
// Errors during this flow surface as ?slack_error=<reason> so the settings
// UI can render a useful message without a separate API call.
func SlackAccountCallback(log *logger.Logger, pipesClient *pipes.Client, store *slackidentity.Store, cfg SlackHandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.Redirect(http.StatusFound, cfg.FrontendURL+"?slack_error=not_authenticated")
			return
		}

		// Validate redirect_to to prevent open-redirect: must be a relative path.
		redirectTo := c.Query("redirect_to")
		if redirectTo == "" || !strings.HasPrefix(redirectTo, "/") || strings.HasPrefix(redirectTo, "//") {
			redirectTo = "/settings/account"
		}

		token, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       SlackProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			log.Error("slack: token unavailable after callback", "error", err, "user", session.UserID)
			c.Redirect(http.StatusFound, cfg.FrontendURL+redirectTo+"?slack_error=token_unavailable")
			return
		}

		// auth.test resolves the slack identity behind the OAuth grant.
		// This is the only call we make against the user's Slack token in
		// this flow — it just needs identity:read effectively.
		identity, err := slackclient.New(token.AccessToken).AuthTest(c.Request.Context())
		if err != nil {
			log.Error("slack: auth.test failed", "error", err, "user", session.UserID)
			c.Redirect(http.StatusFound, cfg.FrontendURL+redirectTo+"?slack_error=identity_failed")
			return
		}

		if upErr := store.Upsert(slackidentity.Mapping{
			TeamID:         identity.TeamID,
			SlackUserID:    identity.UserID,
			WorkOSUserID:   session.UserID,
			OrganizationID: session.OrganizationID,
			Source:         slackidentity.SourcePipes,
			TeamName:       identity.Team,
			TeamDomain:     identity.TeamDomain,
			SlackUsername:  identity.User,
		}); upErr != nil {
			log.Error("slack: persist identity mapping", "error", upErr, "user", session.UserID, "team_id", identity.TeamID)
			c.Redirect(http.StatusFound, cfg.FrontendURL+redirectTo+"?slack_error=persist_failed")
			return
		}

		// Free the Pipes (user, org) slot so the next "Connect" click can
		// link a different workspace. We don't need the slack token after
		// this — auth.test was its only consumer, and the mapping is
		// already persisted. Best-effort: a failure here doesn't affect
		// authorization, just means the slot stays occupied until the
		// user disconnects explicitly.
		if delErr := pipesClient.DeleteConnection(c.Request.Context(), pipes.DeleteConnectionInput{
			Provider:       SlackProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		}); delErr != nil {
			log.Warn("slack: pipes delete connection after link", "error", delErr, "user", session.UserID, "team_id", identity.TeamID)
		}

		params := url.Values{}
		params.Set("slack_connected", "true")
		if identity.User != "" {
			params.Set("slack_user", identity.User)
		}
		if identity.Team != "" {
			params.Set("slack_team", identity.Team)
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("%s%s?%s", cfg.FrontendURL, redirectTo, params.Encode()))
	}
}

// SlackAccountStatus handles GET /api/v1/accounts/:account/slack.
// Returns every active Slack workspace mapping for the current user.
// Source of truth is slack_identity_mappings — we don't ask Pipes
// because the link flow revokes the Pipes connection after persisting
// (so Pipes never has more than transient state for slack).
func SlackAccountStatus(log *logger.Logger, store *slackidentity.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		mappings, err := store.ListByWorkOSUser(session.UserID)
		if err != nil {
			log.Error("slack: list mappings for status", "error", err, "user", session.UserID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load Slack status"})
			return
		}

		workspaces := make([]SlackWorkspace, 0, len(mappings))
		for _, m := range mappings {
			workspaces = append(workspaces, SlackWorkspace{
				TeamID:        m.TeamID,
				SlackUserID:   m.SlackUserID,
				Team:          m.TeamName,
				TeamDomain:    m.TeamDomain,
				SlackUsername: m.SlackUsername,
			})
		}
		c.JSON(http.StatusOK, SlackStatusResponse{Workspaces: workspaces})
	}
}

// SlackAccountDisconnect handles DELETE /api/v1/accounts/:account/slack.
//
// Per-workspace disconnect: pass ?team_id=Txxx to revoke a single mapping.
// Without team_id, every workspace mapping for the user is revoked.
//
// The Pipes connection is normally already absent (the link callback
// revokes it after persisting), but we make a best-effort DeleteConnection
// in the all-mappings case as a defensive cleanup — if a callback half-
// completed and left a Pipes connection orphaned, this clears it.
func SlackAccountDisconnect(log *logger.Logger, pipesClient *pipes.Client, store *slackidentity.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		if teamID := c.Query("team_id"); teamID != "" {
			if _, err := store.RevokeOne(session.UserID, teamID); err != nil {
				log.Error("slack: revoke one mapping", "error", err, "user", session.UserID, "team_id", teamID)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke Slack workspace"})
				return
			}
			c.JSON(http.StatusOK, gin.H{"team_id": teamID, "revoked": true})
			return
		}

		if _, err := store.Revoke(session.UserID); err != nil {
			log.Error("slack: revoke mappings", "error", err, "user", session.UserID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke Slack mappings"})
			return
		}

		// Defensive: if a previous link callback failed before the Pipes-
		// side revoke, the connection might still be parked here. Best-
		// effort cleanup; benign 4xx from WorkOS is expected when the slot
		// is already empty (the common case).
		if err := pipesClient.DeleteConnection(c.Request.Context(), pipes.DeleteConnectionInput{
			Provider:       SlackProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		}); err != nil {
			log.Warn("slack: pipes delete connection on full disconnect (best-effort)", "error", err, "user", session.UserID)
		}

		c.JSON(http.StatusOK, gin.H{"connected": false})
	}
}
