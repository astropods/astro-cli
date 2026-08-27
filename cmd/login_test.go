package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loginTestProfile(accounts []auth.StoredAccount) *auth.Profile {
	return &auth.Profile{
		User: &auth.StoredUser{
			AccountName: "alice",
			AccountID:   "acct_personal",
		},
		Accounts: accounts,
	}
}

func TestApplyPriorLoginAccount(t *testing.T) {
	accounts := []auth.StoredAccount{
		{ID: "acct_personal", Name: "alice", Type: "personal"},
		{ID: "acct_acme", Name: "acme-corp", Type: "organization"},
		{ID: "acct_other", Name: "other-org", Type: "organization"},
	}

	t.Run("no prior selection", func(t *testing.T) {
		profile := loginTestProfile(accounts)
		unavailable := applyPriorLoginAccount(profile, "", "")
		assert.Empty(t, unavailable)
		assert.Empty(t, profile.CurrentAccount)
	})

	t.Run("restores current and previous", func(t *testing.T) {
		profile := loginTestProfile(accounts)
		unavailable := applyPriorLoginAccount(profile, "acme-corp", "other-org")
		assert.Empty(t, unavailable)
		assert.Equal(t, "acme-corp", profile.CurrentAccount)
		assert.Equal(t, "other-org", profile.PreviousAccount)
	})

	t.Run("restores current without invalid previous", func(t *testing.T) {
		profile := loginTestProfile(accounts)
		unavailable := applyPriorLoginAccount(profile, "acme-corp", "gone-org")
		assert.Empty(t, unavailable)
		assert.Equal(t, "acme-corp", profile.CurrentAccount)
		assert.Empty(t, profile.PreviousAccount)
	})

	t.Run("prior account removed from membership", func(t *testing.T) {
		profile := loginTestProfile(accounts)
		unavailable := applyPriorLoginAccount(profile, "gone-org", "alice")
		assert.Equal(t, "gone-org", unavailable)
		assert.Empty(t, profile.CurrentAccount)
	})
}

func TestRestorePriorLoginSelection(t *testing.T) {
	accounts := []auth.StoredAccount{
		{ID: "acct_personal", Name: "alice", Type: "personal"},
		{ID: "acct_acme", Name: "acme-corp", Type: "organization"},
	}

	t.Run("fetch failed preserves selection when prior accounts list backs it", func(t *testing.T) {
		profile := loginTestProfile(accounts)
		unavailable := restorePriorLoginSelection(profile, "acme-corp", "alice", accounts, false)
		require.Empty(t, unavailable)
		require.Equal(t, "acme-corp", profile.CurrentAccount)
		require.Equal(t, "alice", profile.PreviousAccount)
	})

	t.Run("fetch failed skips orphan current without prior accounts", func(t *testing.T) {
		profile := loginTestProfile(nil)
		unavailable := restorePriorLoginSelection(profile, "acme-corp", "alice", nil, false)
		require.Empty(t, unavailable)
		require.Empty(t, profile.CurrentAccount)
	})

	t.Run("fetch succeeded validates membership", func(t *testing.T) {
		profile := loginTestProfile(accounts)
		unavailable := restorePriorLoginSelection(profile, "gone-org", "", accounts, true)
		require.Equal(t, "gone-org", unavailable)
		require.Empty(t, profile.CurrentAccount)
	})

	t.Run("booted from org on fresh list does not restore org", func(t *testing.T) {
		onlyPersonal := []auth.StoredAccount{
			{ID: "acct_personal", Name: "alice", Type: "personal"},
		}
		profile := loginTestProfile(onlyPersonal)
		unavailable := restorePriorLoginSelection(profile, "acme-corp", "", accounts, true)
		require.Equal(t, "acme-corp", unavailable)
		require.Empty(t, profile.CurrentAccount)
	})

	t.Run("fetch succeeded with empty profile does not warn unavailable", func(t *testing.T) {
		profile := loginTestProfile(nil)
		unavailable := restorePriorLoginSelection(profile, "acme-corp", "", accounts, true)
		require.Empty(t, unavailable)
		require.Empty(t, profile.CurrentAccount)
	})
}

