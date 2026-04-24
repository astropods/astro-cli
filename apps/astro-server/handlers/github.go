package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	githubclient "github.com/astropods/astro/apps/astro-server/internal/github"
	"github.com/astropods/astro/apps/astro-server/internal/githubbuild"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// githubBuildQueue is the subset of riverqueue.Queue used by GitHub handlers.
type githubBuildQueue interface {
	EnqueueGitHubBuild(ctx context.Context, args riverqueue.GitHubBuildArgs) error
	CancelGitHubBuildsForConnection(ctx context.Context, connectionID string)
}

// GitHubConnectResponse is returned when an OAuth redirect is needed.
type GitHubConnectResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// GitHubStatusResponse describes the current connection state for an agent.
type GitHubStatusResponse struct {
	Connected    bool                     `json:"connected"`
	RepoFullName string                   `json:"repo_full_name,omitempty"`
	Branch       string                   `json:"branch,omitempty"`
	Builds       []githubconnection.Build `json:"builds"`
	DraftCard    *spec.ParsedAgentCard    `json:"draft_card,omitempty"`
}

const agentMDCachePrefix = "astro:github:agent-md:"
const agentMDCacheTTL = 60 * time.Second

// GitHubLinkRequest is the body for linking a repo after OAuth.
type GitHubLinkRequest struct {
	RepoFullName string `json:"repo_full_name" binding:"required"`
	Branch       string `json:"branch"`
}

// GitHubAccountConnectRequest is the body for the account-level connect endpoint.
type GitHubAccountConnectRequest struct {
	RedirectTo string `json:"redirect_to"`
}

// GitHubAccountConnect handles POST /api/v1/accounts/:account/github/connect.
// Blueprint-agnostic OAuth initiation — the redirect_to body field controls where the
// browser lands after OAuth completes (e.g. "/new/custom").
func GitHubAccountConnect(log *logger.Logger, pipesClient *pipes.Client, cfg GitHubHandlerConfig) gin.HandlerFunc {
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

		var req GitHubAccountConnectRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		// Check if the user already has a GitHub token via Pipes.
		existingToken, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err == nil {
			// Fetch the GitHub login to include in the response so the frontend
			// can display "{login} connected" without a separate API call.
			login := ""
			if gh := githubclient.New(existingToken.AccessToken); gh != nil {
				if l, loginErr := gh.GetLogin(c.Request.Context()); loginErr == nil {
					login = l
				}
			}
			c.JSON(http.StatusOK, gin.H{"connected": true, "github_login": login})
			return
		}

		if !errors.Is(err, pipes.ErrNeedsReauthorization) && !errors.Is(err, pipes.ErrNotInstalled) {
			log.Error("pipes: get access token", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check GitHub connection"})
			return
		}

		// Encode redirect_to into the callback URL so the callback knows where to send the browser.
		callbackURL := fmt.Sprintf("%s/api/v1/accounts/%s/github/callback?redirect_to=%s",
			cfg.WebhookBaseURL, acct.Name, url.QueryEscape(req.RedirectTo))

		authURL, err := pipesClient.GetAuthorizationURL(c.Request.Context(), pipes.GetAuthorizationURLInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
			ReturnTo:       callbackURL,
		})
		if err != nil {
			log.Error("pipes: get authorization URL", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate GitHub authorization URL"})
			return
		}

		c.JSON(http.StatusOK, GitHubConnectResponse{RedirectURL: authURL})
	}
}

// GitHubAccountCallback handles GET /api/v1/accounts/:account/github/callback.
// WorkOS redirects here after GitHub OAuth. Reads redirect_to from the query string
// and sends the browser back to the frontend wizard with ?github_connected=true.
func GitHubAccountCallback(log *logger.Logger, pipesClient *pipes.Client, cfg GitHubHandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.Redirect(http.StatusFound, cfg.FrontendURL+"?github_error=not_authenticated")
			return
		}

		redirectTo := c.Query("redirect_to")
		// Validate to prevent open redirect: must be a relative path.
		if redirectTo == "" || !strings.HasPrefix(redirectTo, "/") || strings.HasPrefix(redirectTo, "//") {
			redirectTo = "/new/custom"
		}

		token, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			log.Error("pipes: token unavailable after account callback", "error", err, "user", session.UserID)
			c.Redirect(http.StatusFound, cfg.FrontendURL+redirectTo+"?github_error=token_unavailable")
			return
		}

		// Fetch the GitHub login to pass to the frontend so it can display "{login} connected"
		// without a separate API call. Best-effort — omit if it fails.
		loginParam := ""
		if gh := githubclient.New(token.AccessToken); gh != nil {
			if login, loginErr := gh.GetLogin(c.Request.Context()); loginErr == nil && login != "" {
				loginParam = "&github_login=" + url.QueryEscape(login)
			}
		}

		c.Redirect(http.StatusFound, fmt.Sprintf("%s%s?github_connected=true%s", cfg.FrontendURL, redirectTo, loginParam))
	}
}

