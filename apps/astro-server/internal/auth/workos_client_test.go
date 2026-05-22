package auth

import (
	"net/url"
	"strings"
	"testing"
)

func TestGetAuthorizationURL_IncludesState(t *testing.T) {
	client := &WorkOSClient{
		clientID:    "client_test123",
		redirectURI: "https://api.astro.dev/auth/callback",
		frontendURL: "https://app.astro.dev",
	}

	authURL, err := client.GetAuthorizationURL("test_state_abc", AuthorizationURLOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	state := parsedURL.Query().Get("state")
	if state != "test_state_abc" {
		t.Errorf("expected state 'test_state_abc', got %q", state)
	}
}

func TestGetAuthorizationURL_IncludesRedirectURI(t *testing.T) {
	client := &WorkOSClient{
		clientID:    "client_test123",
		redirectURI: "https://api.astro.dev/auth/callback",
		frontendURL: "https://app.astro.dev",
	}

	authURL, err := client.GetAuthorizationURL("state", AuthorizationURLOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	redirectURI := parsedURL.Query().Get("redirect_uri")
	if redirectURI != "https://api.astro.dev/auth/callback" {
		t.Errorf("expected redirect_uri 'https://api.astro.dev/auth/callback', got %q", redirectURI)
	}
}

func TestGetAuthorizationURL_UsesAuthKit(t *testing.T) {
	client := &WorkOSClient{
		clientID:    "client_test123",
		redirectURI: "https://api.astro.dev/auth/callback",
		frontendURL: "https://app.astro.dev",
	}

	authURL, err := client.GetAuthorizationURL("state", AuthorizationURLOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	provider := parsedURL.Query().Get("provider")
	if provider != "authkit" {
		t.Errorf("expected provider 'authkit', got %q", provider)
	}
}

func TestExtractSessionIDFromToken_ValidToken(t *testing.T) {
	// Create a test JWT with sid claim
	// Header: {"alg":"RS256","typ":"JWT"}
	// Payload: {"sid":"session_abc123","sub":"user_123"}
	// Note: This is a mock token, signature is not validated here
	header := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	// {"sid":"session_abc123","sub":"user_123","exp":9999999999}
	payload := "eyJzaWQiOiJzZXNzaW9uX2FiYzEyMyIsInN1YiI6InVzZXJfMTIzIiwiZXhwIjo5OTk5OTk5OTk5fQ"
	signature := "test_signature"

	token := header + "." + payload + "." + signature
	sessionID := extractSessionIDFromToken(token)

	if sessionID != "session_abc123" {
		t.Errorf("expected sessionID 'session_abc123', got %q", sessionID)
	}
}

func TestExtractSessionIDFromToken_NoPadding(t *testing.T) {
	// Test with base64 that needs padding
	// The payload has length that requires padding when decoding
	header := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	// {"sid":"sess_x","sub":"u"} - short values that result in non-padded base64
	// Base64 of {"sid":"sess_x","sub":"u"} without padding
	payload := "eyJzaWQiOiJzZXNzX3giLCJzdWIiOiJ1In0"
	signature := "sig"

	token := header + "." + payload + "." + signature
	sessionID := extractSessionIDFromToken(token)

	if sessionID != "sess_x" {
		t.Errorf("expected sessionID 'sess_x', got %q", sessionID)
	}
}

func TestExtractSessionIDFromToken_InvalidFormat(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "empty string",
			token: "",
		},
		{
			name:  "single part",
			token: "single_part_only",
		},
		{
			name:  "two parts",
			token: "header.payload",
		},
		{
			name:  "invalid base64 payload",
			token: "header.!!!invalid-base64!!!.signature",
		},
		{
			name:  "invalid JSON payload",
			token: "header.bm90LWpzb24.signature", // "not-json" in base64
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID := extractSessionIDFromToken(tt.token)
			if sessionID != "" {
				t.Errorf("expected empty sessionID for invalid token, got %q", sessionID)
			}
		})
	}
}

func TestExtractSessionIDFromToken_NoSidClaim(t *testing.T) {
	// Token without sid claim
	header := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9"
	// {"sub":"user_123","exp":9999999999} - no sid claim
	payload := "eyJzdWIiOiJ1c2VyXzEyMyIsImV4cCI6OTk5OTk5OTk5OX0"
	signature := "test_signature"

	token := header + "." + payload + "." + signature
	sessionID := extractSessionIDFromToken(token)

	if sessionID != "" {
		t.Errorf("expected empty sessionID when sid claim missing, got %q", sessionID)
	}
}

func TestBuildCallbackErrorURL_EncodesParams(t *testing.T) {
	client := &WorkOSClient{
		frontendURL: "https://app.astro.dev",
	}

	errorURL := client.BuildCallbackErrorURL("invalid_request", "Missing authorization code")

	parsedURL, err := url.Parse(errorURL)
	if err != nil {
		t.Fatalf("failed to parse error URL: %v", err)
	}

	if parsedURL.Host != "app.astro.dev" {
		t.Errorf("expected host 'app.astro.dev', got %q", parsedURL.Host)
	}

	if parsedURL.Query().Get("error") != "invalid_request" {
		t.Errorf("expected error 'invalid_request', got %q", parsedURL.Query().Get("error"))
	}

	if parsedURL.Query().Get("error_description") != "Missing authorization code" {
		t.Errorf("expected error_description 'Missing authorization code', got %q", parsedURL.Query().Get("error_description"))
	}
}

func TestBuildCallbackErrorURL_EncodesSpecialChars(t *testing.T) {
	client := &WorkOSClient{
		frontendURL: "https://app.astro.dev",
	}

	// Error description with special characters that need URL encoding
	errorURL := client.BuildCallbackErrorURL("error_code", "Error with spaces & special=chars")

	// Verify the URL is valid and can be parsed
	parsedURL, err := url.Parse(errorURL)
	if err != nil {
		t.Fatalf("failed to parse error URL: %v", err)
	}

	// The description should be properly encoded and decoded back
	desc := parsedURL.Query().Get("error_description")
	if desc != "Error with spaces & special=chars" {
		t.Errorf("expected properly decoded error_description, got %q", desc)
	}
}

func TestBuildCallbackSuccessURL_ReturnsBaseURL(t *testing.T) {
	client := &WorkOSClient{
		frontendURL: "https://app.astro.dev",
	}

	successURL := client.BuildCallbackSuccessURL()

	if successURL != "https://app.astro.dev" {
		t.Errorf("expected 'https://app.astro.dev', got %q", successURL)
	}
}

func TestBuildCallbackSuccessURL_PreservesPath(t *testing.T) {
	client := &WorkOSClient{
		frontendURL: "https://app.astro.dev/dashboard",
	}

	successURL := client.BuildCallbackSuccessURL()

	if successURL != "https://app.astro.dev/dashboard" {
		t.Errorf("expected 'https://app.astro.dev/dashboard', got %q", successURL)
	}
}

func TestBuildCallbackErrorURL_WithPath(t *testing.T) {
	client := &WorkOSClient{
		frontendURL: "https://app.astro.dev/auth/error",
	}

	errorURL := client.BuildCallbackErrorURL("test_error", "Test description")

	parsedURL, err := url.Parse(errorURL)
	if err != nil {
		t.Fatalf("failed to parse error URL: %v", err)
	}

	if !strings.HasPrefix(parsedURL.Path, "/auth/error") {
		t.Errorf("expected path to start with '/auth/error', got %q", parsedURL.Path)
	}
}
