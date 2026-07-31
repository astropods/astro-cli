package cmd

import (
	"bytes"
	"encoding/json"
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
					{ID: "acct_acme", Name: "acme-corp", Type: "organization", Role: "member", WorkOSOrganizationID: "org_acme"},
					{ID: "acct_other", Name: "other-org", Type: "organization", Role: "owner", WorkOSOrganizationID: "org_other"},
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
