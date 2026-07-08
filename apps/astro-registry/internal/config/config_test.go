package config

import (
	"os"
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	// Set required env vars
	t.Setenv("REGISTRY_URL", "https://123456789.dkr.ecr.us-east-1.amazonaws.com")
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("WORKOS_CLIENT_ID", "client_test")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("REGISTRY_TOKEN_SECRET", "test-secret")
	t.Setenv("REGISTRY_TOKEN_REALM", "https://registry.test/token")
	defer func() {
		_ = os.Unsetenv("REGISTRY_URL")
		_ = os.Unsetenv("ENVIRONMENT")
		_ = os.Unsetenv("WORKOS_CLIENT_ID")
		_ = os.Unsetenv("DATABASE_URL")
		_ = os.Unsetenv("REGISTRY_TOKEN_SECRET")
		_ = os.Unsetenv("REGISTRY_TOKEN_REALM")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Server.Port != "5000" {
		t.Errorf("expected default port 5000, got %s", cfg.Server.Port)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}

	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level info, got %s", cfg.Log.Level)
	}

	if cfg.Auth.RegistryTokenIssuer != "astro-registry" {
		t.Errorf("expected default RegistryTokenIssuer astro-registry, got %s", cfg.Auth.RegistryTokenIssuer)
	}

	if cfg.Auth.RegistryTokenTTL.Hours() != 1 {
		t.Errorf("expected default RegistryTokenTTL=1h, got %v", cfg.Auth.RegistryTokenTTL)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("REGISTRY_URL", "https://test.ecr.amazonaws.com")
	t.Setenv("AWS_REGION", "us-west-2")
	t.Setenv("ENVIRONMENT", "staging")
	t.Setenv("WORKOS_CLIENT_ID", "client_test")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("REGISTRY_TOKEN_SECRET", "test-secret")
	t.Setenv("REGISTRY_TOKEN_REALM", "https://registry.test/token")

	defer func() {
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("HOST")
		_ = os.Unsetenv("LOG_LEVEL")
		_ = os.Unsetenv("REGISTRY_URL")
		_ = os.Unsetenv("AWS_REGION")
		_ = os.Unsetenv("ENVIRONMENT")
		_ = os.Unsetenv("WORKOS_CLIENT_ID")
		_ = os.Unsetenv("DATABASE_URL")
		_ = os.Unsetenv("REGISTRY_TOKEN_SECRET")
		_ = os.Unsetenv("REGISTRY_TOKEN_REALM")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Server.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Server.Port)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}

	if cfg.Log.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Log.Level)
	}

	if cfg.Registry.AWSRegion != "us-west-2" {
		t.Errorf("expected region us-west-2, got %s", cfg.Registry.AWSRegion)
	}
}

func TestLoad_MissingRegistryURL(t *testing.T) {
	_ = os.Unsetenv("REGISTRY_URL")
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("WORKOS_CLIENT_ID", "client_test")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	defer func() {
		_ = os.Unsetenv("ENVIRONMENT")
		_ = os.Unsetenv("WORKOS_CLIENT_ID")
		_ = os.Unsetenv("DATABASE_URL")
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing REGISTRY_URL")
	}
}

func TestLoad_MissingRegistryTokenSecret(t *testing.T) {
	t.Setenv("REGISTRY_URL", "https://test.ecr.amazonaws.com")
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("WORKOS_CLIENT_ID", "client_test")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	_ = os.Unsetenv("REGISTRY_TOKEN_SECRET")

	defer func() {
		_ = os.Unsetenv("REGISTRY_URL")
		_ = os.Unsetenv("ENVIRONMENT")
		_ = os.Unsetenv("WORKOS_CLIENT_ID")
		_ = os.Unsetenv("DATABASE_URL")
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing REGISTRY_TOKEN_SECRET")
	}
}

func TestLoad_MissingClientID(t *testing.T) {
	t.Setenv("REGISTRY_URL", "https://test.ecr.amazonaws.com")
	t.Setenv("ENVIRONMENT", "test")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	_ = os.Unsetenv("WORKOS_CLIENT_ID")

	defer func() {
		_ = os.Unsetenv("REGISTRY_URL")
		_ = os.Unsetenv("ENVIRONMENT")
		_ = os.Unsetenv("DATABASE_URL")
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing WORKOS_CLIENT_ID")
	}
}

func TestValidate_RealmRequired(t *testing.T) {
	cfg := &Config{
		Server:   ServerConfig{Port: "8080", Mode: "release"},
		Log:      LogConfig{Level: "info"},
		Registry: RegistryConfig{URL: "https://test.ecr.amazonaws.com", Environment: "test"},
		Auth: AuthConfig{
			WorkOSClientID:      "client_test",
			RegistryTokenSecret: "test-secret",
		},
		Database: DatabaseConfig{URL: "postgres://localhost/test"},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing REGISTRY_TOKEN_REALM")
	}

	cfg.Auth.RegistryTokenRealm = "https://registry.test/token"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error with realm set: %v", err)
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Port: "8080", Mode: "release"},
		Log:    LogConfig{Level: "invalid"},
		Registry: RegistryConfig{
			URL: "https://test.ecr.amazonaws.com",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func TestValidate_InvalidServerMode(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Port: "8080", Mode: "invalid"},
		Log:    LogConfig{Level: "info"},
		Registry: RegistryConfig{
			URL: "https://test.ecr.amazonaws.com",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid server mode")
	}
}

func TestGetEnvSlice(t *testing.T) {
	t.Setenv("TEST_SLICE", "a,b,c")

	result := getEnvSlice("TEST_SLICE", []string{})
	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("unexpected values: %v", result)
	}
}

func TestGetEnvSlice_Default(t *testing.T) {
	_ = os.Unsetenv("TEST_SLICE_MISSING")

	result := getEnvSlice("TEST_SLICE_MISSING", []string{"default"})
	if len(result) != 1 || result[0] != "default" {
		t.Errorf("expected default value, got %v", result)
	}
}

func TestValidate_PrimaryPullKeyHash(t *testing.T) {
	base := func() *Config {
		return &Config{
			Server:   ServerConfig{Port: "5000", Mode: "release"},
			Log:      LogConfig{Level: "info"},
			Registry: RegistryConfig{URL: "https://x.dkr.ecr.us-east-1.amazonaws.com", Environment: "test"},
			Auth: AuthConfig{
				WorkOSClientID:      "client_test",
				RegistryTokenSecret: "s",
				RegistryTokenRealm:  "https://r/token",
			},
			Database: DatabaseConfig{URL: "postgres://localhost/test"},
		}
	}

	tests := []struct {
		name    string
		hash    string
		wantErr bool
	}{
		{"empty is allowed", "", false},
		{"valid sha256 hex", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", false},
		{"not hex", "zzzz", true},
		{"wrong length", "abcd", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base()
			c.Auth.PrimaryPullKeyHash = tt.hash
			err := c.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("PrimaryPullKeyHash=%q: wantErr=%v, got err=%v", tt.hash, tt.wantErr, err)
			}
		})
	}
}
