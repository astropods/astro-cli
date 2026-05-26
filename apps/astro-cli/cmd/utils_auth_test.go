package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
)

func authTestJWT(exp time.Time) string {
	payload, err := json.Marshal(map[string]any{"exp": exp.Unix()})
	if err != nil {
		panic(err)
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return "header." + enc + ".sig"
}

func TestApiCallForAccount_RetriesOn401(t *testing.T) {
	_ = os.Unsetenv(auth.EnvAccessToken)

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	writeAccountTestCredentials(t, &auth.Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*auth.Profile{
			"default": {
				AccessToken:  authTestJWT(time.Now().Add(-10 * time.Minute)),
				RefreshToken: "valid_refresh_token",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				User: &auth.StoredUser{
					ID:          "user-1",
					Email:       "test@example.com",
					AccountName: "alice",
					AccountID:   "acct-1",
				},
				Accounts: []auth.StoredAccount{
					{ID: "acct-1", Name: "alice", Type: "personal"},
				},
			},
		},
	})

	workos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "refreshed_access_token",
			RefreshToken: "new_refresh_token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	}))
	defer workos.Close()
	auth.SetWorkOSBaseURLOverride(workos.URL)
	t.Cleanup(func() { auth.SetWorkOSBaseURLOverride("") })

	attempts := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		require.Equal(t, "Bearer refreshed_access_token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer api.Close()

	var dest map[string]string
	status, err := apiCallForAccount(context.Background(), http.MethodGet, api.URL, nil, "alice", false, &dest)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, 2, attempts)
	require.Equal(t, "true", dest["ok"])
}

func TestGetDockerRegistryAuth_UsesFreshAccountToken(t *testing.T) {
	_ = os.Unsetenv(auth.EnvAccessToken)

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	writeAccountTestCredentials(t, &auth.Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*auth.Profile{
			"default": {
				AccessToken:  authTestJWT(time.Now().Add(-10 * time.Minute)),
				RefreshToken: "valid_refresh_token",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				User: &auth.StoredUser{
					ID:          "user-1",
					Email:       "test@example.com",
					AccountName: "alice",
					AccountID:   "acct-1",
				},
				Accounts: []auth.StoredAccount{
					{ID: "acct-1", Name: "alice", Type: "personal"},
				},
			},
		},
	})

	workos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "fresh_push_token",
			RefreshToken: "new_refresh_token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	}))
	defer workos.Close()
	auth.SetWorkOSBaseURLOverride(workos.URL)
	t.Cleanup(func() { auth.SetWorkOSBaseURLOverride("") })

	authStr, err := getDockerRegistryAuth(context.Background(), "alice")
	require.NoError(t, err)
	decoded, err := base64.URLEncoding.DecodeString(authStr)
	require.NoError(t, err)
	require.Contains(t, string(decoded), "fresh_push_token")
}
