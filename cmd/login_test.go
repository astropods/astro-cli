package cmd

import (
	"testing"
	"time"

	"github.com/astropods/astro-cli/internal/auth"
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
