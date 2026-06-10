package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/chatstore"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const oidcIdentityHeader = "X-Amzn-Oidc-Identity"

// streamPersistTimeout bounds detached server-side consumption of an assistant
// stream so a hung upstream cannot leak a goroutine indefinitely.
const streamPersistTimeout = 15 * time.Minute

// chatTitleMaxRunes caps the conversation title derived from the first message.
const chatTitleMaxRunes = 80

// streamPersistMinInterval throttles mid-stream Postgres writes; finish always flushes.
const streamPersistMinInterval = 500 * time.Millisecond

// messagingProxyMaxSendBody bounds the tee buffer for chat message sends (well above
// MaxMessageContentRunes in UTF-8). Matches io.LimitReader patterns elsewhere in handlers.
const messagingProxyMaxSendBody = 1 << 20

// ProxyDeploymentMessaging forwards deployment-scoped messaging API calls to the
// deployment's messaging sidecar. Astro session auth is validated before proxying;
// the WorkOS user ID is injected upstream as x-amzn-oidc-identity so messaging
// auth stays unchanged.
//
// For chat traffic the proxy also makes the server the source of truth for
// history: the user message is persisted to chatstore on send, and the assistant
// reply is consumed with a detached context and persisted incrementally during
// streaming (plus a final write on finish) — so a browser refresh mid-stream
// still shows partial progress and the completed turn is durable after finish.
//
// Routes: /api/v1/deployments/:id/messaging/* → messaging /api/*
func ProxyDeploymentMessaging(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	k8sReg *k8s.Registry,
	cfg *config.Config,
	chatStore *chatstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		proxyPath := c.Param("proxyPath")
		upstreamPath := messagingUpstreamPath(proxyPath)
		if upstreamPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "messaging path is required"})
			return
		}
		if c.Request.URL.RawQuery != "" {
			upstreamPath += "?" + c.Request.URL.RawQuery
		}

		streamConvID, isChatStream := chatStreamConversationID(proxyPath)
		sendConvID, isChatSend := chatSendConversationID(proxyPath, c.Request.Method)

		// Tee the send body so we can persist the user message before forwarding
		// upstream. Synchronous persist avoids a race where assistant stream chunks
		// land before the user row exists and UpsertAssistantProgress overwrites
		// the prior turn's assistant message.
		var upstreamBody io.Reader = c.Request.Body
		if isChatSend {
			raw, readErr := io.ReadAll(io.LimitReader(c.Request.Body, messagingProxyMaxSendBody+1))
			if readErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
				return
			}
			if len(raw) > messagingProxyMaxSendBody {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
				return
			}
			upstreamBody = bytes.NewReader(raw)
			if chatStore != nil {
				bodyCopy := append([]byte(nil), raw...)
				persistUserMessage(log, chatStore, dep.AccountID, dep.ID, user.ID, sendConvID, bodyCopy)
			}
		}

		target, client, resolveErr := resolveMessagingProxyTarget(c.Request.Context(), cfg, k8sReg, dep)
		if resolveErr != nil {
			log.Warn("messaging proxy target resolution failed",
				"deployment", dep.ID, "error", resolveErr)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "messaging endpoint unavailable"})
			return
		}

		// Assistant streams are consumed with a context detached from the client
		// request so generation completes and persists even if the browser
		// disconnects (refresh, navigation, network drop).
		reqCtx := c.Request.Context()
		if isChatStream && chatStore != nil {
			detached, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), streamPersistTimeout)
			defer cancel()
			reqCtx = detached
		}

		upstreamURL := target + upstreamPath
		req, err := http.NewRequestWithContext(reqCtx, c.Request.Method, upstreamURL, upstreamBody)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build upstream request"})
			return
		}

		copyMessagingRequestHeaders(c.Request.Header, req.Header)
		req.Header.Set(oidcIdentityHeader, user.ID)

		resp, err := client.Do(req)
		if err != nil {
			log.Warn("messaging proxy upstream failed", "deployment", dep.ID, "url", upstreamURL, "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "upstream request failed"})
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if isEventStream(resp) {
			var persist *chatStreamPersist
			if isChatStream && chatStore != nil {
				persist = &chatStreamPersist{
					store:          chatStore,
					deploymentID:   dep.ID,
					userID:         user.ID,
					conversationID: streamConvID,
				}
			}
			proxyMessagingEventStream(log, c, resp, dep.ID, persist)
			return
		}

		copyMessagingResponseHeaders(resp.Header, c.Writer.Header())
		c.Status(resp.StatusCode)
		if _, copyErr := io.Copy(c.Writer, resp.Body); copyErr != nil {
			log.Debug("messaging proxy response copy failed", "deployment", dep.ID, "error", copyErr)
		}
	}
}

