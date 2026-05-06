package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	slackclient "github.com/astropods/astro/apps/astro-server/internal/slack"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// SlackHandlerConfig carries the URL-shape values + slack app credentials
// the handlers need. ClientID/ClientSecret come from the slack app at
// api.slack.com/apps → Basic Information; the rest mirror the GitHub
// pattern.
type SlackHandlerConfig struct {
	ClientID     string
	ClientSecret string
	// WebhookBaseURL is the public API base URL the slack OAuth
	// `redirect_uri` is built from. Must match a Redirect URL listed on
	// the slack app's OAuth & Permissions page.
	WebhookBaseURL string
	// FrontendURL is the web app's base URL for the post-callback
	// browser redirect (e.g. back to /settings/account).
	FrontendURL string
}

// SlackConnectResponse mirrors the GitHub one — a single redirect_url
// the frontend navigates to, kicking off the slack OAuth round-trip.
type SlackConnectResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// SlackConnectRequest is the body for the connect endpoint. redirect_to
// controls where the browser lands after the callback completes.
type SlackConnectRequest struct {
	RedirectTo string `json:"redirect_to"`
}

// SlackWorkspace is one linked slack workspace surfaced in the status
// response. A single WorkOS user can hold multiple, one per team_id.
// Icon is empty when the workspace uses slack's default icon — the
// frontend falls back to a generic Slack svg in that case.
type SlackWorkspace struct {
	TeamID        string `json:"team_id"`
	SlackUserID   string `json:"slack_user_id"`
	Team          string `json:"team,omitempty"`
	TeamDomain    string `json:"team_domain,omitempty"`
	Icon          string `json:"icon,omitempty"`
	SlackUsername string `json:"slack_username,omitempty"`
}

// SlackStatusResponse lists every slack workspace the current user has
// linked. Empty list = not connected; the frontend renders an "Add
// workspace" affordance unconditionally so the user can link more.
type SlackStatusResponse struct {
	Workspaces []SlackWorkspace `json:"workspaces"`
}

// CSRF state cookie. Random nonce written on /slack/connect, verified on
// /slack/callback, then cleared. SameSite=Lax so it survives the
// cross-domain redirect from slack.com back to our domain.
const (
	slackOAuthStateCookie = "astro_slack_oauth_state"
	slackOAuthStateMaxAge = 600 // 10 minutes
)

// slackUserScopes is the scope set we ask slack for. user_scope (not
// scope) ensures we get back a USER token whose authed_user.id is the
// linker's actual slack identity — bot scopes would give us a bot token
// whose auth.test resolves to the bot, breaking the mapping.
//
//   - users:read — minimum to identify the human via authed_user.id
//   - team:read  — needed for team.info → workspace icon URL the
//     settings UI renders. team.info also returns name+domain which
//     are already in the oauth.v2.access response, so the icon is the
//     real reason this scope is here.
var slackUserScopes = []string{"users:read", "team:read"}

// SlackAccountConnect handles POST /api/v1/accounts/:account/slack/connect.
// Returns the slack.com OAuth URL the frontend redirects to. A CSRF
// state cookie is set on the response; the callback verifies it.
func SlackAccountConnect(log *logger.Logger, cfg SlackHandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg.ClientID == "" || cfg.ClientSecret == "" {
			log.Error("slack: client credentials not configured (SLACK_CLIENT_ID / SLACK_CLIENT_SECRET)")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "slack integration not configured"})
			return
		}
		if _, ok := middleware.GetSession(c); !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		var req SlackConnectRequest
		_ = c.ShouldBindJSON(&req)

		state, err := randomState()
		if err != nil {
			log.Error("slack: random state", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
			return
		}

		// HttpOnly + SameSite=Lax: cookie survives the slack.com → our
		// domain redirect (Lax permits top-level GETs across origins) and
		// is invisible to JS. Secure tracks the configured frontend
		// scheme so dev (http://localhost) still works.
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     slackOAuthStateCookie,
			Value:    state,
			Path:     "/",
			MaxAge:   slackOAuthStateMaxAge,
			HttpOnly: true,
			Secure:   strings.HasPrefix(cfg.FrontendURL, "https://"),
			SameSite: http.SameSiteLaxMode,
		})

		redirectURI := fmt.Sprintf("%s/api/v1/accounts/%s/slack/callback?redirect_to=%s",
			cfg.WebhookBaseURL, acct.Name, url.QueryEscape(req.RedirectTo))

		oauth := slackclient.NewOAuthClient(cfg.ClientID, cfg.ClientSecret)
		c.JSON(http.StatusOK, SlackConnectResponse{
			RedirectURL: oauth.BuildAuthorizeURL(redirectURI, state, slackUserScopes),
		})
	}
}

