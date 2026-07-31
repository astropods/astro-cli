package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"

	"github.com/astropods/astro-cli/internal/auth"
)

const envDisable = "ASTRO_NO_TELEMETRY"

// TelemetryConfig is persisted at ~/.ast/telemetry.json.
type TelemetryConfig struct {
	Enabled  bool   `json:"enabled"`
	Noticed  bool   `json:"noticed"`
	DeviceID string `json:"device_id,omitempty"`
}

func configPath(binaryName string) (string, error) {
	dir, err := auth.ConfigDir(binaryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "telemetry.json"), nil
}

// LoadConfig reads telemetry.json. Returns defaults if missing.
func LoadConfig(binaryName string) TelemetryConfig {
	path, err := configPath(binaryName)
	if err != nil {
		return TelemetryConfig{Enabled: true}
	}
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		// File doesn't exist — first run
		return TelemetryConfig{Enabled: true}
	}
	var cfg TelemetryConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return TelemetryConfig{Enabled: true}
	}
	return cfg
}

// SaveConfig writes telemetry.json.
func SaveConfig(binaryName string, cfg TelemetryConfig) error {
	path, err := configPath(binaryName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// IsEnabled checks the env var override and telemetry.json.
func IsEnabled(binaryName string) bool {
	if os.Getenv(envDisable) != "" {
		return false
	}
	return LoadConfig(binaryName).Enabled
}

// SetEnabled persists the enabled state.
func SetEnabled(binaryName string, enabled bool) error {
	cfg := LoadConfig(binaryName)
	cfg.Enabled = enabled
	cfg.Noticed = true
	return SaveConfig(binaryName, cfg)
}

// EnsureNoticed prints a first-run notice and marks it as shown.
func EnsureNoticed(binaryName string) bool {
	cfg := LoadConfig(binaryName)
	if cfg.Noticed {
		return false
	}
	cfg.Noticed = true
	cfg.Enabled = true
	if cfg.DeviceID == "" {
		cfg.DeviceID = generateDeviceID()
	}
	_ = SaveConfig(binaryName, cfg)
	return true // caller should print the notice
}

// GetDeviceID returns a stable anonymous device identifier.
func GetDeviceID(binaryName string) string {
	cfg := LoadConfig(binaryName)
	if cfg.DeviceID != "" {
		return cfg.DeviceID
	}
	id := generateDeviceID()
	cfg.DeviceID = id
	_ = SaveConfig(binaryName, cfg)
	return id
}

func generateDeviceID() string {
	hostname, _ := os.Hostname()
	u, _ := user.Current()
	username := ""
	if u != nil {
		username = u.Username
	}
	h := sha256.Sum256([]byte(hostname + ":" + username))
	return hex.EncodeToString(h[:])
}
