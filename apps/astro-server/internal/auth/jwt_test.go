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

// cachedTestKeyPair is a package-level cached key pair to avoid expensive RSA generation in each test
var cachedTestKeyPair *testKeyPair

func init() {
	// Generate once at package init - RSA key generation is expensive
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("failed to generate test key pair: " + err.Error())
	}
	cachedTestKeyPair = &testKeyPair{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		keyID:      "test-key-id",
	}
}

// generateTestKeyPair returns the cached RSA key pair for testing
func generateTestKeyPair() (*testKeyPair, error) {
	return cachedTestKeyPair, nil
}

// generateFreshTestKeyPair creates a new RSA key pair (for tests that need unique keys)
func generateFreshTestKeyPair() (*testKeyPair, error) {
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// Tests for WorkOS-specific issuer format handling
// WorkOS tokens use issuer format: https://api.workos.com/user_management/{client_id}
// The validator must accept this format when configured with base issuer https://api.workos.com

func TestJWTValidator_WorkOSIssuerFormat(t *testing.T) {
	t.Parallel()
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	// Validator configured with base WorkOS issuer
	validator := NewJWTValidator(server.URL, "https://api.workos.com", "client_abc123")

	// Token has WorkOS user_management issuer format (what WorkOS actually sends)
	claims := jwt.MapClaims{
		"iss": "https://api.workos.com/user_management/client_abc123",
		"aud": "client_abc123",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
		"sid": "session_456",
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Should succeed - WorkOS user_management issuer format is accepted
	result, err := validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed for WorkOS issuer format: %v", err)
	}

	if result.Subject != "user_123" {
		t.Errorf("expected subject 'user_123', got %s", result.Subject)
	}
}

func TestJWTValidator_WorkOSIssuerFormat_ExactMatch(t *testing.T) {
	t.Parallel()
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	// Validator configured with exact issuer (non-WorkOS use case)
	validator := NewJWTValidator(server.URL, "https://custom-issuer.com", "my-audience")

	// Token has exact matching issuer
	claims := jwt.MapClaims{
		"iss": "https://custom-issuer.com",
		"aud": "my-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Should succeed with exact issuer match
	result, err := validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed for exact issuer match: %v", err)
	}

	if result.Subject != "user_123" {
		t.Errorf("expected subject 'user_123', got %s", result.Subject)
	}
}

func TestJWTValidator_WorkOSIssuerFormat_RejectsOtherPaths(t *testing.T) {
	t.Parallel()
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	// Validator configured with base WorkOS issuer
	validator := NewJWTValidator(server.URL, "https://api.workos.com", "client_abc123")

	// Token has different WorkOS path (not user_management)
	claims := jwt.MapClaims{
		"iss": "https://api.workos.com/some_other_path/client_abc123",
		"aud": "client_abc123",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Should fail - only user_management path is accepted
	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for non-user_management WorkOS path, got nil")
	}

	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

// TestJWTValidator_StrictAudienceValidation verifies that audience validation
// is strict and rejects tokens that don't have the expected audience.
// This test documents why we can't use jwt.WithAudience() directly -
// it would fail with "aud claim is required" for tokens missing the claim.
func TestJWTValidator_StrictAudienceValidation(t *testing.T) {
	t.Parallel()
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	tests := []struct {
		name        string
		audience    interface{} // nil, string, or []string
		expectError bool
		errorType   error
	}{
		{
			name:        "missing audience claim",
			audience:    nil,
			expectError: true,
			errorType:   ErrInvalidToken,
		},
		{
			name:        "correct audience",
			audience:    "expected-audience",
			expectError: false,
		},
		{
			name:        "wrong audience",
			audience:    "wrong-audience",
			expectError: true,
			errorType:   ErrInvalidToken,
		},
		{
			name:        "audience in array",
			audience:    []string{"expected-audience", "other-audience"},
			expectError: false,
		},
		{
			name:        "audience not in array",
			audience:    []string{"wrong-audience", "other-audience"},
			expectError: true,
			errorType:   ErrInvalidToken,
		},
		{
			name:        "empty audience array",
			audience:    []string{},
			expectError: true,
			errorType:   ErrInvalidToken,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewJWTValidator(server.URL, "https://test-issuer.com", "expected-audience")

			claims := jwt.MapClaims{
				"iss": "https://test-issuer.com",
				"sub": "user_123",
				"exp": time.Now().Add(1 * time.Hour).Unix(),
				"iat": time.Now().Unix(),
			}

			if tc.audience != nil {
				claims["aud"] = tc.audience
			}

			tokenString, err := createTestToken(kp, claims)
			if err != nil {
				t.Fatalf("failed to create token: %v", err)
			}

			_, err = validator.ValidateToken(context.Background(), tokenString)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tc.name)
				}
				if tc.errorType != nil && !errors.Is(err, tc.errorType) {
					t.Errorf("expected %v, got %v", tc.errorType, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %s: %v", tc.name, err)
				}
			}
		})
	}
}

