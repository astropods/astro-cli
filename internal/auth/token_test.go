package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// resetEnvToken resets the cached environment token for testing
func resetEnvToken() {
	envToken = ""
	envTokenOnce = sync.Once{}
}

// setupTokenTestDir creates a temp directory and sets HOME for testing
func setupTokenTestDir(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	return tmpDir, func() {}
}

// writeTokenTestCredentials writes credentials for token tests
func writeTokenTestCredentials(t *testing.T, creds *Credentials) {
	t.Helper()
	path, err := CredentialsPath("ast")
	if err != nil {
		t.Fatalf("failed to get credentials path: %v", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal credentials: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write credentials: %v", err)
	}
}

func TestShouldRefresh_NotExpiring(t *testing.T) {
	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	// Token expires in 1 hour (well beyond 5 min threshold)
	profile := &Profile{
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	if manager.shouldRefresh(profile) {
		t.Error("expected shouldRefresh to return false for token with >5min remaining")
	}
}

func TestShouldRefresh_NearExpiry(t *testing.T) {
	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	// Token expires in 3 minutes (within 5 min threshold)
	profile := &Profile{
		ExpiresAt: time.Now().Add(3 * time.Minute),
	}

	if !manager.shouldRefresh(profile) {
		t.Error("expected shouldRefresh to return true for token expiring within 5min")
	}
}

func TestShouldRefresh_JWTExpiredButStoredExpiryValid(t *testing.T) {
	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	profile := &Profile{
		AccessToken: makeTestJWT(time.Now().Add(-10 * time.Minute)),
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}

	if !manager.shouldRefresh(profile) {
		t.Error("expected shouldRefresh to return true when JWT exp is past but stored ExpiresAt is future")
	}
}

func TestShouldRefresh_ZeroExpiry(t *testing.T) {
	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	// Zero expiry time (not set) — should refresh to be safe
	profile := &Profile{
		ExpiresAt: time.Time{},
	}

	if !manager.shouldRefresh(profile) {
		t.Error("expected shouldRefresh to return true when ExpiresAt is zero (unknown expiry should trigger refresh)")
	}
}

func makeTestJWT(exp time.Time) string {
	payload, err := json.Marshal(map[string]any{"exp": exp.Unix()})
	if err != nil {
		panic(err)
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return "header." + enc + ".sig"
}

func TestGetValidAccessToken_RefreshesWhenJWTExpiredButStoredExpiryValid(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	_ = os.Unsetenv(EnvAccessToken)

	_, cleanup := setupTokenTestDir(t)
	defer cleanup()

	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": {
				AccessToken:  makeTestJWT(time.Now().Add(-10 * time.Minute)),
				RefreshToken: "valid_refresh_token",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
			},
		},
	}
	writeTokenTestCredentials(t, testCreds)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "refreshed_access_token",
			RefreshToken: "new_refresh_token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	}))
	defer server.Close()

	manager := &TokenManager{
		storage: createTestStorage(),
		client: &Client{
			clientID:   "test_client",
			baseURL:    server.URL,
			httpClient: &http.Client{Timeout: 5 * time.Second},
		},
	}

	token, err := manager.GetValidAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "refreshed_access_token" {
		t.Errorf("expected token 'refreshed_access_token', got %q", token)
	}
}

func TestGetValidAccessToken_EnvVarOverride(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	// Set env var
	t.Setenv(EnvAccessToken, "env_access_token_123")

	manager := NewTokenManager("ast")
	token, err := manager.GetValidAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token != "env_access_token_123" {
		t.Errorf("expected token 'env_access_token_123', got %q", token)
	}
}

func TestGetValidAccessToken_ValidToken(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	// Ensure no env var is set
	_ = os.Unsetenv(EnvAccessToken)

	_, cleanup := setupTokenTestDir(t)
	defer cleanup()

	// Create valid credentials with plenty of time remaining
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": {
				AccessToken:  "stored_access_token",
				RefreshToken: "stored_refresh_token",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
			},
		},
	}
	writeTokenTestCredentials(t, testCreds)

	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	token, err := manager.GetValidAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token != "stored_access_token" {
		t.Errorf("expected token 'stored_access_token', got %q", token)
	}
}

