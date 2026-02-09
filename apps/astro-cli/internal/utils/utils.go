package utils

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// DefaultEnvFile is the default name of the environment file (e.g. ".env").
const DefaultEnvFile = ".env"

// LoadEnvFile reads an env file from workingDir named envFile (e.g. ".env").
// If the file does not exist, returns (nil, nil).
// If the file exists but cannot be read, returns (nil, err). Otherwise returns (envMap, nil).
func LoadEnvFile(workingDir, envFile string) (map[string]string, error) {
	path := filepath.Join(workingDir, envFile)
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	m, err := godotenv.Read(path)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ImageNameForLocal returns the image name to use for local runs. When isLocal is true,
// strips the remote registry prefix (everything up to and including the last "/") so
// locally built images are used; otherwise returns image unchanged.
func ImageNameForLocal(image string, isLocal bool) string {
	if !isLocal {
		return image
	}
	if i := strings.LastIndex(image, "/"); i >= 0 {
		return image[i+1:]
	}
	return image
}
