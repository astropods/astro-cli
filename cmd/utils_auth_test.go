package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/auth"
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

func sessionTestCreds(accessToken, refreshToken string, expiresAt time.Time) *auth.Credentials {
	creds := accountTestCreds("alice")
	profile := creds.Profiles["default"]
	profile.AccessToken = accessToken
	profile.RefreshToken = refreshToken
	profile.ExpiresAt = expiresAt
	return creds
}

func serveWorkOSInvalidGrant(t *testing.T) {
	t.Helper()
	workos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(auth.TokenError{Error: "invalid_grant", ErrorDescription: "Session has already ended."})
	}))
	t.Cleanup(workos.Close)
	auth.SetWorkOSBaseURLOverride(workos.URL)
	t.Cleanup(func() { auth.SetWorkOSBaseURLOverride("") })
}

func TestAccountToken_SaysReauthenticateOnce(t *testing.T) {
	expired := time.Now().Add(-time.Hour)
	tests := []struct {
		name    string
		creds   *auth.Credentials
		account string
		force   bool
		want    error
		wantIs  error
		wantRaw bool
	}{
		{
			name:    "refresh rejected with invalid_grant",
			creds:   sessionTestCreds("expired", "ended-refresh", expired),
			account: "alice",
			want:    errAuthSessionEnded(nil),
			wantIs:  auth.ErrSessionEnded,
		},
		{
			name:    "forced refresh rejected with invalid_grant",
			creds:   sessionTestCreds("tok", "ended-refresh", time.Now().Add(time.Hour)),
			account: "alice",
			force:   true,
			want:    errAuthSessionEnded(nil),
			wantIs:  auth.ErrSessionEnded,
		},
		{
			name:    "expired token and no refresh token",
			creds:   sessionTestCreds("expired", "", expired),
			account: "alice",
			want:    errAuthNoRefreshToken(nil),
			wantIs:  auth.ErrNoRefreshToken,
		},
		{
			name:    "org account and no refresh token",
			creds:   sessionTestCreds("tok", "", time.Now().Add(time.Hour)),
			account: "acme-corp",
			want:    errAuthNoRefreshToken(nil),
			wantIs:  auth.ErrNoRefreshToken,
		},
		{
			name:    "org refresh rejected",
			creds:   sessionTestCreds("tok", "ended-refresh", time.Now().Add(time.Hour)),
			account: "acme-corp",
			wantRaw: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Unsetenv(auth.EnvAccessToken)
			t.Setenv("HOME", t.TempDir())
			writeAccountTestCredentials(t, tt.creds)
			serveWorkOSInvalidGrant(t)

			_, err := accountToken(context.Background(), tt.account, tt.force)
			require.Error(t, err)
			want := tt.want
			if tt.wantRaw {
				want = errAuthFailed(errors.Unwrap(err))
			}
			assert.Equal(t, want.Error(), err.Error(), "the message must come from messages.go")
			assert.Equal(t, 1, strings.Count(err.Error(), "re-authenticate"), "the login hint must appear exactly once")
			assert.NotContains(t, err.Error(), "..", "the message must not stutter periods")
			if tt.wantIs != nil {
				assert.ErrorIs(t, err, tt.wantIs, "the auth cause must stay reachable through wrapping")
			}
		})
	}
}

func TestRegisterAgentWithServer_SaysReauthenticateOnce(t *testing.T) {
	tests := []struct {
		name    string
		creds   *auth.Credentials
		wantErr func(body string) error
	}{
		{
			name:    "session ended before the request",
			creds:   sessionTestCreds("expired", "ended-refresh", time.Now().Add(-time.Hour)),
			wantErr: func(string) error { return errAuthSessionEnded(nil) },
		},
		{
			name:    "server rejects the token and the retry refresh fails",
			creds:   sessionTestCreds("tok", "ended-refresh", time.Now().Add(time.Hour)),
			wantErr: errRegisterUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = os.Unsetenv(auth.EnvAccessToken)
			t.Setenv("HOME", t.TempDir())
			writeAccountTestCredentials(t, tt.creds)
			serveWorkOSInvalidGrant(t)

			const body = `{"error":"unauthorized"}`
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(api.Close)

			err := registerAgentWithServer(context.Background(), api.URL, "my-agent", "build-1", "registry.example.com/alice",
				"spec", "", nil, "", false, false, "alice")
			require.Error(t, err)
			assert.Equal(t, tt.wantErr(body).Error(), err.Error(), "the message must come from messages.go")
			assert.Equal(t, 1, strings.Count(err.Error(), "re-authenticate"), "the login hint must appear exactly once")
		})
	}
}