// GitHubAccountListRepos handles GET /api/v1/accounts/:account/github/repos.
// Uses the GitHub Search API for all cases:
//   - no ?q  → returns the user's repos sorted by push date
//   - ?q=foo → returns repos matching "foo" in name, scoped to the user
//
// Pass ?login=<login> to skip an extra GET /user round-trip.
func GitHubAccountListRepos(log *logger.Logger, pipesClient *pipes.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		token, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			if errors.Is(err, pipes.ErrNotInstalled) || errors.Is(err, pipes.ErrNeedsReauthorization) {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "github_not_connected"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get GitHub token"})
			return
		}

		gh := githubclient.New(token.AccessToken)
		repos, err := gh.SearchRepos(c.Request.Context(), strings.TrimSpace(c.Query("q")), strings.TrimSpace(c.Query("login")))
		if err != nil {
			log.Error("github: search account repos", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search GitHub repos"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"repos": repos, "has_more": false})
	}
}

// GitHubAccountScan handles GET /api/v1/accounts/:account/github/scan.
// Returns whether astropods.yml exists in the given repo at the given branch.
func GitHubAccountScan(log *logger.Logger, pipesClient *pipes.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		repo := c.Query("repo")
		branch := c.Query("branch")
		if repo == "" || branch == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "repo and branch are required"})
			return
		}

		token, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "github_not_connected"})
			return
		}

		content, err := githubbuild.FetchFileContent(c.Request.Context(), token.AccessToken, repo, branch, "astropods.yml")
		if err != nil {
			log.Warn("github scan: fetch astropods.yml", "error", err, "repo", repo)
			c.JSON(http.StatusOK, gin.H{"found": false})
			return
		}

		c.JSON(http.StatusOK, gin.H{"found": content != ""})
	}
}