func TestMergePriorAccountsOnFetchFailure(t *testing.T) {
	prior := []auth.StoredAccount{
		{ID: "acct_personal", Name: "alice", Type: "personal"},
		{ID: "acct_acme", Name: "acme-corp", Type: "organization"},
	}
	profile := &auth.Profile{User: &auth.StoredUser{Email: "user@example.com"}}

	mergePriorAccountsOnFetchFailure(profile, prior, "alice", "acct_personal")

	require.Equal(t, prior, profile.Accounts)
	require.Equal(t, "alice", profile.User.AccountName)
	require.Equal(t, "acct_personal", profile.User.AccountID)
}

func TestLoginPreservesCurrentAccountOnReLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	prior := accountTestCreds("acme-corp")
	prior.Profiles["default"].PreviousAccount = "other-org"
	writeAccountTestCredentials(t, prior)

	storage := accountNewStorage()
	newProfile := loginTestProfile(prior.Profiles["default"].Accounts)
	newProfile.AccessToken = "new_tok"
	newProfile.RefreshToken = "new_ref"
	newProfile.ExpiresAt = time.Now().Add(time.Hour)

	require.Empty(t, applyPriorLoginAccount(newProfile, "acme-corp", "other-org"))
	require.NoError(t, storage.SaveProfile("default", newProfile))

	account, err := storage.GetCurrentAccount()
	require.NoError(t, err)
	require.Equal(t, "acme-corp", account)

	loaded, err := storage.GetCurrentProfile()
	require.NoError(t, err)
	require.Equal(t, "other-org", loaded.PreviousAccount)
}

func TestLoginAccountFlagOverridesPriorSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	writeAccountTestCredentials(t, accountTestCreds("acme-corp"))

	storage := accountNewStorage()
	newProfile := loginTestProfile(accountTestCreds("").Profiles["default"].Accounts)
	newProfile.AccessToken = "new_tok"
	newProfile.RefreshToken = "new_ref"
	newProfile.ExpiresAt = time.Now().Add(time.Hour)

	// --account path skips applyPriorLoginAccount; SetCurrentAccount runs after save.
	require.NoError(t, storage.SaveProfile("default", newProfile))
	require.NoError(t, storage.SetCurrentAccount("other-org"))

	account, err := storage.GetCurrentAccount()
	require.NoError(t, err)
	require.Equal(t, "other-org", account)
}

func loginWaitTestServers(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	pending := 2
	workos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/authorize/device") {
			_ = json.NewEncoder(w).Encode(auth.DeviceAuthorizationResponse{
				DeviceCode:      "device-code",
				UserCode:        "DVLD-XQFB",
				VerificationURI: "https://example.test/device",
				ExpiresIn:       30,
				Interval:        1,
			})
			return
		}
		if pending > 0 {
			pending--
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(auth.TokenError{Error: auth.ErrorAuthorizationPending})
			return
		}
		_ = json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
			User:         &auth.User{ID: "user-1", Email: "test@example.com"},
		})
	}))
	t.Cleanup(workos.Close)
	auth.SetWorkOSBaseURLOverride(workos.URL)
	t.Cleanup(func() { auth.SetWorkOSBaseURLOverride("") })

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": []map[string]string{{"id": "acct-1", "name": "alice", "type": "personal"}},
		})
	}))
	t.Cleanup(api.Close)
	priorServerURL := buildinfo.DefaultServerURL
	buildinfo.DefaultServerURL = api.URL
	t.Cleanup(func() { buildinfo.DefaultServerURL = priorServerURL })

	require.NoError(t, loginCmd.Flags().Set("no-browser", "true"))
	t.Cleanup(func() { _ = loginCmd.Flags().Set("no-browser", "false") })
}

