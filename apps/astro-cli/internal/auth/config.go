package auth

import (
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// Build-time configuration (set via ldflags)
var (
	// WorkOSClientID is the public OAuth client ID for device flow
	// Override via: go build -ldflags "-X github.com/postman/astro/apps/astro-cli/internal/auth.WorkOSClientID=client_..."
	WorkOSClientID = "client_01K1VMRDRQ94MV98D9ANFVT7H2"

	// WorkOSBaseURL is the WorkOS API base URL
	WorkOSBaseURL = "https://api.workos.com"

	// Default host (used when not set in profile or env)
	DefaultServerURL = "https://astromode.ai"
)

// Environment variable names
const (
	EnvAccessToken  = "ASTRO_ACCESS_TOKEN"
	EnvRefreshToken = "ASTRO_REFRESH_TOKEN"
)

// ConfigDir returns the path to the astro config directory.
// Returns ~/.ast-preview when binaryName is "ast-preview", otherwise ~/.ast.
func ConfigDir(binaryName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := ".ast"
	if binaryName == "ast-preview" {
		dir = ".ast-preview"
	}
	return filepath.Join(home, dir), nil
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
		envToken = os.Getenv(EnvAccessToken)
		if envToken != "" {
			_ = os.Unsetenv(EnvAccessToken)
		}
	})
	return envToken
}