// GitHubLink handles POST /api/v1/agents/:account/:name/github/link.
// GitHubLink handles POST /api/v1/agents/:account/:name/github/link.
// Installs a webhook on the selected repo and saves the connection.
func GitHubLink(log *logger.Logger, pipesClient *pipes.Client, ghStore *githubconnection.Store, cfg GitHubHandlerConfig) gin.HandlerFunc {
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

		agentName := c.Param("name")

		var req GitHubLinkRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		if req.Branch == "" {
			req.Branch = "main"
		}

		token, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			if errors.Is(err, pipes.ErrNotInstalled) || errors.Is(err, pipes.ErrNeedsReauthorization) {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "github_not_connected"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get GitHub token"})
			return
		}
		req.RepoFullName = strings.Trim(req.RepoFullName, "/")
		if err := validateRepoFullName(req.RepoFullName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		newBase := githubconnection.RepoBase(req.RepoFullName)

		// Remove existing webhook if re-linking to a different base repo,
		// but only if no other connections still reference that base repo.
		existing, err := ghStore.Get(c.Request.Context(), acct.ID, agentName)
		if err == nil && existing.WebhookID != 0 {
			existingBase := githubconnection.RepoBase(existing.RepoFullName)
			if existingBase != newBase {
				// count <= 1 (not == 0) because the existing connection is still in the DB at this point
				// (Upsert hasn't run yet); count == 1 means only this connection references the old base.
				if count, countErr := ghStore.CountByRepoBase(c.Request.Context(), existingBase); countErr == nil && count <= 1 {
					oldGH := githubclient.New(token.AccessToken)
					if delErr := oldGH.DeleteWebhook(c.Request.Context(), existingBase, existing.WebhookID); delErr != nil {
						log.Warn("github: failed to remove old webhook", "error", delErr, "repo", existing.RepoFullName)
					}
				}
			}
		}

		// Reject if the repo+subpath is already connected to a different agent in this account.
		if conflict, err := ghStore.GetByRepoForAccount(c.Request.Context(), acct.ID, req.RepoFullName); err == nil && conflict.AgentName != agentName {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("repo %q is already connected to agent %q", req.RepoFullName, conflict.AgentName)})
			return
		}

		conn := &githubconnection.Connection{
			AccountID:            acct.ID,
			AccountName:          acct.Name,
			AgentName:            agentName,
			WorkOSUserID:         session.UserID,
			WorkOSOrganizationID: session.OrganizationID,
			RepoFullName:         req.RepoFullName,
			Branch:               req.Branch,
			WebhookID:            0,
			WebhookSecret:        "",
		}

		// Webhook dedup: reuse existing webhook if another connection in the same account already
		// targets the same base repo. The AccountID guard prevents account B from inheriting
		// account A's webhook_id and webhook_secret via the account-blind GetByRepoBase lookup.
		// Race: two concurrent link requests for the same base repo can both get ErrNoRows here and
		// both create separate webhooks; the upsert means one silently wins, leaving a duplicate webhook
		// on GitHub. Fixing this properly requires a DB advisory lock or a unique constraint on
		// (webhook_id) per base repo with a retry loop. Accepted as low-probability for now.
		if sharedConn, err := ghStore.GetByRepoBase(c.Request.Context(), newBase); err == nil && sharedConn.WebhookID != 0 && sharedConn.AccountID == acct.ID {
			conn.WebhookID = sharedConn.WebhookID
			conn.WebhookSecret = sharedConn.WebhookSecret
			if err := ghStore.Upsert(c.Request.Context(), conn); err != nil {
				log.Error("githubconnection: upsert (dedup)", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save connection"})
				return
			}
			log.Info("GitHub repo linked (webhook reused)", "account", acct.Name, "agent", agentName, "repo", req.RepoFullName)
			c.JSON(http.StatusCreated, gin.H{"repo_full_name": req.RepoFullName, "branch": req.Branch})
			return
		}

		// Save connection first so the blueprint detail page shows the GitHub
		// setup path and manual rebuilds work even if webhook installation fails.
		if err := ghStore.Upsert(c.Request.Context(), conn); err != nil {
			log.Error("githubconnection: upsert", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save connection"})
			return
		}

		// Install webhook on the base repo.
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err == nil {
			webhookSecret := hex.EncodeToString(secretBytes)
			gh := githubclient.New(token.AccessToken)
			webhookPayloadURL := fmt.Sprintf("%s/webhooks/github", cfg.WebhookBaseURL)
			webhookID, err := gh.CreateWebhook(c.Request.Context(), githubclient.CreateWebhookInput{
				RepoFullName: newBase,
				PayloadURL:   webhookPayloadURL,
				Secret:       webhookSecret,
			})
			if err != nil {
				log.Warn("github: create webhook failed, pushes won't auto-build", "error", err, "repo", newBase)
			} else {
				conn.WebhookID = webhookID
				conn.WebhookSecret = webhookSecret
				if updateErr := ghStore.Upsert(c.Request.Context(), conn); updateErr != nil {
					log.Warn("github: webhook created but failed to persist", "error", updateErr)
				}
			}
		}

		log.Info("GitHub repo linked", "account", acct.Name, "agent", agentName, "repo", req.RepoFullName, "branch", req.Branch)
		c.JSON(http.StatusCreated, gin.H{
			"repo_full_name": req.RepoFullName,
			"branch":         req.Branch,
		})
	}
}

// GitHubAccountListConnections handles GET /api/v1/accounts/:account/github/connections.
// Returns all repos already linked to agents under this account so the client can
// disable them in the repo picker and prevent duplicate connections.
func GitHubAccountListConnections(log *logger.Logger, ghStore *githubconnection.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		conns, err := ghStore.ListByAccount(c.Request.Context(), acct.ID)
		if err != nil {
			log.Error("github: list account connections", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list connections"})
			return
		}

		type connection struct {
			AgentName    string    `json:"agent_name"`
			RepoFullName string    `json:"repo_full_name"`
			CreatedAt    time.Time `json:"created_at"`
		}
		out := make([]connection, 0, len(conns))
		for _, conn := range conns {
			out = append(out, connection{AgentName: conn.AgentName, RepoFullName: conn.RepoFullName, CreatedAt: conn.CreatedAt})
		}
		c.JSON(http.StatusOK, gin.H{"connections": out})
	}
}

