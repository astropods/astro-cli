package config

import (
	"strings"
	"testing"
	"time"
)

// minimalProductionConfig builds a Config that satisfies every Validate()
// prereq except the one under test, so the assertion isolates that check.
func minimalProductionConfig() *Config {
	return &Config{
		RunMode: "all",
		Server: ServerConfig{
			Port:            "8080",
			Mode:            "release",
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    10 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},
		Log: LogConfig{Level: "info", Format: "text"},
		Security: SecurityConfig{
			DeployTokenSecret: "operator-rotated-secret-please-keep-private",
		},
		Deployment: DeploymentConfig{
			RegistryURL:        "registry.example.com",
			Environment:        "prod",
			K8sClientMode:      "eks",
			TemplateSigningKey: []byte("k8s-signing-key-not-the-default"),
		},
		Auth: AuthConfig{
			WorkOSAPIKey:   "workos-key",
			WorkOSClientID: "workos-client",
			RedirectURI:    "https://example.com/auth/callback",
			CookiePassword: strings.Repeat("x", 32),
		},
		Database: DatabaseConfig{URL: "postgres://localhost/test"},
	}
}

// In non-local mode, the dev default is rejected — the literal is in the
// public source so signing prod tokens with it would let anyone forge a
// valid token for any deployment_id.
func TestValidate_RejectsDevDeployTokenSecretInProduction(t *testing.T) {
	cfg := minimalProductionConfig()
	cfg.Security.DeployTokenSecret = DevDeployTokenSecret

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DEPLOY_TOKEN_SECRET") {
		t.Errorf("Validate should refuse the dev default in non-local mode; got err=%v", err)
	}
}

// Empty secret is also rejected — same risk, no signing happens at all.
func TestValidate_RejectsEmptyDeployTokenSecretInProduction(t *testing.T) {
	cfg := minimalProductionConfig()
	cfg.Security.DeployTokenSecret = ""

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DEPLOY_TOKEN_SECRET") {
		t.Errorf("Validate should refuse an empty secret in non-local mode; got err=%v", err)
	}
}

// Local mode keeps accepting the dev default — pins the design intent so a
// future tightening doesn't accidentally break local development.
func TestValidate_AcceptsDevDeployTokenSecretInLocalMode(t *testing.T) {
	cfg := minimalProductionConfig()
	cfg.Deployment.K8sClientMode = "local"
	cfg.Security.DeployTokenSecret = DevDeployTokenSecret

	if err := cfg.Validate(); err != nil {
		t.Errorf("local mode should accept the dev default; got %v", err)
	}
}

// A rotated secret is accepted in non-local mode.
func TestValidate_AcceptsRotatedDeployTokenSecretInProduction(t *testing.T) {
	cfg := minimalProductionConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("rotated secret should be accepted in non-local mode; got %v", err)
	}
}
