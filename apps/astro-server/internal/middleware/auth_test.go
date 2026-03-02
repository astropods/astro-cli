package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/auth"
	"github.com/postman/astro/apps/astro-server/internal/config"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func setupTestRouter() (*gin.Engine, *AuthMiddleware) {
	log := logger.New("error", "json") // Suppress logs in tests

	cfg := &config.Config{
		Auth: config.AuthConfig{
			CookieName:     "test_session",
			CookiePassword: "test-password-that-is-32-chars!!",
			SessionMaxAge:  24 * time.Hour,
		},
	}

	sm := auth.NewSessionManager(cfg.Auth.CookiePassword, cfg.Auth.SessionMaxAge)
	// JWT validator would need JWKS URL, skip for cookie-based tests
	mw := NewAuthMiddleware(log, cfg, sm, nil)

	router := gin.New()
	return router, mw
}

func createValidSessionCookie(sm *auth.SessionManager) (string, error) {
	sessionData := &auth.SessionData{
		Session: &auth.Session{
			ID:           "session_123",
			UserID:       "user_456",
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			CreatedAt:    time.Now(),
		},
		User: &auth.User{
			ID:    "user_456",
			Email: "test@example.com",
		},
	}

	return sm.SealSession(sessionData)
}

func createExpiredSessionCookie(sm *auth.SessionManager) (string, error) {
	sessionData := &auth.SessionData{
		Session: &auth.Session{
			ID:           "session_123",
			UserID:       "user_456",
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
			CreatedAt:    time.Now().Add(-2 * time.Hour),
		},
		User: &auth.User{
			ID:    "user_456",
			Email: "test@example.com",
		},
	}

	return sm.SealSession(sessionData)
}

