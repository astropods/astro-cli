package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/stretchr/testify/require"
)

func init() {
	// Use a real storage instance — testing.Testing() prevents the Keychain probe.
	accountNewStorage = func() *auth.Storage { return auth.NewStorage(buildinfo.BinaryName) }
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func writeAccountTestCredentials(t *testing.T, creds *auth.Credentials) {
	t.Helper()
	path, err := auth.CredentialsPath(buildinfo.BinaryName)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	data, err := json.MarshalIndent(creds, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
}

func accountTestCreds(currentAccount string) *auth.Credentials {
	return &auth.Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*auth.Profile{
			"default": {
				AccessToken:    "tok",
				RefreshToken:   "ref",
				ExpiresAt:      time.Now().Add(time.Hour),
				CurrentAccount: currentAccount,
				User: &auth.StoredUser{
					ID:          "u1",
					Email:       "user@example.com",
					AccountName: "alice",
					AccountID:   "acct_personal",
				},
				Accounts: []auth.StoredAccount{
					{ID: "acct_personal", Name: "alice", Type: "personal", Role: "owner"},
					{ID: "acct_acme", Name: "acme-corp", Type: "organization", Role: "member", OrganizationID: "org_acme"},
					{ID: "acct_other", Name: "other-org", Type: "organization", Role: "owner", OrganizationID: "org_other"},
				},
			},
		},
	}
}

func accountListOutput(t *testing.T, currentAccount string) string {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	writeAccountTestCredentials(t, accountTestCreds(currentAccount))
	buf := &bytes.Buffer{}
	cmd := accountListCmd
	cmd.SetOut(buf)
	require.NoError(t, runAccountList(cmd, nil))
	return buf.String()
}

// ─── storage layer ────────────────────────────────────────────────────────────

// TestCurrentAccount exercises get/set/default on a single credential file.
func TestCurrentAccount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds(""))
	s := accountNewStorage()

	// defaults to personal when not explicitly set
	account, err := s.GetCurrentAccount()
	require.NoError(t, err)
	require.Equal(t, "alice", account)

	// set to an org account
	require.NoError(t, s.SetCurrentAccount("acme-corp"))
	account, err = s.GetCurrentAccount()
	require.NoError(t, err)
	require.Equal(t, "acme-corp", account)

	// unknown account returns an error naming the bad value
	require.ErrorContains(t, s.SetCurrentAccount("does-not-exist"), "does-not-exist")
}

// ─── account list output ──────────────────────────────────────────────────────

func TestAccountList(t *testing.T) {
	cases := []struct {
		name           string
		currentAccount string
		wantOut        string
	}{
		{
			name:           "defaults to personal when unset",
			currentAccount: "",
			wantOut:        "✓ alice (personal)\n  acme-corp\n  other-org\n",
		},
		{
			name:           "checkmark on active org",
			currentAccount: "acme-corp",
			wantOut:        "  alice (personal)\n✓ acme-corp\n  other-org\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			out := accountListOutput(t, tc.currentAccount)
			require.Equal(t, tc.wantOut, out)
		})
	}
}

func TestAccountList_NotLoggedIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, &auth.Credentials{
		CurrentProfile: "default",
		Profiles:       map[string]*auth.Profile{},
	})
	require.Equal(t, errAccountNotLoggedIn(), runAccountList(accountListCmd, nil))
}

// ─── account switch ───────────────────────────────────────────────────────────

// TestAccountSwitch_FromPersonal covers switching away from personal and back,
// including switch -, toggling, and unknown account errors.
func TestAccountSwitch_FromPersonal(t *testing.T) {
	setup := func(t *testing.T) {
		t.Helper()
		t.Setenv("HOME", t.TempDir())
		t.Setenv("NO_COLOR", "1")
		writeAccountTestCredentials(t, accountTestCreds("alice"))
	}

	t.Run("to org", func(t *testing.T) {
		setup(t)
		require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"acme-corp"}))
		account, err := accountNewStorage().GetCurrentAccount()
		require.NoError(t, err)
		require.Equal(t, "acme-corp", account)
	})

	t.Run("unknown account", func(t *testing.T) {
		setup(t)
		require.ErrorContains(t, runAccountSwitch(accountSwitchCmd, []string{"no-such-account"}), "no-such-account")
	})

	t.Run("dash returns to previous", func(t *testing.T) {
		setup(t)
		require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"acme-corp"}))
		require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"-"}))
		account, err := accountNewStorage().GetCurrentAccount()
		require.NoError(t, err)
		require.Equal(t, "alice", account)
	})

	t.Run("dash toggles", func(t *testing.T) {
		setup(t)
		for _, name := range []string{"acme-corp", "-", "-", "-"} {
			require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{name}))
		}
		// acme-corp → alice → acme-corp → alice
		account, err := accountNewStorage().GetCurrentAccount()
		require.NoError(t, err)
		require.Equal(t, "alice", account)
	})
}