func messagingUpstreamPath(proxyPath string) string {
	proxyPath = strings.TrimPrefix(proxyPath, "/")
	if proxyPath == "" {
		return ""
	}
	if strings.HasPrefix(proxyPath, "api/") {
		return "/" + proxyPath
	}
	return "/api/" + proxyPath
}

// chatConversationPathParts normalizes a proxy path to its conversation route
// segments, e.g. "conversations/<id>/stream" → ["conversations","<id>","stream"].
func chatConversationPathParts(proxyPath string) []string {
	p := strings.TrimPrefix(proxyPath, "/")
	p = strings.TrimPrefix(p, "api/")
	return strings.Split(p, "/")
}

// chatStreamConversationID returns the conversation id when proxyPath targets the
// assistant SSE stream (conversations/<id>/stream).
func chatStreamConversationID(proxyPath string) (string, bool) {
	parts := chatConversationPathParts(proxyPath)
	if len(parts) != 3 || parts[0] != "conversations" || parts[2] != "stream" {
		return "", false
	}
	return parseProxyConversationID(parts[1])
}

// chatSendConversationID returns the conversation id when proxyPath is a user
// message send (POST conversations/<id>/messages).
func chatSendConversationID(proxyPath, method string) (string, bool) {
	if method != http.MethodPost {
		return "", false
	}
	parts := chatConversationPathParts(proxyPath)
	if len(parts) != 3 || parts[0] != "conversations" || parts[2] != "messages" {
		return "", false
	}
	return parseProxyConversationID(parts[1])
}

func parseProxyConversationID(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", false
	}
	return id.String(), true
}

