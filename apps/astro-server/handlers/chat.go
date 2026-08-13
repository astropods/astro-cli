// Deployment chat HTTP handlers — the platform chat-page API.
//
// astro-server does NOT persist chat. These handlers authenticate the WorkOS
// session and forward the request, in transit only, to the deployment's
// messaging sidecar, which owns chat persistence in a deployment-local SQLite
// database on the agent's shared persistent disk. No conversation metadata or
// message content is written to astro-server's database.
package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// parseConversationID validates that a route conversation id is a UUID before it
// is spliced into the upstream sidecar URL. gin runs with UnescapePathValues, so
// path params are URL-decoded before the handler sees them; an unvalidated id
// could smuggle `?`/`#`/control characters into the trusted, identity-injected
// upstream request (query injection against the sidecar). Returns the canonical
// (URL-safe) string form.
func parseConversationID(raw string) (string, bool) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", false
	}
	return id.String(), true
}

// The following response types document the chat API contract for OpenAPI. The
// bytes are produced by the messaging sidecar and forwarded verbatim.

type ChatConversationSummaryResponse struct {
	ConversationID     string    `json:"conversation_id"`
	Title              string    `json:"title"`
	UpdatedAt          time.Time `json:"updated_at"`
	AssistantStreaming bool      `json:"assistant_streaming,omitempty"`
}

type ChatMessageResponse struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ListChatConversationsResponse struct {
	Conversations []ChatConversationSummaryResponse `json:"conversations"`
}

type GetChatConversationResponse struct {
	ConversationID     string                `json:"conversation_id"`
	Title              string                `json:"title"`
	UpdatedAt          time.Time             `json:"updated_at"`
	Messages           []ChatMessageResponse `json:"messages"`
	AssistantStreaming bool                  `json:"assistant_streaming"`
	HasMore            bool                  `json:"has_more,omitempty"`
	OldestSeq          int                   `json:"oldest_seq,omitempty"`
}

type UpsertChatConversationInput struct {
	Title string `json:"title"`
}

const chatSidecarSendBodyLimit = 1 << 20 // 1 MiB cap on rename/upsert bodies.

// forwardChat proxies a chat request to the deployment's messaging sidecar.
// It resolves the deployment, verifies the session, injects the WorkOS user id
// as the OIDC identity header, and streams the sidecar response back. astro
// never stores the request or response.
func forwardChat(
	c *gin.Context,
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	deployStore *deploymentstore.Store,
	accountStore *account.AccountStore,
	method, upstreamPath string,
	forwardBody bool,
	entCheck EntitlementChecker,
) {
	user, exists := middleware.GetUser(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	dep, err := resolveDeployment(c, deployStore, accountStore)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	// Writes only. A suspended account must still be able to read its own
	// conversations, and gating a GET here would block that the day history stops
	// being proxied to the agent. Placed before the status check so a refused
	// write names billing instead of reading as an outage.
	if c.Request.Method != http.MethodGet && blockedByBilling(c, entCheck, dep.AccountID) {
		return
	}

	// Not running → nothing to reach; 404 instead of forwarding a dead-backend
	// 5xx. A stopped deployment still serves its /status and /runtime records, so
	// the chat page loads and lists conversations against it; without this guard
	// each call 503s and trips the per-route 5xx alert.
	if dep.Status != deploymentstore.StatusActive {
		c.JSON(http.StatusNotFound, gin.H{"error": "chat endpoint unavailable"})
		return
	}

	upstream, resolveErr := resolveMessagingProxyTarget(c.Request.Context(), cfg, k8sReg, dep)
	if resolveErr != nil {
		// No messaging Service / no ready pod (mid-rollout) / non-web agent →
		// expected, not a fault: 404 so it doesn't trip the per-route 5xx alert.
		if messagingEndpointAbsent(resolveErr) {
			log.Debug("chat endpoint absent", "deployment", dep.ID, "reason", resolveErr)
			c.JSON(http.StatusNotFound, gin.H{"error": "chat endpoint unavailable"})
			return
		}
		log.Warn("chat proxy target resolution failed", "deployment", dep.ID, "error", resolveErr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "chat endpoint unavailable"})
		return
	}

	upstreamURL := upstream.baseURL + upstreamPath
	if c.Request.URL.RawQuery != "" {
		upstreamURL += "?" + c.Request.URL.RawQuery
	}

	var body io.Reader
	if forwardBody {
		// Read cap+1 so an oversized body is rejected with a clean 413 rather than
		// silently truncated (truncation would forward malformed JSON to the
		// sidecar and surface as a confusing 400). Mirrors the messaging proxy.
		raw, readErr := io.ReadAll(io.LimitReader(c.Request.Body, chatSidecarSendBodyLimit+1))
		if readErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}
		if int64(len(raw)) > chatSidecarSendBodyLimit {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		body = bytes.NewReader(raw)
	}

	// Bound the upstream call. The proxy client has no timeout (it carries
	// long-lived SSE streams), and every chat endpoint is a plain
	// request/response, so without a deadline a stuck sidecar would hold the
	// astro-server request open until the client disconnects. Mirrors the
	// messaging proxy's non-stream deadline.
	ctx, cancel := context.WithTimeout(c.Request.Context(), messagingProxyUpstreamTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, upstreamURL, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build chat request"})
		return
	}
	if forwardBody {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set(oidcIdentityHeader, user.ID)
	if upstream.host != "" {
		req.Host = upstream.host
	}

	resp, err := upstream.client.Do(req)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		log.Warn("chat proxy upstream failed", "deployment", dep.ID, "url", upstreamURL, "error", err)
		c.JSON(status, gin.H{"error": "chat request failed"})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		c.Writer.Header().Set("Content-Type", ct)
	}
	c.Status(resp.StatusCode)
	if _, copyErr := io.Copy(c.Writer, resp.Body); copyErr != nil {
		log.Debug("chat proxy response copy failed", "deployment", dep.ID, "error", copyErr)
	}
}

// ListDeploymentChatConversations handles GET /api/v1/deployments/:id/chat/conversations.
func ListDeploymentChatConversations(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	entCheck EntitlementChecker,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		forwardChat(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodGet, "/api/chat/conversations", false, entCheck)
	}
}

// GetDeploymentChatConversation handles GET /api/v1/deployments/:id/chat/conversations/:conversationId.
func GetDeploymentChatConversation(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	entCheck EntitlementChecker,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		convID, ok := parseConversationID(c.Param("conversationId"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}
		forwardChat(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodGet, "/api/chat/conversations/"+url.PathEscape(convID), false, entCheck)
	}
}

// SetDeploymentChatConversationTitle handles PUT /api/v1/deployments/:id/chat/conversations/:conversationId/title.
// It sets the title of an existing, caller-owned conversation (idempotent,
// rename-only); it cannot create a conversation or modify messages.
func SetDeploymentChatConversationTitle(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	entCheck EntitlementChecker,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		convID, ok := parseConversationID(c.Param("conversationId"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}
		forwardChat(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodPut, "/api/chat/conversations/"+url.PathEscape(convID)+"/title", true, entCheck)
	}
}

// DeleteDeploymentChatConversation handles DELETE /api/v1/deployments/:id/chat/conversations/:conversationId.
func DeleteDeploymentChatConversation(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	entCheck EntitlementChecker,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		convID, ok := parseConversationID(c.Param("conversationId"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}
		forwardChat(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodDelete, "/api/chat/conversations/"+url.PathEscape(convID), false, entCheck)
	}
}