// TestAccountSwitch_FromUnset covers the first-time switch case where
// CurrentAccount is "" and must fall back to the personal account for previous.
func TestAccountSwitch_FromUnset(t *testing.T) {
	setup := func(t *testing.T) {
		t.Helper()
		t.Setenv("HOME", t.TempDir())
		t.Setenv("NO_COLOR", "1")
		writeAccountTestCredentials(t, accountTestCreds(""))
	}

	t.Run("persists across reload", func(t *testing.T) {
		setup(t)
		require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"other-org"}))
		profile, err := accountNewStorage().GetCurrentProfile()
		require.NoError(t, err)
		require.Equal(t, "other-org", profile.CurrentAccount)
	})

	t.Run("dash after first switch", func(t *testing.T) {
		// Regression: PreviousAccount must resolve to personal, not stay empty.
		setup(t)
		require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"acme-corp"}))
		require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"-"}))
		account, err := accountNewStorage().GetCurrentAccount()
		require.NoError(t, err)
		require.Equal(t, "alice", account)
	})

	t.Run("dash with no previous errors", func(t *testing.T) {
		setup(t)
		require.ErrorContains(t, runAccountSwitch(accountSwitchCmd, []string{"-"}), "no previous account")
	})
}

// TestAccountSwitch_SwitchBackToPersonal covers switching from an org back to personal.
func TestAccountSwitch_SwitchBackToPersonal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	writeAccountTestCredentials(t, accountTestCreds("acme-corp"))

	require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"alice"}))
	account, err := accountNewStorage().GetCurrentAccount()
	require.NoError(t, err)
	require.Equal(t, "alice", account)
}

// ─── account refresh: switch, list, and selection share one fetch path ────────

func setupAccountRefreshTest(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	writeAccountTestCredentials(t, accountTestCreds("alice"))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	accountServerURLOverride = srv.URL
	t.Cleanup(func() { accountServerURLOverride = "" })
}

func accountsResponse(names ...string) map[string]any {
	accounts := make([]map[string]any, len(names))
	for i, name := range names {
		accounts[i] = map[string]any{"id": "acct_" + name, "name": name, "type": "organization"}
	}
	return map[string]any{"accounts": accounts}
}

// refreshEntryPoints run the three surfaces that read the account list and
// return text that should contain the account name under test.
var refreshEntryPoints = map[string]func(t *testing.T) string{
	"switch": func(t *testing.T) string {
		require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"new-org"}))
		account, err := accountNewStorage().GetCurrentAccount()
		require.NoError(t, err)
		return account
	},
	"list": func(t *testing.T) string {
		buf := &bytes.Buffer{}
		cmd := accountListCmd
		cmd.SetOut(buf)
		require.NoError(t, runAccountList(cmd, nil))
		return buf.String()
	},
	"selection": func(t *testing.T) string {
		accounts, err := accountsForSelection(context.Background(), accountNewStorage())
		require.NoError(t, err)
		names := make([]string, len(accounts))
		for i, a := range accounts {
			names[i] = a.Name
		}
		return strings.Join(names, ",")
	},
}

func TestAccountRefresh_FindsAccountCreatedSinceLastLogin(t *testing.T) {
	for name, run := range refreshEntryPoints {
		t.Run(name, func(t *testing.T) {
			setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(accountsResponse("alice", "acme-corp", "other-org", "new-org")) //nolint:errcheck
			}))

			require.Contains(t, run(t), "new-org")

			profile, err := accountNewStorage().GetCurrentProfile()
			require.NoError(t, err)
			require.True(t, auth.HasAccount(profile.Accounts, "new-org"), "refreshed list should persist to the cache")
		})
	}
}

