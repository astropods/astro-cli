package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/envvar"
)

// fakeMachineServer answers discovery, the client credentials grant, and
// GET /api/v1/me as an app bound to one account.
func fakeMachineServer(t *testing.T, accounts []map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var meCalls atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"authorization_servers": []string{srv.URL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"token_endpoint": srv.URL + "/oauth2/token"})
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "app-token", "token_type": "Bearer", "expires_in": 3600})
		case "/api/v1/me":
			meCalls.Add(1)
			assert.Equal(t, "Bearer app-token", r.Header.Get("Authorization"), "/me is called with the app's token")
			_ = json.NewEncoder(w).Encode(map[string]any{"accounts": accounts})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &meCalls
}

func useMachineCredentials(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	auth.SetMachineCredentialsOverride("client_ci", "secret_ci")
	auth.SetMachineServerURLOverride(srv.URL)
	accountServerURLOverride = srv.URL
	machineAccount = ""
	t.Cleanup(func() {
		auth.SetMachineCredentialsOverride("", "")
		auth.SetMachineServerURLOverride("")
		accountServerURLOverride = ""
		machineAccount = ""
	})
}

func TestGetCurrentAccountTokenWithMachineCredentials(t *testing.T) {
	acme := []map[string]string{{"id": "acct-acme", "name": "acme", "type": "organization"}}
	cases := []struct {
		name        string
		accounts    []map[string]string
		accountEnv  string
		wantAccount string
		wantErr     error
	}{
		{name: "the account comes from the app, with no profile on disk", accounts: acme, wantAccount: "acme"},
		{name: "ASTRO_ACCOUNT naming the app's account is accepted", accounts: acme, accountEnv: "ACME", wantAccount: "acme"},
		{name: "ASTRO_ACCOUNT naming another account is refused", accounts: acme, accountEnv: "other", wantErr: errMachineAccountMismatch("other", "acme")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := fakeMachineServer(t, tc.accounts)
			useMachineCredentials(t, srv)
			t.Setenv(envvar.Account, tc.accountEnv)

			at, err := getCurrentAccountToken(t.Context())

			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantAccount, at.Account)
			assert.Equal(t, "app-token", at.Token)
		})
	}
}

func TestGetCurrentAccountTokenWithMachineCredentialsLooksUpTheAccountOnce(t *testing.T) {
	srv, meCalls := fakeMachineServer(t, []map[string]string{{"id": "acct-acme", "name": "acme"}})
	useMachineCredentials(t, srv)

	for range 3 {
		_, err := getCurrentAccountToken(t.Context())
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), meCalls.Load(), "the bound account is cached for the process")
}

func TestGetCurrentAccountTokenWithMachineCredentialsNeedsExactlyOneAccount(t *testing.T) {
	srv, _ := fakeMachineServer(t, []map[string]string{})
	useMachineCredentials(t, srv)

	_, err := getCurrentAccountToken(t.Context())

	require.EqualError(t, err, errMachineAccountLookup(nil).Error())
}

func TestForceAccountTokenWithMachineCredentialsSkipsTheProfile(t *testing.T) {
	srv, _ := fakeMachineServer(t, []map[string]string{{"id": "acct-acme", "name": "acme"}})
	useMachineCredentials(t, srv)

	token, err := forceAccountToken(t.Context(), "acme")

	require.NoError(t, err, "the registry push forces a token, and must not need a keyring profile")
	assert.Equal(t, "app-token", token)
}

func TestCommandsThatNeedAPersonRefuseMachineCredentials(t *testing.T) {
	srv, _ := fakeMachineServer(t, nil)
	useMachineCredentials(t, srv)

	assert.EqualError(t, runLogin(loginCmd, nil), errNeedsPerson("login").Error())
	assert.EqualError(t, runAccountSwitch(accountSwitchCmd, []string{"acme"}), errNeedsPerson("account switch").Error())
}
