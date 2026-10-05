package auth

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

func setKeyringTokens(t *testing.T, service, accessToken, refreshToken string) {
	t.Helper()
	require.NoError(t, keyring.Set(service, fmt.Sprintf("default_%s", KeyringAccessTokenKey), accessToken))
	require.NoError(t, keyring.Set(service, fmt.Sprintf("default_%s", KeyringRefreshTokenKey), refreshToken))
}

func getKeyringToken(service, key string) (string, error) {
	return keyring.Get(service, fmt.Sprintf("default_%s", key))
}

func writeProfileWithoutTokens(t *testing.T, binaryName string, expiresAt time.Time) {
	t.Helper()
	fileOnly := &Storage{binaryName: binaryName, useKeyring: false}
	require.NoError(t, fileOnly.SaveProfile("default", createTestProfile("", "", expiresAt)))
}

func TestKeyringService(t *testing.T) {
	tests := []struct {
		binaryName string
		want       string
	}{
		{binaryName: "ast", want: "astro-cli"},
		{binaryName: "ast-preview", want: "astro-cli-ast-preview"},
		{binaryName: "ast-dev", want: "astro-cli-ast-dev"},
	}
	for _, tt := range tests {
		t.Run(tt.binaryName, func(t *testing.T) {
			assert.Equal(t, tt.want, keyringService(tt.binaryName))
		})
	}
}

func TestStorage_BinariesKeepSeparateKeyringTokens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	keyring.MockInit()

	binaries := []string{"ast", "ast-preview", "ast-dev"}
	for _, bin := range binaries {
		s := &Storage{binaryName: bin, useKeyring: true}
		require.NoError(t, s.SaveProfile("default", createTestProfile(bin+"-access", bin+"-refresh", time.Now().Add(time.Hour))))
	}

	for _, bin := range binaries {
		t.Run(bin, func(t *testing.T) {
			s := &Storage{binaryName: bin, useKeyring: true}
			profile, err := s.GetCurrentProfile()
			require.NoError(t, err)
			assert.Equal(t, bin+"-access", profile.AccessToken)
			assert.Equal(t, bin+"-refresh", profile.RefreshToken)
		})
	}
}

func TestLoadCredentials_LegacyKeyringItems(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	ownSession := makeTestJWT(expiresAt)
	otherSession := makeTestJWT(expiresAt.Add(time.Hour))

	tests := []struct {
		name         string
		binaryName   string
		legacyAccess string
		ownItems     bool
		wantAccess   string
		wantRefresh  string
	}{
		{
			name:         "ast reads the original service",
			binaryName:   "ast",
			legacyAccess: otherSession,
			wantAccess:   otherSession,
			wantRefresh:  "legacy-refresh",
		},
		{
			name:         "ast-preview falls back to its own legacy session",
			binaryName:   "ast-preview",
			legacyAccess: ownSession,
			wantAccess:   ownSession,
			wantRefresh:  "legacy-refresh",
		},
		{
			name:         "ast-dev ignores another binary's legacy session",
			binaryName:   "ast-dev",
			legacyAccess: otherSession,
		},
		{
			name:         "ast-dev ignores a legacy access token that is not a JWT",
			binaryName:   "ast-dev",
			legacyAccess: "opaque",
		},
		{
			name:         "ast-preview prefers its own service",
			binaryName:   "ast-preview",
			legacyAccess: ownSession,
			ownItems:     true,
			wantAccess:   "own-access",
			wantRefresh:  "own-refresh",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			keyring.MockInit()
			writeProfileWithoutTokens(t, tt.binaryName, expiresAt)
			setKeyringTokens(t, KeyringService, tt.legacyAccess, "legacy-refresh")
			if tt.ownItems {
				setKeyringTokens(t, keyringService(tt.binaryName), "own-access", "own-refresh")
			}

			s := &Storage{binaryName: tt.binaryName, useKeyring: true}
			profile, err := s.GetCurrentProfile()
			require.NoError(t, err)
			assert.Equal(t, tt.wantAccess, profile.AccessToken)
			assert.Equal(t, tt.wantRefresh, profile.RefreshToken)
		})
	}
}

func TestSaveProfile_MovesLegacySessionToOwnService(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	keyring.MockInit()
	expiresAt := time.Now().Add(time.Hour)
	legacyAccess := makeTestJWT(expiresAt)
	writeProfileWithoutTokens(t, "ast-preview", expiresAt)
	setKeyringTokens(t, KeyringService, legacyAccess, "legacy-refresh")

	s := &Storage{binaryName: "ast-preview", useKeyring: true}
	profile, err := s.GetCurrentProfile()
	require.NoError(t, err)
	require.NoError(t, s.SaveProfile("default", profile))

	ownAccess, err := getKeyringToken("astro-cli-ast-preview", KeyringAccessTokenKey)
	require.NoError(t, err)
	assert.Equal(t, legacyAccess, ownAccess)
	ownRefresh, err := getKeyringToken("astro-cli-ast-preview", KeyringRefreshTokenKey)
	require.NoError(t, err)
	assert.Equal(t, "legacy-refresh", ownRefresh)

	// Migration must not log out ast.
	legacyRefresh, err := getKeyringToken(KeyringService, KeyringRefreshTokenKey)
	require.NoError(t, err)
	assert.Equal(t, "legacy-refresh", legacyRefresh)

	// Simulate an ast login.
	setKeyringTokens(t, KeyringService, makeTestJWT(expiresAt.Add(2*time.Hour)), "ast-refresh")
	profile, err = s.GetCurrentProfile()
	require.NoError(t, err)
	assert.Equal(t, legacyAccess, profile.AccessToken)
	assert.Equal(t, "legacy-refresh", profile.RefreshToken)
}

func TestDeleteProfiles_LegacyKeyringItems(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	deletes := map[string]func(*Storage) error{
		"DeleteProfile":     func(s *Storage) error { return s.DeleteProfile("default") },
		"DeleteAllProfiles": func(s *Storage) error { return s.DeleteAllProfiles() },
	}
	tests := []struct {
		name         string
		legacyAccess string
		wantDeleted  bool
	}{
		{name: "own legacy session is deleted", legacyAccess: makeTestJWT(expiresAt), wantDeleted: true},
		{name: "another binary's legacy session is kept", legacyAccess: makeTestJWT(expiresAt.Add(time.Hour))},
	}
	for deleteName, deleteProfiles := range deletes {
		for _, tt := range tests {
			t.Run(deleteName+"/"+tt.name, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				keyring.MockInit()
				writeProfileWithoutTokens(t, "ast-preview", expiresAt)
				setKeyringTokens(t, KeyringService, tt.legacyAccess, "legacy-refresh")
				setKeyringTokens(t, "astro-cli-ast-preview", "own-access", "own-refresh")

				s := &Storage{binaryName: "ast-preview", useKeyring: true}
				require.NoError(t, deleteProfiles(s))

				for _, key := range []string{KeyringAccessTokenKey, KeyringRefreshTokenKey} {
					_, err := getKeyringToken("astro-cli-ast-preview", key)
					assert.ErrorIs(t, err, keyring.ErrNotFound, key)

					_, err = getKeyringToken(KeyringService, key)
					if tt.wantDeleted {
						assert.ErrorIs(t, err, keyring.ErrNotFound, key)
					} else {
						assert.NoError(t, err, key)
					}
				}
			})
		}
	}
}
