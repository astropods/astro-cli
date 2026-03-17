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
	if err := os.WriteFile(specPath, []byte("spec: package/v1\nname: test-agent\nagent:\n  image: test:latest\n"), 0600); err != nil {
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

	err := registerAgent(srv.URL, "test-agent", "abc123", "registry.example.com", specPath, "", "", "", false, true)

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

	err := registerAgent(srv.URL, "test-agent", "abc123", "registry.example.com", specPath, "", "", "", false, true)

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
	if err := os.WriteFile(specPath, []byte("spec: package/v1\nname: test-agent\nagent:\n  image: test:latest\n"), 0600); err != nil {
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