func TestRequireAuth_NoCookie(t *testing.T) {
	router, mw := setupTestRouter()

	router.GET("/protected", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuth_ValidCookie(t *testing.T) {
	log := logger.New("error", "json")

	cfg := &config.Config{
		Auth: config.AuthConfig{
			CookieName:     "test_session",
			CookiePassword: "test-password-that-is-32-chars!!",
			SessionMaxAge:  24 * time.Hour,
		},
	}
	sm := auth.NewSessionManager(cfg.Auth.CookiePassword, cfg.Auth.SessionMaxAge)
	mw := NewAuthMiddleware(log, cfg, sm, nil)

	router := gin.New()
	router.GET("/protected", mw.RequireAuth(), func(c *gin.Context) {
		user, exists := GetUser(c)
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user not in context"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": user.ID})
	})

	cookie, err := createValidSessionCookie(sm)
	if err != nil {
		t.Fatalf("failed to create session cookie: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  "test_session",
		Value: cookie,
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestRequireAuth_ExpiredCookie(t *testing.T) {
	router, mw := setupTestRouter()

	cfg := &config.Config{
		Auth: config.AuthConfig{
			CookieName:     "test_session",
			CookiePassword: "test-password-that-is-32-chars!!",
			SessionMaxAge:  24 * time.Hour,
		},
	}
	sm := auth.NewSessionManager(cfg.Auth.CookiePassword, cfg.Auth.SessionMaxAge)

	router.GET("/protected", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	cookie, err := createExpiredSessionCookie(sm)
	if err != nil {
		t.Fatalf("failed to create session cookie: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  "test_session",
		Value: cookie,
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d for expired session, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuth_InvalidCookie(t *testing.T) {
	router, mw := setupTestRouter()

	router.GET("/protected", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  "test_session",
		Value: "invalid-cookie-data",
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d for invalid cookie, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestOptionalAuth_NoCookie(t *testing.T) {
	router, mw := setupTestRouter()

	router.GET("/public", mw.OptionalAuth(), func(c *gin.Context) {
		user, exists := GetUser(c)
		if exists {
			c.JSON(http.StatusOK, gin.H{"authenticated": true, "user_id": user.ID})
		} else {
			c.JSON(http.StatusOK, gin.H{"authenticated": false})
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestOptionalAuth_ValidCookie(t *testing.T) {
	router, mw := setupTestRouter()

	cfg := &config.Config{
		Auth: config.AuthConfig{
			CookieName:     "test_session",
			CookiePassword: "test-password-that-is-32-chars!!",
			SessionMaxAge:  24 * time.Hour,
		},
	}
	sm := auth.NewSessionManager(cfg.Auth.CookiePassword, cfg.Auth.SessionMaxAge)

	router.GET("/public", mw.OptionalAuth(), func(c *gin.Context) {
		user, exists := GetUser(c)
		if exists {
			c.JSON(http.StatusOK, gin.H{"authenticated": true, "user_id": user.ID})
		} else {
			c.JSON(http.StatusOK, gin.H{"authenticated": false})
		}
	})

	cookie, err := createValidSessionCookie(sm)
	if err != nil {
		t.Fatalf("failed to create session cookie: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	req.AddCookie(&http.Cookie{
		Name:  "test_session",
		Value: cookie,
	})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestGetUser_NotSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	user, exists := GetUser(c)

	if exists {
		t.Error("expected exists to be false when user not set")
	}
	if user != nil {
		t.Error("expected user to be nil when not set")
	}
}

func TestGetSession_NotSet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	session, exists := GetSession(c)

	if exists {
		t.Error("expected exists to be false when session not set")
	}
	if session != nil {
		t.Error("expected session to be nil when not set")
	}
}

func TestRequirePermission_Granted(t *testing.T) {
	_, mw := setupTestRouter()
	router := gin.New()

	// Inject session, then use the real middleware
	router.GET("/admin", func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{
			Permissions: []string{"admin:view", "agents:read"},
		})
		c.Next()
	}, mw.RequirePermission("admin:view"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestRequirePermission_Denied(t *testing.T) {
	_, mw := setupTestRouter()
	router := gin.New()

	router.GET("/admin", func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{
			Permissions: []string{"agents:read"},
		})
		c.Next()
	}, mw.RequirePermission("admin:view"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestRequirePermission_NoPermissions(t *testing.T) {
	_, mw := setupTestRouter()
	router := gin.New()

	router.GET("/admin", func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{
			Permissions: []string{},
		})
		c.Next()
	}, mw.RequirePermission("admin:view"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestRequirePermission_NoSession(t *testing.T) {
	_, mw := setupTestRouter()
	router := gin.New()

	// No session injected
	router.GET("/admin", mw.RequirePermission("admin:view"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireRole_Granted(t *testing.T) {
	_, mw := setupTestRouter()
	router := gin.New()

	router.GET("/admin", func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{
			Role: "admin",
		})
		c.Next()
	}, mw.RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRequireRole_Denied(t *testing.T) {
	_, mw := setupTestRouter()
	router := gin.New()

	router.GET("/admin", func(c *gin.Context) {
		c.Set(string(auth.SessionContextKey), &auth.Session{
			Role: "member",
		})
		c.Next()
	}, mw.RequireRole("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestRequireRole_HasRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/admin", func(c *gin.Context) {
		// Manually set session with role
		c.Set(string(auth.SessionContextKey), &auth.Session{
			Role: "admin",
		})
		c.Next()
	}, func(c *gin.Context) {
		// Check role
		session, exists := c.Get(string(auth.SessionContextKey))
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		s := session.(*auth.Session)
		if s.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "admin access granted"})
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRequirePermission_HasPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.GET("/deploy", func(c *gin.Context) {
		// Manually set session with permissions
		c.Set(string(auth.SessionContextKey), &auth.Session{
			Permissions: []string{"agents:read", "agents:deploy"},
		})
		c.Next()
	}, func(c *gin.Context) {
		// Check permission
		session, exists := c.Get(string(auth.SessionContextKey))
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		s := session.(*auth.Session)
		hasPermission := false
		for _, p := range s.Permissions {
			if p == "agents:deploy" {
				hasPermission = true
				break
			}
		}
		if !hasPermission {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "deploy access granted"})
	})

	req := httptest.NewRequest(http.MethodGet, "/deploy", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
