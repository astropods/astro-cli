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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
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

// filesProxyMaxErrorBody bounds upstream error bodies read for normalization
// and logging. Successful file content is always streamed and never buffered.
const filesProxyMaxErrorBody = 16 << 10

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

func writeFilesError(c *gin.Context, status int, code, details string) {
	c.JSON(status, ErrorResponse{Error: code, Details: details})
}

func canonicalFilesError(code string) (ErrorResponse, bool) {
	detailsByCode := map[string]string{
		"authentication_required":  "Authentication is required.",
		"file_access_forbidden":    "You don't have permission to access this file.",
		"file_create_failed":       "The file couldn't be created. Try again.",
		"file_delete_failed":       "The file couldn't be deleted. Try again.",
		"file_download_failed":     "The file couldn't be downloaded. Try again.",
		"file_list_failed":         "Files couldn't be loaded. Try again.",
		"file_not_found":           "This file is no longer available.",
		"file_read_failed":         "The file couldn't be loaded. Try again.",
		"file_storage_unavailable": "File storage isn't available for this deployment yet.",
		"file_too_large":           "This file is too large. Choose a smaller file and try again.",
		"file_upload_failed":       "The file couldn't be uploaded. Try again.",
		"files_unavailable":        "File storage is temporarily unavailable. Try again.",
		"insufficient_storage":     "The deployment's storage is full. Delete files to free space, then try again.",
		"invalid_file_name":        "This file name isn't supported.",
		"invalid_file_request":     "The file request is invalid.",
	}
	details, ok := detailsByCode[code]
	return ErrorResponse{Error: code, Details: details}, ok
}

