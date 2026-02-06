package config

import (
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
	URL       string // Backend registry URL (ECR)
	AWSRegion string // AWS region for ECR
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Enabled        bool
	JWKSEndpoint   string // WorkOS JWKS endpoint for JWT validation
	JWTIssuer      string // Expected JWT issuer
	WorkOSClientID string // WorkOS client ID (JWT audience)
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
			URL:       normalizeRegistryURL(getEnv("REGISTRY_URL", "")),
			AWSRegion: getEnv("AWS_REGION", ""),
		},
		Auth: AuthConfig{
			Enabled:        getEnv("AUTH_ENABLED", "true") == "true",
			JWKSEndpoint:   getEnv("JWKS_ENDPOINT", "https://api.workos.com/sso/jwks"),
			JWTIssuer:      getEnv("JWT_ISSUER", ""), // Will be auto-constructed if empty
			WorkOSClientID: getEnv("WORKOS_CLIENT_ID", ""),
		},
	}

	// Auto-construct JWT issuer and JWKS endpoint from WorkOS client ID if not explicitly set
	if cfg.Auth.Enabled && cfg.Auth.WorkOSClientID != "" {
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

	if c.Auth.Enabled && c.Auth.WorkOSClientID == "" {
		return fmt.Errorf("WORKOS_CLIENT_ID environment variable is required when auth is enabled")
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
