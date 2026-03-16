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
