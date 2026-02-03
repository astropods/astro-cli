package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/postman/astro/apps/astro-registry/internal/auth"
	"github.com/postman/astro/apps/astro-registry/internal/config"
	"github.com/postman/astro/apps/astro-registry/internal/logger"
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
		json.NewEncoder(w).Encode(jwks)
	}))
}

func setupTestMiddleware(jwksURL string) (*gin.Engine, *AuthMiddleware) {
	log := logger.New("error", "json")

	cfg := &config.Config{
		Auth: config.AuthConfig{
			Enabled:        true,
			JWKSEndpoint:   jwksURL,
			JWTIssuer:      "https://test-issuer.com",
			WorkOSClientID: "test-client-id",
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
