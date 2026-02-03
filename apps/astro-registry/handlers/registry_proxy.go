package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-registry/internal/auth"
	"github.com/postman/astro/apps/astro-registry/internal/logger"
	"github.com/postman/astro/apps/astro-registry/internal/middleware"
	"github.com/postman/astro/apps/astro-registry/internal/registry"
)

// RegistryProxyConfig holds configuration for the registry proxy.
type RegistryProxyConfig struct {
	RegistryURL  string
	AuthProvider *registry.ECRAuthProvider
	Logger       *logger.Logger
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

		// Validate namespace for write operations
		if !validateNamespaceAccess(c, path, cfg.Logger) {
			return
		}

		// For write operations, ensure the repository exists before proxying
		if isWriteOperation(c.Request.Method) {
			repoName := extractRepositoryName(path)
			if repoName != "" {
				if err := cfg.AuthProvider.CreateRepository(c.Request.Context(), repoName); err != nil {
					cfg.Logger.Error("Failed to ensure repository exists", "repository", repoName, "error", err)
					// Continue anyway - the repo might already exist or we'll get a clearer error from ECR
				} else {
					cfg.Logger.Debug("Ensured repository exists", "repository", repoName)
				}
			}
		}

		// Build target URL
		targetURL, err := buildTargetURL(cfg.RegistryURL, path, c.Request.URL.RawQuery)
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
		resp, err := client.Do(proxyReq)
		if err != nil {
			cfg.Logger.Error("Failed to proxy request", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"errors": []gin.H{{"code": "PROXY_ERROR", "message": "Failed to connect to registry"}}})
			return
		}
		defer resp.Body.Close()

		// Copy response headers to client
		for key, values := range resp.Header {
			for _, value := range values {
				// Rewrite Location headers to point back to our proxy
				if strings.EqualFold(key, "Location") {
					value = rewriteLocationHeader(value, cfg.RegistryURL, c.Request.Host)
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

// extractRepositoryName extracts the ECR repository name from a registry path.
// Path format: /{namespace}/{image}/manifests/{ref} or /{namespace}/{image}/blobs/{digest}
// Returns: tenant-{namespace}/{image}
func extractRepositoryName(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return "tenant-" + parts[0] + "/" + parts[1]
}

// validateNamespaceAccess validates that the user has access to the requested namespace.
// For write operations (PUT, POST, PATCH, DELETE), the namespace must match the user's ID or org ID.
// For read operations (GET, HEAD), any authenticated user can access.
func validateNamespaceAccess(c *gin.Context, path string, log *logger.Logger) bool {
	// Extract namespace from path (first segment after /v2/)
	// Path format: /{namespace}/{name}/manifests/{ref} or /{namespace}/{name}/blobs/{digest}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 {
		// No namespace in path, allow (probably a version check or similar)
		return true
	}

	namespace := parts[0]

	// Read operations are allowed for any authenticated user
	if c.Request.Method == "GET" || c.Request.Method == "HEAD" {
		return true
	}

	// Get authenticated user from context
	user, ok := middleware.GetUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"errors": []gin.H{{"code": "UNAUTHORIZED", "message": "Authentication required"}}})
		return false
	}

	// Get session for organization ID
	session, _ := middleware.GetSession(c)

	// Check if namespace matches user ID or organization ID (case-insensitive)
	if strings.EqualFold(namespace, user.ID) {
		return true
	}

	if session != nil && session.OrganizationID != "" && strings.EqualFold(namespace, session.OrganizationID) {
		return true
	}

	if log != nil {
		log.Warn("Namespace access denied",
			"user_id", user.ID,
			"requested_namespace", namespace,
		)
	}

	c.JSON(http.StatusForbidden, gin.H{
		"errors": []gin.H{{
			"code":    "DENIED",
			"message": fmt.Sprintf("Access denied: cannot push to namespace %q", namespace),
		}},
	})
	return false
}

// buildTargetURL builds the full URL to the backend ECR registry.
// It adds the "tenant-" prefix to the namespace to comply with ECR IAM policy.
func buildTargetURL(registryURL, path, query string) (string, error) {
	base, err := url.Parse(registryURL)
	if err != nil {
		return "", err
	}

	// Ensure the registry URL ends with /v2
	basePath := strings.TrimSuffix(base.Path, "/")
	if !strings.HasSuffix(basePath, "/v2") {
		basePath = basePath + "/v2"
	}

	// Add "tenant-" prefix to the namespace in the path
	// Path format: /{namespace}/{image}/... -> /tenant-{namespace}/{image}/...
	ecrPath := addTenantPrefix(path)

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

// addTenantPrefix adds "tenant-" prefix to the namespace in a registry path.
// Input:  /{namespace}/{image}/manifests/{ref}
// Output: /tenant-{namespace}/{image}/manifests/{ref}
func addTenantPrefix(path string) string {
	// Remove leading slash for splitting
	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(trimmed, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		return path
	}

	// Add tenant- prefix to namespace
	parts[0] = "tenant-" + parts[0]

	return "/" + strings.Join(parts, "/")
}

// stripTenantPrefix removes "tenant-" prefix from the namespace in a registry path.
// Input:  /tenant-{namespace}/{image}/manifests/{ref}
// Output: /{namespace}/{image}/manifests/{ref}
func stripTenantPrefix(path string) string {
	// Remove leading slash for splitting
	trimmed := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(trimmed, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		return path
	}

	// Strip tenant- prefix from namespace if present
	if strings.HasPrefix(parts[0], "tenant-") {
		parts[0] = strings.TrimPrefix(parts[0], "tenant-")
	}

	return "/" + strings.Join(parts, "/")
}

// rewriteLocationHeader rewrites Location headers from the backend ECR to point to our proxy.
// It also strips the "tenant-" prefix from the namespace.
func rewriteLocationHeader(location, registryURL, proxyHost string) string {
	// Parse the location URL
	locURL, err := url.Parse(location)
	if err != nil {
		return location
	}

	// Parse the registry URL
	regURL, err := url.Parse(registryURL)
	if err != nil {
		return location
	}

	// If the location points to the registry, rewrite it to point to our proxy
	if locURL.Host == regURL.Host {
		// Extract the path after /v2
		path := locURL.Path
		if idx := strings.Index(path, "/v2"); idx >= 0 {
			path = path[idx+3:] // Get path after "/v2"
		}

		// Strip tenant- prefix from the path
		path = stripTenantPrefix(path)

		locURL.Scheme = ""
		locURL.Host = proxyHost
		locURL.Path = "/v2" + path

		return locURL.String()
	}

	return location
}

// GetUserNamespace returns the user's namespace (user ID and org ID).
// This endpoint is called by the CLI to determine where to push images.
func GetUserNamespace(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := c.Get(string(auth.UserContextKey))
		if !ok {
			c.JSON(http.StatusUnauthorized, auth.ErrorResponse{
				Error:       "unauthorized",
				Description: "Authentication required",
			})
			return
		}

		u, ok := user.(*auth.User)
		if !ok {
			c.JSON(http.StatusInternalServerError, auth.ErrorResponse{
				Error:       "server_error",
				Description: "Invalid user data",
			})
			return
		}

		// Also get organization ID if available
		session, _ := middleware.GetSession(c)
		var orgID string
		if session != nil {
			orgID = session.OrganizationID
		}

		c.JSON(http.StatusOK, gin.H{
			"user_id":         u.ID,
			"organization_id": orgID,
		})
	}
}

// HealthCheck returns a simple health check handler
func HealthCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
