package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds astro-queen connection settings loaded from ~/.astro-queen/config.yaml.
type Config struct {
	Server       string `yaml:"-"`             // set at runtime by the environment subcommand
	OPAccount    string `yaml:"op_account"`    // 1Password account name for desktop app integration
	RefreshToken string `yaml:"refresh_token"` // WorkOS refresh token for API client auth
}

// CertFile returns the conventional client cert path.
func CertFile() string { return filepath.Join(filepath.Dir(DefaultPath()), "client.crt") }

// KeyFile returns the conventional client key path.
func KeyFile() string { return filepath.Join(filepath.Dir(DefaultPath()), "client.key") }

// CAFile returns the conventional CA cert path.
func CAFile() string { return filepath.Join(filepath.Dir(DefaultPath()), "ca.crt") }

// DefaultPath returns the default config file path.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".astro-queen", "config.yaml")
}

// Load reads the config file.
// If the file doesn't exist, returns a default config.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}
