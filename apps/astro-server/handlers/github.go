package handlers

import (
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
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	githubclient "github.com/astropods/astro/apps/astro-server/internal/github"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
)

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
}

// GitHubLinkRequest is the body for linking a repo after OAuth.
type GitHubLinkRequest struct {
	RepoFullName string `json:"repo_full_name" binding:"required"`
	Branch       string `json:"branch"`
}

// GitHubConnect handles POST /api/v1/agents/:account/:name/github/connect.
// Checks if the user already has a GitHub Pipes connection. If not, returns an OAuth redirect URL.
// If already connected, the client should proceed to GitHubLink.
func GitHubConnect(log *logger.Logger, pipesClient *pipes.Client, cfg GitHubHandlerConfig) gin.HandlerFunc {
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

		// Check if user already has GitHub connected via Pipes.
		_, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})

		if err == nil {
			// Already connected — client can proceed to list repos and link.
			c.JSON(http.StatusOK, gin.H{"connected": true})
			return
		}

		if !errors.Is(err, pipes.ErrNeedsReauthorization) && !errors.Is(err, pipes.ErrNotInstalled) {
			log.Error("pipes: get access token", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check GitHub connection"})
			return
		}

		// Build return_to URL: server callback that will complete the connection.
		returnTo := fmt.Sprintf("%s/api/v1/agents/%s/%s/github/callback",
			cfg.WebhookBaseURL, acct.Name, agentName)

		authURL, err := pipesClient.GetAuthorizationURL(c.Request.Context(), pipes.GetAuthorizationURLInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
			ReturnTo:       returnTo,
		})
		if err != nil {
			log.Error("pipes: get authorization URL", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate GitHub authorization URL"})
			return
		}

		c.JSON(http.StatusOK, GitHubConnectResponse{RedirectURL: authURL})
	}
}

// GitHubCallback handles GET /api/v1/agents/:account/:name/github/callback.
// WorkOS redirects the user here after completing GitHub OAuth. No body — the user session
// carries auth context. After getting the token, we redirect to the frontend repo selector.
func GitHubCallback(log *logger.Logger, pipesClient *pipes.Client, accountStore *account.AccountStore, cfg GitHubHandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.Redirect(http.StatusFound, cfg.FrontendURL+"?github_error=not_authenticated")
			return
		}

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.Redirect(http.StatusFound, cfg.FrontendURL+"?github_error=account_not_found")
			return
		}

		agentName := c.Param("name")

		// Verify token is now available.
		_, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       "github",
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			log.Error("pipes: token unavailable after callback", "error", err, "user", session.UserID)
			c.Redirect(http.StatusFound, cfg.FrontendURL+"?github_error=token_unavailable")
			return
		}

		log.Info("GitHub OAuth completed", "account", acct.Name, "agent", agentName, "user", session.UserID)

		// Redirect to frontend — user can now select a repo.
		c.Redirect(http.StatusFound, fmt.Sprintf("%s/%s/%s?github_connected=true",
			cfg.FrontendURL, acct.Name, agentName))
	}
}

