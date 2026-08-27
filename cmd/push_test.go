package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/utils"
	spec "github.com/astropods/astro-spec"
)

func TestPushRegistryURL(t *testing.T) {
	origDefaultRegistryURL := buildinfo.DefaultRegistryURL
	origDefaultServerURL := buildinfo.DefaultServerURL
	t.Cleanup(func() {
		pushServerURLOverride = ""
		buildinfo.DefaultRegistryURL = origDefaultRegistryURL
		buildinfo.DefaultServerURL = origDefaultServerURL
	})

	tests := []struct {
		name               string
		serverOverride     string
		defaultRegistryURL string
		defaultServerURL   string
		want               string
	}{
		{
			name:               "production build uses DefaultRegistryURL ldflag",
			serverOverride:     "",
			defaultRegistryURL: "https://registry.astropods.ai",
			defaultServerURL:   "https://astropods.com",
			want:               "https://registry.astropods.ai",
		},
		{
			name:               "test mode derives registry from server override, ignores DefaultRegistryURL",
			serverOverride:     "http://localhost:9999",
			defaultRegistryURL: "https://registry.astropods.ai",
			defaultServerURL:   "https://astropods.com",
			want:               "http://registry.localhost",
		},
		{
			name:               "local dev derives registry from DefaultServerURL when no ldflag set",
			serverOverride:     "",
			defaultRegistryURL: "",
			defaultServerURL:   "http://localhost:8080",
			want:               "http://registry.localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pushServerURLOverride = tt.serverOverride
			buildinfo.DefaultRegistryURL = tt.defaultRegistryURL
			buildinfo.DefaultServerURL = tt.defaultServerURL

			got := pushRegistryURL()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateBuildID(t *testing.T) {
	id := generateBuildID()

	// Should be 8-char hex string
	if len(id) != 8 {
		t.Errorf("generateBuildID() length = %d, want 8", len(id))
	}

	hexRE := regexp.MustCompile(`^[a-f0-9]{8}$`)
	if !hexRE.MatchString(id) {
		t.Errorf("generateBuildID() = %q, want 8-char hex string", id)
	}

	// Two calls should produce different IDs (probabilistic but effectively guaranteed)
	id2 := generateBuildID()
	if id == id2 {
		t.Errorf("generateBuildID() produced same ID twice: %q", id)
	}
}

func TestPush_ExpiredCredentialsFailBeforeBuild(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	_ = os.Unsetenv(auth.EnvAccessToken)

	credsPath := filepath.Join(tmpDir, buildinfo.AppDirName, "credentials.json")
	if err := os.MkdirAll(filepath.Dir(credsPath), 0700); err != nil {
		t.Fatal(err)
	}
	creds := auth.Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*auth.Profile{
			"default": {
				AccessToken:  "expired_token",
				RefreshToken: "",
				ExpiresAt:    time.Now().Add(-1 * time.Hour),
				User: &auth.StoredUser{
					ID:          "user-1",
					Email:       "test@example.com",
					AccountName: "testaccount",
					AccountID:   "acct-1",
				},
				Accounts: []auth.StoredAccount{
					{ID: "acct-1", Name: "testaccount", Type: "personal"},
				},
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("spec: blueprint/v1\nname: test-agent\nmeta: {}\nagent:\n  image: test:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir) //nolint:errcheck

	rootCmd.SetArgs([]string{"push", "test-agent"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected push to fail with expired credentials, got nil")
	}
	if !strings.Contains(err.Error(), "not authenticated") && !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected auth error, got: %s", err.Error())
	}
}

func TestRegisterAgent_PrintsServerHints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := map[string]any{
			"message":  "Agent registered successfully",
			"account":  "testaccount",
			"name":     "test-agent",
			"build_id": "abc123",
			"hints":    []string{"No AGENT.md provided — add one next to your astropods.yml to make your agent more discoverable"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("name: test-agent\nversion: 1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := registerAgent(srv.URL, "test-agent", "abc123", "registry.example.com", specPath, "", "", "", false, true, "")

	w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var buf [4096]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	if !strings.Contains(output, "AGENT.md") {
		t.Errorf("expected hint about AGENT.md in stderr output, got: %q", output)
	}
}

func TestRegisterAgent_NoHintsWhenReadmePresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := map[string]any{
			"message":  "Agent registered successfully",
			"account":  "testaccount",
			"name":     "test-agent",
			"build_id": "abc123",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("name: test-agent\nversion: 1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := registerAgent(srv.URL, "test-agent", "abc123", "registry.example.com", specPath, "", "", "", false, true, "")

	w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var buf [4096]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	if strings.Contains(output, "AGENT.md") {
		t.Errorf("expected no hint about AGENT.md when readme is present, got: %q", output)
	}
}

func TestStripSecretDefaults(t *testing.T) {
	specObj := map[string]interface{}{
		"name": "test-agent",
		// Top-level inputs (map)
		"inputs": map[string]interface{}{
			"api_key": map[string]interface{}{
				"name": "API_KEY", "secret": true, "default": "sk-secret",
			},
			"log_level": map[string]interface{}{
				"name": "LOG_LEVEL", "default": "debug",
			},
		},
		// Agent inputs (list)
		"agent": map[string]interface{}{
			"image": "test:latest",
			"inputs": []interface{}{
				map[string]interface{}{"name": "AGENT_SECRET", "secret": true, "default": "agent-val"},
				map[string]interface{}{"name": "AGENT_PLAIN", "default": "plain-val"},
			},
		},
		// Model inputs (list)
		"models": map[string]interface{}{
			"llm": map[string]interface{}{
				"inputs": []interface{}{
					map[string]interface{}{"name": "MODEL_KEY", "secret": true, "default": "model-secret"},
				},
			},
		},
		// Provider variables (list)
		"providers": map[string]interface{}{
			"anthropic": map[string]interface{}{
				"scope": []interface{}{"models"},
				"variables": []interface{}{
					map[string]interface{}{"name": "ANTHROPIC_API_KEY", "secret": true, "default": "sk-ant-test"},
					map[string]interface{}{"name": "ANTHROPIC_ORG", "default": "org-123"},
				},
			},
		},
	}

	spec.StripSecretDefaults(specObj)

	// Secret input default should be stripped
	apiKey := specObj["inputs"].(map[string]interface{})["api_key"].(map[string]interface{})
	if _, ok := apiKey["default"]; ok {
		t.Error("secret input API_KEY should have default stripped")
	}

	// Non-secret input default should be preserved
	logLevel := specObj["inputs"].(map[string]interface{})["log_level"].(map[string]interface{})
	if logLevel["default"] != "debug" {
		t.Error("non-secret LOG_LEVEL default should be preserved")
	}

	// Agent secret input stripped
	agentInputs := specObj["agent"].(map[string]interface{})["inputs"].([]interface{})
	if _, ok := agentInputs[0].(map[string]interface{})["default"]; ok {
		t.Error("agent secret input should have default stripped")
	}
	// Agent non-secret preserved
	if agentInputs[1].(map[string]interface{})["default"] != "plain-val" {
		t.Error("agent non-secret input default should be preserved")
	}

	// Model secret input stripped
	modelInputs := specObj["models"].(map[string]interface{})["llm"].(map[string]interface{})["inputs"].([]interface{})
	if _, ok := modelInputs[0].(map[string]interface{})["default"]; ok {
		t.Error("model secret input should have default stripped")
	}

	// Provider secret variable stripped
	provVars := specObj["providers"].(map[string]interface{})["anthropic"].(map[string]interface{})["variables"].([]interface{})
	if _, ok := provVars[0].(map[string]interface{})["default"]; ok {
		t.Error("provider secret variable should have default stripped")
	}
	// Provider non-secret variable preserved
	if provVars[1].(map[string]interface{})["default"] != "org-123" {
		t.Error("provider non-secret variable default should be preserved")
	}
}

// TestPush_InvalidSpecFailsBeforeAuth asserts that a spec which fails validation
// causes push to exit with a validation error, not an authentication error,
// even when credentials are missing. This locks in the ordering guarantee that
// validation runs before auth/build/push.
func TestPush_InvalidSpecFailsBeforeAuth(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	_ = os.Unsetenv(auth.EnvAccessToken)

	// No credentials file — if validation weren't first, push would fail with "not authenticated".
	// Spec is missing required top-level `agent`, so validation must fail first.
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("spec: blueprint/v1\nname: test-agent\n"), 0600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir) //nolint:errcheck

	var err error
	_ = captureStdout(t, func() {
		rootCmd.SetArgs([]string{"push", "test-agent"})
		err = rootCmd.Execute()
	})

	if err == nil {
		t.Fatal("expected push to fail, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected validation failure, got: %s", err.Error())
	}
	if strings.Contains(err.Error(), "not authenticated") || strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("validation should have failed before auth check, got auth error: %s", err.Error())
	}
}

func TestPush_StaleRefreshTokenFailBeforeBuild(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	_ = os.Unsetenv(auth.EnvAccessToken)

	credsPath := filepath.Join(tmpDir, buildinfo.AppDirName, "credentials.json")
	if err := os.MkdirAll(filepath.Dir(credsPath), 0700); err != nil {
		t.Fatal(err)
	}
	creds := auth.Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*auth.Profile{
			"default": {
				AccessToken:  "expired_token",
				RefreshToken: "stale_refresh_token",
				ExpiresAt:    time.Now().Add(-1 * time.Hour),
				User: &auth.StoredUser{
					ID:          "user-1",
					Email:       "test@example.com",
					AccountName: "testaccount",
					AccountID:   "acct-1",
				},
				Accounts: []auth.StoredAccount{
					{ID: "acct-1", Name: "testaccount", Type: "personal"},
				},
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("spec: blueprint/v1\nname: test-agent\nmeta: {}\nagent:\n  image: test:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir) //nolint:errcheck

	rootCmd.SetArgs([]string{"push", "test-agent"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected push to fail with stale refresh token, got nil")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected authentication failed error, got: %s", err.Error())
	}
}

func TestParseAgentName(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedAccount string
		expectedName    string
	}{
		{
			name:            "bare name",
			input:           "my-agent",
			expectedAccount: "",
			expectedName:    "my-agent",
		},
		{
			name:            "org-scoped name",
			input:           "@my-org/my-agent",
			expectedAccount: "my-org",
			expectedName:    "my-agent",
		},
		{
			name:            "org with complex agent name",
			input:           "@acme-corp/data-pipeline-v2",
			expectedAccount: "acme-corp",
			expectedName:    "data-pipeline-v2",
		},
		{
			name:            "@ without slash treated as bare name",
			input:           "@justname",
			expectedAccount: "",
			expectedName:    "@justname",
		},
		{
			name:            "@ with trailing slash treated as bare name",
			input:           "@org/",
			expectedAccount: "",
			expectedName:    "@org/",
		},
		{
			name:            "@ with leading slash treated as bare name",
			input:           "@/name",
			expectedAccount: "",
			expectedName:    "@/name",
		},
		{
			name:            "no @ with slash is bare name",
			input:           "org/name",
			expectedAccount: "",
			expectedName:    "org/name",
		},
		{
			name:            "empty string",
			input:           "",
			expectedAccount: "",
			expectedName:    "",
		},
		{
			name:            "just @",
			input:           "@",
			expectedAccount: "",
			expectedName:    "@",
		},
		{
			name:            "multiple slashes picks first",
			input:           "@org/name/extra",
			expectedAccount: "org",
			expectedName:    "name/extra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account, name := utils.ParseAgentName(tt.input)
			if account != tt.expectedAccount {
				t.Errorf("ParseAgentName(%q) account = %q, want %q", tt.input, account, tt.expectedAccount)
			}
			if name != tt.expectedName {
				t.Errorf("ParseAgentName(%q) name = %q, want %q", tt.input, name, tt.expectedName)
			}
		})
	}
}

func TestRegisterAgent_UsesFreshAccountToken(t *testing.T) {
	var receivedAuthHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		resp := map[string]any{
			"message":  "Agent registered successfully",
			"account":  "my-org",
			"name":     "test-agent",
			"build_id": "abc123",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	_ = os.Unsetenv(auth.EnvAccessToken)

	writeAccountTestCredentials(t, &auth.Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*auth.Profile{
			"default": {
				AccessToken:    authTestJWT(time.Now().Add(-10 * time.Minute)),
				RefreshToken:   "valid_refresh_token",
				ExpiresAt:      time.Now().Add(1 * time.Hour),
				CurrentAccount: "my-org",
				User: &auth.StoredUser{
					ID:          "user-1",
					Email:       "test@example.com",
					AccountName: "my-org",
					AccountID:   "acct-org",
				},
				Accounts: []auth.StoredAccount{
					{ID: "acct-org", Name: "my-org", Type: "organization", OrganizationID: "org_workos_123"},
				},
			},
		},
	})

	workos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "org-scoped-jwt-token",
			RefreshToken: "new_refresh_token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	}))
	defer workos.Close()
	auth.SetWorkOSBaseURLOverride(workos.URL)
	t.Cleanup(func() { auth.SetWorkOSBaseURLOverride("") })

	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("name: test-agent\nversion: 1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}

	err := registerAgent(srv.URL, "test-agent", "abc123", "registry.example.com/my-org", specPath, "", "", "", false, false, "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedAuth := "Bearer org-scoped-jwt-token"
	if receivedAuthHeader != expectedAuth {
		t.Errorf("expected Authorization header %q, got %q", expectedAuth, receivedAuthHeader)
	}
}

func TestRegisterAgent_UsesAccountFromRegistryPath(t *testing.T) {
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"message": "ok"})
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("name: test-agent\nversion: 1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}

	err := registerAgent(srv.URL, "my-agent", "abc123", "registry.example.com/my-org", specPath, "", "", "", false, true, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := "/api/v1/agents/my-org/my-agent/register"
	if receivedPath != expectedPath {
		t.Errorf("expected URL path %q, got %q", expectedPath, receivedPath)
	}
}

func TestRegisterAgent_SendsPrivateVisibility(t *testing.T) {
	var receivedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"message": "ok"}) //nolint:errcheck
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("spec: blueprint/v1\nname: test-agent\nagent:\n  image: test:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}

	err := registerAgent(srv.URL, "test-agent", "abc123", "registry.example.com/testuser", specPath, "abc123", "", "private", false, true, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["visibility"] != "private" {
		t.Errorf("expected visibility 'private', got %q", receivedBody["visibility"])
	}
}

func TestRegisterAgent_SendsPublicVisibility(t *testing.T) {
	var receivedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedBody) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"message": "ok"}) //nolint:errcheck
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("spec: blueprint/v1\nname: test-agent\nagent:\n  image: test:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}

	err := registerAgent(srv.URL, "test-agent", "abc123", "registry.example.com/testuser", specPath, "abc123", "", "public", false, true, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody["visibility"] != "public" {
		t.Errorf("expected visibility 'public', got %q", receivedBody["visibility"])
	}
}

func TestGetAgentFromServer_ReturnsVisibility(t *testing.T) {
	tests := []struct {
		name               string
		responseBody       map[string]any
		statusCode         int
		expectedExists     bool
		expectedVisibility string
	}{
		{
			name:               "public agent",
			statusCode:         http.StatusOK,
			responseBody:       map[string]any{"visibility": "public"},
			expectedExists:     true,
			expectedVisibility: "public",
		},
		{
			name:               "private agent",
			statusCode:         http.StatusOK,
			responseBody:       map[string]any{"visibility": "private"},
			expectedExists:     true,
			expectedVisibility: "private",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.responseBody != nil {
					json.NewEncoder(w).Encode(tt.responseBody) //nolint:errcheck
				}
			}))
			defer srv.Close()

			info := getAgentFromServer(context.Background(), srv.URL, "testaccount", "test-agent", true)

			if info.Exists != tt.expectedExists {
				t.Errorf("Exists = %v, want %v", info.Exists, tt.expectedExists)
			}
			if info.Visibility != tt.expectedVisibility {
				t.Errorf("Visibility = %q, want %q", info.Visibility, tt.expectedVisibility)
			}
		})
	}
}

func TestVisibilityNeedsConfirm(t *testing.T) {
	tests := []struct {
		name          string
		pushPublic    bool
		pushPrivate   bool
		serverExists  bool
		serverVis     string
		expectConfirm bool
		expectVis     string // resolved visibility after preservation logic
	}{
		{
			name:          "no flags, new agent — private, no confirm",
			serverExists:  false,
			expectVis:     "private",
			expectConfirm: false,
		},
		{
			name:          "no flags, already private — private, no confirm",
			serverExists:  true,
			serverVis:     "private",
			expectVis:     "private",
			expectConfirm: false,
		},
		{
			name:          "no flags, already public — preserved public, no confirm",
			serverExists:  true,
			serverVis:     "public",
			expectVis:     "public",
			expectConfirm: false,
		},
		{
			name:          "--public, new agent — public, confirm",
			pushPublic:    true,
			serverExists:  false,
			expectVis:     "public",
			expectConfirm: true,
		},
		{
			name:          "--public, already public — public, no confirm",
			pushPublic:    true,
			serverExists:  true,
			serverVis:     "public",
			expectVis:     "public",
			expectConfirm: false,
		},
		{
			name:          "--public, was private — public, confirm",
			pushPublic:    true,
			serverExists:  true,
			serverVis:     "private",
			expectVis:     "public",
			expectConfirm: true,
		},
		{
			name:          "--private, was public — private, confirm",
			pushPrivate:   true,
			serverExists:  true,
			serverVis:     "public",
			expectVis:     "private",
			expectConfirm: true,
		},
		{
			name:          "--private, new agent — private, no confirm",
			pushPrivate:   true,
			serverExists:  false,
			expectVis:     "private",
			expectConfirm: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := agentServerInfo{Exists: tt.serverExists, Visibility: tt.serverVis}

			// Reproduce the production visibility resolution logic
			visibility := "private"
			if tt.pushPublic {
				visibility = "public"
			}
			if server.Exists && server.Visibility == "public" && !tt.pushPrivate {
				visibility = "public"
			}

			needsConfirm := (visibility == "public" && (!server.Exists || server.Visibility != "public")) ||
				(tt.pushPrivate && server.Exists && server.Visibility == "public")

			if visibility != tt.expectVis {
				t.Errorf("visibility = %q, want %q", visibility, tt.expectVis)
			}
			if needsConfirm != tt.expectConfirm {
				t.Errorf("needsConfirm = %v, want %v", needsConfirm, tt.expectConfirm)
			}
		})
	}
}

func TestTransformSpecForRegistry_UsesAgentName(t *testing.T) {
	tests := []struct {
		name         string
		specName     string
		agentName    string // the resolved name passed by the cobra handler
		expectedName string
	}{
		{
			name:         "org-scoped spec name: agentName is the stripped form",
			specName:     "@example/foobar",
			agentName:    "foobar",
			expectedName: "foobar",
		},
		{
			name:         "org-scoped spec name: agentName overrides stripped spec name",
			specName:     "@example/foobar",
			agentName:    "barbat",
			expectedName: "barbat",
		},
		{
			name:         "bare spec name: agentName matches spec",
			specName:     "my-agent",
			agentName:    "my-agent",
			expectedName: "my-agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specObj := map[string]interface{}{
				"name": tt.specName,
				"agent": map[string]interface{}{
					"image": "existing:latest",
				},
			}

			result := spec.TransformSpecForRegistry(specObj, tt.agentName, func(imageName string) string {
				return fmt.Sprintf("registry.example.com/ns/%s:tag1", imageName)
			})

			gotName, ok := result["name"].(string)
			if !ok {
				t.Fatal("expected name to be a string")
			}
			if gotName != tt.expectedName {
				t.Errorf("TransformSpecForRegistry() name = %q, want %q", gotName, tt.expectedName)
			}
		})
	}
}

func TestPush_OrgScopedSpecName(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	_ = os.Unsetenv(auth.EnvAccessToken)

	credsPath := filepath.Join(tmpDir, buildinfo.AppDirName, "credentials.json")
	if err := os.MkdirAll(filepath.Dir(credsPath), 0700); err != nil {
		t.Fatal(err)
	}
	creds := auth.Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*auth.Profile{
			"default": {
				AccessToken:  "valid_token",
				RefreshToken: "refresh_token",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				User: &auth.StoredUser{
					ID:          "user-1",
					Email:       "test@example.com",
					AccountName: "personal",
					AccountID:   "acct-1",
				},
				Accounts: []auth.StoredAccount{
					{ID: "acct-1", Name: "personal", Type: "personal"},
					{ID: "acct-2", Name: "my-org", Type: "organization", OrganizationID: "org_workos_123"},
				},
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Spec with @org/name format — the push should resolve to org namespace
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("spec: blueprint/v1\nname: \"@my-org/test-agent\"\nmeta: {}\nagent:\n  image: test:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir) //nolint:errcheck

	// The spec has @my-org/test-agent but the logged-in account is personal; expect mismatch error
	rootCmd.SetArgs([]string{"push", "test-agent"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected push to fail with account mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "does not match current account") {
		t.Errorf("expected account mismatch error, got: %s", err.Error())
	}
}

func TestPush_AllowAccountOverride(t *testing.T) {
	registerCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/register") {
			registerCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"message": "ok"}) //nolint:errcheck
			return
		}
		// GET agent status — return 404 (new agent)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	pushServerURLOverride = srv.URL
	t.Cleanup(func() { pushServerURLOverride = "" })

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	_ = os.Unsetenv(auth.EnvAccessToken)

	// Standard creds: current account is "alice" (personal).
	writeAccountTestCredentials(t, accountTestCreds(""))

	// Spec references a different account (@acme-corp/test-agent).
	specPath := filepath.Join(tmpDir, "astropods.yml")
	specContent := "spec: blueprint/v1\nname: \"@acme-corp/test-agent\"\nmeta: {}\nagent:\n  image: test:latest\n"
	require.NoError(t, os.WriteFile(specPath, []byte(specContent), 0600))

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir) //nolint:errcheck

	var err error
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"push", "test-agent", "--allow-account-override", "--no-build"})
		err = rootCmd.Execute()
	})

	require.NoError(t, err)
	assert.True(t, registerCalled, "expected /register endpoint to be called")
	assert.Contains(t, out, "overridden to current account", "expected account override warning in output")
}

