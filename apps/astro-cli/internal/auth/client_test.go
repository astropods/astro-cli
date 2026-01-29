package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// createMockWorkOSServer creates a test server simulating WorkOS API
func createMockWorkOSServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// createTestClient creates a Client pointing to a test server
func createTestClient(baseURL string) *Client {
	return &Client{
		clientID: "test_client_id",
		baseURL:  baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func TestRequestDeviceAuthorization_Success(t *testing.T) {
	expectedResp := DeviceAuthorizationResponse{
		DeviceCode:              "dev_abc123",
		UserCode:                "ABCD-EFGH",
		VerificationURI:         "https://auth.workos.com/device",
		VerificationURIComplete: "https://auth.workos.com/device?user_code=ABCD-EFGH",
		ExpiresIn:               600,
		Interval:                10,
	}

	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/user_management/authorize/device" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedResp)
	})
	defer server.Close()

	client := createTestClient(server.URL)
	resp, err := client.RequestDeviceAuthorization(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.DeviceCode != expectedResp.DeviceCode {
		t.Errorf("expected DeviceCode %q, got %q", expectedResp.DeviceCode, resp.DeviceCode)
	}
	if resp.UserCode != expectedResp.UserCode {
		t.Errorf("expected UserCode %q, got %q", expectedResp.UserCode, resp.UserCode)
	}
	if resp.VerificationURI != expectedResp.VerificationURI {
		t.Errorf("expected VerificationURI %q, got %q", expectedResp.VerificationURI, resp.VerificationURI)
	}
	if resp.ExpiresIn != expectedResp.ExpiresIn {
		t.Errorf("expected ExpiresIn %d, got %d", expectedResp.ExpiresIn, resp.ExpiresIn)
	}
	if resp.Interval != expectedResp.Interval {
		t.Errorf("expected Interval %d, got %d", expectedResp.Interval, resp.Interval)
	}
}

func TestRequestDeviceAuthorization_DefaultValues(t *testing.T) {
	// Return a response without ExpiresIn and Interval
	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_code":      "dev_xyz",
			"user_code":        "TEST-CODE",
			"verification_uri": "https://auth.workos.com/device",
			// No expires_in or interval
		})
	})
	defer server.Close()

	client := createTestClient(server.URL)
	resp, err := client.RequestDeviceAuthorization(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should set default values
	if resp.ExpiresIn != 300 {
		t.Errorf("expected default ExpiresIn 300, got %d", resp.ExpiresIn)
	}
	if resp.Interval != 5 {
		t.Errorf("expected default Interval 5, got %d", resp.Interval)
	}
}

func TestRequestDeviceAuthorization_HTTPError(t *testing.T) {
	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TokenError{
			Error:            "invalid_request",
			ErrorDescription: "Client ID is invalid",
		})
	})
	defer server.Close()

	client := createTestClient(server.URL)
	_, err := client.RequestDeviceAuthorization(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 400 response, got nil")
	}
}

func TestPollForTokens_Success(t *testing.T) {
	var pollCount int32

	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&pollCount, 1)

		w.Header().Set("Content-Type", "application/json")

		// Return authorization_pending for first 2 requests, then success
		if count < 3 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(TokenError{
				Error:            ErrorAuthorizationPending,
				ErrorDescription: "User has not yet authorized",
			})
			return
		}

		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "access_token_123",
			RefreshToken: "refresh_token_456",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	})
	defer server.Close()

	client := createTestClient(server.URL)
	resp, err := client.PollForTokens(context.Background(), "device_code", 1, 60)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.AccessToken != "access_token_123" {
		t.Errorf("expected AccessToken 'access_token_123', got %q", resp.AccessToken)
	}
	if resp.RefreshToken != "refresh_token_456" {
		t.Errorf("expected RefreshToken 'refresh_token_456', got %q", resp.RefreshToken)
	}
	if atomic.LoadInt32(&pollCount) != 3 {
		t.Errorf("expected 3 poll requests, got %d", pollCount)
	}
}

