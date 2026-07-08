package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds the application configuration
type Config struct {
	Server   ServerConfig
	Log      LogConfig
	Security SecurityConfig
	Registry RegistryConfig
	Auth     AuthConfig
	Database DatabaseConfig
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Port            string
	Host            string
	Mode            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	AllowedOrigins []string
	TrustedProxies []string
}

// RegistryConfig holds registry-related configuration
type RegistryConfig struct {
	URL         string // Backend registry URL (ECR)
	AWSRegion   string // AWS region for ECR
	Environment string // Environment prefix for ECR repos (e.g. "prod", "preview")
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	URL string // PostgreSQL connection URL
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWKSEndpoint   string // WorkOS JWKS endpoint for JWT validation
	JWTIssuer      string // Expected JWT issuer
	WorkOSClientID string // WorkOS client ID (JWT audience)

	// Registry token authentication (Docker Registry v2 token-auth flow).
	// See docs/03-architecture/registry-token-auth.md.
	RegistryTokenSecret string        // HMAC key for signing registry-scope tokens
	RegistryTokenIssuer string        // iss claim for registry-scope tokens
	RegistryTokenTTL    time.Duration // Lifetime of registry-scope tokens
	RegistryTokenRealm  string        // Public URL of /token; advertised in WWW-Authenticate. Derived from request host if empty.

	// Cluster pull credential (CPC) — machine credential clusters present at
	// /token to obtain a pull-scoped registry token. The primary cluster has no
	// clusters row, so its hash is configured here; additional clusters store
	// their hash in clusters.pull_key_hash. Hex-encoded sha256 of the secret.
	// See docs/01-spec/registry-pull-through-spec.md.
	PrimaryPullKeyHash string
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnv("PORT", "5000"),
			Host:            getEnv("HOST", "0.0.0.0"),
			Mode:            getEnv("GIN_MODE", "release"),
			ReadTimeout:     getEnvDuration("READ_TIMEOUT", 10*time.Minute),
			WriteTimeout:    getEnvDuration("WRITE_TIMEOUT", 10*time.Minute),
			ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "text"),
		},
		Security: SecurityConfig{
			AllowedOrigins: getEnvSlice("ALLOWED_ORIGINS", []string{"*"}),
			TrustedProxies: getEnvSlice("TRUSTED_PROXIES", []string{}),
		},
		Registry: RegistryConfig{
			URL:         normalizeRegistryURL(getEnv("REGISTRY_URL", "")),
			AWSRegion:   getEnv("AWS_REGION", ""),
			Environment: getEnv("ENVIRONMENT", ""),
		},
		Auth: AuthConfig{
			JWKSEndpoint:        getEnv("JWKS_ENDPOINT", "https://api.workos.com/sso/jwks"),
			JWTIssuer:           getEnv("JWT_ISSUER", ""), // Will be auto-constructed if empty
			WorkOSClientID:      getEnv("WORKOS_CLIENT_ID", ""),
			RegistryTokenSecret: getEnv("REGISTRY_TOKEN_SECRET", ""),
			RegistryTokenIssuer: getEnv("REGISTRY_TOKEN_ISSUER", "astro-registry"),
			RegistryTokenTTL:    getEnvDuration("REGISTRY_TOKEN_TTL", 1*time.Hour),
			RegistryTokenRealm:  getEnv("REGISTRY_TOKEN_REALM", ""),
			PrimaryPullKeyHash:  strings.ToLower(strings.TrimSpace(getEnv("PRIMARY_PULL_KEY_HASH", ""))),
		},
		Database: DatabaseConfig{
			URL: getEnv("DATABASE_URL", ""),
		},
	}

	// Auto-construct JWT issuer and JWKS endpoint from WorkOS client ID if not explicitly set
	if cfg.Auth.WorkOSClientID != "" {
		if cfg.Auth.JWTIssuer == "" {
			cfg.Auth.JWTIssuer = fmt.Sprintf("https://api.workos.com/user_management/%s", cfg.Auth.WorkOSClientID)
		}
		if cfg.Auth.JWKSEndpoint == "" || cfg.Auth.JWKSEndpoint == "https://api.workos.com/sso/jwks" {
			cfg.Auth.JWKSEndpoint = fmt.Sprintf("https://api.workos.com/sso/jwks/%s", cfg.Auth.WorkOSClientID)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}

	validModes := map[string]bool{"debug": true, "release": true, "test": true}
	if !validModes[c.Server.Mode] {
		return fmt.Errorf("invalid server mode: %s (must be debug, release, or test)", c.Server.Mode)
	}

	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[c.Log.Level] {
		return fmt.Errorf("invalid log level: %s", c.Log.Level)
	}

	if c.Registry.URL == "" {
		return fmt.Errorf("REGISTRY_URL environment variable is required")
	}

	if c.Registry.Environment == "" {
		return fmt.Errorf("ENVIRONMENT environment variable is required")
	}

	if c.Auth.WorkOSClientID == "" {
		return fmt.Errorf("WORKOS_CLIENT_ID environment variable is required")
	}

	if c.Auth.RegistryTokenSecret == "" {
		return fmt.Errorf("REGISTRY_TOKEN_SECRET environment variable is required")
	}

	if c.Auth.RegistryTokenRealm == "" {
		return fmt.Errorf("REGISTRY_TOKEN_REALM environment variable is required")
	}

	if c.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required")
	}

	// PRIMARY_PULL_KEY_HASH is optional, but a set-but-malformed value would
	// silently disable primary cluster pulls (opaque 401 → ImagePullBackOff),
	// so fail loudly at boot instead. Must be hex-encoded sha256 (32 bytes).
	if c.Auth.PrimaryPullKeyHash != "" {
		b, err := hex.DecodeString(c.Auth.PrimaryPullKeyHash)
		if err != nil || len(b) != sha256.Size {
			return fmt.Errorf("PRIMARY_PULL_KEY_HASH must be a hex-encoded sha256 (64 hex chars); got %d-char value", len(c.Auth.PrimaryPullKeyHash))
		}
	}

	return nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvDuration gets a duration from environment variable or returns default
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// getEnvSlice gets a comma-separated environment variable as a slice
func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		var result []string
		for i := 0; i < len(value); i++ {
			start := i
			for i < len(value) && value[i] != ',' {
				i++
			}
			if item := value[start:i]; item != "" {
				result = append(result, item)
			}
		}
		return result
	}
	return defaultValue
}

// normalizeRegistryURL ensures the registry URL has an https:// scheme
func normalizeRegistryURL(url string) string {
	if url == "" {
		return url
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "https://" + url
	}
	return url
}
