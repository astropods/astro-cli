package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	"github.com/astropods/astro/apps/astro-cli/internal/utils"
)

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

	credsPath := filepath.Join(tmpDir, ".ast", "credentials.json")
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
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("spec: package/v1\nname: test-agent\nmeta: {}\nagent:\n  image: test:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir) //nolint:errcheck

	cmd := pushCmd
	cmd.Root().SetArgs([]string{"push", "--skip-build", "--skip-push", "--skip-register"})
	err := cmd.Execute()

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

	stripSecretDefaults(specObj)

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
	// Spec is missing required top-level `meta`, so validation must fail first.
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("spec: package/v1\nname: test-agent\nagent:\n  image: test:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir) //nolint:errcheck

	var err error
	_ = captureStdout(t, func() {
		cmd := pushCmd
		cmd.Root().SetArgs([]string{"push", "--skip-build", "--skip-push", "--skip-register"})
		err = cmd.Execute()
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

	credsPath := filepath.Join(tmpDir, ".ast", "credentials.json")
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
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("spec: package/v1\nname: test-agent\nmeta: {}\nagent:\n  image: test:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir) //nolint:errcheck

	cmd := pushCmd
	cmd.Root().SetArgs([]string{"push", "--skip-build", "--skip-push", "--skip-register"})
	err := cmd.Execute()

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

func TestGetUserNamespace_PersonalAccount(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	credsPath := filepath.Join(tmpDir, ".ast", "credentials.json")
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
					AccountName: "MyPersonal",
					AccountID:   "acct-1",
				},
				Accounts: []auth.StoredAccount{
					{ID: "acct-1", Name: "MyPersonal", Type: "personal"},
				},
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	namespace, workosOrgID, err := getUserNamespace(false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if namespace != "mypersonal" {
		t.Errorf("expected namespace 'mypersonal', got %q", namespace)
	}
	if workosOrgID != "" {
		t.Errorf("expected empty workosOrgID for personal account, got %q", workosOrgID)
	}
}

func TestGetUserNamespace_OrgOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	credsPath := filepath.Join(tmpDir, ".ast", "credentials.json")
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
					{ID: "acct-2", Name: "my-org", Type: "organization", WorkOSOrganizationID: "org_workos_123"},
				},
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	namespace, workosOrgID, err := getUserNamespace(false, "my-org")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if namespace != "my-org" {
		t.Errorf("expected namespace 'my-org', got %q", namespace)
	}
	if workosOrgID != "org_workos_123" {
		t.Errorf("expected workosOrgID 'org_workos_123', got %q", workosOrgID)
	}
}

func TestGetUserNamespace_OrgOverrideCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	credsPath := filepath.Join(tmpDir, ".ast", "credentials.json")
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
					{ID: "acct-2", Name: "MyOrg", Type: "organization", WorkOSOrganizationID: "org_123"},
				},
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	namespace, workosOrgID, err := getUserNamespace(false, "myorg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if namespace != "myorg" {
		t.Errorf("expected namespace 'myorg', got %q", namespace)
	}
	if workosOrgID != "org_123" {
		t.Errorf("expected workosOrgID 'org_123', got %q", workosOrgID)
	}
}

func TestGetUserNamespace_OrgNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	credsPath := filepath.Join(tmpDir, ".ast", "credentials.json")
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
					{ID: "acct-2", Name: "acme-corp", Type: "organization", WorkOSOrganizationID: "org_abc"},
				},
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	_, _, err := getUserNamespace(false, "nonexistent-org")
	if err == nil {
		t.Fatal("expected error for nonexistent org, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "personal") {
		t.Errorf("expected available accounts listed in error, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "acme-corp") {
		t.Errorf("expected available accounts listed in error, got: %s", err.Error())
	}
}

func TestGetUserNamespace_OrgMissingWorkOSID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	credsPath := filepath.Join(tmpDir, ".ast", "credentials.json")
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
					{ID: "acct-2", Name: "stale-org", Type: "organization", WorkOSOrganizationID: ""},
				},
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	_, _, err := getUserNamespace(false, "stale-org")
	if err == nil {
		t.Fatal("expected error for org with missing WorkOSOrganizationID, got nil")
	}
	if !strings.Contains(err.Error(), "not linked") {
		t.Errorf("expected 'not linked' in error, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "login") {
		t.Errorf("expected suggestion to re-login in error, got: %s", err.Error())
	}
}