// GitHubAccountStatus handles GET /api/v1/accounts/:account/github.
// Returns whether the account has a live GitHub OAuth token and the authenticated login.
func GitHubAccountStatus(log *logger.Logger, pipesClient *pipes.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		token, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"connected": false})
			return
		}

		resp := gin.H{"connected": true}
		if gh := githubclient.New(token.AccessToken); gh != nil {
			login, loginErr := gh.GetLogin(c.Request.Context())
			if loginErr != nil {
				log.Warn("github: failed to fetch login for account status", "error", loginErr)
			} else if login != "" {
				resp["github_login"] = login
			}
		}
		c.JSON(http.StatusOK, resp)
	}
}

// GitHubAccountDisconnect handles DELETE /api/v1/accounts/:account/github.
// Removes all agent repo connections and their webhooks for the account.
func GitHubAccountDisconnect(log *logger.Logger, pipesClient *pipes.Client, ghStore *githubconnection.Store) gin.HandlerFunc {
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

		conns, err := ghStore.ListByAccount(c.Request.Context(), acct.ID)
		if err != nil {
			log.Error("github: list connections for disconnect", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load connections"})
			return
		}

		// Best-effort webhook removal using the account's OAuth token.
		// deletedWebhooks tracks webhook IDs already removed so that multiple
		// blueprints sharing the same repo (e.g. different monorepo subpaths)
		// don't trigger redundant GitHub API calls for the same webhook.
		token, tokenErr := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		deletedWebhooks := make(map[int64]bool)
		for _, conn := range conns {
			repoBase := githubconnection.RepoBase(conn.RepoFullName)

			if delErr := ghStore.Delete(c.Request.Context(), acct.ID, conn.AgentName); delErr != nil {
				log.Error("github: delete connection on account disconnect", "error", delErr, "agent", conn.AgentName)
			}

			// Delete the webhook only after removing the connection row so that
			// CountByRepoBase reflects the post-deletion state. If another account's
			// connection still references the same base repo, count > 0 and we leave
			// the webhook in place.
			if tokenErr == nil && conn.WebhookID != 0 && !deletedWebhooks[conn.WebhookID] {
				if count, countErr := ghStore.CountByRepoBase(c.Request.Context(), repoBase); countErr == nil && count == 0 {
					if gh := githubclient.New(token.AccessToken); gh != nil {
						if delErr := gh.DeleteWebhook(c.Request.Context(), repoBase, conn.WebhookID); delErr != nil {
							log.Warn("github: delete webhook on account disconnect", "error", delErr, "repo", conn.RepoFullName)
						} else {
							deletedWebhooks[conn.WebhookID] = true
						}
					}
				}
			}
		}

		// Revoke the OAuth token in WorkOS Pipes so subsequent status checks return not-connected.
		if err := pipesClient.DeleteConnection(c.Request.Context(), pipes.DeleteConnectionInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		}); err != nil {
			log.Error("github: revoke pipes connection on account disconnect", "error", err)
		}

		log.Info("GitHub account disconnected", "account", acct.Name, "connections_removed", len(conns))
		c.Status(http.StatusNoContent)
	}
}

// GitHubDisconnect handles DELETE /api/v1/agents/:account/:name/github.
// Removes the webhook from GitHub and deletes the connection record.
func GitHubDisconnect(log *logger.Logger, pipesClient *pipes.Client, ghStore *githubconnection.Store) gin.HandlerFunc {
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

		agentName := c.Param("name")

		conn, err := ghStore.Get(c.Request.Context(), acct.ID, agentName)
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no GitHub connection for this agent"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load connection"})
			return
		}

		repoBase := githubconnection.RepoBase(conn.RepoFullName)

		if err := ghStore.Delete(c.Request.Context(), acct.ID, agentName); err != nil {
			log.Error("githubconnection: delete", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete connection"})
			return
		}

		// Remove webhook only if no other connections remain for this base repo.
		if conn.WebhookID != 0 {
			if count, countErr := ghStore.CountByRepoBase(c.Request.Context(), repoBase); countErr == nil && count == 0 && pipesClient != nil {
				token, tokenErr := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
					Provider:       "github",
					UserID:         session.UserID,
					OrganizationID: session.OrganizationID,
				})
				if tokenErr == nil {
					gh := githubclient.New(token.AccessToken)
					if delErr := gh.DeleteWebhook(c.Request.Context(), repoBase, conn.WebhookID); delErr != nil {
						log.Warn("github: delete webhook", "error", delErr, "repo", repoBase)
					}
				}
			}
		}

		log.Info("GitHub connection removed", "account", acct.Name, "agent", agentName)
		c.Status(http.StatusNoContent)
	}
}

