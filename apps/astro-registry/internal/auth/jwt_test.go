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
		_ = json.NewEncoder(w).Encode(jwks)
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
		"iss":    "https://test-issuer.com",
		"aud":    "correct-audience",
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
	if result.OrganizationID != "org_789" {
		t.Errorf("expected org ID 'org_789', got %s", result.OrganizationID)
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

	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "correct-audience",
		"sub": "user_123",
		"exp": time.Now().Add(-1 * time.Hour).Unix(), // Expired
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	}

	tokenString, err := createTestToken(kp, claims)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for expired token")
	}

	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestJWTValidator_WrongAudience(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "correct-audience")

	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "wrong-audience",
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
		t.Fatal("expected error for wrong audience")
	}

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

	claims := jwt.MapClaims{
		"iss": "https://wrong-issuer.com",
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
		t.Fatal("expected error for wrong issuer")
	}
}

func TestJWTValidator_InvalidToken(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "correct-audience")

	_, err = validator.ValidateToken(context.Background(), "not-a-valid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}

	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTValidator_MissingKeyID(t *testing.T) {
	kp, err := generateTestKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	server := createMockJWKSServer(kp)
	defer server.Close()

	validator := NewJWTValidator(server.URL, "https://test-issuer.com", "correct-audience")

	// Create token without kid in header
	claims := jwt.MapClaims{
		"iss": "https://test-issuer.com",
		"aud": "correct-audience",
		"sub": "user_123",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	// Don't set kid
	tokenString, _ := token.SignedString(kp.privateKey)

	_, err = validator.ValidateToken(context.Background(), tokenString)
	if err == nil {
		t.Fatal("expected error for missing key ID")
	}
}

func TestBase64URLDecode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SGVsbG8", "Hello"},
		{"SGVsbG8gV29ybGQ", "Hello World"},
	}

	for _, tt := range tests {
		decoded, err := base64URLDecode(tt.input)
		if err != nil {
			t.Errorf("base64URLDecode(%s) failed: %v", tt.input, err)
			continue
		}
		if string(decoded) != tt.expected {
			t.Errorf("base64URLDecode(%s) = %s, want %s", tt.input, string(decoded), tt.expected)
		}
	}
}
