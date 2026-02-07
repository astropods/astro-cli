package auth

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	DefaultServerURL = "https://odesdaz.com"
)

// Environment variable names
const (
	EnvServerURL    = "ASTRO_SERVER_URL"
	EnvRegistryURL  = "ASTRO_REGISTRY_URL"
	EnvAccessToken  = "ASTRO_ACCESS_TOKEN"
	EnvRefreshToken = "ASTRO_REFRESH_TOKEN"
)

// ConfigDir returns the path to the astro config directory (~/.astro)
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".astro"), nil
}

// CredentialsPath returns the path to the credentials file
func CredentialsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

// getCurrentProfileURLs returns server_url and registry_url from the current profile in credentials.json.
// Used for URL resolution only; does not load tokens from keyring.
func getCurrentProfileURLs() (serverURL, registryURL string) {
	path, err := CredentialsPath()
	if err != nil {
		return "", ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", ""
	}
	if creds.Profiles == nil {
		return "", ""
	}
	profile, ok := creds.Profiles[creds.CurrentProfile]
	if !ok || profile == nil {
		return "", ""
	}
	return profile.ServerURL, profile.RegistryURL
}

// GetServerURL returns the server URL.
// Priority: ASTRO_SERVER_URL env var > current profile server_url > default (example.com)
func GetServerURL() string {
	if url := os.Getenv(EnvServerURL); url != "" {
		return url
	}
	if serverURL, _ := getCurrentProfileURLs(); serverURL != "" {
		return serverURL
	}
	return DefaultServerURL
}

// GetRegistryURL returns the registry URL.
// Priority: ASTRO_REGISTRY_URL env var > current profile registry_url > default (registry.example.com)
func GetRegistryURL() string {
	if url := os.Getenv(EnvRegistryURL); url != "" {
		return url
	}
	if _, registryURL := getCurrentProfileURLs(); registryURL != "" {
		return registryURL
	}
	return RegistryURLFromServerURL(GetServerURL())
}

// NormalizeServerURL normalizes a host or URL to a full server URL (adds https if no scheme).
func NormalizeServerURL(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return DefaultServerURL
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	u, err := url.Parse(host)
	if err != nil {
		return host
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	return strings.TrimSuffix(u.String(), "/")
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
			os.Unsetenv(EnvAccessToken)
		}
	})
	return envToken
}
