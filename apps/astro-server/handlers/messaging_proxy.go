package handlers

import (
	"bufio"
	"context"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

const oidcIdentityHeader = "X-Amzn-Oidc-Identity"

// ProxyDeploymentMessaging forwards deployment-scoped messaging API calls to the
// deployment's messaging sidecar. Astro session auth is validated before proxying;
// the WorkOS user ID is injected upstream as x-amzn-oidc-identity so messaging
// auth stays unchanged.
//
// TODO: When Langfuse-backed chat history lands, reintroduce server-side mirroring
// of assistant SSE there instead of astro-server Postgres.
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
			log.Warn("messaging proxy target resolution failed",
				"deployment", dep.ID, "error", resolveErr)
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "messaging endpoint unavailable"})
			return
		}

		upstreamURL := target + upstreamPath
		req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, upstreamURL, c.Request.Body)
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
	// Allow large SSE lines (long assistant chunks) beyond bufio's 64KB default.
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
	if err := scanner.Err(); err != nil {
		log.Debug("messaging proxy SSE scan failed", "deployment", deploymentID, "error", err)
	}
}
