//go:build integration

package e2e

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real CLI processes against a fake WorkOS server whose refresh tokens are single-use, as WorkOS's are.
//
//	go test -tags integration -run TestAuth ./e2e/...

var e2eAccounts = []map[string]string{
	{"id": "acct_p", "name": "tester", "type": "personal", "workos_org_id": "org_p"},
	{"id": "acct_a", "name": "team-a", "type": "organization", "workos_org_id": "org_a"},
	{"id": "acct_b", "name": "team-b", "type": "organization", "workos_org_id": "org_b"},
}

type fakeAuthServer struct {
	*httptest.Server

	mu          sync.Mutex
	refresh     string
	issued      int
	exchanges   map[string]int // by organization ID, "" for the personal token
	rejected    int
	tokenOrg    map[string]string
	loginTTL    time.Duration
	exchangeTTL time.Duration
}

func newFakeAuthServer(t *testing.T) *fakeAuthServer {
	t.Helper()
	f := &fakeAuthServer{
		exchanges:   map[string]int{},
		tokenOrg:    map[string]string{},
		loginTTL:    15 * time.Minute,
		exchangeTTL: 15 * time.Minute,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /user_management/authorize/device", f.deviceAuthorization)
	mux.HandleFunc("POST /user_management/authenticate", f.authenticate)
	mux.HandleFunc("GET /api/v1/me", f.me)
	mux.HandleFunc("GET /api/v1/agents/{account}", f.listAgents)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeAuthServer) deviceAuthorization(w http.ResponseWriter, _ *http.Request) {
	writeTestJSON(w, http.StatusOK, map[string]any{
		"device_code": "device_1", "user_code": "TEST-CODE",
		"verification_uri": f.URL + "/device", "expires_in": 60, "interval": 1,
	})
}

func (f *fakeAuthServer) authenticate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeTestJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.Form.Get("grant_type") {
	case "urn:ietf:params:oauth:grant-type:device_code":
		writeTestJSON(w, http.StatusOK, f.issueLocked("", f.loginTTL, true))
	case "refresh_token":
		// Real latency, so concurrent processes overlap and an unserialized exchange always collides.
		f.mu.Unlock()
		time.Sleep(200 * time.Millisecond)
		f.mu.Lock()
		if r.Form.Get("refresh_token") != f.refresh {
			f.rejected++
			writeTestJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "refresh token already used"})
			return
		}
		org := r.Form.Get("organization_id")
		f.exchanges[org]++
		writeTestJSON(w, http.StatusOK, f.issueLocked(org, f.exchangeTTL, false))
	default:
		writeTestJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
	}
}

func (f *fakeAuthServer) issueLocked(org string, ttl time.Duration, withUser bool) map[string]any {
	f.issued++
	payload, _ := json.Marshal(map[string]any{"exp": time.Now().Add(ttl).Unix(), "org_id": org, "n": f.issued})
	access := "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
	f.tokenOrg[access] = org
	f.refresh = fmt.Sprintf("refresh_%d", f.issued)
	resp := map[string]any{"access_token": access, "refresh_token": f.refresh, "expires_in": int(ttl.Seconds()), "token_type": "Bearer"}
	if withUser {
		resp["user"] = map[string]string{"id": "user_1", "email": "tester@example.com"}
	}
	return resp
}

func (f *fakeAuthServer) tokenOrgFor(r *http.Request) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	org, ok := f.tokenOrg[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	return org, ok
}

func (f *fakeAuthServer) me(w http.ResponseWriter, r *http.Request) {
	if _, ok := f.tokenOrgFor(r); !ok {
		writeTestJSON(w, http.StatusUnauthorized, map[string]string{"error": "unknown token"})
		return
	}
	writeTestJSON(w, http.StatusOK, map[string]any{"accounts": e2eAccounts})
}

func (f *fakeAuthServer) listAgents(w http.ResponseWriter, r *http.Request) {
	org, ok := f.tokenOrgFor(r)
	if !ok {
		writeTestJSON(w, http.StatusUnauthorized, map[string]string{"error": "unknown token"})
		return
	}
	for _, a := range e2eAccounts {
		if a["name"] == r.PathValue("account") && a["workos_org_id"] != org {
			writeTestJSON(w, http.StatusForbidden, map[string]string{"error": "token is scoped to " + org})
			return
		}
	}
	writeTestJSON(w, http.StatusOK, map[string]any{"agents": []any{}, "count": 0})
}

func (f *fakeAuthServer) stats() (map[string]int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	counts := make(map[string]int, len(f.exchanges))
	for k, v := range f.exchanges {
		counts[k] = v
	}
	return counts, f.rejected
}

func writeTestJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type authHarness struct {
	t    *testing.T
	bin  string
	home string
	auth *fakeAuthServer
}

func newAuthHarness(t *testing.T) *authHarness {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows reads the profile from USERPROFILE, so a temp HOME would not isolate the real credentials")
	}
	f := newFakeAuthServer(t)
	bin := filepath.Join(t.TempDir(), "ast-e2e")
	build := exec.Command("go", "build", "-tags", "integration",
		"-ldflags", "-X github.com/astropods/astro-cli/internal/buildinfo.DefaultServerURL="+f.URL,
		"-o", bin, "./authharness")
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))
	return &authHarness{t: t, bin: bin, home: t.TempDir(), auth: f}
}

type cliResult struct {
	code   int
	output string
}

