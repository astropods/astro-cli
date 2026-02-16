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
	Database   DatabaseConfig
	Admin      AdminConfig
}

// AdminConfig holds admin API configuration
type AdminConfig struct {
	Username string
	Password string
	Enabled  bool
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	URL string // DATABASE_URL connection string (postgres://user:pass@host:port/dbname?sslmode=disable)
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
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	Port            string
	Host            string
	Mode            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	StaticDir       string // Directory containing static frontend assets (empty to disable)
	CLIDir          string // Path to CLI binaries in container for /download/* (empty to disable)
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
	RegistryURL       string // ECR registry URL (required for eks mode, defaults to "docker.io/library" for local)
	ProxyRegistryHost string // Proxy registry host (e.g., registry.example.com)
	Environment       string // Environment prefix for ECR tenant repos (e.g. "prod", "preview")
	EKSClusterName    string // EKS cluster name (required for eks mode)
	K8sMasterURL      string // K8s API server endpoint (required for eks mode)
	AWSRegion         string // AWS region (optional, auto-detected from IRSA)
	// K8s client mode: "eks" (default) or "local" (Docker Desktop / kind / minikube)
	K8sClientMode  string // K8S_CLIENT_MODE
	KubeconfigPath string // KUBECONFIG path (local mode, defaults to ~/.kube/config)
	KubeContext    string // KUBE_CONTEXT (local mode, defaults to current-context)
	// Ingress configuration for agent workloads (agents.astromode.ai)
	IngressDomain     string // Domain for agent ingress (e.g., agents.astromode.ai)
	ACMCertificateARN string // ACM certificate ARN for HTTPS
	ALBGroupName      string // ALB group name for agent ALB
	// Ingress configuration for ingestion workloads (ingestion.astromode.ai)
	IngestionIngressDomain string // Domain for ingestion webhook ingress (e.g., ingestion.astromode.ai)
	IngestionACMCertARN    string // ACM certificate ARN for ingestion wildcard cert
	IngestionALBGroupName  string // ALB group name for ingestion ALB (separate from agents)
	// Observability (Galileo) — injected into every collector sidecar
	GalileoAPIKey  string // GALILEO_API_KEY
	GalileoProject string // GALILEO_PROJECT
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
			StaticDir:       getEnv("STATIC_DIR", ""),
			CLIDir:          getEnv("CLI_DIR", ""),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "text"),
		},
		Security: SecurityConfig{
			AllowedOrigins: getEnvSlice("ALLOWED_ORIGINS", []string{"*"}),
			TrustedProxies: getEnvSlice("TRUSTED_PROXIES", []string{}),
		},
		Deployment: DeploymentConfig{
			RegistryURL:       getEnv("REGISTRY_URL", ""),
			ProxyRegistryHost: getEnv("PROXY_REGISTRY_HOST", ""),
			Environment:       getEnv("ENVIRONMENT", ""),
			EKSClusterName:    getEnv("EKS_CLUSTER_NAME", ""),
			K8sMasterURL:      getEnv("K8S_MASTER_URL", ""),
			AWSRegion:         getEnv("AWS_REGION", ""),
			K8sClientMode:     getEnv("K8S_CLIENT_MODE", "eks"),
			KubeconfigPath:    getEnv("KUBECONFIG", ""),
			KubeContext:       getEnv("KUBE_CONTEXT", ""),
			IngressDomain:          getEnv("INGRESS_DOMAIN", ""),
			ACMCertificateARN:      getEnv("ACM_CERTIFICATE_ARN", ""),
			ALBGroupName:           getEnv("ALB_GROUP_NAME", "astro-agents"),
			IngestionIngressDomain: getEnv("INGESTION_INGRESS_DOMAIN", ""),
			IngestionACMCertARN:    getEnv("INGESTION_ACM_CERTIFICATE_ARN", ""),
			IngestionALBGroupName:  getEnv("INGESTION_ALB_GROUP_NAME", ""),
			GalileoAPIKey:     getEnv("GALILEO_API_KEY", ""),
			GalileoProject:    getEnv("GALILEO_PROJECT", ""),
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
		},
		Database: DatabaseConfig{
			URL: getEnv("DATABASE_URL", ""),
		},
		Admin: AdminConfig{
			Username: getEnv("ADMIN_BASIC_USER", ""),
			Password: getEnv("ADMIN_BASIC_PASSWORD", ""),
			Enabled:  getEnv("ADMIN_ENABLED", "true") == "true",
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
		if c.Deployment.K8sClientMode == "local" {
			c.Deployment.RegistryURL = "docker.io/library"
		} else {
			return fmt.Errorf("REGISTRY_URL environment variable is required")
		}
	}

	if c.Deployment.Environment == "" {
		return fmt.Errorf("ENVIRONMENT environment variable is required")
	}

	if c.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL environment variable is required")
	}

	// Validate auth configuration
	if c.Auth.WorkOSAPIKey == "" {
		return fmt.Errorf("WORKOS_API_KEY environment variable is required")
	}
	if c.Auth.WorkOSClientID == "" {
		return fmt.Errorf("WORKOS_CLIENT_ID environment variable is required")
	}
	if c.Auth.RedirectURI == "" {
		return fmt.Errorf("WORKOS_REDIRECT_URI environment variable is required")
	}
	if c.Auth.CookiePassword == "" {
		return fmt.Errorf("AUTH_COOKIE_PASSWORD environment variable is required (must be at least 32 characters)")
	}
	if len(c.Auth.CookiePassword) < 32 {
		return fmt.Errorf("AUTH_COOKIE_PASSWORD must be at least 32 characters for secure encryption")
	}

	// Validate admin configuration when enabled
	if c.Admin.Enabled {
		if c.Admin.Username == "" {
			return fmt.Errorf("ADMIN_BASIC_USER environment variable is required when admin is enabled")
		}
		if c.Admin.Password == "" {
			return fmt.Errorf("ADMIN_BASIC_PASSWORD environment variable is required when admin is enabled")
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
