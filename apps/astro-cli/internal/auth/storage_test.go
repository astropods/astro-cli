package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupTestDir creates a temp directory and sets XDG_CONFIG_HOME for testing
func setupTestDir(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "astro-cli-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Save original XDG_CONFIG_HOME
	originalXDG := os.Getenv("XDG_CONFIG_HOME")

	// Set XDG_CONFIG_HOME to temp dir so CredentialsPath uses it
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	return tmpDir, func() {
		// Restore original XDG_CONFIG_HOME
		if originalXDG == "" {
			os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			os.Setenv("XDG_CONFIG_HOME", originalXDG)
		}
		os.RemoveAll(tmpDir)
	}
}

// createTestStorage creates a Storage that doesn't use keyring
func createTestStorage() *Storage {
	return &Storage{useKeyring: false}
}

// writeTestCredentials writes credentials JSON to the test credentials path
func writeTestCredentials(t *testing.T, creds *Credentials) string {
	t.Helper()
	path, err := CredentialsPath()
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
	return path
}

// createTestProfile creates a profile with specified parameters
func createTestProfile(accessToken, refreshToken string, expiresAt time.Time) *Profile {
	return &Profile{
		ServerURL:    "https://api.astro.dev",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		User: &StoredUser{
			ID:    "user_123",
			Email: "test@example.com",
		},
	}
}

func TestLoadCredentials_FileNotExists(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	storage := createTestStorage()
	creds, err := storage.LoadCredentials()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if creds.CurrentProfile != "default" {
		t.Errorf("expected CurrentProfile 'default', got %q", creds.CurrentProfile)
	}
	if len(creds.Profiles) != 0 {
		t.Errorf("expected empty profiles map, got %d profiles", len(creds.Profiles))
	}
}

func TestLoadCredentials_ValidFile(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create test credentials
	testCreds := &Credentials{
		CurrentProfile: "production",
		Profiles: map[string]*Profile{
			"production": {
				ServerURL:    "https://api.astro.dev",
				AccessToken:  "access_123",
				RefreshToken: "refresh_456",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				User: &StoredUser{
					ID:    "user_abc",
					Email: "user@example.com",
				},
			},
		},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	creds, err := storage.LoadCredentials()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if creds.CurrentProfile != "production" {
		t.Errorf("expected CurrentProfile 'production', got %q", creds.CurrentProfile)
	}
	if len(creds.Profiles) != 1 {
		t.Errorf("expected 1 profile, got %d", len(creds.Profiles))
	}

	profile := creds.Profiles["production"]
	if profile == nil {
		t.Fatal("expected 'production' profile to exist")
	}
	if profile.AccessToken != "access_123" {
		t.Errorf("expected AccessToken 'access_123', got %q", profile.AccessToken)
	}
	if profile.User.Email != "user@example.com" {
		t.Errorf("expected email 'user@example.com', got %q", profile.User.Email)
	}
}

func TestLoadCredentials_CorruptedFile(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	path, err := CredentialsPath()
	if err != nil {
		t.Fatalf("failed to get credentials path: %v", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	// Write invalid JSON
	if err := os.WriteFile(path, []byte("{invalid json"), 0600); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	storage := createTestStorage()
	_, err = storage.LoadCredentials()
	if err == nil {
		t.Fatal("expected error for corrupted file, got nil")
	}
}

func TestSaveCredentials_CreatesDirectory(t *testing.T) {
	tmpDir, cleanup := setupTestDir(t)
	defer cleanup()

	// Verify the astro subdirectory doesn't exist yet
	astroDir := filepath.Join(tmpDir, "astro")
	if _, err := os.Stat(astroDir); !os.IsNotExist(err) {
		t.Fatal("expected astro directory to not exist initially")
	}

	storage := createTestStorage()
	creds := &Credentials{
		CurrentProfile: "default",
		Profiles:       map[string]*Profile{},
	}

	err := storage.SaveCredentials(creds)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify the file was created
	path, _ := CredentialsPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected credentials file to be created")
	}

	// Verify the directory was created
	info, err := os.Stat(astroDir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory to be created")
	}
}

func TestSaveCredentials_FilePermissions(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	storage := createTestStorage()
	creds := &Credentials{
		CurrentProfile: "default",
		Profiles:       map[string]*Profile{},
	}

	err := storage.SaveCredentials(creds)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	path, _ := CredentialsPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	// Check file permissions are 0600 (owner read/write only)
	perms := info.Mode().Perm()
	if perms != 0600 {
		t.Errorf("expected file permissions 0600, got %04o", perms)
	}
}

func TestSaveProfile_NewProfile(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	storage := createTestStorage()

	// Save a new profile
	profile := createTestProfile("token_abc", "refresh_xyz", time.Now().Add(1*time.Hour))
	err := storage.SaveProfile("staging", profile)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Load and verify
	creds, err := storage.LoadCredentials()
	if err != nil {
		t.Fatalf("failed to load credentials: %v", err)
	}

	if _, ok := creds.Profiles["staging"]; !ok {
		t.Error("expected 'staging' profile to exist")
	}
	if creds.Profiles["staging"].AccessToken != "token_abc" {
		t.Errorf("expected AccessToken 'token_abc', got %q", creds.Profiles["staging"].AccessToken)
	}
}

func TestSaveProfile_UpdateExisting(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	storage := createTestStorage()

	// Save initial profile
	initialProfile := createTestProfile("old_token", "old_refresh", time.Now().Add(1*time.Hour))
	err := storage.SaveProfile("default", initialProfile)
	if err != nil {
		t.Fatalf("failed to save initial profile: %v", err)
	}

	// Update the profile
	updatedProfile := createTestProfile("new_token", "new_refresh", time.Now().Add(2*time.Hour))
	err = storage.SaveProfile("default", updatedProfile)
	if err != nil {
		t.Fatalf("failed to update profile: %v", err)
	}

	// Verify update
	creds, err := storage.LoadCredentials()
	if err != nil {
		t.Fatalf("failed to load credentials: %v", err)
	}

	if creds.Profiles["default"].AccessToken != "new_token" {
		t.Errorf("expected AccessToken 'new_token', got %q", creds.Profiles["default"].AccessToken)
	}
}

func TestDeleteProfile_Exists(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create credentials with a profile
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"staging": createTestProfile("token", "refresh", time.Now().Add(1*time.Hour)),
		},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	err := storage.DeleteProfile("staging")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify deletion
	creds, err := storage.LoadCredentials()
	if err != nil {
		t.Fatalf("failed to load credentials: %v", err)
	}

	if _, ok := creds.Profiles["staging"]; ok {
		t.Error("expected 'staging' profile to be deleted")
	}
}

func TestDeleteProfile_NotExists(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create empty credentials
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles:       map[string]*Profile{},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	err := storage.DeleteProfile("nonexistent")
	// Should not return an error when deleting non-existent profile
	if err != nil {
		t.Fatalf("expected no error when deleting non-existent profile, got %v", err)
	}
}

func TestSetCurrentProfile_Valid(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create credentials with multiple profiles
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default":    createTestProfile("token1", "refresh1", time.Now().Add(1*time.Hour)),
			"production": createTestProfile("token2", "refresh2", time.Now().Add(1*time.Hour)),
		},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	err := storage.SetCurrentProfile("production")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify the switch
	creds, err := storage.LoadCredentials()
	if err != nil {
		t.Fatalf("failed to load credentials: %v", err)
	}

	if creds.CurrentProfile != "production" {
		t.Errorf("expected CurrentProfile 'production', got %q", creds.CurrentProfile)
	}
}

