package auth

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// WorkOSBaseURL is the WorkOS API base URL.
const WorkOSBaseURL = "https://api.workos.com"

// Environment variable names
const (
	EnvAccessToken  = "ASTRO_ACCESS_TOKEN"
	EnvRefreshToken = "ASTRO_REFRESH_TOKEN"
)

// ConfigDir returns ~/.{binaryName} (e.g. ~/.ast, ~/.ast-dev, ~/.ast-preview).
// binaryName must be non-empty; callers should pass buildinfo.BinaryName.
func ConfigDir(binaryName string) (string, error) {
	binaryName = strings.TrimSpace(binaryName)
	if binaryName == "" {
		return "", fmt.Errorf("ConfigDir: binaryName must not be empty")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "."+binaryName), nil
}

// CredentialsPath returns the path to the credentials file
func CredentialsPath(binaryName string) (string, error) {
	dir, err := ConfigDir(binaryName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// RegistryURLFromServerURL derives the registry URL from a server URL.
// Registry is always the subdomain registry.<hostname> with the same scheme.
func RegistryURLFromServerURL(serverURL string) string {
	u, err := url.Parse(serverURL)
	if err != nil {
		return ""
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	hostname := u.Hostname()
	if hostname == "" {
		return ""
	}
	return scheme + "://registry." + hostname
}

// envToken caches the access token read from environment
// This allows us to clear the env var after first read to prevent
// child processes from inheriting it
var (
	envToken     string
	envTokenOnce sync.Once
)

// GetEnvAccessToken returns the access token from environment if set.
// On first call, it reads and caches the token, then clears the environment
// variable to prevent child processes from inheriting it.
func GetEnvAccessToken() string {
	envTokenOnce.Do(func() {
		envToken = strings.TrimSpace(os.Getenv(EnvAccessToken))
		if envToken != "" {
			_ = os.Unsetenv(EnvAccessToken)
		}
	})
	return envToken
}
