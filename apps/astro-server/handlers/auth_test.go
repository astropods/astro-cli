package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// createTestAuthHandler creates an AuthHandler with minimal config for testing
// Note: This won't have a functional WorkOS client, but we can test cookie behavior
func createTestAuthHandler(sameSite string) *AuthHandler {
	log := logger.New("error", "json")

	cfg := &config.Config{
		Auth: config.AuthConfig{
			CookieName:     "test_session",
			CookiePassword: "test-password-that-is-32-chars!!",
			CookieDomain:   "",
			CookieSecure:   false,
			CookieSameSite: sameSite,
			CookieMaxAge:   7 * 24 * time.Hour,
			SessionMaxAge:  24 * time.Hour,
			FrontendURL:    "http://localhost:5173",
		},
	}

	sessionManager := auth.NewSessionManager(cfg.Auth.CookiePassword, cfg.Auth.SessionMaxAge)

	return &AuthHandler{
		log:            log,
		cfg:            cfg,
		sessionManager: sessionManager,
	}
}

// TestSetSameSiteMode_Lax verifies that SameSite=Lax is set correctly
func TestSetSameSiteMode_Lax(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	// Check the Set-Cookie header
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax, got %v", cookie.SameSite)
	}
}

// TestSetSameSiteMode_Strict verifies that SameSite=Strict is set correctly
func TestSetSameSiteMode_Strict(t *testing.T) {
	handler := createTestAuthHandler("Strict")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("expected SameSite=Strict, got %v", cookie.SameSite)
	}
}

// TestSetSameSiteMode_None verifies that SameSite=None is set correctly
func TestSetSameSiteMode_None(t *testing.T) {
	handler := createTestAuthHandler("None")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", true, true) // Secure must be true for SameSite=None
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteNoneMode {
		t.Errorf("expected SameSite=None, got %v", cookie.SameSite)
	}
}

// TestSetSameSiteMode_DefaultsToLax verifies that unknown values default to Lax
func TestSetSameSiteMode_DefaultsToLax(t *testing.T) {
	handler := createTestAuthHandler("InvalidValue")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax for invalid config value, got %v", cookie.SameSite)
	}
}

// TestSetSameSiteMode_EmptyDefaultsToLax verifies that empty string defaults to Lax
func TestSetSameSiteMode_EmptyDefaultsToLax(t *testing.T) {
	handler := createTestAuthHandler("")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("test_cookie", "value", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax for empty config, got %v", cookie.SameSite)
	}
}

// TestSameSiteInSetCookieHeader verifies the raw Set-Cookie header contains SameSite
func TestSameSiteInSetCookieHeader(t *testing.T) {
	tests := []struct {
		name             string
		sameSiteConfig   string
		expectedInHeader string
	}{
		{
			name:             "Lax mode",
			sameSiteConfig:   "Lax",
			expectedInHeader: "SameSite=Lax",
		},
		{
			name:             "Strict mode",
			sameSiteConfig:   "Strict",
			expectedInHeader: "SameSite=Strict",
		},
		{
			name:             "None mode",
			sameSiteConfig:   "None",
			expectedInHeader: "SameSite=None",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := createTestAuthHandler(tt.sameSiteConfig)

			router := gin.New()
			router.GET("/test", func(c *gin.Context) {
				handler.setSameSiteMode(c)
				c.SetCookie("test_cookie", "value", 3600, "/", "", tt.sameSiteConfig == "None", true)
				c.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			// Check the raw Set-Cookie header
			setCookieHeader := rec.Header().Get("Set-Cookie")
			if setCookieHeader == "" {
				t.Fatal("expected Set-Cookie header to be present")
			}

			if !strings.Contains(setCookieHeader, tt.expectedInHeader) {
				t.Errorf("expected Set-Cookie header to contain %q, got: %s", tt.expectedInHeader, setCookieHeader)
			}
		})
	}
}

// TestMultipleCookiesSameSite verifies SameSite is applied to multiple cookies
func TestMultipleCookiesSameSite(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		c.SetCookie("cookie1", "value1", 3600, "/", "", false, true)
		c.SetCookie("cookie2", "value2", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("expected 2 cookies, got %d", len(cookies))
	}

	for _, cookie := range cookies {
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Errorf("expected cookie %s to have SameSite=Lax, got %v", cookie.Name, cookie.SameSite)
		}
	}
}

// TestCookieClearingSameSite verifies SameSite is set even when clearing cookies
func TestCookieClearingSameSite(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		handler.setSameSiteMode(c)
		// Clear a cookie by setting maxAge to -1
		c.SetCookie("session", "", -1, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie to be set (for clearing)")
	}

	cookie := cookies[0]
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("expected SameSite=Lax even when clearing cookie, got %v", cookie.SameSite)
	}
}

// TestMe_ReturnsPermissions verifies that the /auth/me endpoint includes
// permissions from the sealed session in the response.
func TestMe_ReturnsPermissions(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	// Create a session with permissions and seal it
	sessionData := &auth.SessionData{
		Session: &auth.Session{
			ID:          "session_perm",
			UserID:      "user_perm",
			Role:        "admin",
			Permissions: []string{"admin:view", "deployments:write"},
			AccessToken: "token",
			ExpiresAt:   time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
		},
		User: &auth.User{
			ID:    "user_perm",
			Email: "admin@example.com",
		},
	}

	sealed, err := handler.sessionManager.SealSession(sessionData)
	if err != nil {
		t.Fatalf("failed to seal session: %v", err)
	}

	router := gin.New()
	router.GET("/auth/me", handler.Me())

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  handler.cfg.Auth.CookieName,
		Value: sealed,
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var resp auth.AuthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Role != "admin" {
		t.Errorf("Role = %q, want %q", resp.Role, "admin")
	}
	if len(resp.Permissions) != 2 {
		t.Fatalf("Permissions length = %d, want 2", len(resp.Permissions))
	}
	if resp.Permissions[0] != "admin:view" || resp.Permissions[1] != "deployments:write" {
		t.Errorf("Permissions = %v, want [admin:view deployments:write]", resp.Permissions)
	}
}

// TestMe_ReturnsEmptyPermissions verifies that the response always contains
// a permissions array (never null), even when the session has no permissions.
func TestMe_ReturnsEmptyPermissions(t *testing.T) {
	handler := createTestAuthHandler("Lax")

	sessionData := &auth.SessionData{
		Session: &auth.Session{
			ID:          "session_no_perms",
			UserID:      "user_no_perms",
			AccessToken: "token",
			ExpiresAt:   time.Now().Add(1 * time.Hour),
			CreatedAt:   time.Now(),
		},
		User: &auth.User{
			ID:    "user_no_perms",
			Email: "user@example.com",
		},
	}

	sealed, err := handler.sessionManager.SealSession(sessionData)
	if err != nil {
		t.Fatalf("failed to seal session: %v", err)
	}

	router := gin.New()
	router.GET("/auth/me", handler.Me())

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  handler.cfg.Auth.CookieName,
		Value: sealed,
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	// Decode as raw JSON to verify permissions is [] not null
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	permsRaw, ok := raw["permissions"]
	if !ok {
		t.Fatal("response missing 'permissions' field")
	}
	if string(permsRaw) != "[]" {
		t.Errorf("permissions = %s, want []", string(permsRaw))
	}
}
