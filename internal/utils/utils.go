package utils

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// DefaultEnvFile is the default name of the environment file (e.g. ".env").
const DefaultEnvFile = ".env"

// ParseAgentName splits a spec name into an optional account and the bare agent name.
// "@my-org/my-agent" → ("my-org", "my-agent")
// "my-agent"         → ("", "my-agent")
func ParseAgentName(raw string) (account, name string) {
	if trimmed, ok := strings.CutPrefix(raw, "@"); ok {
		if idx := strings.Index(trimmed, "/"); idx > 0 && idx < len(trimmed)-1 {
			return trimmed[:idx], trimmed[idx+1:]
		}
	}
	return "", raw
}

// LoadEnvFile reads an env file from workingDir named envFile (e.g. ".env").
// If the file does not exist, returns (nil, nil).
// If the file exists but cannot be read, returns (nil, err). Otherwise returns (envMap, nil).
func LoadEnvFile(workingDir, envFile string) (map[string]string, error) {
	path := filepath.Join(workingDir, envFile)
	// An empty envFile joins to workingDir, so only a regular file counts.
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, nil
	}
	m, err := godotenv.Read(path)
	if err != nil {
		return nil, err
	}
	return m, nil
}