// GitHubListRepos handles GET /api/v1/agents/:account/:name/github/repos.
// Returns the user's GitHub repos so the frontend can show a repo selector.
func GitHubListRepos(log *logger.Logger, pipesClient *pipes.Client) gin.HandlerFunc {
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
		repos, err := gh.ListRepos(c.Request.Context())
		if err != nil {
			log.Error("github: list repos", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list GitHub repos"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"repos": repos})
	}
}

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

		// Remove existing webhook if re-linking a different repo.
		existing, err := ghStore.Get(c.Request.Context(), acct.ID, agentName)
		if err == nil && existing.WebhookID != 0 {
			oldGH := githubclient.New(token.AccessToken)
			if delErr := oldGH.DeleteWebhook(c.Request.Context(), existing.RepoFullName, existing.WebhookID); delErr != nil {
				log.Warn("github: failed to remove old webhook", "error", delErr, "repo", existing.RepoFullName)
			}
		}

		// Generate webhook secret.
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate webhook secret"})
			return
		}
		webhookSecret := hex.EncodeToString(secretBytes)

		// Install webhook.
		gh := githubclient.New(token.AccessToken)
		webhookPayloadURL := fmt.Sprintf("%s/webhooks/github", cfg.WebhookBaseURL)
		webhookID, err := gh.CreateWebhook(c.Request.Context(), githubclient.CreateWebhookInput{
			RepoFullName: req.RepoFullName,
			PayloadURL:   webhookPayloadURL,
			Secret:       webhookSecret,
		})
		if err != nil {
			log.Error("github: create webhook", "error", err, "repo", req.RepoFullName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to install webhook: %v", err)})
			return
		}

		if err := ghStore.Upsert(c.Request.Context(), &githubconnection.Connection{
			AccountID:     acct.ID,
			AccountName:   acct.Name,
			AgentName:     agentName,
			WorkOSUserID:  session.UserID,
			RepoFullName:  req.RepoFullName,
			Branch:        req.Branch,
			WebhookID:     webhookID,
			WebhookSecret: webhookSecret,
		}); err != nil {
			log.Error("githubconnection: upsert", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save connection"})
			return
		}

		log.Info("GitHub repo linked", "account", acct.Name, "agent", agentName, "repo", req.RepoFullName, "branch", req.Branch)
		c.JSON(http.StatusCreated, gin.H{
			"repo_full_name": req.RepoFullName,
			"branch":         req.Branch,
		})
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

		// Best-effort webhook removal.
		if conn.WebhookID != 0 {
			token, tokenErr := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
				Provider:       "github",
				UserID:         session.UserID,
				OrganizationID: session.OrganizationID,
			})
			if tokenErr == nil {
				gh := githubclient.New(token.AccessToken)
				if delErr := gh.DeleteWebhook(c.Request.Context(), conn.RepoFullName, conn.WebhookID); delErr != nil {
					log.Warn("github: delete webhook", "error", delErr, "repo", conn.RepoFullName)
				}
			}
		}

		if err := ghStore.Delete(c.Request.Context(), acct.ID, agentName); err != nil {
			log.Error("githubconnection: delete", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete connection"})
			return
		}

		log.Info("GitHub connection removed", "account", acct.Name, "agent", agentName)
		c.Status(http.StatusNoContent)
	}
}

