package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWKSCache_InitialFetch(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	var fetchCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetchCount, 1)
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
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "test-audience")

	// Create a valid token
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "test-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// First validation should fetch JWKS
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if atomic.LoadInt32(&fetchCount) != 1 {
		t.Errorf("expected 1 JWKS fetch, got %d", fetchCount)
	}
}

func TestJWKSCache_UsesCachedKeys(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	var fetchCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetchCount, 1)
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
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "test-audience")

	// Create a valid token
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "test-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// First validation - fetches JWKS
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("first ValidateToken failed: %v", err)
	}

	// Second validation - should use cached JWKS
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("second ValidateToken failed: %v", err)
	}

	// Third validation - should still use cached JWKS
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("third ValidateToken failed: %v", err)
	}

	// Should only have fetched once since cache hasn't expired
	if atomic.LoadInt32(&fetchCount) != 1 {
		t.Errorf("expected 1 JWKS fetch (using cache), got %d", fetchCount)
	}
}

func TestJWKSCache_RefreshesExpiredCache(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	var fetchCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetchCount, 1)
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
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "test-audience")
	// Set a very short cache TTL for testing
	validator.cacheTTL = 10 * time.Millisecond

	// Create a valid token
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "test-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// First validation - fetches JWKS
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("first ValidateToken failed: %v", err)
	}

	if atomic.LoadInt32(&fetchCount) != 1 {
		t.Errorf("expected 1 JWKS fetch after first validation, got %d", fetchCount)
	}

	// Wait for cache to expire
	time.Sleep(50 * time.Millisecond)

	// Second validation - should fetch again due to expired cache
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("second ValidateToken failed: %v", err)
	}

	if atomic.LoadInt32(&fetchCount) != 2 {
		t.Errorf("expected 2 JWKS fetches after cache expiry, got %d", fetchCount)
	}
}

func TestJWKSCache_HandlesServerError(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	var fetchCount int32
	serverError := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetchCount, 1)
		if serverError {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
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
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "test-audience")
	// Set a very short cache TTL for testing
	validator.cacheTTL = 10 * time.Millisecond

	// Create a valid token
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "test-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// First validation - fetches JWKS successfully
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("first ValidateToken failed: %v", err)
	}

	// Wait for cache to expire
	time.Sleep(50 * time.Millisecond)

	// Make server return errors
	serverError = true

	// Second validation - server error, but should use stale cache (graceful degradation)
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("second ValidateToken failed (should use stale cache): %v", err)
	}

	// Verify refresh was attempted
	if atomic.LoadInt32(&fetchCount) < 2 {
		t.Errorf("expected at least 2 JWKS fetch attempts, got %d", fetchCount)
	}
}

func TestJWKSCache_NoStaleCache_ServerError(t *testing.T) {
	// Test case where server errors and there's no stale cache to fall back on
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "test-audience")

	// Create a mock token (won't be validated anyway since JWKS fetch fails)
	kp, _ := rsa.GenerateKey(rand.Reader, 2048)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "test-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	})
	token.Header["kid"] = "test-key"
	tokenString, _ := token.SignedString(kp)

	// Validation should fail because JWKS can't be fetched
	_, err := validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error when JWKS fetch fails and no cache, got nil")
	}
}

func TestJWKSCache_KeyRotation(t *testing.T) {
	// Test that a new key ID triggers a refresh attempt
	kp1, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair 1: %v", err)
	}
	kp1.keyID = "key-1"

	kp2, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair 2: %v", err)
	}
	kp2.keyID = "key-2"

	var currentKeyPair *testKeyPair = kp1
	var fetchCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&fetchCount, 1)
		jwks := JWKS{
			Keys: []JWKSKey{
				{
					Kty: "RSA",
					Kid: currentKeyPair.keyID,
					Use: "sig",
					Alg: "RS256",
					N:   base64.RawURLEncoding.EncodeToString(currentKeyPair.publicKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(currentKeyPair.publicKey.E)).Bytes()),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	}))
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "test-audience")
	validator.cacheTTL = 10 * time.Millisecond

	// Create and validate token with first key
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "test-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	token1, _ := createTestToken(kp1, claims)
	_, err = validator.ValidateToken(context.Background(), token1)
	if err != nil {
		t.Fatalf("first token validation failed: %v", err)
	}

	// Simulate key rotation
	currentKeyPair = kp2
	time.Sleep(50 * time.Millisecond) // Wait for cache to expire

	// Create token with new key
	token2, _ := createTestToken(kp2, claims)
	_, err = validator.ValidateToken(context.Background(), token2)
	if err != nil {
		t.Fatalf("second token validation (after key rotation) failed: %v", err)
	}

	// Should have fetched JWKS at least twice (initial + after rotation)
	if atomic.LoadInt32(&fetchCount) < 2 {
		t.Errorf("expected at least 2 JWKS fetches after key rotation, got %d", fetchCount)
	}
}