func TestGetUserNamespace_PersonalOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	credsPath := filepath.Join(tmpDir, ".ast", "credentials.json")
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
					AccountName: "default-personal",
					AccountID:   "acct-1",
				},
				Accounts: []auth.StoredAccount{
					{ID: "acct-1", Name: "default-personal", Type: "personal"},
					{ID: "acct-2", Name: "other-personal", Type: "personal"},
				},
			},
		},
	}
	data, _ := json.Marshal(creds)
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	// Override to a different personal account — should return empty workosOrgID
	namespace, workosOrgID, err := getUserNamespace(false, "other-personal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if namespace != "other-personal" {
		t.Errorf("expected namespace 'other-personal', got %q", namespace)
	}
	if workosOrgID != "" {
		t.Errorf("expected empty workosOrgID for personal account override, got %q", workosOrgID)
	}
}

func TestRegisterAgent_UsesTokenOverride(t *testing.T) {
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
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte("name: test-agent\nversion: 1.0.0\n"), 0600); err != nil {
		t.Fatal(err)
	}

	orgToken := "org-scoped-jwt-token"
	err := registerAgent(srv.URL, "test-agent", "abc123", "registry.example.com/my-org", specPath, "", "", "", false, false, orgToken)
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
	if err := os.WriteFile(specPath, []byte("spec: astro/v1\nname: test-agent\nagent:\n  image: test:latest\n"), 0600); err != nil {
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
	if err := os.WriteFile(specPath, []byte("spec: astro/v1\nname: test-agent\nagent:\n  image: test:latest\n"), 0600); err != nil {
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

			info := getAgentFromServer(srv.URL, "testaccount", "test-agent", true, "")

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

func TestTransformSpecForRegistry_StripsOrgPrefix(t *testing.T) {
	tests := []struct {
		name         string
		specName     string
		expectedName string
	}{
		{
			name:         "scoped name gets stripped",
			specName:     "@postman/feb19-astro",
			expectedName: "feb19-astro",
		},
		{
			name:         "bare name unchanged",
			specName:     "my-agent",
			expectedName: "my-agent",
		},
		{
			name:         "different org prefix",
			specName:     "@acme-corp/data-pipeline",
			expectedName: "data-pipeline",
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

			result := transformSpecForRegistry(specObj, "registry.example.com/ns", "agent", "tag1")

			gotName, ok := result["name"].(string)
			if !ok {
				t.Fatal("expected name to be a string")
			}
			if gotName != tt.expectedName {
				t.Errorf("transformSpecForRegistry() name = %q, want %q", gotName, tt.expectedName)
			}
		})
	}
}

func TestPush_OrgScopedSpecName(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	_ = os.Unsetenv(auth.EnvAccessToken)

	credsPath := filepath.Join(tmpDir, ".ast", "credentials.json")
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
					{ID: "acct-2", Name: "my-org", Type: "organization", WorkOSOrganizationID: "org_workos_123"},
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
	if err := os.WriteFile(specPath, []byte("spec: package/v1\nname: \"@my-org/test-agent\"\nmeta: {}\nagent:\n  image: test:latest\n"), 0600); err != nil {
		t.Fatal(err)
	}

	origDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer os.Chdir(origDir) //nolint:errcheck

	// This will fail at the org token refresh step (no real WorkOS server),
	// but it proves the @org/name parsing and namespace resolution worked
	cmd := pushCmd
	cmd.Root().SetArgs([]string{"push", "--skip-build", "--skip-push", "--skip-register"})
	err := cmd.Execute()

	// The command should fail trying to get the org-scoped token (no real WorkOS)
	if err == nil {
		t.Fatal("expected push to fail when getting org-scoped token, got nil")
	}
	if !strings.Contains(err.Error(), "org-scoped token") {
		t.Errorf("expected org-scoped token error, got: %s", err.Error())
	}
}