// SlackAccountCallback handles GET /api/v1/accounts/:account/slack/callback.
// Slack redirects here with `code` + `state` after the user authorizes.
// Verifies the state cookie, exchanges the code for an OAuth response,
// reads team_id and authed_user.id directly (no auth.test follow-up),
// upserts the mapping, and redirects back to the frontend.
//
// On any failure the user is sent back to the frontend with
// `?slack_error=<reason>` so the settings panel can render a useful
// message without a separate API round-trip.
func SlackAccountCallback(log *logger.Logger, store *slackidentity.Store, cfg SlackHandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.Redirect(http.StatusFound, cfg.FrontendURL+"?slack_error=not_authenticated")
			return
		}
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.Redirect(http.StatusFound, cfg.FrontendURL+"?slack_error=account_unresolved")
			return
		}

		// Validate redirect_to: must be a relative path (open-redirect guard).
		redirectTo := c.Query("redirect_to")
		if redirectTo == "" || !strings.HasPrefix(redirectTo, "/") || strings.HasPrefix(redirectTo, "//") {
			redirectTo = "/settings/account"
		}

		// Slack puts user-cancelled / scope-rejected errors directly on
		// the redirect (?error=access_denied). Surface them cleanly.
		if slackErr := c.Query("error"); slackErr != "" {
			clearStateCookie(c, cfg)
			c.Redirect(http.StatusFound, cfg.FrontendURL+redirectTo+"?slack_error="+url.QueryEscape(slackErr))
			return
		}

		// CSRF: the cookie value must match the state= query param. Use a
		// constant-time compare to avoid timing leaks (state is random
		// nonce so the leak is mostly theoretical, but free here).
		gotState := c.Query("state")
		stateCookie, err := c.Request.Cookie(slackOAuthStateCookie)
		if err != nil || stateCookie.Value == "" || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(gotState)) != 1 {
			clearStateCookie(c, cfg)
			log.Warn("slack: state mismatch on callback", "user", session.UserID)
			c.Redirect(http.StatusFound, cfg.FrontendURL+redirectTo+"?slack_error=state_mismatch")
			return
		}
		clearStateCookie(c, cfg)

		code := c.Query("code")
		if code == "" {
			c.Redirect(http.StatusFound, cfg.FrontendURL+redirectTo+"?slack_error=missing_code")
			return
		}

		// Same redirect_uri value as the one sent during BuildAuthorizeURL —
		// slack rejects on mismatch.
		redirectURI := fmt.Sprintf("%s/api/v1/accounts/%s/slack/callback?redirect_to=%s",
			cfg.WebhookBaseURL, acct.Name, url.QueryEscape(c.Query("redirect_to")))

		oauth := slackclient.NewOAuthClient(cfg.ClientID, cfg.ClientSecret)
		resp, err := oauth.ExchangeCode(c.Request.Context(), code, redirectURI)
		if err != nil {
			log.Error("slack: oauth exchange failed", "error", err, "user", session.UserID)
			c.Redirect(http.StatusFound, cfg.FrontendURL+redirectTo+"?slack_error=exchange_failed")
			return
		}

		// Best-effort enrichment: team.info for the workspace's display
		// fields + icon, users.info for the linker's slack username. Both
		// are non-fatal — if either fails (missing scope, slack 5xx) we
		// persist what we have and the settings UI degrades gracefully.
		//
		// team.info is preferred over the OAuth response for name/domain
		// because oauth.v2.access doesn't always include team.domain;
		// team.info reliably does.
		teamName := resp.Team.Name
		teamDomain := resp.Team.Domain
		var iconURL, slackUsername string
		if resp.AuthedUser.AccessToken != "" {
			info, infoErr := oauth.TeamInfo(c.Request.Context(), resp.AuthedUser.AccessToken)
			if infoErr != nil {
				log.Warn("slack: team.info failed; persisting without icon",
					"error", infoErr, "user", session.UserID, "team_id", resp.Team.ID)
			} else {
				if info.Name != "" {
					teamName = info.Name
				}
				if info.Domain != "" {
					teamDomain = info.Domain
				}
				iconURL = info.IconURL
			}

			user, userErr := oauth.UserInfo(c.Request.Context(), resp.AuthedUser.AccessToken, resp.AuthedUser.ID)
			if userErr != nil {
				log.Warn("slack: users.info failed; persisting without username",
					"error", userErr, "user", session.UserID, "slack_user_id", resp.AuthedUser.ID)
			} else {
				// Prefer the user-set display_name; fall back to handle.
				slackUsername = user.DisplayName
				if slackUsername == "" {
					slackUsername = user.Name
				}
			}
		}

		if upErr := store.Upsert(slackidentity.Mapping{
			TeamID:         resp.Team.ID,
			SlackUserID:    resp.AuthedUser.ID,
			WorkOSUserID:   session.UserID,
			OrganizationID: session.OrganizationID,
			Source:         slackidentity.SourceOAuth,
			TeamName:       teamName,
			TeamDomain:     teamDomain,
			TeamIconURL:    iconURL,
			SlackUsername:  slackUsername,
		}); upErr != nil {
			log.Error("slack: persist identity mapping", "error", upErr, "user", session.UserID, "team_id", resp.Team.ID)
			c.Redirect(http.StatusFound, cfg.FrontendURL+redirectTo+"?slack_error=persist_failed")
			return
		}

		params := url.Values{}
		params.Set("slack_connected", "true")
		if resp.Team.Name != "" {
			params.Set("slack_team", resp.Team.Name)
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("%s%s?%s", cfg.FrontendURL, redirectTo, params.Encode()))
	}
}

