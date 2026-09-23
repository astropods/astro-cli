package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	spec "github.com/astropods/astro-spec"
)

const (
	fakeSessionToken  = "eyJhbGciOiJIUzI1NiJ9.fake.token"
	fakeSessionExpiry = "2026-09-23T06:00:00Z"
)

// sandboxServer stands in for the control plane and records what the CLI sent.
type sandboxServer struct {
	status int
	body   string
	method string
	path   string
	// escaped is the path as it arrived on the wire. r.URL.Path is already
	// unescaped, so only this shows whether the CLI escaped the agent name.
	escaped string
	request openDevSessionRequest
	bearer  string
	calls   int
}

func newSandboxServer(t *testing.T, s *sandboxServer) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls++
		s.method = r.Method
		s.path = r.URL.Path
		s.escaped = r.URL.EscapedPath()
		s.bearer = r.Header.Get("Authorization")
		if body, err := io.ReadAll(r.Body); err == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &s.request)
		}

		status := s.status
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		body := s.body
		if body == "" {
			body = `{"session_id":"sess-1","token":"` + fakeSessionToken +
				`","expires_at":"` + fakeSessionExpiry + `"}`
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	prev := sandboxServerURLOverride
	sandboxServerURLOverride = srv.URL
	t.Cleanup(func() { sandboxServerURLOverride = prev })
	return srv
}

func loggedIn(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("alice"))
}

func TestTheSandboxTokenIsInjectedOnlyWhenAskedFor(t *testing.T) {
	for _, tc := range []struct {
		name      string
		wanted    bool
		wantToken bool
		wantCalls int
	}{
		{name: "asked for", wanted: true, wantToken: true, wantCalls: 1},
		{name: "not asked for", wanted: false, wantToken: false, wantCalls: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loggedIn(t)
			srv := &sandboxServer{}
			newSandboxServer(t, srv)

			envVars := map[string]string{}
			var out strings.Builder
			require.NoError(t, injectSandboxDevToken(
				context.Background(), &out, "my-agent", envVars, tc.wanted, false))

			assert.Equal(t, tc.wantCalls, srv.calls,
				"an author who did not ask for a sandbox must not be charged an API call, or a login prompt")

			if tc.wantToken {
				assert.Equal(t, fakeSessionToken, envVars[sandboxTokenEnvVar],
					"the agent SDK reads this variable and takes the control plane's URL from the token itself")
				assert.Contains(t, out.String(), msgSandboxSessionOpened(fakeSessionExpiry))
			} else {
				assert.NotContains(t, envVars, sandboxTokenEnvVar)
				assert.Empty(t, out.String())
			}
		})
	}
}

func TestOpeningASessionAddressesTheAccountAndNamesTheAgent(t *testing.T) {
	loggedIn(t)
	srv := &sandboxServer{}
	newSandboxServer(t, srv)

	require.NoError(t, injectSandboxDevToken(
		context.Background(), io.Discard, "my-agent", map[string]string{}, true, false))

	assert.Equal(t, http.MethodPost, srv.method)
	assert.Equal(t, "/api/v1/accounts/alice/dev-sessions", srv.path,
		"the route is account-scoped, because a developer opens the session, not a deployment")
	assert.Equal(t, "my-agent", srv.request.AgentName,
		"one session per developer per agent, so the agent has to be named")
	assert.True(t, strings.HasPrefix(srv.bearer, "Bearer "),
		"the call is authenticated as the developer")
}

func TestOpeningASessionWithoutLoginSaysToLogIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := &sandboxServer{}
	newSandboxServer(t, srv)

	err := injectSandboxDevToken(
		context.Background(), io.Discard, "my-agent", map[string]string{}, true, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires login",
		"a sandbox is the developer's, so an author who is not logged in needs telling")
	assert.Zero(t, srv.calls)
}

func TestARefusedSessionExplainsWhichSwitchIsOff(t *testing.T) {
	loggedIn(t)
	srv := &sandboxServer{status: http.StatusConflict, body: `{"error":"sandboxes are not enabled for this account"}`}
	newSandboxServer(t, srv)

	err := injectSandboxDevToken(
		context.Background(), io.Discard, "my-agent", map[string]string{}, true, false)
	require.Error(t, err)
	assert.Equal(t, errSandboxNotEnabled("alice").Error(), err.Error(),
		"a 409 means the account switch or the cluster's image, and the author can act on neither without being told")
}

func TestASessionWithNoTokenIsAFailureNotAnEmptyVariable(t *testing.T) {
	loggedIn(t)
	srv := &sandboxServer{body: `{"session_id":"sess-1","token":"","expires_at":"` + fakeSessionExpiry + `"}`}
	newSandboxServer(t, srv)

	envVars := map[string]string{}
	err := injectSandboxDevToken(
		context.Background(), io.Discard, "my-agent", envVars, true, false)

	require.Error(t, err, "an empty token would start the agent with authorization off, which is what this replaces")
	assert.NotContains(t, envVars, sandboxTokenEnvVar)
}

func TestAnAgentWithNoNameCannotOpenASession(t *testing.T) {
	loggedIn(t)
	srv := &sandboxServer{}
	newSandboxServer(t, srv)

	err := injectSandboxDevToken(
		context.Background(), io.Discard, "", map[string]string{}, true, false)
	assert.Equal(t, errSandboxNeedsAgentName().Error(), err.Error())
	assert.Zero(t, srv.calls)
}

func TestClosingASessionDeletesItForThatAgent(t *testing.T) {
	loggedIn(t)
	srv := &sandboxServer{status: http.StatusNoContent, body: " "}
	newSandboxServer(t, srv)

	var out strings.Builder
	closeSandboxDevSession(context.Background(), &out, "my agent", false)

	assert.Equal(t, http.MethodDelete, srv.method)
	assert.Equal(t, "/api/v1/accounts/alice/dev-sessions/my%20agent", srv.escaped,
		"an agent name can hold characters that need escaping in a path")
	assert.Equal(t, "/api/v1/accounts/alice/dev-sessions/my agent", srv.path,
		"and the server sees the name the author gave")
	assert.Empty(t, out.String(), "a clean close says nothing")
}

func TestAFailedCloseWarnsRatherThanStoppingTeardown(t *testing.T) {
	loggedIn(t)
	srv := &sandboxServer{status: http.StatusInternalServerError, body: `{"error":"boom"}`}
	newSandboxServer(t, srv)

	var out strings.Builder
	closeSandboxDevSession(context.Background(), &out, "my-agent", false)

	assert.Contains(t, out.String(), "Could not close the sandbox session",
		"teardown has containers to stop, and the session expires and is swept regardless")
}

func TestClosingWithoutLoginIsSilent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := &sandboxServer{}
	newSandboxServer(t, srv)

	var out strings.Builder
	closeSandboxDevSession(context.Background(), &out, "my-agent", false)

	assert.Zero(t, srv.calls)
	assert.Empty(t, out.String(),
		"project stop closes unconditionally, so an author who never used a sandbox sees nothing")
}

func TestSpecAgentNameToleratesNoSpec(t *testing.T) {
	assert.Empty(t, specAgentName(nil))
	assert.Equal(t, "my-agent", specAgentName(&spec.AstroSpec{Name: "my-agent"}))
}
