package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGitHubTokenWithNoDevSessionReturnsEmpty(t *testing.T) {
	t.Setenv(sandboxTokenEnvVar, "")
	token := resolveGitHubToken(context.Background(), false)
	assert.Empty(t, token, "with no dev session open, the caller should fall back to the operator's own gh login")
}

func TestResolveGitHubTokenFetchesFromTheConnectionsEndpoint(t *testing.T) {
	var gotAuth string
	var gotBody issueConnectionTokenRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		assert.Equal(t, "/api/v1/deployments/connections/token", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(issueConnectionTokenResponse{AccessToken: "gho_fromdevsession"})) //nolint:errcheck,gosec
	}))
	defer srv.Close()

	prevURL := sandboxServerURLOverride
	sandboxServerURLOverride = srv.URL
	t.Cleanup(func() { sandboxServerURLOverride = prevURL })
	t.Setenv(sandboxTokenEnvVar, "the-dev-session-bearer")

	token := resolveGitHubToken(context.Background(), false)

	assert.Equal(t, "gho_fromdevsession", token)
	assert.Equal(t, "Bearer the-dev-session-bearer", gotAuth)
	assert.Equal(t, "github", gotBody.Provider)
}

func TestResolveGitHubTokenGivesUpOnAnUnresponsiveServer(t *testing.T) {
	// Regression test: a stray ASTRO_AUTHZ_TOKEN left over from another
	// session, pointed at an unreachable server, used to let this call hang
	// for however long the OS's TCP timeout took before falling back.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(resolveGitHubTokenTimeout + 5*time.Second)
	}))
	defer srv.Close()

	prevURL := sandboxServerURLOverride
	sandboxServerURLOverride = srv.URL
	t.Cleanup(func() { sandboxServerURLOverride = prevURL })
	t.Setenv(sandboxTokenEnvVar, "the-dev-session-bearer")

	start := time.Now()
	token := resolveGitHubToken(context.Background(), false)
	elapsed := time.Since(start)

	assert.Empty(t, token)
	assert.Less(t, elapsed, resolveGitHubTokenTimeout+2*time.Second,
		"took %s; must give up at roughly resolveGitHubTokenTimeout, not hang on the slow server", elapsed)
}

func TestResolveGitHubTokenFallsBackSilentlyOnServerFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	prevURL := sandboxServerURLOverride
	sandboxServerURLOverride = srv.URL
	t.Cleanup(func() { sandboxServerURLOverride = prevURL })
	t.Setenv(sandboxTokenEnvVar, "the-dev-session-bearer")

	token := resolveGitHubToken(context.Background(), false)

	assert.Empty(t, token, "a refused or failed token fetch should not be fatal; the caller falls back to local gh")
}
