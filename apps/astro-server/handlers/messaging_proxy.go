package handlers

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const oidcIdentityHeader = "X-Amzn-Oidc-Identity"

// Upstream deadline for non-stream messaging proxy requests (agent/config,
// conversation CRUD, history, sends). The proxy's upstream http.Client has no
// timeout of its own (it also carries long-lived SSE streams), so an
// unresponsive sidecar would otherwise hang until the client disconnects.
// Streaming requests are exempt — they keep the unbounded context.
const messagingProxyUpstreamTimeout = 15 * time.Second

// messagingProxySendBodyLimit bounds request bodies proxied to the sidecar (chat
// sends, conversation create). It matches forwardChat's title-body cap so both
// chat write paths share the same cheap boundary defense — the sidecar enforces
// its own per-message content limit; this just stops an authenticated user from
// streaming an arbitrarily large body through astro-server. GET/stream paths
// carry no body and are unaffected.
//
// FLAG (roadmap): image/attachment sends will carry base64 payloads far larger
// than 1 MiB. When they land, raise this cap or make it per-path (a larger limit
// for the send endpoint) — a single 1 MiB cap will reject attachment sends.
const messagingProxySendBodyLimit = 1 << 20 // 1 MiB

// ProxyDeploymentMessaging forwards deployment-scoped messaging API calls to the
// deployment's messaging sidecar. Astro session auth is validated before proxying;
// the WorkOS user ID is injected upstream as x-amzn-oidc-identity so messaging
// auth stays unchanged.
//
// This is a pure in-transit proxy: it never persists chat content. The messaging
// sidecar owns chat persistence (deployment-local SQLite on the shared disk; no
// Langfuse access), so no conversation metadata or message bodies are written to
// astro-server's database.
//
// Routes: /api/v1/deployments/:id/messaging/* → messaging /api/*
func ProxyDeploymentMessaging(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	k8sReg *k8s.Registry,
	cfg *config.Config,
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

		// Not running → nothing to reach; 404 instead of forwarding a dead-backend
		// 5xx. A stopped deployment still serves its /status and /runtime records
		// (read from the DB), so the chat page can load and fire messaging calls at
		// it; without this guard each one 503s and trips the per-route 5xx alert.
		if dep.Status != deploymentstore.StatusActive {
			c.JSON(http.StatusNotFound, gin.H{"error": "messaging endpoint unavailable"})
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

		target, client, resolveErr := resolveMessagingProxyTarget(c.Request.Context(), cfg, k8sReg, dep)
		if resolveErr != nil {
			// No messaging Service / no ready pod (mid-rollout) / non-web agent →
			// expected, not a fault: 404 so it doesn't trip the per-route 5xx alert.
			if messagingEndpointAbsent(resolveErr) {
				log.Debug("messaging endpoint absent", "deployment", dep.ID, "reason", resolveErr)
				c.JSON(http.StatusNotFound, gin.H{"error": "messaging endpoint unavailable"})
				return
			}
			log.Warn("messaging proxy target resolution failed",
				"deployment", dep.ID, "error", resolveErr)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "messaging endpoint unavailable"})
			return
		}

		// Bound non-stream upstream calls so a stuck sidecar fails fast (504)
		// instead of holding the connection open. SSE streams are exempt — they
		// keep the unbounded request context.
		//
		// NOTE: this exemption is keyed on the request *path*
		// (conversations/{id}/stream), decided before the response is seen,
		// whereas SSE responses are detected generically downstream via
		// isEventStream(resp). Today that path is the only streamed one the client
		// hits (sends are plain JSON POSTs), so the two agree. If the messaging
		// sidecar ever exposes another long-lived SSE endpoint on a different
		// path, this 15s deadline would truncate it — make the bound
		// response-aware at that point.
		reqCtx := c.Request.Context()
		if !isMessagingStreamPath(proxyPath) {
			timed, cancel := context.WithTimeout(reqCtx, messagingProxyUpstreamTimeout)
			defer cancel()
			reqCtx = timed
		}

		// Bound body-carrying requests (sends, create) before forwarding. Buffer up
		// to the cap+1 to detect overflow and return 413; these are small JSON
		// payloads, so buffering is cheap and gives a clean status instead of a mid-
		// stream upstream failure.
		upstreamBody := c.Request.Body
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			raw, readErr := io.ReadAll(io.LimitReader(c.Request.Body, messagingProxySendBodyLimit+1))
			if readErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
				return
			}
			if int64(len(raw)) > messagingProxySendBodyLimit {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
				return
			}
			upstreamBody = io.NopCloser(bytes.NewReader(raw))
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
			proxyMessagingEventStream(log, c, resp, dep.ID)
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

// isMessagingStreamPath reports whether the proxied path is the SSE stream
// endpoint (conversations/{id}/stream), which must be exempt from the non-stream
// upstream timeout.
func isMessagingStreamPath(proxyPath string) bool {
	p := strings.TrimPrefix(proxyPath, "/")
	p = strings.TrimPrefix(p, "api/")
	parts := strings.Split(p, "/")
	return len(parts) == 3 && parts[0] == "conversations" && parts[2] == "stream"
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

// errMessagingNoHTTPPort means the deployment has no web adapter, so it exposes
// no messaging/files endpoint. Callers should map it to a 4xx, not a 5xx.
var errMessagingNoHTTPPort = errors.New("messaging service has no http port")

// messagingEndpointAbsent reports whether a resolveMessagingProxyTarget error
// means the deployment simply has no reachable messaging sidecar rather than a
// genuine fault: a non-web agent with no messaging Service or one without an http
// port (errMessagingNoHTTPPort), or a Service that no longer exists because the
// deployment is stopped or mid-rollout (NotFound). All are expected conditions
// the proxy handlers answer with a 4xx, not the 503 that would trip
// AstroServerHigh5xxRateByRoute for a deployment nobody is actually running.
func messagingEndpointAbsent(err error) bool {
	return errors.Is(err, errMessagingNoHTTPPort) || apierrors.IsNotFound(err)
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
	return 0, fmt.Errorf("%w (service %q)", errMessagingNoHTTPPort, svc.Name)
}

func copyMessagingRequestHeaders(src, dst http.Header) {
	// Last-Event-ID carries the SSE resume cursor: on an EventSource reconnect the
	// browser replays it so the sidecar can resend only the events missed while
	// disconnected. It must reach the sidecar for stream resumption to work.
	for _, key := range []string{"Content-Type", "Accept", "Accept-Language", "Cache-Control", "Last-Event-ID"} {
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

// proxyMessagingEventStream streams an upstream SSE response to the client. It is
// a pure passthrough — chat persistence happens in the messaging sidecar, not
// here.
func proxyMessagingEventStream(
	log *logger.Logger,
	c *gin.Context,
	resp *http.Response,
	deploymentID string,
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
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if _, err := fmt.Fprintf(c.Writer, "%s\n", line); err != nil {
			return
		}
		if canFlush {
			flusher.Flush()
		}
	}
	// A cancelled context is the normal end of an SSE stream — the browser closed
	// the EventSource on a conversation switch or a finished turn, not a failure.
	// Only log a genuine scan error so the signal isn't buried in disconnect noise.
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		log.Debug("messaging proxy SSE scan failed", "deployment", deploymentID, "error", err)
	}
}