func TestAccountRefresh_FallsBackToCacheWhenFetchFails(t *testing.T) {
	for name, run := range refreshEntryPoints {
		if name == "switch" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			require.Contains(t, run(t), "acme-corp")
		})
	}
}

// setupExpiringTokenTest writes credentials with an access token close to
// expiry and points the WorkOS client at a mock token-refresh server.
func setupExpiringTokenTest(t *testing.T, workOSHandler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")
	writeAccountTestCredentials(t, &auth.Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*auth.Profile{
			"default": {
				AccessToken:    "stale-token",
				RefreshToken:   "refresh-tok",
				ExpiresAt:      time.Now().Add(2 * time.Minute),
				CurrentAccount: "alice",
				Accounts: []auth.StoredAccount{
					{ID: "acct_personal", Name: "alice", Type: "personal", Role: "owner"},
					{ID: "acct_acme", Name: "acme-corp", Type: "organization", Role: "member", OrganizationID: "org_acme"},
				},
			},
		},
	})

	workOSServer := httptest.NewServer(workOSHandler)
	t.Cleanup(workOSServer.Close)
	auth.SetWorkOSBaseURLOverride(workOSServer.URL)
	t.Cleanup(func() { auth.SetWorkOSBaseURLOverride("") })
}

func TestAccountRefresh_FallsBackToCacheWhenTokenRefreshFails(t *testing.T) {
	for name, run := range refreshEntryPoints {
		if name == "switch" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			setupExpiringTokenTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			}))
			require.Contains(t, run(t), "acme-corp")
		})
	}
}

func TestAccountRefresh_RefreshesExpiringAccessTokenFirst(t *testing.T) {
	setupExpiringTokenTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(auth.TokenResponse{AccessToken: "fresh-token", ExpiresIn: 3600}) //nolint:errcheck
	}))

	var sawAuth string
	accountServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(accountsResponse("alice", "new-org")) //nolint:errcheck
	}))
	t.Cleanup(accountServer.Close)
	accountServerURLOverride = accountServer.URL
	t.Cleanup(func() { accountServerURLOverride = "" })

	accounts, err := accountsForSelection(context.Background(), accountNewStorage())
	require.NoError(t, err)
	require.True(t, auth.HasAccount(accounts, "new-org"))
	require.Equal(t, "Bearer fresh-token", sawAuth, "must fetch with the refreshed token, not the stale cached one")
}

// Unlike list and selection, switch reports its own error instead of
// falling back to the cache.
func TestAccountSwitch_StillErrorsWhenRefreshDoesNotFindIt(t *testing.T) {
	setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(accountsResponse("alice", "acme-corp", "other-org")) //nolint:errcheck
	}))

	require.ErrorContains(t, runAccountSwitch(accountSwitchCmd, []string{"no-such-account"}), "no-such-account")
}

// Unlike list and selection, switch only fetches on a miss, so a known name
// must never hit the network.
func TestAccountSwitch_KnownAccountNeverHitsTheNetwork(t *testing.T) {
	var called bool
	setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		json.NewEncoder(w).Encode(accountsResponse("alice", "acme-corp", "other-org")) //nolint:errcheck
	}))

	require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"acme-corp"}))
	require.False(t, called, "switching to an already-known account must not fetch")
}

func TestAccountOrgID(t *testing.T) {
	accounts := []auth.StoredAccount{
		{ID: "acct_personal", Name: "alice", Type: "personal", OrganizationID: "org_alice"},
		{ID: "acct_acme", Name: "acme-corp", Type: "organization", OrganizationID: "org_acme"},
		{ID: "acct_pending", Name: "pending", Type: "personal"},
	}

	tests := []struct {
		name    string
		account string
		want    string
	}{
		{name: "personal account is scoped like any other", account: "alice", want: "org_alice"},
		{name: "organization account", account: "acme-corp", want: "org_acme"},
		{name: "name match ignores case", account: "ACME-Corp", want: "org_acme"},
		{name: "account whose organization is not linked yet", account: "pending", want: ""},
		{name: "account absent from the profile", account: "nope", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, accountOrgID(accounts, tt.account))
		})
	}
}