// GitHubStatus handles GET /api/v1/agents/:account/:name/github.
// Returns the current connection info, recent builds, and AGENT.md draft card (Redis-cached).
func GitHubStatus(log *logger.Logger, ghStore *githubconnection.Store, pipesClient *pipes.Client, cache k8scache.Cache) gin.HandlerFunc {
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

		agentName := c.Param("name")

		conn, err := ghStore.Get(c.Request.Context(), acct.ID, agentName)
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusOK, GitHubStatusResponse{Connected: false, Builds: []githubconnection.Build{}})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load connection"})
			return
		}

		builds, err := ghStore.ListBuilds(c.Request.Context(), acct.ID, agentName, 10)
		if err != nil {
			log.Warn("githubconnection: list builds", "error", err)
			builds = []githubconnection.Build{}
		}
		if builds == nil {
			builds = []githubconnection.Build{}
		}

		// Attach per-component status to each build.
		for i := range builds {
			components, err := ghStore.ListBuildComponents(c.Request.Context(), builds[i].ID)
			if err != nil {
				log.Warn("githubconnection: list build components", "error", err)
			}
			builds[i].Components = components
		}

		// Fetch AGENT.md and return as draft_card. Only relevant before the first build;
		// we still always return it so the client can display it during the draft window.
		// Redis cache keyed by repo+branch with a 60s TTL avoids repeated GitHub API calls
		// during the 5s polling interval while a build is in-flight.
		var draftCard *spec.ParsedAgentCard
		cacheKey := agentMDCachePrefix + conn.RepoFullName + ":" + conn.Branch
		if cached, ok := cache.Get(c.Request.Context(), cacheKey); ok {
			var card spec.ParsedAgentCard
			if json.Unmarshal(cached, &card) == nil {
				draftCard = &card
			}
		} else {
			token, tokenErr := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
				Provider:       "github",
				UserID:         session.UserID,
				OrganizationID: session.OrganizationID,
			})
			if tokenErr == nil {
				if content, fetchErr := githubbuild.FetchFileContent(c.Request.Context(), token.AccessToken, conn.RepoFullName, conn.Branch, "AGENT.md"); fetchErr == nil && content != "" {
					if card, parseErr := spec.ParseAgentCard(content); parseErr == nil && card != nil {
						draftCard = card
						if b, marshalErr := json.Marshal(card); marshalErr == nil {
							_ = cache.Set(c.Request.Context(), cacheKey, b, agentMDCacheTTL)
						}
					}
				}
			}
		}

		c.JSON(http.StatusOK, GitHubStatusResponse{
			Connected:    true,
			RepoFullName: conn.RepoFullName,
			Branch:       conn.Branch,
			Builds:       builds,
			DraftCard:    draftCard,
		})
	}
}

