package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestGetAuthorizationURL_InvitationToken(t *testing.T) {
	client := &WorkOSClient{
		clientID:    "client_test",
		redirectURI: "http://localhost:8080/auth/callback",
	}

	t.Run("empty opts — no invitation_token in URL", func(t *testing.T) {
		u, err := client.GetAuthorizationURL("state123", AuthorizationURLOpts{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(u, "invitation_token") {
			t.Errorf("expected no invitation_token in URL, got %s", u)
		}
	})

	t.Run("empty InvitationToken — no invitation_token in URL", func(t *testing.T) {
		u, err := client.GetAuthorizationURL("state123", AuthorizationURLOpts{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(u, "invitation_token") {
			t.Errorf("expected no invitation_token in URL, got %s", u)
		}
	})

	t.Run("InvitationToken set — appended to URL", func(t *testing.T) {
		u, err := client.GetAuthorizationURL("state123", AuthorizationURLOpts{InvitationToken: "tok_abc"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(u, "invitation_token=tok_abc") {
			t.Errorf("expected invitation_token=tok_abc in URL, got %s", u)
		}
	})

	t.Run("InvitationToken does not replace state", func(t *testing.T) {
		u, err := client.GetAuthorizationURL("state123", AuthorizationURLOpts{InvitationToken: "tok_abc"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(u, "state=state123") {
			t.Errorf("expected state=state123 still in URL, got %s", u)
		}
	})
}

func TestExtractSessionIDFromToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "valid token with session ID",
			token:    createTestJWT(map[string]interface{}{"sid": "session_12345", "sub": "user_123"}),
			expected: "session_12345",
		},
		{
			name:     "valid token without session ID",
			token:    createTestJWT(map[string]interface{}{"sub": "user_123"}),
			expected: "",
		},
		{
			name:     "empty token",
			token:    "",
			expected: "",
		},
		{
			name:     "invalid token format - no dots",
			token:    "not-a-jwt",
			expected: "",
		},
		{
			name:     "invalid token format - only one part",
			token:    "header.payload",
			expected: "",
		},
		{
			name:     "invalid base64 in payload",
			token:    "header.!!invalid!!.signature",
			expected: "",
		},
		{
			name:     "invalid JSON in payload",
			token:    "header." + base64.URLEncoding.EncodeToString([]byte("not json")) + ".signature",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSessionIDFromToken(tt.token)
			if got != tt.expected {
				t.Errorf("extractSessionIDFromToken() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildCallbackErrorURL(t *testing.T) {
	client := &WorkOSClient{
		frontendURL: "http://localhost:5173",
	}

	url := client.BuildCallbackErrorURL("auth_failed", "Authentication failed")

	if !strings.HasPrefix(url, "http://localhost:5173") {
		t.Errorf("URL should start with frontend URL, got %s", url)
	}
	if !strings.Contains(url, "error=auth_failed") {
		t.Errorf("URL should contain error param, got %s", url)
	}
	if !strings.Contains(url, "error_description=Authentication") {
		t.Errorf("URL should contain error_description param, got %s", url)
	}
}

func TestBuildCallbackErrorURL_SpecialCharacters(t *testing.T) {
	client := &WorkOSClient{
		frontendURL: "http://localhost:5173",
	}

	url := client.BuildCallbackErrorURL("error_code", "Error with spaces & special=chars")

	// URL should be properly encoded
	if strings.Contains(url, " ") {
		t.Errorf("URL should not contain unencoded spaces, got %s", url)
	}
	if !strings.Contains(url, "error_description=") {
		t.Errorf("URL should contain error_description param, got %s", url)
	}
}

func TestBuildCallbackSuccessURL(t *testing.T) {
	client := &WorkOSClient{
		frontendURL: "http://localhost:5173",
	}

	url := client.BuildCallbackSuccessURL()

	if url != "http://localhost:5173" {
		t.Errorf("BuildCallbackSuccessURL() = %s, want http://localhost:5173", url)
	}
}

func TestBuildCallbackSuccessURL_WithPath(t *testing.T) {
	client := &WorkOSClient{
		frontendURL: "http://localhost:5173/app",
	}

	url := client.BuildCallbackSuccessURL()

	if url != "http://localhost:5173/app" {
		t.Errorf("BuildCallbackSuccessURL() = %s, want http://localhost:5173/app", url)
	}
}

// createTestJWT creates a minimal JWT for testing
// Note: This is NOT a valid signed JWT, just structured for testing payload extraction
func createTestJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))

	payload, err := json.Marshal(claims)
	if err != nil {
		panic("failed to marshal claims: " + err.Error())
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)

	signature := base64.RawURLEncoding.EncodeToString([]byte("fake-signature"))

	return header + "." + encodedPayload + "." + signature
}
