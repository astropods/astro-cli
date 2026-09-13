package utils

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// DefaultEnvFile is the default name of the environment file (e.g. ".env").
const DefaultEnvFile = ".env"

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
