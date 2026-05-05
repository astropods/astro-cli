package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-registry/internal/auth"
	"github.com/astropods/astro/apps/astro-registry/internal/config"
	"github.com/astropods/astro/apps/astro-registry/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type testKeyPair struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
}

func generateTestKeyPair() (*testKeyPair, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &testKeyPair{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		keyID:      "test-key-id",
	}, nil
}

func createTestToken(kp *testKeyPair, claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kp.keyID
	return token.SignedString(kp.privateKey)
}

func createMockJWKSServer(kp *testKeyPair) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwks := auth.JWKS{
			Keys: []auth.JWKSKey{
				{
					Kty: "RSA",
					Kid: kp.keyID,
					Use: "sig",
					Alg: "RS256",
					N:   base64.RawURLEncoding.EncodeToString(kp.publicKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(kp.publicKey.E)).Bytes()),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
}

func setupTestMiddleware(jwksURL string) (*gin.Engine, *AuthMiddleware) {
	log := logger.New("error", "json")

	cfg := &config.Config{
		Auth: config.AuthConfig{
			JWKSEndpoint:        jwksURL,
			JWTIssuer:           "https://test-issuer.com",
			WorkOSClientID:      "test-client-id",
			RegistryTokenSecret: "test-secret",
			RegistryTokenIssuer: "astro-registry",
			RegistryTokenTTL:    time.Hour,
			RegistryTokenRealm:  "https://registry.test/token",
		},
	}

	mw := NewAuthMiddleware(log, cfg)
	router := gin.New()
	return router, mw
}

func TestRequireAuth_NoHeader(t *testing.T) {
	kp, _ := generateTestKeyPair()
	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)

	router.GET("/protected", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	// Should have WWW-Authenticate header
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected WWW-Authenticate header")
	}
}

