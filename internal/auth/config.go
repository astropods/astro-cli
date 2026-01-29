package auth

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Build-time configuration (set via ldflags)
var (
	// WorkOSClientID is the public OAuth client ID for device flow
	// Override via: go build -ldflags "-X github.com/postman/astro/apps/astro-cli/internal/auth.WorkOSClientID=client_..."
	WorkOSClientID = "client_01K1VMRDRQ94MV98D9ANFVT7H2"

	// WorkOSBaseURL is the WorkOS API base URL
	WorkOSBaseURL = "https://api.workos.com"

	// ServerURL is the default Astro server URL.
	// Override via: go build -ldflags "-X github.com/postman/astro/apps/astro-cli/internal/auth.ServerURL=https://..."
	// For local dev, use ASTRO_SERVER_URL env var to override.
	ServerURL = ""
)

// Environment variable names
const (
	EnvServerURL    = "ASTRO_SERVER_URL"
	EnvAccessToken  = "ASTRO_ACCESS_TOKEN"
	EnvRefreshToken = "ASTRO_REFRESH_TOKEN"
)

// Config holds CLI configuration
type Config struct {
	ServerURL string `yaml:"server_url,omitempty"`
}

// ConfigDir returns the path to the astro config directory
func ConfigDir() (string, error) {
	// Use XDG_CONFIG_HOME if set, otherwise ~/.config
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "astro"), nil
}

// ConfigPath returns the path to the config file
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// CredentialsPath returns the path to the credentials file
func CredentialsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// LoadConfig loads the CLI configuration from disk
func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// SaveConfig saves the CLI configuration to disk
func SaveConfig(config *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// GetServerURL returns the server URL.
// Priority: ASTRO_SERVER_URL env var > build-time ServerURL constant
func GetServerURL() string {
	if url := os.Getenv(EnvServerURL); url != "" {
		return url
	}
	return ServerURL
}

// GetEnvAccessToken returns the access token from environment if set
func GetEnvAccessToken() string {
	return os.Getenv(EnvAccessToken)
}