// githubPushPayload is the minimal GitHub push webhook payload we need.
type githubPushPayload struct {
	Ref        string `json:"ref"` // e.g. "refs/heads/main"
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit *struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"head_commit"`
	Commits []struct {
		Added    []string `json:"added"`
		Removed  []string `json:"removed"`
		Modified []string `json:"modified"`
	} `json:"commits"`
}

// GitHubWebhook handles POST /webhooks/github.
// Receives push events from GitHub. No session auth — verified via HMAC.
// Fan-out: each connection for the pushed repo+branch gets its own build,
// subject to path filtering for subpath connections.
func GitHubWebhook(log *logger.Logger, ghStore *githubconnection.Store, queue githubBuildQueue) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-GitHub-Event") != "push" {
			c.Status(http.StatusOK)
			return
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, 5<<20))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
			return
		}

		var payload githubPushPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
			return
		}

		// Use GetByRepoBase to retrieve the shared webhook secret for HMAC verification.
		baseConn, err := ghStore.GetByRepoBase(c.Request.Context(), payload.Repository.FullName)
		if errors.Is(err, sql.ErrNoRows) {
			c.Status(http.StatusOK)
			return
		}
		if err != nil {
			log.Error("github webhook: lookup connection", "error", err, "repo", payload.Repository.FullName)
			c.Status(http.StatusInternalServerError)
			return
		}

		// Verify HMAC signature.
		sig := c.GetHeader("X-Hub-Signature-256")
		if !verifyGitHubSignature(body, baseConn.WebhookSecret, sig) {
			log.Warn("github webhook: invalid signature", "repo", payload.Repository.FullName)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}

		// Extract branch from ref.
		branch, found := strings.CutPrefix(payload.Ref, "refs/heads/")
		if !found {
			c.Status(http.StatusOK)
			return
		}

		if payload.After == "" || payload.After == "0000000000000000000000000000000000000000" {
			// Branch deletion — ignore.
			c.Status(http.StatusOK)
			return
		}

		var commitMsg, commitAuthor string
		if payload.HeadCommit != nil {
			commitMsg = firstCommitLine(payload.HeadCommit.Message)
			commitAuthor = payload.HeadCommit.Author.Name
		}

		// Collect all changed files across all commits for path filtering.
		var changedFiles []string
		for _, commit := range payload.Commits {
			changedFiles = append(changedFiles, commit.Added...)
			changedFiles = append(changedFiles, commit.Removed...)
			changedFiles = append(changedFiles, commit.Modified...)
		}

		// Fan-out: query all connections for this repo+branch.
		conns, err := ghStore.ListByRepoAndBranch(c.Request.Context(), payload.Repository.FullName, branch)
		if err != nil {
			log.Error("github webhook: list connections", "error", err, "repo", payload.Repository.FullName)
			c.Status(http.StatusInternalServerError)
			return
		}

		var attempted, enqueued int
		for _, conn := range conns {
			subPath := githubconnection.RepoSubPath(conn.RepoFullName)
			if subPath != "" && !filesTouchSubPath(changedFiles, subPath) {
				continue
			}

			attempted++
			buildID := randomHex(8)
			buildRecordID, err := ghStore.CreateBuild(c.Request.Context(), &githubconnection.Build{
				ConnectionID:  conn.ID,
				AccountID:     conn.AccountID,
				AgentName:     conn.AgentName,
				BuildID:       buildID,
				CommitSHA:     payload.After,
				Branch:        branch,
				Status:        "pending",
				CommitMessage: commitMsg,
				CommitAuthor:  commitAuthor,
			})
			if err != nil {
				log.Error("github webhook: create build record", "error", err, "agent", conn.AgentName)
				continue
			}

			if err := ghStore.CancelOlderBuilds(c.Request.Context(), conn.ID, buildRecordID); err != nil {
				log.Warn("github webhook: cancel older builds", "error", err, "connection_id", conn.ID)
			}
			queue.CancelGitHubBuildsForConnection(c.Request.Context(), conn.ID)

			if err := queue.EnqueueGitHubBuild(c.Request.Context(), riverqueue.GitHubBuildArgs{
				ConnectionID:  conn.ID,
				CommitSHA:     payload.After,
				BuildID:       buildID,
				BuildRecordID: buildRecordID,
			}); err != nil {
				log.Error("github webhook: enqueue build", "error", err, "agent", conn.AgentName)
				_ = ghStore.UpdateBuildStatus(c.Request.Context(), buildRecordID, "failed", "failed to enqueue build job")
				continue
			}

			enqueued++
			log.Info("GitHub push enqueued for build",
				"repo", payload.Repository.FullName,
				"branch", branch,
				"agent", conn.AgentName,
				"commit", payload.After[:min(7, len(payload.After))],
				"build_id", buildID,
			)
		}

		// If any eligible connection failed to enqueue (partial or total), return 500 so
		// GitHub's redelivery UI surfaces the problem and operators have a signal to act on.
		if attempted > 0 && enqueued < attempted {
			c.Status(http.StatusInternalServerError)
			return
		}
		if enqueued > 0 {
			c.Status(http.StatusAccepted)
		} else {
			c.Status(http.StatusOK)
		}
	}
}

// verifyGitHubSignature checks the HMAC-SHA256 signature from GitHub.
func verifyGitHubSignature(body []byte, secret, sig string) bool {
	if len(sig) < 8 || sig[:7] != "sha256=" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

// firstCommitLine returns the subject line of a commit message (text before the first newline).
func firstCommitLine(msg string) string {
	if line, _, found := strings.Cut(msg, "\n"); found {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(msg)
}

func randomHex(n int) string {
	b := make([]byte, n/2)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}

// validRepoSegment matches the characters GitHub allows in owner and repo names.
var validRepoSegment = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateRepoFullName checks that name has at least two slash-separated segments
// (owner/repo), that each segment contains only [A-Za-z0-9._-], and that no segment
// exceeds GitHub's 100-character limit. This prevents shell/URL injection because
// repo_full_name flows into GitHub API URLs and git-clone init container commands.
func validateRepoFullName(name string) error {
	segments := strings.Split(name, "/")
	if len(segments) < 2 {
		return errors.New("repo_full_name must be owner/repo[/subpath]")
	}
	for _, seg := range segments {
		if seg == "" || seg == "." || seg == ".." {
			return errors.New("invalid repo_full_name: illegal path segment")
		}
		if len(seg) > 100 {
			return errors.New("invalid repo_full_name: segment exceeds maximum length")
		}
		if !validRepoSegment.MatchString(seg) {
			return errors.New("invalid repo_full_name: segment contains invalid characters")
		}
	}
	return nil
}

// filesTouchSubPath reports whether any file in files has subPath+"/" as a prefix.
func filesTouchSubPath(files []string, subPath string) bool {
	prefix := subPath + "/"
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// GitHubHandlerConfig holds config values needed by GitHub handlers.
type GitHubHandlerConfig struct {
	// WebhookBaseURL is the public API base URL (e.g. https://astropods.com).
	WebhookBaseURL string
	// FrontendURL is the base URL of the web app for redirects.
	FrontendURL string
}

// GitHubRebuild handles POST /api/v1/agents/:account/:name/github/rebuild.
// Fetches the latest commit on the connected branch and enqueues a new build.
func GitHubRebuild(log *logger.Logger, pipesClient *pipes.Client, ghStore *githubconnection.Store, queue githubBuildQueue) gin.HandlerFunc {
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

		agentName := c.Param("name")

		conn, err := ghStore.Get(c.Request.Context(), acct.ID, agentName)
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no GitHub connection for this agent"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load connection"})
			return
		}

		token, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "github_not_connected"})
			return
		}

		log.Info("rebuild: token fetched",
			"session_user_id", session.UserID,
			"conn_workos_user_id", conn.WorkOSUserID,
			"user_id_matches", session.UserID == conn.WorkOSUserID,
			"connection_id", conn.ID,
		)

		gh := githubclient.New(token.AccessToken)
		repoBase := githubconnection.RepoBase(conn.RepoFullName)
		sha, err := gh.GetBranchHead(c.Request.Context(), repoBase, conn.Branch)
		if err != nil {
			log.Error("github: get branch head", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get latest commit"})
			return
		}

		commit, err := gh.GetCommit(c.Request.Context(), repoBase, sha)
		if err != nil {
			log.Warn("github: get commit metadata", "error", err, "sha", sha)
		}

		buildID := randomHex(8)
		buildRecordID, err := ghStore.CreateBuild(c.Request.Context(), &githubconnection.Build{
			ConnectionID:  conn.ID,
			AccountID:     conn.AccountID,
			AgentName:     agentName,
			BuildID:       buildID,
			CommitSHA:     sha,
			Branch:        conn.Branch,
			CommitMessage: commit.Message,
			CommitAuthor:  commit.Author,
			Status:        "pending",
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create build record"})
			return
		}

		if err := queue.EnqueueGitHubBuild(c.Request.Context(), riverqueue.GitHubBuildArgs{
			ConnectionID:  conn.ID,
			CommitSHA:     sha,
			BuildID:       buildID,
			BuildRecordID: buildRecordID,
		}); err != nil {
			_ = ghStore.UpdateBuildStatus(c.Request.Context(), buildRecordID, "failed", "failed to enqueue build job")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enqueue build"})
			return
		}

		log.Info("Manual rebuild enqueued", "account", acct.Name, "agent", agentName, "sha", sha[:7], "build_id", buildID)
		c.JSON(http.StatusAccepted, gin.H{"build_id": buildID, "commit_sha": sha})
	}
}

// GitHubBuildLogs handles GET /api/v1/agents/:account/:name/github/builds/:build_id/logs.
// Fetches the tail of logs from all containers of the build job pod.
func GitHubBuildLogs(log *logger.Logger, ghStore *githubconnection.Store, k8sClient k8s.ClusterClient, buildNamespace string) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		agentName := c.Param("name")
		buildID := c.Param("build_id")

		build, err := ghStore.GetBuildByBuildID(c.Request.Context(), acct.ID, agentName, buildID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "build not found"})
			return
		}

		components, err := ghStore.ListBuildComponents(c.Request.Context(), build.ID)
		if err != nil {
			log.Warn("githubconnection: list build components for logs", "error", err)
		}

		ns := buildNamespace
		if ns == "" {
			ns = "as0-builds"
		}
		isActive := build.Status == "pending" || build.Status == "building"

		type componentLog struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Logs   string `json:"logs"`
		}

		var result []componentLog
		for _, comp := range components {
			logs := comp.Logs
			// For active builds with no persisted logs yet, fetch live from K8s.
			if logs == "" && isActive && comp.K8sJobName != "" && k8sClient != nil {
				logs = fetchK8sJobLogs(c.Request.Context(), k8sClient, ns, comp.K8sJobName)
			}
			result = append(result, componentLog{
				Name:   comp.ComponentName,
				Status: comp.Status,
				Logs:   logs,
			})
		}

		// Fallback: no components (old build) — fetch from K8s using legacy job name.
		if len(result) == 0 && k8sClient != nil {
			legacyJobName := "build-" + buildID + "-agent"
			logs := fetchK8sJobLogs(c.Request.Context(), k8sClient, ns, legacyJobName)
			if logs != "" {
				result = append(result, componentLog{
					Name:   "agent",
					Status: build.Status,
					Logs:   logs,
				})
			}
		}

		// Backward compat: populate flat fields from first component.
		flatLogs := ""
		if len(result) > 0 {
			flatLogs = result[0].Logs
		}

		c.JSON(http.StatusOK, gin.H{
			"components": result,
			"job":        "build-" + buildID,
			"pod":        "",
			"phase":      build.Status,
			"logs":       flatLogs,
		})
	}
}

// fetchK8sJobLogs fetches logs from all containers of a K8s Job pod.
func fetchK8sJobLogs(ctx context.Context, k8sClient k8s.ClusterClient, ns, jobName string) string {
	pods, err := k8sClient.Clientset().CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil || len(pods.Items) == 0 {
		return ""
	}
	pod := pods.Items[0]

	tailLines := int64(500)
	var sb strings.Builder

	// Events first — scheduling/pull errors are the most common build issues.
	events, evErr := k8sClient.Clientset().CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + pod.Name,
	})
	if evErr == nil && len(events.Items) > 0 {
		fmt.Fprintf(&sb, "=== events ===\n")
		for _, ev := range events.Items {
			fmt.Fprintf(&sb, "[%s] %s: %s\n", ev.Type, ev.Reason, ev.Message)
		}
	}

	var containers []string
	for _, ct := range pod.Spec.InitContainers {
		containers = append(containers, ct.Name)
	}
	for _, ct := range pod.Spec.Containers {
		containers = append(containers, ct.Name)
	}

	for _, name := range containers {
		req := k8sClient.Clientset().CoreV1().Pods(ns).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container: name,
			TailLines: &tailLines,
		})
		stream, err := req.Stream(ctx)
		if err != nil {
			fmt.Fprintf(&sb, "=== %s ===\n(logs unavailable: %v)\n", name, err)
			continue
		}
		body, _ := io.ReadAll(stream)
		_ = stream.Close()
		if len(body) > 0 {
			fmt.Fprintf(&sb, "=== %s ===\n%s\n", name, string(body))
		} else {
			fmt.Fprintf(&sb, "=== %s ===\n(no output)\n", name)
		}
	}

	return sb.String()
}
