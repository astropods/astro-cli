package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/envvar"
)

// machineServerURLOverride is set in tests to redirect discovery to a test server.
var machineServerURLOverride string

// SetMachineServerURLOverride redirects machine-credential discovery in tests.
func SetMachineServerURLOverride(url string) {
	machineServerURLOverride = url
}

// SetMachineCredentialsOverride replaces the client credentials read from the
// environment, in tests. Empty values turn machine credentials off.
func SetMachineCredentialsOverride(clientID, secret string) {
	machineOnce.Do(func() {})
	machineMu.Lock()
	defer machineMu.Unlock()
	machineID, machineSecret, machineSource = "", "", nil
	if clientID != "" && secret != "" {
		machineID, machineSecret = clientID, secret
	}
}

var (
	machineOnce   sync.Once
	machineID     string
	machineSecret string

	machineMu     sync.Mutex
	machineSource oauth2.TokenSource
)

// MachineClientID returns ASTRO_CLIENT_ID when both it and ASTRO_CLIENT_SECRET
// are set, and "" otherwise. The first call reads and unsets both variables so
// child processes don't inherit them.
func MachineClientID() string {
	machineOnce.Do(func() {
		id := strings.TrimSpace(os.Getenv(envvar.ClientID))
		secret := strings.TrimSpace(os.Getenv(envvar.ClientSecret))
		if id != "" && secret != "" {
			machineID, machineSecret = id, secret
		}
		_ = os.Unsetenv(envvar.ClientID)
		_ = os.Unsetenv(envvar.ClientSecret)
	})
	return machineID
}

// UsesMachineCredentials reports whether this process signs in as an app.
func UsesMachineCredentials() bool {
	return MachineClientID() != "" && GetEnvAccessToken() == ""
}

func (m *TokenManager) machineAccessToken(ctx context.Context, force bool) (string, error) {
	machineMu.Lock()
	defer machineMu.Unlock()
	if machineSource == nil || force {
		tokenURL, err := discoverTokenURL(ctx)
		if err != nil {
			return "", err
		}
		cfg := clientcredentials.Config{
			ClientID:     machineID,
			ClientSecret: machineSecret,
			TokenURL:     tokenURL,
			AuthStyle:    oauth2.AuthStyleInParams,
		}
		machineSource = oauth2.ReuseTokenSourceWithExpiry(nil, cfg.TokenSource(context.Background()), RefreshThreshold)
	}
	token, err := machineSource.Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.ErrorCode == "invalid_client" {
			return "", fmt.Errorf("%s was refused: %w", envvar.ClientID, err)
		}
		return "", fmt.Errorf("get app access token: %w", err)
	}
	return token.AccessToken, nil
}

// discoverTokenURL follows the server's protected-resource metadata (RFC 9728)
// to its authorization server's metadata (RFC 8414).
func discoverTokenURL(ctx context.Context) (string, error) {
	server := machineServerURLOverride
	if server == "" {
		server = buildinfo.DefaultServerURL
	}
	var resource struct {
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := getJSON(ctx, strings.TrimSuffix(server, "/")+"/.well-known/oauth-protected-resource", &resource); err != nil {
		return "", fmt.Errorf("discover authorization server: %w", err)
	}
	if len(resource.AuthorizationServers) == 0 {
		return "", errors.New("discover authorization server: the server lists none")
	}
	var metadata struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	issuer := strings.TrimSuffix(resource.AuthorizationServers[0], "/")
	if err := getJSON(ctx, issuer+"/.well-known/oauth-authorization-server", &metadata); err != nil {
		return "", fmt.Errorf("discover token endpoint: %w", err)
	}
	if metadata.TokenEndpoint == "" {
		return "", errors.New("discover token endpoint: the authorization server lists none")
	}
	return metadata.TokenEndpoint, nil
}

func getJSON(ctx context.Context, url string, dest any) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s returned status %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}
