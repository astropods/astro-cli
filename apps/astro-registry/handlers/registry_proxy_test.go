package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-registry/internal/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRegistryVersionCheck(t *testing.T) {
	t.Parallel()
	router := gin.New()
	router.GET("/v2/", RegistryVersionCheck())

	req := httptest.NewRequest(http.MethodGet, "/v2/", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if rec.Header().Get("Docker-Distribution-API-Version") != "registry/2.0" {
		t.Error("expected Docker-Distribution-API-Version header")
	}
}

func TestBuildTargetURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		registryURL string
		path        string
		query       string
		expected    string
	}{
		{
			name:        "simple path adds tenant prefix",
			registryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			path:        "/user123/myapp/manifests/latest",
			query:       "",
			expected:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com/v2/tenant-user123/myapp/manifests/latest",
		},
		{
			name:        "with query string",
			registryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			path:        "/user123/myapp/blobs/uploads/",
			query:       "digest=sha256:abc123",
			expected:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com/v2/tenant-user123/myapp/blobs/uploads/?digest=sha256:abc123",
		},
		{
			name:        "registry URL with trailing slash",
			registryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com/",
			path:        "/user123/myapp/manifests/v1.0.0",
			query:       "",
			expected:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com/v2/tenant-user123/myapp/manifests/v1.0.0",
		},
		{
			name:        "registry URL already has /v2",
			registryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com/v2",
			path:        "/user123/myapp/manifests/latest",
			query:       "",
			expected:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com/v2/tenant-user123/myapp/manifests/latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildTargetURL(tt.registryURL, tt.path, tt.query)
			if err != nil {
				t.Fatalf("buildTargetURL failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestAddTenantPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple namespace",
			input:    "/user123/myapp/manifests/latest",
			expected: "/tenant-user123/myapp/manifests/latest",
		},
		{
			name:     "org namespace",
			input:    "/org456/myapp/blobs/sha256:abc",
			expected: "/tenant-org456/myapp/blobs/sha256:abc",
		},
		{
			name:     "empty path",
			input:    "/",
			expected: "/",
		},
		{
			name:     "no path",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := addTenantPrefix(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestStripTenantPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "with tenant prefix",
			input:    "/tenant-user123/myapp/manifests/latest",
			expected: "/user123/myapp/manifests/latest",
		},
		{
			name:     "without tenant prefix",
			input:    "/user123/myapp/manifests/latest",
			expected: "/user123/myapp/manifests/latest",
		},
		{
			name:     "empty path",
			input:    "/",
			expected: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripTenantPrefix(tt.input)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestRewriteLocationHeader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		location    string
		registryURL string
		proxyHost   string
		expected    string
	}{
		{
			name:        "rewrite registry location and strip tenant prefix",
			location:    "https://123456789.dkr.ecr.us-east-1.amazonaws.com/v2/tenant-user123/myapp/blobs/uploads/abc-123",
			registryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			proxyHost:   "registry.astro.example.com",
			expected:    "//registry.astro.example.com/v2/user123/myapp/blobs/uploads/abc-123",
		},
		{
			name:        "don't rewrite external location",
			location:    "https://other-registry.example.com/v2/something",
			registryURL: "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
			proxyHost:   "registry.astro.example.com",
			expected:    "https://other-registry.example.com/v2/something",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rewriteLocationHeader(tt.location, tt.registryURL, tt.proxyHost)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestValidateNamespaceAccess_ReadOperations(t *testing.T) {
	t.Parallel()
	// GET and HEAD should always pass for authenticated users
	methods := []string{http.MethodGet, http.MethodHead}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(method, "/v2/otheruser/myapp/manifests/latest", nil)

			// Set user context
			c.Set(string(auth.UserContextKey), &auth.User{ID: "user123"})

			result := validateNamespaceAccess(c, "/otheruser/myapp/manifests/latest", nil)
			if !result {
				t.Errorf("%s request should be allowed for read operations", method)
			}
		})
	}
}

func TestValidateNamespaceAccess_WriteOperations_OwnNamespace(t *testing.T) {
	t.Parallel()
	methods := []string{http.MethodPut, http.MethodPost, http.MethodPatch, http.MethodDelete}

	for _, method := range methods {
		t.Run(method+"_own_namespace", func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(method, "/v2/user123/myapp/manifests/latest", nil)

			// Set user context with matching namespace
			c.Set(string(auth.UserContextKey), &auth.User{ID: "user123"})

			result := validateNamespaceAccess(c, "/user123/myapp/manifests/latest", nil)
			if !result {
				t.Errorf("%s request should be allowed for own namespace", method)
			}
		})
	}
}

func TestValidateNamespaceAccess_WriteOperations_OrgNamespace(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/v2/org456/myapp/manifests/latest", nil)

	// Set user context with org that matches namespace
	c.Set(string(auth.UserContextKey), &auth.User{ID: "user123", OrganizationID: "org456"})
	c.Set(string(auth.SessionContextKey), &auth.Session{OrganizationID: "org456"})

	result := validateNamespaceAccess(c, "/org456/myapp/manifests/latest", nil)
	if !result {
		t.Error("PUT request should be allowed for org namespace")
	}
}

func TestValidateNamespaceAccess_WriteOperations_ForeignNamespace(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/v2/otheruser/myapp/manifests/latest", nil)

	// Set user context with different namespace
	c.Set(string(auth.UserContextKey), &auth.User{ID: "user123"})
	c.Set(string(auth.SessionContextKey), &auth.Session{OrganizationID: "org456"})

	result := validateNamespaceAccess(c, "/otheruser/myapp/manifests/latest", nil)
	if result {
		t.Error("PUT request should be denied for foreign namespace")
	}

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestValidateNamespaceAccess_NoUser(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPut, "/v2/user123/myapp/manifests/latest", nil)

	// Don't set user context
	result := validateNamespaceAccess(c, "/user123/myapp/manifests/latest", nil)
	if result {
		t.Error("PUT request should be denied when user not authenticated")
	}

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestGetUserNamespace(t *testing.T) {
	t.Parallel()
	router := gin.New()

	// Mock the auth context
	router.GET("/api/namespace", func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{
			ID:             "user_123",
			OrganizationID: "org_456",
		})
		c.Set(string(auth.SessionContextKey), &auth.Session{
			OrganizationID: "org_456",
		})
		c.Next()
	}, GetUserNamespace(nil))

	req := httptest.NewRequest(http.MethodGet, "/api/namespace", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Check response contains user_id
	body := rec.Body.String()
	if body == "" {
		t.Error("expected response body")
	}
}

func TestGetUserNamespace_Unauthorized(t *testing.T) {
	t.Parallel()
	router := gin.New()
	router.GET("/api/namespace", GetUserNamespace(nil))

	req := httptest.NewRequest(http.MethodGet, "/api/namespace", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestHealthCheck(t *testing.T) {
	t.Parallel()
	router := gin.New()
	router.GET("/healthz", HealthCheck())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