func TestLoginWaitLine(t *testing.T) {
	tests := []struct {
		name   string
		isTTY  bool
		width  int
		assert func(t *testing.T, out string)
	}{
		{
			name:  "piped output prints the line once",
			isTTY: false,
			width: 120,
			assert: func(t *testing.T, out string) {
				assert.Equal(t, 1, strings.Count(out, "Waiting for authentication"))
				assert.Contains(t, out, "Waiting for authentication...\n")
				assert.NotContains(t, out, "\r")
			},
		},
		{
			name:  "terminal redraws the line in place",
			isTTY: true,
			width: 120,
			assert: func(t *testing.T, out string) {
				assert.Greater(t, strings.Count(out, "Waiting for authentication"), 1)
				assert.Contains(t, out, "\r")
			},
		},
		{
			// A frame wider than the terminal wraps, and \r rewinds only the
			// current row, so animating stacks a fresh copy on every tick.
			name:  "a terminal too narrow for the frame prints the line once",
			isTTY: true,
			width: utf8.RuneCountInString("→ Waiting for authentication...") - 1,
			assert: func(t *testing.T, out string) {
				assert.Equal(t, 1, strings.Count(out, "Waiting for authentication"))
				assert.NotContains(t, out, "\r")
			},
		},
		{
			name:  "an unmeasurable terminal prints the line once",
			isTTY: true,
			width: 0,
			assert: func(t *testing.T, out string) {
				assert.Equal(t, 1, strings.Count(out, "Waiting for authentication"))
				assert.NotContains(t, out, "\r")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loginWaitTestServers(t)
			prior := stdoutIsTerminal
			stdoutIsTerminal = func() bool { return tt.isTTY }
			priorWidth := stdoutWidth
			stdoutWidth = func() int { return tt.width }
			t.Cleanup(func() {
				stdoutIsTerminal = prior
				stdoutWidth = priorWidth
			})

			var loginErr error
			out := captureStdout(t, func() { loginErr = runLogin(loginCmd, nil) })
			require.NoError(t, loginErr)
			tt.assert(t, out)
		})
	}
}

// fatih/color writes to color.Output, which captured the real stdout at init, so
// swapping os.Stdout alone loses every colored write.
func captureColoredStdout(t *testing.T, fn func()) string {
	t.Helper()
	return captureStdout(t, func() {
		prior := color.Output
		color.Output = os.Stdout
		defer func() { color.Output = prior }()
		fn()
	})
}

func TestPrintVerificationCode(t *testing.T) {
	const boxWidth = 44

	tests := []struct {
		name   string
		width  int
		assert func(t *testing.T, out string)
	}{
		{
			name:  "a wide terminal gets the box",
			width: boxWidth,
			assert: func(t *testing.T, out string) {
				assert.Contains(t, out, "┌")
				assert.Contains(t, out, "│")
				assert.Contains(t, out, "└")
				assert.Contains(t, out, "HGBR-XXXX")
			},
		},
		{
			// The borders are fixed-width, so drawing them one column too wide
			// wraps every row and splits the code off mid-word.
			name:  "a terminal one column too narrow drops the box",
			width: boxWidth - 1,
			assert: func(t *testing.T, out string) {
				assert.NotContains(t, out, "┌")
				assert.NotContains(t, out, "│")
				assert.NotContains(t, out, "└")
				assert.Contains(t, out, "Your verification code is: HGBR-XXXX")
			},
		},
		{
			name:  "an unmeasurable stdout keeps the box",
			width: 0,
			assert: func(t *testing.T, out string) {
				assert.Contains(t, out, "┌")
				assert.Contains(t, out, "HGBR-XXXX")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prior := stdoutWidth
			stdoutWidth = func() int { return tt.width }
			t.Cleanup(func() { stdoutWidth = prior })

			out := captureColoredStdout(t, func() {
				printVerificationCode("HGBR-XXXX", color.New(color.FgYellow))
			})
			tt.assert(t, out)
		})
	}
}

func TestVerificationBoxWidthMatchesItsBorders(t *testing.T) {
	out := captureColoredStdout(t, func() {
		prior := stdoutWidth
		stdoutWidth = func() int { return 200 }
		t.Cleanup(func() { stdoutWidth = prior })
		printVerificationCode("HGBR-XXXX", color.New(color.FgYellow))
	})

	widths := map[int]bool{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		widths[utf8.RuneCountInString(line)] = true
	}
	assert.Len(t, widths, 1,
		"every row must be the same width or the box is already crooked: %q", out)
}