func TestRequireAuth_InvalidHeaderFormat(t *testing.T) {
	kp, _ := generateTestKeyPair()
	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)

	router.GET("/protected", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic sometoken") // Wrong format
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuth_ValidToken(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)

	router.GET("/protected", mw.RequireAuth(), func(c *gin.Context) {
		user, exists := GetUser(c)
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"user_id": user.ID})
	})

	claims := jwt.MapClaims{
		"iss":    "https://test-issuer.com",
		"aud":    "test-client-id",
		"sub":    "user_123",
		"exp":    time.Now().Add(1 * time.Hour).Unix(),
		"iat":    time.Now().Unix(),
		"sid":    "session_456",
		"org_id": "org_789",
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)

	router.GET("/protected", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "test-client-id",
		"sub": "user_123",
		"exp": time.Now().Add(-1 * time.Hour).Unix(), // Expired
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d for expired token, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestGetUser_NotSet(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	user, exists := GetUser(c)

	if exists {
		t.Error("expected exists to be false")
	}
	if user != nil {
		t.Error("expected user to be nil")
	}
}

func TestGetSession_NotSet(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	session, exists := GetSession(c)

	if exists {
		t.Error("expected exists to be false")
	}
	if session != nil {
		t.Error("expected session to be nil")
	}
}

func TestGetUser_Set(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	expectedUser := &auth.User{
		ID:             "user_123",
		OrganizationID: "org_456",
	}
	c.Set(string(auth.UserContextKey), expectedUser)

	user, exists := GetUser(c)

	if !exists {
		t.Error("expected exists to be true")
	}
	if user.ID != expectedUser.ID {
		t.Errorf("expected user ID %s, got %s", expectedUser.ID, user.ID)
	}
}

func TestGetSession_Set(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	expectedSession := &auth.Session{
		ID:             "session_123",
		UserID:         "user_456",
		OrganizationID: "org_789",
	}
	c.Set(string(auth.SessionContextKey), expectedSession)

	session, exists := GetSession(c)

	if !exists {
		t.Error("expected exists to be true")
	}
	if session.ID != expectedSession.ID {
		t.Errorf("expected session ID %s, got %s", expectedSession.ID, session.ID)
	}
}

// --- Registry-token dual-mode tests ---

func mintRegistryToken(t *testing.T, access []auth.ResourceAccess) string {
	t.Helper()
	signer := auth.NewRegistryTokenSigner("test-secret", "astro-registry", "astro-registry", time.Hour)
	tok, _, _, err := signer.Issue("user_123", access)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return tok
}

func TestRequireAuth_RegistryToken_Accepted(t *testing.T) {
	kp, _ := generateTestKeyPair()
	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)
	router.PUT("/v2/*path", mw.RequireAuth(), func(c *gin.Context) {
		claims, ok := GetRegistryClaims(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no claims"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"sub": claims.Subject})
	})

	tok := mintRegistryToken(t, []auth.ResourceAccess{{
		Type:      "repository",
		Name:      "saswatds/myapp",
		Actions:   []string{"pull", "push"},
		AccountID: "acc_uuid",
	}})

	req := httptest.NewRequest(http.MethodPut, "/v2/saswatds/myapp/manifests/latest", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRequireAuth_RegistryToken_RejectedWrongScope(t *testing.T) {
	kp, _ := generateTestKeyPair()
	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)
	router.PUT("/v2/*path", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	tok := mintRegistryToken(t, []auth.ResourceAccess{{
		Type:    "repository",
		Name:    "saswatds/different-app",
		Actions: []string{"pull", "push"},
	}})

	// Request targets saswatds/myapp but token only grants saswatds/different-app.
	req := httptest.NewRequest(http.MethodPut, "/v2/saswatds/myapp/manifests/latest", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuth_RegistryToken_RejectedPullOnlyForPush(t *testing.T) {
	kp, _ := generateTestKeyPair()
	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)
	router.PUT("/v2/*path", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	// Only pull granted, request is a push.
	tok := mintRegistryToken(t, []auth.ResourceAccess{{
		Type:    "repository",
		Name:    "saswatds/myapp",
		Actions: []string{"pull"},
	}})

	req := httptest.NewRequest(http.MethodPut, "/v2/saswatds/myapp/manifests/latest", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("pull-only token must be rejected for push, got %d", rec.Code)
	}
}

func TestRequireAuth_RegistryToken_PushImpliesPull(t *testing.T) {
	kp, _ := generateTestKeyPair()
	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)
	router.GET("/v2/*path", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	// Only push granted; pull (GET) must succeed because push implies pull.
	tok := mintRegistryToken(t, []auth.ResourceAccess{{
		Type:    "repository",
		Name:    "saswatds/myapp",
		Actions: []string{"push"},
	}})

	req := httptest.NewRequest(http.MethodGet, "/v2/saswatds/myapp/manifests/latest", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("push token must allow pull, got %d", rec.Code)
	}
}

func TestRequireAuth_BearerChallenge_HasRealmAndScope(t *testing.T) {
	kp, _ := generateTestKeyPair()
	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)
	router.PUT("/v2/*path", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	req := httptest.NewRequest(http.MethodPut, "/v2/saswatds/myapp/blobs/uploads/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	challenge := rec.Header().Get("WWW-Authenticate")
	if !strings.HasPrefix(challenge, "Bearer ") {
		t.Errorf("expected Bearer challenge, got %q", challenge)
	}
	for _, want := range []string{
		`realm="https://registry.test/token"`,
		`service="astro-registry"`,
		`scope="repository:saswatds/myapp:push,pull"`,
	} {
		if !strings.Contains(challenge, want) {
			t.Errorf("challenge missing %s; got %s", want, challenge)
		}
	}
}

func TestRequireAuth_BearerChallenge_ScopeOmittedForShortPaths(t *testing.T) {
	kp, _ := generateTestKeyPair()
	server := createMockJWKSServer(kp)
	defer server.Close()

	router, mw := setupTestMiddleware(server.URL)
	router.GET("/v2/*path", mw.RequireAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	// /v2/ version check has no repository → no scope= in the challenge.
	req := httptest.NewRequest(http.MethodGet, "/v2/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	challenge := rec.Header().Get("WWW-Authenticate")
	if strings.Contains(challenge, "scope=") {
		t.Errorf("expected no scope= for /v2/, got %s", challenge)
	}
}

func TestRepositoryAndAction(t *testing.T) {
	t.Parallel()
	cases := []struct {
		method   string
		path     string
		wantRepo string
		wantAct  string
		wantOK   bool
	}{
		{http.MethodPut, "/v2/saswatds/myapp/manifests/latest", "saswatds/myapp", "push", true},
		{http.MethodGet, "/v2/saswatds/myapp/manifests/latest", "saswatds/myapp", "pull", true},
		{http.MethodHead, "/v2/saswatds/myapp/blobs/sha256:abc", "saswatds/myapp", "pull", true},
		{http.MethodPost, "/v2/saswatds/myapp/blobs/uploads/", "saswatds/myapp", "push", true},
		{http.MethodDelete, "/v2/saswatds/myapp/manifests/v1", "saswatds/myapp", "push", true},
		{http.MethodGet, "/v2/", "", "", false},
		{http.MethodGet, "/v2/_catalog", "", "", false},
		// Multi-segment repository names (not currently used by astro but spec-allowed).
		{http.MethodPut, "/v2/ns/sub/img/manifests/latest", "ns/sub/img", "push", true},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			repo, action, ok := repositoryAndAction(req)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if repo != tc.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tc.wantRepo)
			}
			if action != tc.wantAct {
				t.Errorf("action = %q, want %q", action, tc.wantAct)
			}
		})
	}
}
