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
	// Shortened rather than slept out, and the handler blocks on the client
	// giving up instead of sleeping past the real timeout.
	prev := resolveGitHubTokenTimeout
	resolveGitHubTokenTimeout = 50 * time.Millisecond
	t.Cleanup(func() { resolveGitHubTokenTimeout = prev })

	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done(): // the client gave up, as expected
		case <-released: // the test is over
		}
	}))
	t.Cleanup(func() { close(released); srv.Close() })

	prevURL := sandboxServerURLOverride
	sandboxServerURLOverride = srv.URL
	t.Cleanup(func() { sandboxServerURLOverride = prevURL })
	t.Setenv(sandboxTokenEnvVar, "the-dev-session-bearer")

	token := resolveGitHubToken(context.Background(), false)

	assert.Empty(t, token, "an unresponsive connections server must fall back to the operator's own credentials")
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
