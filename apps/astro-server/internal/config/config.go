package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the application configuration
type Config struct {
	Server     ServerConfig
	Log        LogConfig
	Security   SecurityConfig
	Deployment DeploymentConfig
	Auth       AuthConfig
}

// AuthConfig holds WorkOS authentication configuration
type AuthConfig struct {
	WorkOSAPIKey   string
	WorkOSClientID string
	RedirectURI    string
	FrontendURL    string
	CookieName     string
	CookiePassword string
	CookieDomain   string
	CookieSecure   bool
	CookieSameSite string // "Strict", "Lax", or "None"
	CookieMaxAge   time.Duration
	SessionMaxAge  time.Duration
	JWTIssuer      string
	Enabled        bool
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

// DeploymentConfig holds deployment-related configuration
type DeploymentConfig struct {
	ArtifactDir       string
	RegistryURL       string
	K8sKubeconfigPath string
	K8sInCluster      bool
	K8sMasterURL      string
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnv("PORT", "8080"),
			Host:            getEnv("HOST", "0.0.0.0"),
			Mode:            getEnv("GIN_MODE", "release"),
			ReadTimeout:     getEnvDuration("READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getEnvDuration("WRITE_TIMEOUT", 10*time.Second),
			ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		Security: SecurityConfig{
			AllowedOrigins: getEnvSlice("ALLOWED_ORIGINS", []string{"*"}),
			TrustedProxies: getEnvSlice("TRUSTED_PROXIES", []string{}),
		},
		Deployment: DeploymentConfig{
			ArtifactDir:       getEnv("ARTIFACT_DIR", "/tmp/astro-artifacts"),
			RegistryURL:       getEnv("REGISTRY_URL", "ghcr.io/saswatds"),
			K8sKubeconfigPath: getEnv("K8S_KUBECONFIG_PATH", ""),
			K8sInCluster:      getEnv("K8S_IN_CLUSTER", "false") == "true",
			K8sMasterURL:      getEnv("K8S_MASTER_URL", ""),
		},
		Auth: AuthConfig{
			WorkOSAPIKey:   getEnv("WORKOS_API_KEY", ""),
			WorkOSClientID: getEnv("WORKOS_CLIENT_ID", ""),
			RedirectURI:    getEnv("WORKOS_REDIRECT_URI", "http://localhost:8080/auth/callback"),
			FrontendURL:    getEnv("FRONTEND_URL", "http://localhost:5173"),
			CookieName:     getEnv("AUTH_COOKIE_NAME", "astro_session"),
			CookiePassword: getEnv("AUTH_COOKIE_PASSWORD", ""),
			CookieDomain:   getEnv("AUTH_COOKIE_DOMAIN", ""),
			CookieSecure:   getEnv("AUTH_COOKIE_SECURE", "false") == "true",
			CookieSameSite: getEnv("AUTH_COOKIE_SAMESITE", "Lax"),
			CookieMaxAge:   getEnvDuration("AUTH_COOKIE_MAX_AGE", 7*24*time.Hour),
			SessionMaxAge:  getEnvDuration("AUTH_SESSION_MAX_AGE", 24*time.Hour),
			JWTIssuer:      getEnv("AUTH_JWT_ISSUER", "https://api.workos.com"),
			Enabled:        getEnv("AUTH_ENABLED", "true") == "true",
		},
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

	if c.Deployment.RegistryURL == "" {
		return fmt.Errorf("REGISTRY_URL environment variable is required")
	}

	// Validate auth configuration when enabled
	if c.Auth.Enabled {
		if c.Auth.WorkOSAPIKey == "" {
			return fmt.Errorf("WORKOS_API_KEY environment variable is required when auth is enabled")
		}
		if c.Auth.WorkOSClientID == "" {
			return fmt.Errorf("WORKOS_CLIENT_ID environment variable is required when auth is enabled")
		}
		if c.Auth.RedirectURI == "" {
			return fmt.Errorf("WORKOS_REDIRECT_URI environment variable is required when auth is enabled")
		}
		if c.Auth.CookiePassword == "" {
			return fmt.Errorf("AUTH_COOKIE_PASSWORD environment variable is required when auth is enabled (must be at least 32 characters)")
		}
		if len(c.Auth.CookiePassword) < 32 {
			return fmt.Errorf("AUTH_COOKIE_PASSWORD must be at least 32 characters for secure encryption")
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
		// Simple split by comma, can be enhanced based on needs
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

// getEnvInt gets an integer from environment variable or returns default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
