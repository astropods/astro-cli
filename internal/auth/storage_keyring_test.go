package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

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

func TestStorage_BinariesKeepSeparateOrgTokens(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	keyring.MockInit()

	binaries := []string{"ast", "ast-preview", "ast-dev"}
	for _, bin := range binaries {
		s := &Storage{binaryName: bin, useKeyring: true}
		profile := createTestProfile(bin+"-access", bin+"-refresh", time.Now().Add(time.Hour))
		profile.OrgTokens = map[string]*OrgToken{"org_x": {AccessToken: bin + "-org", ExpiresAt: time.Now().Add(time.Hour)}}
		require.NoError(t, s.SaveProfile("default", profile))
	}

	for _, bin := range binaries {
		t.Run(bin, func(t *testing.T) {
			s := &Storage{binaryName: bin, useKeyring: true}
			profile, err := s.GetCurrentProfile()
			require.NoError(t, err)
			assert.Equal(t, bin+"-org", s.cachedOrgToken("default", "org_x", profile.OrgTokens["org_x"]))
		})
	}

	// Logging out of one binary leaves the others' org tokens in place.
	require.NoError(t, (&Storage{binaryName: "ast-preview", useKeyring: true}).DeleteAllProfiles())
	_, err := keyring.Get(keyringService("ast-preview"), orgTokenKeyringKey("default", "org_x"))
	assert.ErrorIs(t, err, keyring.ErrNotFound)
	for _, bin := range []string{"ast", "ast-dev"} {
		got, err := keyring.Get(keyringService(bin), orgTokenKeyringKey("default", "org_x"))
		require.NoError(t, err, bin)
		assert.Equal(t, bin+"-org", got)
	}
}