// A HOME inside TMPDIR keeps the CLI off the real keychain, so TMPDIR must reach the child unchanged.
func (h *authHarness) run(args ...string) cliResult {
	c := exec.Command(h.bin, args...)
	c.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + h.home,
		"TMPDIR=" + os.TempDir(),
		"NO_COLOR=1",
		"ASTRO_E2E_WORKOS_URL=" + h.auth.URL,
	}
	out, err := c.CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		code = -1
	}
	return cliResult{code: code, output: string(out)}
}

func (h *authHarness) runConcurrently(n int, args ...string) []string {
	results := make([]cliResult, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = h.run(args...)
		}()
	}
	wg.Wait()
	var failures []string
	for i, r := range results {
		if r.code != 0 {
			failures = append(failures, fmt.Sprintf("process %d exited %d: %s", i, r.code, strings.TrimSpace(r.output)))
		}
	}
	return failures
}

func (h *authHarness) credentials() map[string]any {
	h.t.Helper()
	data, err := os.ReadFile(filepath.Join(h.home, ".ast-dev", "credentials.json"))
	require.NoError(h.t, err)
	var creds map[string]any
	require.NoError(h.t, json.Unmarshal(data, &creds))
	return creds
}

func (h *authHarness) profile(creds map[string]any) map[string]any {
	h.t.Helper()
	profiles, ok := creds["profiles"].(map[string]any)
	require.True(h.t, ok, "credentials have no profiles")
	profile, ok := profiles["default"].(map[string]any)
	require.True(h.t, ok, "credentials have no default profile")
	return profile
}

func (h *authHarness) setOrgTokenExpiry(orgID string, expiresAt time.Time) {
	h.t.Helper()
	path := filepath.Join(h.home, ".ast-dev", "credentials.json")
	creds := h.credentials()
	tokens, ok := h.profile(creds)["org_tokens"].(map[string]any)
	require.True(h.t, ok, "no cached org tokens")
	token, ok := tokens[orgID].(map[string]any)
	require.True(h.t, ok, "no cached token for %s", orgID)
	token["expires_at"] = expiresAt.Format(time.RFC3339)
	data, err := json.MarshalIndent(creds, "", "  ")
	require.NoError(h.t, err)
	require.NoError(h.t, os.WriteFile(path, data, 0o600))
}

func (h *authHarness) login() {
	h.t.Helper()
	r := h.run("login", "--no-browser")
	require.Equal(h.t, 0, r.code, r.output)
	require.NotEmpty(h.t, h.profile(h.credentials())["refresh_token"], "tokens must be in the file: a keychain write here would reach the real keychain")
}

func TestAuth_ConcurrentCommandsShareOneExchange(t *testing.T) {
	h := newAuthHarness(t)
	h.login()
	r := h.run("account", "switch", "team-a")
	require.Equal(t, 0, r.code, r.output)

	assert.Empty(t, h.runConcurrently(12, "blueprint", "list"))
	exchanges, rejected := h.auth.stats()
	assert.Equal(t, 1, exchanges["org_a"], "every process after the first must reuse the cached token")
	assert.Zero(t, rejected, "no process may send a refresh token another one already used")

	assert.Empty(t, h.runConcurrently(12, "blueprint", "list"))
	exchanges, _ = h.auth.stats()
	assert.Equal(t, 1, exchanges["org_a"], "a fresh cached token needs no exchange")
}

func TestAuth_ConcurrentRenewalOfAnExpiringToken(t *testing.T) {
	h := newAuthHarness(t)
	h.login()
	require.Equal(t, 0, h.run("account", "switch", "team-a").code)
	require.Equal(t, 0, h.run("blueprint", "list").code)

	h.setOrgTokenExpiry("org_a", time.Now().Add(time.Minute))
	assert.Empty(t, h.runConcurrently(12, "blueprint", "list"))
	exchanges, rejected := h.auth.stats()
	assert.Equal(t, 2, exchanges["org_a"], "the expiring token is renewed once")
	assert.Zero(t, rejected)
}

func TestAuth_ConcurrentPersonalTokenRefresh(t *testing.T) {
	h := newAuthHarness(t)
	h.auth.loginTTL = time.Minute // inside the refresh threshold, so every command refreshes it
	h.login()

	assert.Empty(t, h.runConcurrently(12, "whoami"))
	exchanges, rejected := h.auth.stats()
	assert.Equal(t, 1, exchanges[""], "the personal token is refreshed once")
	assert.Zero(t, rejected)
}

func TestAuth_EachAccountGetsItsOwnOrgToken(t *testing.T) {
	h := newAuthHarness(t)
	h.login()
	for _, account := range []string{"team-a", "team-b", "tester", "team-a"} {
		require.Equal(t, 0, h.run("account", "switch", account).code)
		assert.Empty(t, h.runConcurrently(4, "blueprint", "list"), account)
	}
	exchanges, rejected := h.auth.stats()
	assert.Equal(t, map[string]int{"org_a": 1, "org_b": 1, "org_p": 1}, exchanges)
	assert.Zero(t, rejected)
}

func TestAuth_LogoutClearsTheSession(t *testing.T) {
	h := newAuthHarness(t)
	h.login()
	require.Equal(t, 0, h.run("account", "switch", "team-a").code)
	require.Equal(t, 0, h.run("blueprint", "list").code)

	r := h.run("logout")
	require.Equal(t, 0, r.code, r.output)
	profiles, _ := h.credentials()["profiles"].(map[string]any)
	assert.NotContains(t, profiles, "default")

	r = h.run("blueprint", "list")
	assert.NotEqual(t, 0, r.code)
	assert.Contains(t, r.output, "not logged in")
}