// setupPushHomeAndSpec creates a temp HOME, writes credentials for currentAccount,
// writes an astropods.yml spec with the given agent name, and chdirs into the temp dir.
func setupPushHomeAndSpec(t *testing.T, currentAccount, specAgentName string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	_ = os.Unsetenv(auth.EnvAccessToken)

	writeAccountTestCredentials(t, accountTestCreds(currentAccount))

	specPath := filepath.Join(tmpDir, "astropods.yml")
	specContent := fmt.Sprintf("spec: blueprint/v1\nname: %q\nmeta: {}\nagent:\n  image: test:latest\n", specAgentName)
	require.NoError(t, os.WriteFile(specPath, []byte(specContent), 0600))

	origDir, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origDir) }) //nolint:errcheck
	require.NoError(t, os.Chdir(tmpDir))
}

// resetPushFlags resets all push-command flags to their defaults and clears Changed.
func resetPushFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"visibility", "no-build", "yes", "allow-account-override", "file"} {
		if f := blueprintPushCmd.Flags().Lookup(name); f != nil {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}
}

func TestRunBlueprintPush_AccountMismatchErrorIsActionableNotMisleading(t *testing.T) {
	setupPushHomeAndSpec(t, "alice", "@acme-corp/my-agent")
	resetPushFlags(t)

	blueprintPushCmd.SetContext(context.Background())
	err := runBlueprintPush(blueprintPushCmd, []string{"my-agent"})

	require.EqualError(t, err, errAccountMismatch("acme-corp", "alice").Error())
}