// persistUserMessage writes the outgoing user message to chatstore, creating the
// conversation row on first use. Runs synchronously before the upstream POST so
// assistant stream upserts always see the user row (avoids overwriting the prior
// turn). Persistence is best-effort in the sense that failures are logged but
// do not fail the proxy response — upstream delivery proceeds either way.
func persistUserMessage(
	log *logger.Logger,
	chatStore *chatstore.Store,
	accountID, deploymentID, userID, conversationID string,
	body []byte,
) {
	content := parseSendContent(body)
	if content == "" {
		return
	}
	if utf8.RuneCountInString(content) > chatstore.MaxMessageContentRunes {
		log.Warn("chat user message exceeds limit; skipping persistence", "deployment", deploymentID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	title := truncateRunes(content, chatTitleMaxRunes)
	if err := chatStore.AppendUserMessage(ctx, accountID, deploymentID, userID, conversationID, title, chatstore.Message{
		ID:      uuid.NewString(),
		Role:    "user",
		Content: content,
	}); err != nil {
		log.Error("chat persist: append user message",
			"deployment", deploymentID, "conversation", conversationID, "error", err)
	}
}

// parseSendContent extracts the message text from a messaging send body. The web
// client sends {"content": "..."}; "text" is accepted as a fallback.
func parseSendContent(body []byte) string {
	var payload struct {
		Content string `json:"content"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if c := strings.TrimSpace(payload.Content); c != "" {
		return c
	}
	return strings.TrimSpace(payload.Text)
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

func resolveMessagingProxyTarget(
	ctx context.Context,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	dep *deploymentstore.Deployment,
) (string, *http.Client, error) {
	if override := strings.TrimSuffix(cfg.Deployment.MessagingURLOverride, "/"); override != "" {
		return override, http.DefaultClient, nil
	}

	kc, err := deploymentClusterClient(ctx, k8sReg, dep)
	if err != nil {
		return "", nil, err
	}

	svcName := deployment.GenerateAgentResourceName(dep.AgentName, "messaging")
	svc, err := kc.Clientset().CoreV1().Services(dep.Namespace).Get(ctx, svcName, metav1.GetOptions{})
	if err != nil {
		return "", nil, fmt.Errorf("get messaging service %q: %w", svcName, err)
	}

	port, portErr := messagingHTTPPort(svc)
	if portErr != nil {
		return "", nil, portErr
	}

	restCfg := kc.Config()
	if restCfg == nil {
		return "", nil, fmt.Errorf("kubernetes client config unavailable")
	}

	transport, err := rest.TransportFor(restCfg)
	if err != nil {
		return "", nil, fmt.Errorf("kubernetes transport: %w", err)
	}

	baseURL := fmt.Sprintf("%s/api/v1/namespaces/%s/services/%s:%d/proxy",
		strings.TrimSuffix(restCfg.Host, "/"),
		dep.Namespace,
		svcName,
		port,
	)
	return baseURL, &http.Client{Transport: transport}, nil
}

func messagingHTTPPort(svc *corev1.Service) (int32, error) {
	for _, p := range svc.Spec.Ports {
		if p.Name == "http" {
			return p.Port, nil
		}
	}
	for _, p := range svc.Spec.Ports {
		if p.Name != "grpc" {
			return p.Port, nil
		}
	}
	return 0, fmt.Errorf("messaging service %q has no http port", svc.Name)
}

func copyMessagingRequestHeaders(src, dst http.Header) {
	for _, key := range []string{"Content-Type", "Accept", "Accept-Language", "Cache-Control"} {
		if v := src.Get(key); v != "" {
			dst.Set(key, v)
		}
	}
}

func copyMessagingResponseHeaders(src, dst http.Header) {
	for key, values := range src {
		switch strings.ToLower(key) {
		case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
			"te", "trailers", "transfer-encoding", "upgrade":
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

func isEventStream(resp *http.Response) bool {
	return strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
}

// chatStreamPersist accumulates an assistant SSE stream and mirrors progress
// into chatstore so refresh mid-stream can reload partial assistant text.
type chatStreamPersist struct {
	store          *chatstore.Store
	deploymentID   string
	userID         string
	conversationID string
	content        strings.Builder
	messageID      string
	lastPersistAt  time.Time
}

// consume folds one SSE `data:` payload into the accumulated assistant text and
// returns the resolved event type ("chunk"/"finish"/"error"/...). eventName is
// the preceding SSE `event:` line, used as a fallback when the payload omits a
// `type` field (mirrors the client's chatActionFromSse).
func (p *chatStreamPersist) consume(data, eventName string) string {
	var payload struct {
		Type      string `json:"type"`
		ChunkType string `json:"chunk_type"`
		Content   string `json:"content"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return ""
	}
	typ := payload.Type
	if typ == "" {
		typ = eventName
	}
	if typ == "chunk" {
		if payload.ChunkType == "replace" {
			p.content.Reset()
		}
		if p.content.Len() < chatstore.StreamPersistMaxAccumBytes {
			p.content.WriteString(payload.Content)
		}
	}
	return typ
}

func (p *chatStreamPersist) resetTurn() {
	p.content.Reset()
	p.messageID = ""
	p.lastPersistAt = time.Time{}
}

func (p *chatStreamPersist) normalizedContent(trim bool) string {
	text := p.content.String()
	if trim {
		text = strings.TrimSpace(text)
	}
	if text == "" {
		return ""
	}
	if utf8.RuneCountInString(text) > chatstore.MaxMessageContentRunes {
		runes := []rune(text)
		text = string(runes[:chatstore.MaxMessageContentRunes])
	}
	return text
}

// writeProgress upserts the in-flight assistant row. Detached context so a
// client disconnect does not abort the write.
func (p *chatStreamPersist) writeProgress(log *logger.Logger, trim bool) {
	text := p.normalizedContent(trim)
	if text == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	messageID, err := p.store.UpsertAssistantProgress(ctx, p.deploymentID, p.userID, p.conversationID, text)
	if err != nil {
		log.Error("chat persist: upsert assistant message",
			"deployment", p.deploymentID, "conversation", p.conversationID, "error", err)
		return
	}
	p.messageID = messageID
	p.lastPersistAt = time.Now()
}

func (p *chatStreamPersist) maybeWriteProgress(log *logger.Logger, trim, force bool) {
	if !force &&
		!p.lastPersistAt.IsZero() &&
		time.Since(p.lastPersistAt) < streamPersistMinInterval {
		return
	}
	p.writeProgress(log, trim)
}

func setAssistantStreamActive(
	log *logger.Logger,
	store *chatstore.Store,
	deploymentID, userID, conversationID string,
	active bool,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := store.SetAssistantStreamActive(ctx, deploymentID, userID, conversationID, active); err != nil {
		log.Warn("chat persist: set assistant stream active",
			"deployment", deploymentID, "conversation", conversationID, "active", active, "error", err)
	}
}

func proxyMessagingEventStream(
	log *logger.Logger,
	c *gin.Context,
	resp *http.Response,
	deploymentID string,
	persist *chatStreamPersist,
) {
	if persist != nil {
		setAssistantStreamActive(log, persist.store, persist.deploymentID, persist.userID, persist.conversationID, true)
		defer setAssistantStreamActive(log, persist.store, persist.deploymentID, persist.userID, persist.conversationID, false)
	}

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("upstream returned %d", resp.StatusCode)})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})

	flusher, canFlush := c.Writer.(http.Flusher)
	clientGone := false
	eventName := ""
	scanner := bufio.NewScanner(resp.Body)
	// Allow large SSE lines (long assistant chunks) beyond bufio's 64KB default.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		// Forward to the client while it is connected. With a persist sink we
		// keep consuming upstream even after the client disconnects so the
		// assistant turn is still saved (refresh/disconnect durability).
		if !clientGone {
			if _, err := fmt.Fprintf(c.Writer, "%s\n", line); err != nil {
				clientGone = true
			} else if canFlush {
				flusher.Flush()
			}
		}
		if clientGone && persist == nil {
			return
		}

		if persist == nil {
			continue
		}

		// Track SSE framing so we can persist per turn. The stream is
		// long-lived (keep-alive across turns); a `finish` event — not a
		// connection close — marks the end of an assistant reply.
		switch {
		case line == "":
			eventName = ""
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		default:
			data, ok := strings.CutPrefix(line, "data:")
			if !ok {
				continue
			}
			typ := persist.consume(strings.TrimSpace(data), eventName)
			if typ == "chunk" {
				persist.maybeWriteProgress(log, false, false)
			}
			switch typ {
			case "finish":
				persist.writeProgress(log, true)
				persist.resetTurn()
			case "error":
				persist.writeProgress(log, true)
				persist.resetTurn()
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Debug("messaging proxy SSE scan failed", "deployment", deploymentID, "error", err)
	}

	// Upstream closed without an explicit finish (e.g. agent crash); persist
	// whatever assistant text arrived so it is not lost.
	if persist != nil {
		persist.writeProgress(log, true)
		persist.resetTurn()
	}
}