// TestJWTValidator_StrictIssuerValidation verifies that issuer validation
// is strict and rejects tokens with mismatched issuers.
// This test documents why we can't use jwt.WithIssuer() directly -
// it performs exact string matching and would reject WorkOS's
// "https://api.workos.com/user_management/client_id" format when
// configured with "https://api.workos.com".
func TestJWTValidator_StrictIssuerValidation(t *testing.T) {
	t.Parallel()
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	tests := []struct {
		name            string
		configuredIss   string
		tokenIss        string
		expectError     bool
		errorContains   string
	}{
		{
			name:          "exact issuer match",
			configuredIss: "https://example.com",
			tokenIss:      "https://example.com",
			expectError:   false,
		},
		{
			name:          "issuer mismatch",
			configuredIss: "https://example.com",
			tokenIss:      "https://other.com",
			expectError:   true,
			errorContains: "invalid issuer",
		},
		{
			name:          "WorkOS base accepts user_management path",
			configuredIss: "https://api.workos.com",
			tokenIss:      "https://api.workos.com/user_management/client_123",
			expectError:   false,
		},
		{
			name:          "WorkOS base rejects other paths",
			configuredIss: "https://api.workos.com",
			tokenIss:      "https://api.workos.com/sso/client_123",
			expectError:   true,
			errorContains: "invalid issuer",
		},
		{
			name:          "non-WorkOS issuer rejects path suffix",
			configuredIss: "https://example.com",
			tokenIss:      "https://example.com/user_management/client_123",
			expectError:   true,
			errorContains: "invalid issuer",
		},
		{
			name:          "issuer with trailing slash mismatch",
			configuredIss: "https://example.com",
			tokenIss:      "https://example.com/",
			expectError:   true,
			errorContains: "invalid issuer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			validator := NewJWTValidator(server.URL, tc.configuredIss, "test-audience")

			claims := jwt.MapClaims{
				"iss": tc.tokenIss,
				"aud": "test-audience",
				"sub": "user_123",
				"exp": time.Now().Add(1 * time.Hour).Unix(),
				"iat": time.Now().Unix(),
			}

			tokenString, err := createTestToken(kp, claims)
			if err != nil {
				t.Fatalf("failed to create token: %v", err)
			}

			_, err = validator.ValidateToken(context.Background(), tokenString)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tc.name)
				}
				if tc.errorContains != "" && !contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error containing %q, got %v", tc.errorContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %s: %v", tc.name, err)
				}
			}
		})
	}
}

// TestJWTValidator_EmptyAudienceConfig tests behavior when validator has no expected audience
func TestJWTValidator_EmptyAudienceConfig(t *testing.T) {
	t.Parallel()
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	// Validator with empty audience - should skip audience validation
	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "")

	// Token without audience claim
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Should succeed - no audience validation when not configured
	result, err := validator.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if result.Subject != "user_123" {
		t.Errorf("expected subject 'user_123', got %s", result.Subject)
	}
}

// helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