// SlackAccountStatus handles GET /api/v1/accounts/:account/slack.
// Returns every active slack workspace mapping for the current user. The
// store is the source of truth — we don't talk to slack here.
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
				Icon:          m.TeamIconURL,
				SlackUsername: m.SlackUsername,
			})
		}
		c.JSON(http.StatusOK, SlackStatusResponse{Workspaces: workspaces})
	}
}

// SlackAccountDisconnect handles DELETE /api/v1/accounts/:account/slack.
//
// Per-workspace disconnect: pass ?team_id=Txxx to revoke a single
// mapping. Without team_id, every workspace mapping for the user is
// revoked.
//
// We don't touch slack.com — the user token we issued is short-lived and
// not stored anywhere. Authorization stops matching as soon as the
// mapping row is soft-deleted.
func SlackAccountDisconnect(log *logger.Logger, store *slackidentity.Store) gin.HandlerFunc {
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
		c.JSON(http.StatusOK, gin.H{"connected": false})
	}
}

// randomState returns 32 bytes of cryptographic randomness, hex-encoded.
// 64 chars; collision-resistant for CSRF state.
func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// clearStateCookie removes the CSRF state cookie. Called on every code
// path the callback can take so a stale cookie can't be reused.
func clearStateCookie(c *gin.Context, cfg SlackHandlerConfig) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     slackOAuthStateCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(cfg.FrontendURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}
