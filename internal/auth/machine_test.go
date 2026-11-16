package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/envvar"
)

// fakeAuthorizationServer serves astro-server's protected-resource metadata,
// AuthKit's metadata, and AuthKit's token endpoint from one host.
type fakeAuthorizationServer struct {
	*httptest.Server

	expiresIn int
	resource  map[string]any
	metadata  map[string]any

	mu        sync.Mutex
	exchanges int
	forms     []map[string]string
}

func newFakeAuthorizationServer(t *testing.T, expiresIn int) *fakeAuthorizationServer {
	t.Helper()
	f := &fakeAuthorizationServer{expiresIn: expiresIn}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	f.resource = map[string]any{"authorization_servers": []string{f.URL}}
	f.metadata = map[string]any{"token_endpoint": f.URL + "/oauth2/token"}
	return f
}

func (f *fakeAuthorizationServer) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/.well-known/oauth-protected-resource":
		_ = json.NewEncoder(w).Encode(f.resource)
	case "/.well-known/oauth-authorization-server":
		_ = json.NewEncoder(w).Encode(f.metadata)
	case "/oauth2/token":
		_ = r.ParseForm()
		f.mu.Lock()
		f.exchanges++
		n := f.exchanges
		f.forms = append(f.forms, map[string]string{
			"grant_type":    r.PostForm.Get("grant_type"),
			"client_id":     r.PostForm.Get("client_id"),
			"client_secret": r.PostForm.Get("client_secret"),
		})
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("app-token-%d", n),
			"token_type":   "Bearer",
			"expires_in":   f.expiresIn,
		})
	default:
		http.NotFound(w, r)
	}
}

func useMachineCredentials(t *testing.T, server *fakeAuthorizationServer) {
	t.Helper()
	SetMachineCredentialsOverride("client_ci", "secret_ci")
	SetMachineServerURLOverride(server.URL)
	t.Cleanup(func() {
		SetMachineCredentialsOverride("", "")
		SetMachineServerURLOverride("")
	})
}

func TestMachineCredentialsDiscoverTheTokenEndpointAndReuseTheToken(t *testing.T) {
	server := newFakeAuthorizationServer(t, 3600)
	useMachineCredentials(t, server)
	m := NewTokenManager("ast-test")

	first, err := m.GetValidAccessToken(context.Background())
	require.NoError(t, err)
	second, err := m.GetValidAccessToken(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "app-token-1", first)
	assert.Equal(t, first, second, "a token an hour from expiry is reused")
	assert.Equal(t, 1, server.exchanges)
	assert.Equal(t, map[string]string{"grant_type": "client_credentials", "client_id": "client_ci", "client_secret": "secret_ci"}, server.forms[0])
	assert.True(t, m.IsAuthenticated(), "client credentials count as signed in with no keyring profile")

	forced, err := m.ForceRefreshAccessToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "app-token-2", forced, "a forced refresh exchanges the credentials again")
}

func TestMachineCredentialsExchangeAgainInsideTheRefreshThreshold(t *testing.T) {
	server := newFakeAuthorizationServer(t, int(RefreshThreshold.Seconds())-60)
	useMachineCredentials(t, server)
	m := NewTokenManager("ast-test")

	_, err := m.GetValidAccessToken(context.Background())
	require.NoError(t, err)
	token, err := m.GetValidAccessToken(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "app-token-2", token, "a token inside the refresh threshold is replaced before it is used")
}

func TestMachineCredentialsDiscoveryFailures(t *testing.T) {
	cases := []struct {
		name     string
		resource map[string]any
		metadata map[string]any
		wantErr  string
	}{
		{name: "server lists no authorization server", resource: map[string]any{"authorization_servers": []string{}}, wantErr: "the server lists none"},
		{name: "authorization server lists no token endpoint", metadata: map[string]any{"issuer": "x"}, wantErr: "the authorization server lists none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := newFakeAuthorizationServer(t, 3600)
			if tc.resource != nil {
				server.resource = tc.resource
			}
			if tc.metadata != nil {
				server.metadata = tc.metadata
			}
			useMachineCredentials(t, server)

			_, err := NewTokenManager("ast-test").GetValidAccessToken(context.Background())

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Zero(t, server.exchanges, "no token request is sent without a discovered endpoint")
		})
	}
}

func TestMachineClientIDReadsTheEnvironmentOnceAndUnsetsIt(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		secret string
		wantID string
	}{
		{name: "both set", id: "client_ci", secret: "secret_ci", wantID: "client_ci"},
		{name: "secret missing", id: "client_ci"},
		{name: "id missing", secret: "secret_ci"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			machineOnce = sync.Once{}
			machineID, machineSecret = "", ""
			t.Cleanup(func() { SetMachineCredentialsOverride("", "") })
			t.Setenv(envvar.ClientID, tc.id)
			t.Setenv(envvar.ClientSecret, tc.secret)

			assert.Equal(t, tc.wantID, MachineClientID())
			_, idSet := os.LookupEnv(envvar.ClientID)
			_, secretSet := os.LookupEnv(envvar.ClientSecret)
			assert.False(t, idSet, "child processes must not inherit the client ID")
			assert.False(t, secretSet, "child processes must not inherit the secret")
		})
	}
}

func TestEnvAccessTokenWinsOverMachineCredentials(t *testing.T) {
	server := newFakeAuthorizationServer(t, 3600)
	useMachineCredentials(t, server)
	envTokenOnce = sync.Once{}
	t.Setenv(envvar.AccessToken, "raw-token")
	t.Cleanup(func() {
		envTokenOnce = sync.Once{}
		envToken = ""
	})

	token, err := NewTokenManager("ast-test").GetValidAccessToken(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "raw-token", token)
	assert.False(t, UsesMachineCredentials())
	assert.Zero(t, server.exchanges)
}
