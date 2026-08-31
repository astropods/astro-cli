package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	require.Error(t, runAccountList(accountListCmd, nil))
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

// ─── account switch: refresh on miss ──────────────────────────────────────────

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

func TestAccountSwitch_RefreshesOnMiss(t *testing.T) {
	setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(accountsResponse("alice", "acme-corp", "other-org", "new-org")) //nolint:errcheck
	}))

	require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"new-org"}))

	account, err := accountNewStorage().GetCurrentAccount()
	require.NoError(t, err)
	require.Equal(t, "new-org", account)

	profile, err := accountNewStorage().GetCurrentProfile()
	require.NoError(t, err)
	require.True(t, auth.HasAccount(profile.Accounts, "new-org"), "refreshed list should persist to the cache")
}

func TestAccountSwitch_StillErrorsWhenRefreshDoesNotFindIt(t *testing.T) {
	setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(accountsResponse("alice", "acme-corp", "other-org")) //nolint:errcheck
	}))

	require.ErrorContains(t, runAccountSwitch(accountSwitchCmd, []string{"no-such-account"}), "no-such-account")
}

func TestAccountSwitch_KnownAccountNeverHitsTheNetwork(t *testing.T) {
	var called bool
	setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		json.NewEncoder(w).Encode(accountsResponse("alice", "acme-corp", "other-org")) //nolint:errcheck
	}))

	require.NoError(t, runAccountSwitch(accountSwitchCmd, []string{"acme-corp"}))
	require.False(t, called, "switching to an already-known account must not fetch")
}

// ─── account list: always refreshes ───────────────────────────────────────────

func TestAccountList_ShowsAccountsCreatedSinceLastLogin(t *testing.T) {
	setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(accountsResponse("alice", "acme-corp", "other-org", "new-org")) //nolint:errcheck
	}))

	buf := &bytes.Buffer{}
	cmd := accountListCmd
	cmd.SetOut(buf)
	require.NoError(t, runAccountList(cmd, nil))
	require.Contains(t, buf.String(), "new-org")
}

func TestAccountList_FallsBackToCacheWhenRefreshFails(t *testing.T) {
	setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	buf := &bytes.Buffer{}
	cmd := accountListCmd
	cmd.SetOut(buf)
	require.NoError(t, runAccountList(cmd, nil))
	require.Contains(t, buf.String(), "acme-corp")
}

// ─── accountsForSelection ──────────────────────────────────────────────────────

func TestAccountsForSelection_PersistsRefreshedList(t *testing.T) {
	setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(accountsResponse("alice", "acme-corp", "other-org", "new-org")) //nolint:errcheck
	}))

	storage := accountNewStorage()
	accounts, err := accountsForSelection(storage)
	require.NoError(t, err)
	require.True(t, auth.HasAccount(accounts, "new-org"))

	profile, err := storage.GetCurrentProfile()
	require.NoError(t, err)
	require.True(t, auth.HasAccount(profile.Accounts, "new-org"), "refreshed list should persist to the cache")
}

func TestAccountsForSelection_FallsBackToCacheOnFailure(t *testing.T) {
	setupAccountRefreshTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	accounts, err := accountsForSelection(accountNewStorage())
	require.NoError(t, err)
	require.True(t, auth.HasAccount(accounts, "acme-corp"))
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