func TestSetCurrentProfile_NotFound(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create credentials with only default profile
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": createTestProfile("token", "refresh", time.Now().Add(1*time.Hour)),
		},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	err := storage.SetCurrentProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error when setting non-existent profile, got nil")
	}
}

func TestHasValidCredentials_Valid(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create valid credentials (not expired)
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": createTestProfile("valid_token", "refresh", time.Now().Add(1*time.Hour)),
		},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	if !storage.HasValidCredentials() {
		t.Error("expected HasValidCredentials to return true for valid token")
	}
}

func TestHasValidCredentials_Expired(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create expired credentials
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": createTestProfile("expired_token", "refresh", time.Now().Add(-1*time.Hour)),
		},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	if storage.HasValidCredentials() {
		t.Error("expected HasValidCredentials to return false for expired token")
	}
}

func TestHasValidCredentials_NoToken(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create credentials without a token
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": {
				ServerURL:   "https://api.astro.dev",
				AccessToken: "", // Empty token
				ExpiresAt:   time.Now().Add(1 * time.Hour),
			},
		},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	if storage.HasValidCredentials() {
		t.Error("expected HasValidCredentials to return false when no token")
	}
}

func TestGetCurrentProfile_Success(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{
			"default": createTestProfile("token", "refresh", time.Now().Add(1*time.Hour)),
		},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	profile, err := storage.GetCurrentProfile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.AccessToken != "token" {
		t.Errorf("expected AccessToken 'token', got %q", profile.AccessToken)
	}
}

func TestGetCurrentProfile_NoProfile(t *testing.T) {
	_, cleanup := setupTestDir(t)
	defer cleanup()

	// Create credentials without the current profile
	testCreds := &Credentials{
		CurrentProfile: "default",
		Profiles:       map[string]*Profile{},
	}
	writeTestCredentials(t, testCreds)

	storage := createTestStorage()
	_, err := storage.GetCurrentProfile()
	if err == nil {
		t.Fatal("expected error when current profile doesn't exist, got nil")
	}
}
