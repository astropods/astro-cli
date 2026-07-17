// Deployment files HTTP handlers — the agent file upload/download API.
//
// astro-server does NOT persist files. These handlers authenticate the WorkOS
// session and forward the request, in transit only, to the deployment's
// messaging sidecar, which owns file storage on the agent's shared persistent
// disk. The metadata/collection endpoints are small JSON forwards; the content
// endpoints stream bytes and are exempt from the short upstream timeout.
//
// The proxy deliberately does NOT follow upstream redirects: the download
// endpoint may answer with a 3xx (a presigned object URL once an S3-backed store
// lands), which must reach the client so its fetch can follow it directly.
package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"unicode"
	"unicode/utf8"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

// filesProxyMaxUploadBytes bounds a single uploaded file at the proxy. It
// matches the sidecar's own cap so an oversized upload is rejected early (before
// streaming) when the client sends a Content-Length; unknown-length bodies are
// still bounded by the sidecar.
const filesProxyMaxUploadBytes = 100 << 20 // 100 MiB

// filesKeyMaxRunes bounds a file key (UUID or adopted filename), matching the
// sidecar's filename cap.
const filesKeyMaxRunes = 255

// The following response types document the files API contract for OpenAPI. The
// bytes are produced by the messaging sidecar and forwarded verbatim.

type DeploymentFileMetaResponse struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	UpdatedAt   string `json:"updated_at"`
	UploadedBy  string `json:"uploaded_by,omitempty"`
}

type ListDeploymentFilesResponse struct {
	Files []DeploymentFileMetaResponse `json:"files"`
}

// DeploymentStorageUsageResponse reports capacity of the volume backing the
// deployment's file store, so the client can warn as it fills. Produced by the
// messaging sidecar and forwarded verbatim. Available is false when the store
// can't report usage (S3-backed, or a platform without statfs).
type DeploymentStorageUsageResponse struct {
	Available      bool    `json:"available"`
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	PercentUsed    float64 `json:"percent_used"`
}

type CreateDeploymentFileUploadDescriptor struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
}

type CreateDeploymentFileResponse struct {
	Key    string                               `json:"key"`
	File   DeploymentFileMetaResponse           `json:"file"`
	Upload CreateDeploymentFileUploadDescriptor `json:"upload"`
}

// parseFileKey validates a route file key before it is spliced into the upstream
// sidecar URL. API-created files use an opaque UUID, but the sidecar also exposes
// plain files an agent drops into the files directory (keyed by filename), so any
// single safe path segment is accepted: no path separators, traversal, or
// control characters (the last of which could smuggle query/fragment characters
// into the trusted upstream request). The key is url.PathEscape'd at the call
// site, keeping it a single path segment.
func parseFileKey(raw string) (string, bool) {
	if raw == "" || raw == "." || raw == ".." {
		return "", false
	}
	if utf8.RuneCountInString(raw) > filesKeyMaxRunes {
		return "", false
	}
	for _, r := range raw {
		if r == '/' || r == '\\' || r == 0 || unicode.IsControl(r) {
			return "", false
		}
	}
	return raw, true
}