func TestPollForTokens_AccessDenied(t *testing.T) {
	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TokenError{
			Error:            ErrorAccessDenied,
			ErrorDescription: "User denied the request",
		})
	})
	defer server.Close()

	client := createTestClient(server.URL)
	_, err := client.PollForTokens(context.Background(), "device_code", 1, 60)
	if err == nil {
		t.Fatal("expected error for access_denied, got nil")
	}

	expectedMsg := "authorization was denied by the user"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestPollForTokens_ExpiredToken(t *testing.T) {
	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TokenError{
			Error:            ErrorExpiredToken,
			ErrorDescription: "Device code has expired",
		})
	})
	defer server.Close()

	client := createTestClient(server.URL)
	_, err := client.PollForTokens(context.Background(), "device_code", 1, 60)
	if err == nil {
		t.Fatal("expected error for expired_token, got nil")
	}

	expectedMsg := "device code has expired"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestPollForTokens_ContextCancellation(t *testing.T) {
	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TokenError{
			Error:            ErrorAuthorizationPending,
			ErrorDescription: "Waiting for user",
		})
	})
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	client := createTestClient(server.URL)
	_, err := client.PollForTokens(ctx, "device_code", 1, 60)
	if err == nil {
		t.Fatal("expected error for context cancellation, got nil")
	}

	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestRefreshAccessToken_Success(t *testing.T) {
	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/user_management/authenticate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Parse form data
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		if r.FormValue("grant_type") != "refresh_token" {
			t.Errorf("expected grant_type 'refresh_token', got %q", r.FormValue("grant_type"))
		}
		if r.FormValue("refresh_token") != "old_refresh_token" {
			t.Errorf("expected refresh_token 'old_refresh_token', got %q", r.FormValue("refresh_token"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "new_access_token",
			RefreshToken: "new_refresh_token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	})
	defer server.Close()

	client := createTestClient(server.URL)
	resp, err := client.RefreshAccessToken(context.Background(), "old_refresh_token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.AccessToken != "new_access_token" {
		t.Errorf("expected AccessToken 'new_access_token', got %q", resp.AccessToken)
	}
	if resp.RefreshToken != "new_refresh_token" {
		t.Errorf("expected RefreshToken 'new_refresh_token', got %q", resp.RefreshToken)
	}
}

func TestRefreshAccessToken_InvalidToken(t *testing.T) {
	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TokenError{
			Error:            "invalid_grant",
			ErrorDescription: "Refresh token is invalid or expired",
		})
	})
	defer server.Close()

	client := createTestClient(server.URL)
	_, err := client.RefreshAccessToken(context.Background(), "invalid_refresh_token")
	if err == nil {
		t.Fatal("expected error for invalid refresh token, got nil")
	}
}

func TestPollError_Error(t *testing.T) {
	tests := []struct {
		name     string
		pollErr  PollError
		expected string
	}{
		{
			name: "with description",
			pollErr: PollError{
				Code:        "authorization_pending",
				Description: "User has not yet authorized",
			},
			expected: "authorization_pending: User has not yet authorized",
		},
		{
			name: "without description",
			pollErr: PollError{
				Code: "access_denied",
			},
			expected: "access_denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pollErr.Error() != tt.expected {
				t.Errorf("expected error %q, got %q", tt.expected, tt.pollErr.Error())
			}
		})
	}
}

func TestPollForTokens_Timeout(t *testing.T) {
	server := createMockWorkOSServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TokenError{
			Error:            ErrorAuthorizationPending,
			ErrorDescription: "Waiting for user",
		})
	})
	defer server.Close()

	client := createTestClient(server.URL)
	// Use very short expiry to test timeout
	_, err := client.PollForTokens(context.Background(), "device_code", 1, 1)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	expectedMsg := "device authorization expired"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}