// GitHubStatus handles GET /api/v1/agents/:account/:name/github.
// Returns the current connection info and recent builds.
func GitHubStatus(log *logger.Logger, ghStore *githubconnection.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := middleware.GetSession(c); !ok {
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

		c.JSON(http.StatusOK, GitHubStatusResponse{
			Connected:    true,
			RepoFullName: conn.RepoFullName,
			Branch:       conn.Branch,
			Builds:       builds,
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
}

// GitHubWebhook handles POST /webhooks/github.
// Receives push events from GitHub. No session auth — verified via HMAC.
func GitHubWebhook(log *logger.Logger, ghStore *githubconnection.Store, queue *riverqueue.Queue) gin.HandlerFunc {
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

		conn, err := ghStore.GetByRepo(c.Request.Context(), payload.Repository.FullName)
		if errors.Is(err, sql.ErrNoRows) {
			// No connection for this repo — not an error, just ignore.
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
		if !verifyGitHubSignature(body, conn.WebhookSecret, sig) {
			log.Warn("github webhook: invalid signature", "repo", payload.Repository.FullName)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return
		}

		// Only act on pushes to the configured branch.
		expectedRef := "refs/heads/" + conn.Branch
		if payload.Ref != expectedRef {
			c.Status(http.StatusOK)
			return
		}

		if payload.After == "" || payload.After == "0000000000000000000000000000000000000000" {
			// Branch deletion — ignore.
			c.Status(http.StatusOK)
			return
		}

		buildID := randomHex(8)

		var commitMsg, commitAuthor string
		if payload.HeadCommit != nil {
			commitMsg = firstCommitLine(payload.HeadCommit.Message)
			commitAuthor = payload.HeadCommit.Author.Name
		}

		buildRecordID, err := ghStore.CreateBuild(c.Request.Context(), &githubconnection.Build{
			ConnectionID:  conn.ID,
			AccountID:     conn.AccountID,
			AgentName:     conn.AgentName,
			BuildID:       buildID,
			CommitSHA:     payload.After,
			Branch:        conn.Branch,
			Status:        "pending",
			CommitMessage: commitMsg,
			CommitAuthor:  commitAuthor,
		})
		if err != nil {
			log.Error("github webhook: create build record", "error", err)
			c.Status(http.StatusInternalServerError)
			return
		}

		if err := queue.EnqueueGitHubBuild(c.Request.Context(), riverqueue.GitHubBuildArgs{
			ConnectionID:  conn.ID,
			CommitSHA:     payload.After,
			BuildID:       buildID,
			BuildRecordID: buildRecordID,
		}); err != nil {
			log.Error("github webhook: enqueue build", "error", err)
			_ = ghStore.UpdateBuildStatus(c.Request.Context(), buildRecordID, "failed", "failed to enqueue build job")
			c.Status(http.StatusInternalServerError)
			return
		}

		log.Info("GitHub push enqueued for build",
			"repo", payload.Repository.FullName,
			"branch", conn.Branch,
			"commit", payload.After[:min(7, len(payload.After))],
			"build_id", buildID,
		)
		c.Status(http.StatusAccepted)
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

// GitHubHandlerConfig holds config values needed by GitHub handlers.
type GitHubHandlerConfig struct {
	// WebhookBaseURL is the public API base URL (e.g. https://api.astropods.ai).
	WebhookBaseURL string
	// FrontendURL is the base URL of the web app for redirects.
	FrontendURL string
}

// GitHubRebuild handles POST /api/v1/agents/:account/:name/github/rebuild.
// Fetches the latest commit on the connected branch and enqueues a new build.
func GitHubRebuild(log *logger.Logger, pipesClient *pipes.Client, ghStore *githubconnection.Store, queue *riverqueue.Queue) gin.HandlerFunc {
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
		sha, err := gh.GetBranchHead(c.Request.Context(), conn.RepoFullName, conn.Branch)
		if err != nil {
			log.Error("github: get branch head", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get latest commit"})
			return
		}

		commit, err := gh.GetCommit(c.Request.Context(), conn.RepoFullName, sha)
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

		// Verify the build belongs to this account/agent.
		builds, err := ghStore.ListBuilds(c.Request.Context(), acct.ID, agentName, 50)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load builds"})
			return
		}
		var found bool
		for _, b := range builds {
			if b.BuildID == buildID {
				found = true
				break
			}
		}
		if !found {
			c.JSON(http.StatusNotFound, gin.H{"error": "build not found"})
			return
		}

		if k8sClient == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "k8s not configured"})
			return
		}

		jobName := "build-" + buildID + "-agent"
		ns := buildNamespace
		if ns == "" {
			ns = "as0-builds"
		}

		pods, err := k8sClient.Clientset().CoreV1().Pods(ns).List(c.Request.Context(), metav1.ListOptions{
			LabelSelector: "job-name=" + jobName,
		})
		if err != nil || len(pods.Items) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "no pod found for this build"})
			return
		}
		pod := pods.Items[0]

		tailLines := int64(500)
		var sb strings.Builder

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
			stream, err := req.Stream(c.Request.Context())
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

		// Append pod events (warnings/errors from scheduler, kubelet, etc.).
		events, evErr := k8sClient.Clientset().CoreV1().Events(ns).List(c.Request.Context(), metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + pod.Name,
		})
		if evErr == nil && len(events.Items) > 0 {
			fmt.Fprintf(&sb, "\n=== events ===\n")
			for _, ev := range events.Items {
				fmt.Fprintf(&sb, "[%s] %s: %s\n", ev.Type, ev.Reason, ev.Message)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"job":   jobName,
			"pod":   pod.Name,
			"phase": string(pod.Status.Phase),
			"logs":  sb.String(),
		})
	}
}
