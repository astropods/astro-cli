package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const (
	streamPersistTimeout       = 15 * time.Minute
	chatTitleMaxRunes          = 80
	streamPersistMinInterval   = 500 * time.Millisecond
	messagingProxyMaxSendBody  = 1 << 20
	// Upstream deadline for non-stream messaging proxy requests (agent/config,
	// conversation CRUD, history, sends). The proxy's upstream http.Client has
	// no timeout of its own (it also carries long-lived SSE streams), so an
	// unresponsive sidecar would otherwise hang until the client disconnects.
	// Streaming requests are exempt — they keep the unbounded context.
	messagingProxyUpstreamTimeout = 15 * time.Second
)

// ProxyDeploymentMessaging forwards deployment-scoped messaging API calls to the
// deployment's messaging sidecar. Astro session auth is validated before proxying;
// the WorkOS user ID is injected upstream as x-amzn-oidc-identity so messaging
// auth stays unchanged.
//
// For chat traffic the proxy also persists user sends and assistant SSE streams
// into chatstore so history survives navigation and reloads even when Langfuse
// traces are not yet available.
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
				if err := persistUserMessage(log, chatStore, dep.AccountID, dep.ID, user.ID, sendConvID, bodyCopy); err != nil {
					switch {
					case errors.Is(err, chatstore.ErrActiveAssistantStream):
						c.JSON(http.StatusConflict, gin.H{"error": "assistant is still responding; wait for the current reply to finish"})
						return
					case errors.Is(err, chatstore.ErrMessageLimitReached):
						c.JSON(http.StatusConflict, gin.H{"error": chatstore.ErrMessageLimitReached.Error()})
						return
					case errors.Is(err, chatstore.ErrConversationIDConflict):
						c.JSON(http.StatusConflict, gin.H{"error": chatstore.ErrConversationIDConflict.Error()})
						return
					}
				}
			}
		}

		target, client, resolveErr := resolveMessagingProxyTarget(c.Request.Context(), cfg, k8sReg, dep)
		if resolveErr != nil {
			log.Warn("messaging proxy target resolution failed",
				"deployment", dep.ID, "error", resolveErr)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "messaging endpoint unavailable"})
			return
		}

		reqCtx := c.Request.Context()
		if isChatStream && chatStore != nil {
			detached, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), streamPersistTimeout)
			defer cancel()
			reqCtx = detached
		} else if !isChatStream {
			// Bound non-stream upstream calls so a stuck sidecar fails fast
			// (504) instead of holding the connection open. Streams are exempt.
			//
			// NOTE: this exemption is keyed on the request *path* (isChatStream
			// = conversations/{id}/stream), decided before the response is seen,
			// whereas SSE responses are actually detected generically downstream
			// via isEventStream(resp). Today that path is the only streamed one
			// the client hits (sends are plain JSON POSTs), so the two agree. If
			// the messaging sidecar ever exposes another long-lived SSE endpoint
			// on a different path, this 15s deadline would truncate it — make the
			// bound response-aware (or widen isChatStream) at that point.
			timed, cancel := context.WithTimeout(reqCtx, messagingProxyUpstreamTimeout)
			defer cancel()
			reqCtx = timed
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
			status := http.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			c.JSON(status, gin.H{"error": "upstream request failed"})
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

func chatConversationPathParts(proxyPath string) []string {
	p := strings.TrimPrefix(proxyPath, "/")
	p = strings.TrimPrefix(p, "api/")
	return strings.Split(p, "/")
}

func chatStreamConversationID(proxyPath string) (string, bool) {
	parts := chatConversationPathParts(proxyPath)
	if len(parts) != 3 || parts[0] != "conversations" || parts[2] != "stream" {
		return "", false
	}
	return parseProxyConversationID(parts[1])
}

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

func persistUserMessage(
	log *logger.Logger,
	chatStore *chatstore.Store,
	accountID, deploymentID, userID, conversationID string,
	body []byte,
) error {
	content := parseSendContent(body)
	if content == "" {
		return nil
	}
	if utf8.RuneCountInString(content) > chatstore.MaxMessageContentRunes {
		log.Warn("chat user message exceeds limit; skipping persistence", "deployment", deploymentID)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	title := truncateRunes(content, chatTitleMaxRunes)
	err := chatStore.AppendUserMessage(ctx, accountID, deploymentID, userID, conversationID, title, chatstore.Message{
		ID:      uuid.NewString(),
		Role:    "user",
		Content: content,
	})
	if err != nil {
		log.Error("chat persist: append user message",
			"deployment", deploymentID, "conversation", conversationID, "error", err)
	}
	return err
}

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

type chatStreamPersist struct {
	store          *chatstore.Store
	deploymentID   string
	userID         string
	conversationID string
	content        strings.Builder
	messageID      string
	lastPersistAt  time.Time
	turnMarked     bool
}

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

func (p *chatStreamPersist) markTurnActive(log *logger.Logger) {
	if p.turnMarked {
		return
	}
	setAssistantStreamActive(log, p.store, p.deploymentID, p.userID, p.conversationID, true)
	p.turnMarked = true
}

func (p *chatStreamPersist) clearTurnActive(log *logger.Logger) {
	if !p.turnMarked {
		return
	}
	setAssistantStreamActive(log, p.store, p.deploymentID, p.userID, p.conversationID, false)
	p.turnMarked = false
}

func proxyMessagingEventStream(
	log *logger.Logger,
	c *gin.Context,
	resp *http.Response,
	deploymentID string,
	persist *chatStreamPersist,
) {
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
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

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
			switch typ {
			case "chunk":
				persist.markTurnActive(log)
				persist.maybeWriteProgress(log, false, false)
			case "finish", "error":
				persist.writeProgress(log, true)
				persist.resetTurn()
				persist.clearTurnActive(log)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		log.Debug("messaging proxy SSE scan failed", "deployment", deploymentID, "error", err)
	}

	if persist != nil {
		persist.writeProgress(log, true)
		persist.resetTurn()
		persist.clearTurnActive(log)
	}
}
