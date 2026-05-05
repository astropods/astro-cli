package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/astropods/astro/apps/astro-registry/internal/account"
	"github.com/astropods/astro/apps/astro-registry/internal/logger"
	"github.com/astropods/astro/apps/astro-registry/internal/middleware"
	"github.com/astropods/astro/apps/astro-registry/internal/registry"
	"github.com/gin-gonic/gin"
)

// RegistryProxyConfig holds configuration for the registry proxy.
type RegistryProxyConfig struct {
	RegistryURL       string
	Environment       string // Environment prefix for ECR repos (e.g. "prod", "preview")
	AuthProvider      *registry.ECRAuthProvider
	Logger            *logger.Logger
	MembershipChecker *account.MembershipChecker
}

// RegistryVersionCheck handles the /v2/ endpoint for Docker Registry V2 API version check.
func RegistryVersionCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Docker-Distribution-API-Version", "registry/2.0")
		c.Status(http.StatusOK)
	}
}

// RegistryProxy creates a handler that proxies Docker Registry V2 API requests to the backend registry.
// It automatically creates ECR repositories for write operations before proxying.
func RegistryProxy(cfg RegistryProxyConfig) gin.HandlerFunc {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects - return them to the client
			return http.ErrUseLastResponse
		},
	}

	return func(c *gin.Context) {
		path := c.Param("path")
		if path == "" || path == "/" {
			// Version check endpoint - respond directly without proxying
			c.Header("Docker-Distribution-API-Version", "registry/2.0")
			c.Status(http.StatusOK)
			return
		}

		// Validate namespace access and resolve account ID
		ok, accountID := validateNamespaceAccess(c, path, cfg.Logger, cfg.MembershipChecker)
		if !ok {
			return
		}

		// For write operations, ensure the repository exists before proxying
		if isWriteOperation(c.Request.Method) {
			repoName := extractRepositoryName(path, cfg.Environment, accountID)
			if repoName != "" {
				if err := cfg.AuthProvider.CreateRepository(c.Request.Context(), repoName); err != nil {
					cfg.Logger.Error("Failed to ensure repository exists", "repository", repoName, "error", err)
				} else {
					cfg.Logger.Debug("Ensured repository exists", "repository", repoName)
				}
			}
		}

		// Build target URL
		targetURL, err := buildTargetURL(cfg.RegistryURL, path, c.Request.URL.RawQuery, cfg.Environment, accountID)
		if err != nil {
			cfg.Logger.Error("Failed to build target URL", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"errors": []gin.H{{"code": "SERVER_ERROR", "message": "Failed to build target URL"}}})
			return
		}

		// Get ECR auth token
		token, err := cfg.AuthProvider.GetAuthToken(c.Request.Context())
		if err != nil {
			cfg.Logger.Error("Failed to get ECR auth token", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"errors": []gin.H{{"code": "SERVER_ERROR", "message": "Failed to authenticate with registry"}}})
			return
		}

		// Create proxy request
		proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, c.Request.Body)
		if err != nil {
			cfg.Logger.Error("Failed to create proxy request", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"errors": []gin.H{{"code": "SERVER_ERROR", "message": "Failed to create proxy request"}}})
			return
		}

		// Add ECR auth header
		proxyReq.Header.Set("Authorization", "Basic "+token)

		// Forward relevant headers
		for _, h := range []string{"Content-Type", "Content-Length", "Docker-Content-Digest", "Accept", "Accept-Encoding"} {
			if v := c.GetHeader(h); v != "" {
				proxyReq.Header.Set(h, v)
			}
		}

		cfg.Logger.Debug("Proxying registry request",
			"method", c.Request.Method,
			"path", path,
			"target", targetURL,
		)

		// Execute the request
		resp, err := client.Do(proxyReq) //nolint:gosec
		if err != nil {
			cfg.Logger.Error("Failed to proxy request", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"errors": []gin.H{{"code": "PROXY_ERROR", "message": "Failed to connect to registry"}}})
			return
		}
		defer resp.Body.Close() //nolint:errcheck

		// Copy response headers to client
		namespace := extractNamespace(path)
		for key, values := range resp.Header {
			for _, value := range values {
				// Rewrite Location headers to point back to our proxy
				if strings.EqualFold(key, "Location") {
					value = rewriteLocationHeader(value, cfg.RegistryURL, c.Request.Host, namespace)
				}
				c.Writer.Header().Add(key, value)
			}
		}

		// Set status code and stream body
		c.Status(resp.StatusCode)
		if _, err := io.Copy(c.Writer, resp.Body); err != nil {
			cfg.Logger.Error("Failed to stream response body", "error", err)
		}
	}
}

// isWriteOperation returns true if the HTTP method is a write operation.
func isWriteOperation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
}