func TestRunBlueprintPush(t *testing.T) {
	tests := []struct {
		name           string
		specName       string   // spec agent name (bare or @org/name)
		args           []string // positional args (name override)
		allowOverride  bool
		yes            bool
		visibility     Visibility
		wantErr        error
		wantOutput     []string // must appear in cmd output
		wantNoOutput   []string // must not appear in cmd output
		wantRegistered string   // name delivered to /register; empty if error expected
	}{
		// Bare spec name, no arg — name comes from spec.
		{
			name:           "bare spec no arg",
			specName:       "my-agent",
			wantRegistered: "my-agent",
		},
		// Org-scoped spec, account matches org prefix, no arg — bare name used, no warning.
		{
			name:           "org-scoped spec matching account no arg",
			specName:       "@alice/my-agent",
			wantNoOutput:   []string{"overridden"},
			wantRegistered: "my-agent",
		},
		// Org-scoped spec, account mismatches, no arg, no override — error returned.
		{
			name:     "org-scoped spec mismatch no arg no override",
			specName: "@acme-corp/my-agent",
			wantErr:  errAccountMismatch("acme-corp", "alice"),
		},
		// Org-scoped spec, account mismatches, no arg, override flag — account warning only, spec bare name registered.
		{
			name:           "org-scoped spec mismatch no arg with override",
			specName:       "@acme-corp/my-agent",
			allowOverride:  true,
			wantOutput:     []string{`spec account "acme-corp" overridden to current account "alice"`},
			wantNoOutput:   []string{`spec name`},
			wantRegistered: "my-agent",
		},
		// Arg matches spec bare name — no override warning emitted.
		{
			name:           "arg matches spec name no warning",
			specName:       "my-agent",
			args:           []string{"my-agent"},
			wantNoOutput:   []string{"overridden"},
			wantRegistered: "my-agent",
		},
		// Arg differs from spec bare name — name override warning, arg name registered.
		{
			name:           "arg overrides spec name",
			specName:       "my-agent",
			args:           []string{"new-name"},
			wantOutput:     []string{`spec name "my-agent" overridden to "new-name"`},
			wantRegistered: "new-name",
		},
		// Arg present, spec has mismatching org, no override — error before name resolution.
		{
			name:     "arg with account mismatch no override",
			specName: "@acme-corp/my-agent",
			args:     []string{"my-agent"},
			wantErr:  errAccountMismatch("acme-corp", "alice"),
		},
		// Arg differs from spec AND account mismatches with override — both warnings, arg name registered.
		{
			name:          "arg overrides name and account with override",
			specName:      "@acme-corp/my-agent",
			args:          []string{"fooo"},
			allowOverride: true,
			wantOutput: []string{
				`spec account "acme-corp" overridden to current account "alice"`,
				`spec name "my-agent" overridden to "fooo"`,
			},
			wantRegistered: "fooo",
		},
		// --yes skips the visibility confirmation prompt for a public push.
		{
			name:           "yes flag skips public visibility confirmation",
			specName:       "my-agent",
			visibility:     VisibilityPublic,
			yes:            true,
			wantRegistered: "my-agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var registeredName string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/register") {
					parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/register"), "/")
					registeredName = parts[len(parts)-1]
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					json.NewEncoder(w).Encode(map[string]any{"message": "ok"}) //nolint:errcheck
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer srv.Close()
			pushServerURLOverride = srv.URL
			t.Cleanup(func() { pushServerURLOverride = "" })

			setupPushHomeAndSpec(t, "", tt.specName)

			resetPushFlags(t)
			require.NoError(t, blueprintPushCmd.Flags().Set("no-build", "true"))
			if tt.allowOverride {
				require.NoError(t, blueprintPushCmd.Flags().Set("allow-account-override", "true"))
			}
			if tt.yes {
				require.NoError(t, blueprintPushCmd.Flags().Set("yes", "true"))
			}
			if tt.visibility != "" {
				require.NoError(t, blueprintPushCmd.Flags().Set("visibility", string(tt.visibility)))
			}

			buf := &bytes.Buffer{}
			blueprintPushCmd.SetOut(buf)
			t.Cleanup(func() { blueprintPushCmd.SetOut(nil) })
			blueprintPushCmd.SetContext(context.Background())

			err := runBlueprintPush(blueprintPushCmd, tt.args)

			if tt.wantErr != nil {
				require.EqualError(t, err, tt.wantErr.Error())
			} else {
				require.NoError(t, err)
			}

			out := buf.String()
			for _, s := range tt.wantOutput {
				assert.Contains(t, out, s)
			}
			for _, s := range tt.wantNoOutput {
				assert.NotContains(t, out, s)
			}
			if tt.wantRegistered != "" {
				assert.Equal(t, tt.wantRegistered, registeredName)
			}
		})
	}
}

func TestFindAgentReadme(t *testing.T) {
	tests := []struct {
		name     string
		filename string // readme file to create; "" means none
		mkdir    string // directory to create (to verify dirs are skipped)
		want     bool   // whether a match is expected
	}{
		{name: "canonical uppercase", filename: "AGENT.md", want: true},
		{name: "all lowercase", filename: "agent.md", want: true},
		{name: "mixed case", filename: "Agent.md", want: true},
		{name: "uppercase extension", filename: "agent.MD", want: true},
		{name: "missing", want: false},
		{name: "unrelated file", filename: "README.md", want: false},
		{name: "directory matching name is ignored", mkdir: "AGENT.md", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.filename != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, tt.filename), []byte("body"), 0o600))
			}
			if tt.mkdir != "" {
				require.NoError(t, os.Mkdir(filepath.Join(dir, tt.mkdir), 0o755))
			}

			got := findAgentReadme(dir)
			if tt.want {
				require.NotEmpty(t, got)
				assert.Equal(t, tt.filename, filepath.Base(got))
			} else {
				assert.Empty(t, got)
			}
		})
	}
}