func TestGetValidAccessToken_RefreshesExpiring(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	_ = os.Unsetenv(EnvAccessToken)

	_, cleanup := setupTokenTestDir(t)
	defer cleanup()

	// Create credentials that are about to expire
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": {
				AccessToken:  "old_access_token",
				RefreshToken: "valid_refresh_token",
				ExpiresAt:    time.Now().Add(2 * time.Minute), // About to expire
			},
		},
	}
	writeTokenTestCredentials(t, testCreds)

	// Create mock server for token refresh
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "refreshed_access_token",
			RefreshToken: "new_refresh_token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	}))
	defer server.Close()

	manager := &TokenManager{
		storage: createTestStorage(),
		client: &Client{
			clientID:   "test_client",
			baseURL:    server.URL,
			httpClient: &http.Client{Timeout: 5 * time.Second},
		},
	}

	token, err := manager.GetValidAccessToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if token != "refreshed_access_token" {
		t.Errorf("expected token 'refreshed_access_token', got %q", token)
	}
}

func TestGetValidAccessToken_NoRefreshToken(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	_ = os.Unsetenv(EnvAccessToken)

	_, cleanup := setupTokenTestDir(t)
	defer cleanup()

	// Create credentials that are expired but have no refresh token
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": {
				AccessToken:  "expired_access_token",
				RefreshToken: "", // No refresh token
				ExpiresAt:    time.Now().Add(2 * time.Minute),
			},
		},
	}
	writeTokenTestCredentials(t, testCreds)

	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	_, err := manager.GetValidAccessToken(context.Background())
	if err == nil {
		t.Fatal("expected error when no refresh token available, got nil")
	}

	expectedMsg := "token expired and no refresh token available"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestIsAuthenticated_EnvVar(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	t.Setenv(EnvAccessToken, "env_token")

	manager := NewTokenManager("ast")
	if !manager.IsAuthenticated() {
		t.Error("expected IsAuthenticated to return true when env var is set")
	}
}

func TestIsAuthenticated_ValidCredentials(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	_ = os.Unsetenv(EnvAccessToken)

	_, cleanup := setupTokenTestDir(t)
	defer cleanup()

	// Create valid credentials
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": {
				AccessToken: "valid_token",
				ExpiresAt:   time.Now().Add(1 * time.Hour),
			},
		},
	}
	writeTokenTestCredentials(t, testCreds)

	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	if !manager.IsAuthenticated() {
		t.Error("expected IsAuthenticated to return true with valid stored credentials")
	}
}

func TestIsAuthenticated_NotAuthenticated(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	_ = os.Unsetenv(EnvAccessToken)

	_, cleanup := setupTokenTestDir(t)
	defer cleanup()

	// Don't create any credentials file

	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	if manager.IsAuthenticated() {
		t.Error("expected IsAuthenticated to return false with no credentials")
	}
}

func TestRequireAuth_NotAuthenticated(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	_ = os.Unsetenv(EnvAccessToken)

	_, cleanup := setupTokenTestDir(t)
	defer cleanup()

	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	err := manager.RequireAuth()
	if err == nil {
		t.Fatal("expected error from RequireAuth when not authenticated, got nil")
	}

	expectedMsg := "not authenticated. Run 'ast login' to authenticate"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestRequireAuth_Authenticated(t *testing.T) {
	resetEnvToken()
	defer resetEnvToken()

	t.Setenv(EnvAccessToken, "valid_token")

	manager := NewTokenManager("ast")
	err := manager.RequireAuth()
	if err != nil {
		t.Errorf("expected no error from RequireAuth when authenticated, got %v", err)
	}
}

func TestGetCurrentUser_Success(t *testing.T) {
	_, cleanup := setupTokenTestDir(t)
	defer cleanup()

	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": {
				AccessToken: "token",
				ExpiresAt:   time.Now().Add(1 * time.Hour),
				User: &StoredUser{
					ID:        "user_123",
					Email:     "test@example.com",
					FirstName: "Test",
					LastName:  "User",
				},
			},
		},
	}
	writeTokenTestCredentials(t, testCreds)

	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	user, err := manager.GetCurrentUser()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ID != "user_123" {
		t.Errorf("expected user ID 'user_123', got %q", user.ID)
	}
	if user.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %q", user.Email)
	}
}

func TestGetCurrentUser_NoUser(t *testing.T) {
	_, cleanup := setupTokenTestDir(t)
	defer cleanup()

	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": {
				AccessToken: "token",
				ExpiresAt:   time.Now().Add(1 * time.Hour),
				User:        nil, // No user info
			},
		},
	}
	writeTokenTestCredentials(t, testCreds)

	manager := &TokenManager{
		storage: createTestStorage(),
		client:  NewClient(),
	}

	_, err := manager.GetCurrentUser()
	if err == nil {
		t.Fatal("expected error when no user info, got nil")
	}
}
