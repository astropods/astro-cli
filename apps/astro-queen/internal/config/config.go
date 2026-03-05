package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds astro-queen connection settings loaded from ~/.astro-queen/config.yaml.
// Cert/Key/CA values accept file paths or inline PEM (auto-detected by "-----BEGIN" prefix).
type Config struct {
	Server   string `yaml:"server"`    // e.g. "localhost:9091"
	CertFile string `yaml:"cert_file"` // client cert — file path or inline PEM (mTLS)
	KeyFile  string `yaml:"key_file"`  // client key  — file path or inline PEM (mTLS)
	CAFile   string `yaml:"ca_file"`   // CA cert     — file path or inline PEM (mTLS)
}

// DefaultPath returns the default config file path.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".astro-queen", "config.yaml")
}

// Load reads the config file, expanding ~ in file paths.
// If the file doesn't exist, returns a default config pointing to localhost:9091.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Server: "localhost:9091"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.CertFile = expandHome(cfg.CertFile)
	cfg.KeyFile = expandHome(cfg.KeyFile)
	cfg.CAFile = expandHome(cfg.CAFile)

	if cfg.Server == "" {
		cfg.Server = "localhost:9091"
	}

	return &cfg, nil
}

func expandHome(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
