package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testKeyPair holds an RSA key pair for testing
type testKeyPair struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
}

// generateTestKeyPair creates an RSA key pair for testing
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

// createTestToken creates a signed JWT for testing
func createTestToken(kp *testKeyPair, claims jwt.MapClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kp.keyID
	return token.SignedString(kp.privateKey)
}

// createMockJWKSServer creates a test server that serves a JWKS
func createMockJWKSServer(kp *testKeyPair) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Encode the public key as JWK
		jwks := JWKS{
			Keys: []JWKSKey{
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

func TestJWTValidator_ValidToken(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "correct-audience")

	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "correct-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"sid": "session_456",
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	result, err := validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if result.Subject != "user_123" {
		t.Errorf("expected subject 'user_123', got %s", result.Subject)
	}
	if result.SessionID != "session_456" {
		t.Errorf("expected session ID 'session_456', got %s", result.SessionID)
	}
}

func TestJWTValidator_WrongAudience(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	// Validator expects "correct-audience"
	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "correct-audience")

	// But token has "wrong-audience"
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "wrong-audience", // Different audience!
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for wrong audience, got nil")
	}

	// The error should indicate invalid token
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTValidator_WrongIssuer(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "correct-audience")

	// Token has wrong issuer
	claims := jwt.MapClaims{
		"iss": "https://wrong-issuer.com", // Different issuer!
		"aud": "correct-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for wrong issuer, got nil")
	}
}

func TestJWTValidator_ExpiredToken(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "correct-audience")

	// Token is already expired
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "correct-audience",
		"sub": "user_123",
		"exp": time.Now().Add(-1 * time.Hour).Unix(), // Expired!
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}

	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestJWTValidator_TokenFromDifferentApp(t *testing.T) {
	// This test simulates the "token confusion" attack scenario:
	// A token issued for a different application (different audience)
	// should be rejected even if the signature is valid.

	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	// Astro's validator expects its own client ID as audience
	astroValidator := NewJWTValidator(server.URL, "https://api.workos.com", "client_astro")

	// But the token was issued for a different WorkOS application
	tokenForOtherApp := jwt.MapClaims{
		"iss":    "https://api.workos.com", // Same issuer (WorkOS)
		"aud":    "client_other_app",       // Different application's client ID
		"sub":    "user_attacker",
		"exp":    time.Now().Add(1 * time.Hour).Unix(),
		"iat":    time.Now().Unix(),
		"sid":    "session_from_other_app",
		"org_id": "org_shared",
	}

	tokenString, err := createTestToken(kp, tokenForOtherApp)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// The token has a valid signature (same JWKS), valid issuer (same WorkOS),
	// but WRONG audience - it should be rejected
	_, err = astroValidator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("SECURITY: Token from different application was accepted! Audience validation may not be working.")
	}

	t.Logf("Correctly rejected token with wrong audience: %v", err)
}

func TestJWTValidator_MissingAudience(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "expected-audience")

	// Token has no audience claim at all
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		// No "aud" claim!
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for missing audience, got nil")
	}
}

func TestJWTValidator_MultipleAudiences(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "audience-one")

	// Token has multiple audiences, including the expected one
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": []string{"audience-one", "audience-two"}, // Multiple audiences
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Should succeed because "audience-one" is in the list
	result, err := validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed for token with multiple audiences: %v", err)
	}

	if result.Subject != "user_123" {
		t.Errorf("expected subject 'user_123', got %s", result.Subject)
	}
}
