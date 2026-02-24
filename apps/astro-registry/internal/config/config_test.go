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
	defer func() {
		_ = os.Unsetenv("REGISTRY_URL")
		_ = os.Unsetenv("ENVIRONMENT")
		_ = os.Unsetenv("WORKOS_CLIENT_ID")
		_ = os.Unsetenv("DATABASE_URL")
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

	defer func() {
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("HOST")
		_ = os.Unsetenv("LOG_LEVEL")
		_ = os.Unsetenv("REGISTRY_URL")
		_ = os.Unsetenv("AWS_REGION")
		_ = os.Unsetenv("ENVIRONMENT")
		_ = os.Unsetenv("WORKOS_CLIENT_ID")
		_ = os.Unsetenv("DATABASE_URL")
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
