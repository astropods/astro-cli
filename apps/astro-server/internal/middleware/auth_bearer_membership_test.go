package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestAuthenticateWithToken_SetsWorkOSMembershipIDFromClaim(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	keyID := "test-key-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwks := auth.JWKS{
			Keys: []auth.JWKSKey{
				{
					Kty: "RSA",
					Kid: keyID,
					Use: "sig",
					Alg: "RS256",
					N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	validator := auth.NewJWTValidator(server.URL, "https://test-issuer.com", "")

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":                        "https://test-issuer.com",
		"sub":                        "user_123",
		"exp":                        time.Now().Add(1 * time.Hour).Unix(),
		"iat":                        time.Now().Unix(),
		"sid":                        "session_456",
		"org_id":                     "org_01KNMZ60FGSGJN46KWY4C1HVQC",
		"organization_membership_id": "om_01KRC3M3HC3T700J1SZ173FWHH",
	})
	token.Header["kid"] = keyID
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	log := logger.New("error", "json")
	cfg := &config.Config{
		Auth: config.AuthConfig{
			CookieName:     "test_session",
			CookiePassword: "test-password-that-is-32-chars!!",
			SessionMaxAge:  24 * time.Hour,
		},
	}
	sm := auth.NewSessionManager(cfg.Auth.CookiePassword, cfg.Auth.SessionMaxAge)
	mw := NewAuthMiddleware(log, cfg, sm, validator)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request = c.Request.WithContext(context.Background())

	if !mw.authenticateWithToken(c, tokenString) {
		t.Fatal("authenticateWithToken() = false, want true")
	}

	session, ok := GetSession(c)
	if !ok {
		t.Fatal("GetSession() ok = false, want true")
	}
	if session.WorkOSMembershipID != "om_01KRC3M3HC3T700J1SZ173FWHH" {
		t.Fatalf("WorkOSMembershipID = %q, want om_01KRC3M3HC3T700J1SZ173FWHH", session.WorkOSMembershipID)
	}
}

func TestAuthenticateWithToken_LeavesWorkOSMembershipIDEmptyWithoutClaim(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	keyID := "test-key-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwks := auth.JWKS{
			Keys: []auth.JWKSKey{
				{
					Kty: "RSA",
					Kid: keyID,
					Use: "sig",
					Alg: "RS256",
					N:   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.PublicKey.E)).Bytes()),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	validator := auth.NewJWTValidator(server.URL, "https://test-issuer.com", "")

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":    "https://test-issuer.com",
		"sub":    "user_123",
		"exp":    time.Now().Add(1 * time.Hour).Unix(),
		"iat":    time.Now().Unix(),
		"sid":    "session_456",
		"org_id": "org_01KNMZ60FGSGJN46KWY4C1HVQC",
	})
	token.Header["kid"] = keyID
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	log := logger.New("error", "json")
	cfg := &config.Config{
		Auth: config.AuthConfig{
			CookieName:     "test_session",
			CookiePassword: "test-password-that-is-32-chars!!",
			SessionMaxAge:  24 * time.Hour,
		},
	}
	sm := auth.NewSessionManager(cfg.Auth.CookiePassword, cfg.Auth.SessionMaxAge)
	mw := NewAuthMiddleware(log, cfg, sm, validator)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request = c.Request.WithContext(context.Background())

	if !mw.authenticateWithToken(c, tokenString) {
		t.Fatal("authenticateWithToken() = false, want true")
	}

	session, ok := GetSession(c)
	if !ok {
		t.Fatal("GetSession() ok = false, want true")
	}
	if session.WorkOSMembershipID != "" {
		t.Fatalf("WorkOSMembershipID = %q, want empty without JWT claim (no DB fallback on Bearer path)", session.WorkOSMembershipID)
	}
}
