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
//
// The regular-file check is load-bearing, not defensive. An empty envFile makes
// filepath.Join collapse to workingDir, which Stat resolves happily because a
// directory exists there, so the not-found branch never fired and godotenv got
// handed a directory — surfacing as "failed to read .env file: read <project>:
// is a directory". A caller reading an unregistered flag gets "" for free (see
// flagString), so any command that forgets to register --env would break this
// way. Requiring a regular file fixes every caller at once, present and future.
func LoadEnvFile(workingDir, envFile string) (map[string]string, error) {
	path := filepath.Join(workingDir, envFile)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil
	}
	m, err := godotenv.Read(path)
	if err != nil {
		return nil, err
	}
	return m, nil
}