// extractNamespace extracts the first path segment (account name) from a registry path.
func extractNamespace(path string) string {
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// extractRepositoryName returns the ECR repository name for a registry path.
// ECR path becomes {env}-tenant-{account_id}/{image}.
func extractRepositoryName(path string, env string, accountID string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return env + "-tenant-" + accountID + "/" + parts[1]
}

// validateNamespaceAccess validates that the user has access to the requested namespace.
// All operations require account membership. Org accounts also require the appropriate
// permission: agents:read for pulls, agents:write for pushes.
// Returns (allowed, accountID) — accountID is the resolved UUID, empty for short paths.
//
// When the request authenticated via a registry-scope token, the middleware
// already enforced scope and the token's access entry carries the account ID;
// the per-request membership/permission DB check is skipped.
func validateNamespaceAccess(c *gin.Context, path string, log *logger.Logger, mc *account.MembershipChecker) (bool, string) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return true, ""
	}

	namespace := parts[0]
	isRead := c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead

	// Registry-token fast path: trust scope + account_id from the token.
	if claims, ok := middleware.GetRegistryClaims(c); ok {
		repoEnd := len(parts)
		for i, seg := range parts {
			if seg == "manifests" || seg == "blobs" || seg == "tags" {
				repoEnd = i
				break
			}
		}
		if repoEnd >= 2 {
			repo := strings.Join(parts[:repoEnd], "/")
			action := "push"
			if isRead {
				action = "pull"
			}
			if entry := claims.AccessFor(repo, action); entry != nil && entry.AccountID != "" {
				return true, entry.AccountID
			}
		}
		// Fall through to the WorkOS path on mismatch — should not happen because
		// the middleware already verified scope.
	}

	// Get authenticated user from context
	user, ok := middleware.GetUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"errors": []gin.H{{"code": "UNAUTHORIZED", "message": "Authentication required"}}})
		return false, ""
	}

	// Check if user is a member of the account; resolve the account UUID in the same query.
	memberOK := false
	var accountID string
	if mc != nil {
		isMember, id, err := mc.IsMemberWithID(namespace, user.ID)
		if err != nil {
			if log != nil {
				log.Warn("Membership check failed",
					"namespace", namespace,
					"user_id", user.ID,
					"error", err,
				)
			}
		} else if isMember {
			memberOK = true
			accountID = id
		}
	}

	if !memberOK {
		action := "push to"
		if isRead {
			action = "pull from"
		}
		if log != nil {
			log.Warn("Namespace access denied",
				"user_id", user.ID,
				"namespace", namespace,
				"method", c.Request.Method,
				"path", c.Request.URL.Path,
				"user_agent", c.Request.UserAgent(),
			)
		}
		c.JSON(http.StatusForbidden, gin.H{
			"errors": []gin.H{{
				"code":    "DENIED",
				"message": fmt.Sprintf("Access denied: cannot %s namespace %q", action, namespace),
			}},
		})
		return false, ""
	}

	// Check permission for org accounts (session carries permissions from JWT).
	// Personal accounts have no org-scoped JWT, so skip the check for them.
	session, hasSession := middleware.GetSession(c)
	if hasSession && session.OrganizationID != "" {
		required := "agents:write"
		if isRead {
			required = "agents:read"
		}
		if !slices.Contains(session.Permissions, required) {
			if log != nil {
				log.Warn("Permission denied for registry operation",
					"user_id", user.ID,
					"namespace", namespace,
					"required", required,
				)
			}
			c.JSON(http.StatusForbidden, gin.H{
				"errors": []gin.H{{
					"code":    "DENIED",
					"message": fmt.Sprintf("Insufficient permissions: %s required", required),
				}},
			})
			return false, ""
		}
	}

	return true, accountID
}

// buildTargetURL builds the full URL to the backend ECR registry.
func buildTargetURL(registryURL, path, query, env, accountID string) (string, error) {
	base, err := url.Parse(registryURL)
	if err != nil {
		return "", err
	}

	// Ensure the registry URL ends with /v2
	basePath := strings.TrimSuffix(base.Path, "/")
	if !strings.HasSuffix(basePath, "/v2") {
		basePath = basePath + "/v2"
	}

	// Add tenant prefix to path for ECR
	ecrPath := addTenantPrefix(path, env, accountID)

	// Build full path
	fullPath := basePath + ecrPath

	target := &url.URL{
		Scheme:   base.Scheme,
		Host:     base.Host,
		Path:     fullPath,
		RawQuery: query,
	}

	return target.String(), nil
}

// addTenantPrefix replaces the namespace segment with {env}-tenant-{accountID} for ECR.
func addTenantPrefix(path string, env string, accountID string) string {
	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(trimmed, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		return path
	}

	parts[0] = env + "-tenant-" + accountID
	return "/" + strings.Join(parts, "/")
}

// stripTenantPrefix removes the "{env}-tenant-" prefix from the namespace in a registry path.
func stripTenantPrefix(path string) string {
	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(trimmed, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		return path
	}

	if idx := strings.Index(parts[0], "tenant-"); idx >= 0 {
		parts[0] = parts[0][idx+len("tenant-"):]
	}

	return "/" + strings.Join(parts, "/")
}

// rewriteLocationHeader rewrites Location headers from the backend ECR to point to our proxy.
// It restores the original account name the client used.
func rewriteLocationHeader(location, registryURL, proxyHost, originalNamespace string) string {
	locURL, err := url.Parse(location)
	if err != nil {
		return location
	}

	regURL, err := url.Parse(registryURL)
	if err != nil {
		return location
	}

	if locURL.Host == regURL.Host {
		path := locURL.Path
		if idx := strings.Index(path, "/v2"); idx >= 0 {
			path = path[idx+3:]
		}

		// Replace the tenant-prefixed namespace with the original account name
		trimmed := strings.TrimPrefix(path, "/")
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			parts[0] = originalNamespace
			path = "/" + strings.Join(parts, "/")
		}

		locURL.Scheme = "https"
		locURL.Host = proxyHost
		locURL.Path = "/v2" + path

		return locURL.String()
	}

	return location
}

// HealthCheck returns a simple health check handler
func HealthCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