// forwardFiles proxies a files request to the deployment's messaging sidecar. It
// resolves the deployment, verifies the session, injects the WorkOS user id as
// the OIDC identity header, and streams the sidecar response back. astro never
// stores the request or response.
//
// content controls the transfer mode:
//   - content=false (metadata/collection): bounded upstream timeout, small JSON
//     body buffered up to the shared send cap.
//   - content=true (upload/download): no timeout (files can be large/slow), the
//     request body is streamed rather than buffered, and 3xx responses are
//     forwarded to the client instead of followed here.
func forwardFiles(
	c *gin.Context,
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	deployStore *deploymentstore.Store,
	accountStore *account.AccountStore,
	method, upstreamPath string,
	content bool,
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

	// Reject oversized uploads before we open the upstream connection, but only
	// when the client declared a length; chunked/unknown-length bodies are
	// bounded by the sidecar's own MaxBytesReader.
	if content && method == http.MethodPut && c.Request.ContentLength > filesProxyMaxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file too large"})
		return
	}

	target, client, resolveErr := resolveMessagingProxyTarget(c.Request.Context(), cfg, k8sReg, dep)
	if resolveErr != nil {
		// No web adapter → no files endpoint. Expected, so 404 not 503.
		if errors.Is(resolveErr, errMessagingNoHTTPPort) {
			log.Debug("files not available: messaging service has no http port", "deployment", dep.ID)
			c.JSON(http.StatusNotFound, gin.H{"error": "file storage is not enabled for this deployment"})
			return
		}
		log.Warn("files proxy target resolution failed", "deployment", dep.ID, "error", resolveErr)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "files endpoint unavailable"})
		return
	}

	upstreamURL := target + upstreamPath
	if c.Request.URL.RawQuery != "" {
		upstreamURL += "?" + c.Request.URL.RawQuery
	}

	reqCtx := c.Request.Context()
	if !content {
		// Bound small metadata calls so a stuck sidecar fails fast instead of
		// holding the connection open. Content transfers keep the unbounded
		// context — they can legitimately run long.
		timed, cancel := context.WithTimeout(reqCtx, messagingProxyUpstreamTimeout)
		defer cancel()
		reqCtx = timed
	}

	var body io.Reader
	switch {
	case content && method == http.MethodPut:
		// Stream the upload straight through; never buffer a whole file.
		body = c.Request.Body
	case method == http.MethodPost:
		// Small JSON metadata (create). Buffer up to the shared cap so an
		// oversized body is a clean 413 rather than a mid-stream upstream error.
		raw, readErr := io.ReadAll(io.LimitReader(c.Request.Body, messagingProxySendBodyLimit+1))
		if readErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}
		if int64(len(raw)) > messagingProxySendBodyLimit {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, upstreamURL, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build files request"})
		return
	}
	// Preserve the declared length on streamed uploads so the upstream (and the
	// K8s apiserver proxy) can frame the request correctly.
	if content && method == http.MethodPut {
		req.ContentLength = c.Request.ContentLength
		if ct := c.GetHeader("Content-Type"); ct != "" {
			req.Header.Set("Content-Type", ct)
		}
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(oidcIdentityHeader, user.ID)

	// Do not follow redirects: a download may answer 3xx (presigned URL) that
	// must reach the client. Share the resolved transport so auth/TLS wiring is
	// identical to the metadata path.
	forwardClient := &http.Client{
		Transport:     client.Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	resp, err := forwardClient.Do(req)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		log.Warn("files proxy upstream failed", "deployment", dep.ID, "url", upstreamURL, "error", err)
		c.JSON(status, gin.H{"error": "files request failed"})
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Forward all response headers (Content-Type, Content-Disposition,
	// Content-Length, and Location on a redirect) minus hop-by-hop.
	copyMessagingResponseHeaders(resp.Header, c.Writer.Header())
	c.Status(resp.StatusCode)
	if _, copyErr := io.Copy(c.Writer, resp.Body); copyErr != nil {
		log.Debug("files proxy response copy failed", "deployment", dep.ID, "error", copyErr)
	}
}

// ListDeploymentFiles handles GET /api/v1/deployments/:id/files.
func ListDeploymentFiles(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		forwardFiles(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodGet, "/api/files", false)
	}
}

// GetDeploymentStorageUsage handles GET /api/v1/deployments/:id/files/usage —
// capacity of the volume backing the deployment's file store, for the client's
// storage-capacity warning.
func GetDeploymentStorageUsage(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		forwardFiles(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodGet, "/api/files/usage", false)
	}
}

// CreateDeploymentFile handles POST /api/v1/deployments/:id/files. It reserves a
// key and returns an upload descriptor; it does not carry the file bytes.
func CreateDeploymentFile(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		forwardFiles(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodPost, "/api/files", false)
	}
}

// GetDeploymentFile handles GET /api/v1/deployments/:id/files/:fileKey (metadata).
func GetDeploymentFile(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := parseFileKey(c.Param("fileKey"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file key"})
			return
		}
		forwardFiles(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodGet, "/api/files/"+url.PathEscape(key), false)
	}
}

// DeleteDeploymentFile handles DELETE /api/v1/deployments/:id/files/:fileKey.
func DeleteDeploymentFile(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := parseFileKey(c.Param("fileKey"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file key"})
			return
		}
		forwardFiles(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodDelete, "/api/files/"+url.PathEscape(key), false)
	}
}

// UploadDeploymentFileContent handles PUT /api/v1/deployments/:id/files/:fileKey/content.
func UploadDeploymentFileContent(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := parseFileKey(c.Param("fileKey"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file key"})
			return
		}
		forwardFiles(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodPut, "/api/files/"+url.PathEscape(key)+"/content", true)
	}
}

// DownloadDeploymentFileContent handles GET /api/v1/deployments/:id/files/:fileKey/content.
func DownloadDeploymentFileContent(
	log *logger.Logger,
	cfg *config.Config,
	k8sReg *k8s.Registry,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := parseFileKey(c.Param("fileKey"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file key"})
			return
		}
		forwardFiles(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodGet, "/api/files/"+url.PathEscape(key)+"/content", true)
	}
}