// normalizedFilesError converts the sidecar's legacy text/plain errors and
// Kubernetes proxy failures into Astro's standard JSON error envelope. Only
// known, user-safe sidecar messages are exposed; infrastructure responses are
// logged by the caller and replaced with an actionable generic message.
func normalizedFilesError(status int, body []byte) (int, ErrorResponse) {
	trimmed := strings.TrimSpace(string(body))

	var k8sStatus struct {
		Kind   string `json:"kind"`
		Status string `json:"status"`
	}
	if json.Unmarshal(body, &k8sStatus) == nil &&
		k8sStatus.Kind == "Status" && k8sStatus.Status == "Failure" {
		// Backend pod unreachable (e.g. mid-rollout); 4xx so the route doesn't 5xx.
		return http.StatusNotFound, ErrorResponse{
			Error:   "file_storage_unavailable",
			Details: "File storage isn't available for this deployment yet.",
		}
	}

	var upstream ErrorResponse
	if json.Unmarshal(body, &upstream) == nil {
		if canonical, ok := canonicalFilesError(strings.TrimSpace(upstream.Error)); ok {
			return status, canonical
		}
		if upstream.Details != "" {
			trimmed = strings.TrimSpace(upstream.Details)
		} else if upstream.Error != "" {
			trimmed = strings.TrimSpace(upstream.Error)
		}
	}

	switch trimmed {
	case "invalid file name":
		return status, ErrorResponse{Error: "invalid_file_name", Details: "This file name isn't supported."}
	case "invalid file key", "invalid file size":
		return status, ErrorResponse{Error: "invalid_file_request", Details: "The file request is invalid."}
	case "file too large", "request body too large":
		return status, ErrorResponse{Error: "file_too_large", Details: "This file is too large. Choose a smaller file and try again."}
	case "not enough storage available on the deployment volume", "storage full":
		return status, ErrorResponse{Error: "insufficient_storage", Details: "The deployment's storage is full. Delete files to free space, then try again."}
	case "file storage is not enabled":
		return status, ErrorResponse{Error: "file_storage_unavailable", Details: "File storage isn't available for this deployment yet."}
	case "file not found", "file content not found":
		return status, ErrorResponse{Error: "file_not_found", Details: "This file is no longer available."}
	case "Unauthorized":
		return status, ErrorResponse{Error: "authentication_required", Details: "Authentication is required."}
	case "Forbidden":
		return status, ErrorResponse{Error: "file_access_forbidden", Details: "You don't have permission to access this file."}
	case "Authentication error", "Authorization unavailable":
		return http.StatusServiceUnavailable, ErrorResponse{Error: "files_unavailable", Details: "File storage is temporarily unavailable. Try again."}
	}

	switch status {
	case http.StatusBadRequest:
		return status, ErrorResponse{Error: "invalid_file_request", Details: "The file request is invalid."}
	case http.StatusUnauthorized:
		return status, ErrorResponse{Error: "authentication_required", Details: "Authentication is required."}
	case http.StatusForbidden:
		return status, ErrorResponse{Error: "file_access_forbidden", Details: "You don't have permission to access this file."}
	case http.StatusNotFound:
		return status, ErrorResponse{Error: "file_not_found", Details: "This file is no longer available."}
	case http.StatusRequestEntityTooLarge:
		return status, ErrorResponse{Error: "file_too_large", Details: "This file is too large. Choose a smaller file and try again."}
	case http.StatusInsufficientStorage:
		return status, ErrorResponse{Error: "insufficient_storage", Details: "The deployment's storage is full. Delete files to free space, then try again."}
	default:
		return status, ErrorResponse{Error: "files_unavailable", Details: "File storage is temporarily unavailable. Try again."}
	}
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
		writeFilesError(c, http.StatusUnauthorized, "authentication_required", "Authentication is required.")
		return
	}

	dep, err := resolveDeployment(c, deployStore, accountStore)
	if err != nil {
		writeFilesError(c, http.StatusForbidden, "file_access_forbidden", "You don't have permission to access files for this deployment.")
		return
	}

	// Not running → nothing to reach; 404 instead of forwarding the backend's 5xx.
	if dep.Status != deploymentstore.StatusActive {
		writeFilesError(c, http.StatusNotFound, "file_storage_unavailable", "File storage isn't available for this deployment yet.")
		return
	}

	// Reject oversized uploads before we open the upstream connection, but only
	// when the client declared a length; chunked/unknown-length bodies are
	// bounded by the sidecar's own MaxBytesReader.
	if content && method == http.MethodPut && c.Request.ContentLength > filesProxyMaxUploadBytes {
		writeFilesError(c, http.StatusRequestEntityTooLarge, "file_too_large", "This file is too large. Choose a smaller file and try again.")
		return
	}

	target, client, resolveErr := resolveMessagingProxyTarget(c.Request.Context(), cfg, k8sReg, dep)
	if resolveErr != nil {
		// A non-web agent has no messaging Service, or one without an http port, or
		// its sidecar has no ready pod (stopped / mid-rollout). All are expected, not
		// faults: answer 404, not the 503 that would trip the per-route 5xx alert.
		if messagingEndpointAbsent(resolveErr) {
			log.Debug("files not available", "deployment", dep.ID, "reason", resolveErr)
			writeFilesError(c, http.StatusNotFound, "file_storage_unavailable", "File storage isn't available for this deployment yet.")
			return
		}
		log.Warn("files proxy target resolution failed", "deployment", dep.ID, "error", resolveErr)
		writeFilesError(c, http.StatusServiceUnavailable, "files_unavailable", "File storage is temporarily unavailable. Try again.")
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
			writeFilesError(c, http.StatusBadRequest, "invalid_file_request", "The file request could not be read.")
			return
		}
		if int64(len(raw)) > messagingProxySendBodyLimit {
			writeFilesError(c, http.StatusRequestEntityTooLarge, "file_too_large", "This file request is too large.")
			return
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, upstreamURL, body)
	if err != nil {
		writeFilesError(c, http.StatusInternalServerError, "files_unavailable", "File storage is temporarily unavailable. Try again.")
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
		writeFilesError(c, status, "files_unavailable", "File storage is temporarily unavailable. Try again.")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, filesProxyMaxErrorBody))
		log.Warn("files proxy upstream error",
			"deployment", dep.ID,
			"upstream_status", resp.StatusCode,
			"upstream_body", strings.TrimSpace(string(body)),
		)
		status, apiErr := normalizedFilesError(resp.StatusCode, body)
		writeFilesError(c, status, apiErr.Error, apiErr.Details)
		return
	}

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
			writeFilesError(c, http.StatusBadRequest, "invalid_file_request", "The file key is invalid.")
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
			writeFilesError(c, http.StatusBadRequest, "invalid_file_request", "The file key is invalid.")
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
			writeFilesError(c, http.StatusBadRequest, "invalid_file_request", "The file key is invalid.")
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
			writeFilesError(c, http.StatusBadRequest, "invalid_file_request", "The file key is invalid.")
			return
		}
		forwardFiles(c, log, cfg, k8sReg, deployStore, accountStore,
			http.MethodGet, "/api/files/"+url.PathEscape(key)+"/content", true)
	}
}
