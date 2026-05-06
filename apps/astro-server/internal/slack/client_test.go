package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// redirectTransport rewrites request host/scheme to a test server so we
// don't hit slack.com/api directly in unit tests.
type redirectTransport struct {
	target *url.URL
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	r2.URL.Host = rt.target.Host
	r2.URL.Scheme = rt.target.Scheme
	return http.DefaultTransport.RoundTrip(r2)
}

func newTestClient(t *testing.T, srv *httptest.Server) *OAuthClient {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	c := NewOAuthClient("client-id-1", "client-secret-2")
	c.httpClient = &http.Client{Transport: &redirectTransport{target: u}}
	return c
}

// BuildAuthorizeURL serializes the authorize URL with the right params.
// We pass user_scope (not scope) — bot scopes would issue a bot token and
// auth.test would resolve to the bot, not the human. State and redirect
// URI must round-trip to the callback exactly.
func TestOAuthClient_BuildAuthorizeURL(t *testing.T) {
	c := NewOAuthClient("client-id-1", "")
	got := c.BuildAuthorizeURL("https://astropods.com/api/v1/accounts/x/slack/callback", "csrf-state", []string{"users:read"})

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Host != "slack.com" || u.Path != "/oauth/v2/authorize" {
		t.Errorf("unexpected URL: %s", got)
	}
	q := u.Query()
	if q.Get("client_id") != "client-id-1" {
		t.Errorf("client_id: %q", q.Get("client_id"))
	}
	if q.Get("user_scope") != "users:read" {
		t.Errorf("user_scope: %q", q.Get("user_scope"))
	}
	if q.Get("scope") != "" {
		t.Errorf("scope must be empty (we don't want a bot install): %q", q.Get("scope"))
	}
	if q.Get("state") != "csrf-state" {
		t.Errorf("state: %q", q.Get("state"))
	}
	if q.Get("redirect_uri") != "https://astropods.com/api/v1/accounts/x/slack/callback" {
		t.Errorf("redirect_uri: %q", q.Get("redirect_uri"))
	}
}

// ExchangeCode happy path: ok:true, identity in team + authed_user.
func TestOAuthClient_ExchangeCode_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth.v2.access" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method: %s", r.Method)
		}
		_ = r.ParseForm()
		if r.Form.Get("client_id") != "client-id-1" || r.Form.Get("client_secret") != "client-secret-2" {
			t.Errorf("missing creds: %+v", r.Form)
		}
		if r.Form.Get("code") != "ok-code" {
			t.Errorf("code: %s", r.Form.Get("code"))
		}
		if r.Form.Get("redirect_uri") != "https://astropods.com/cb" {
			t.Errorf("redirect_uri: %s", r.Form.Get("redirect_uri"))
		}
		_, _ = w.Write([]byte(`{
			"ok": true,
			"team": { "id": "T123", "name": "Acme", "domain": "acme" },
			"authed_user": { "id": "U456", "scope": "users:read" }
		}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	r, err := c.ExchangeCode(context.Background(), "ok-code", "https://astropods.com/cb")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if r.Team.ID != "T123" || r.AuthedUser.ID != "U456" {
		t.Errorf("identity: %+v", r)
	}
	if r.Team.Name != "Acme" || r.Team.Domain != "acme" {
		t.Errorf("team display: %+v", r.Team)
	}
}

// Slack returns 200 with ok:false on auth failures (bad_redirect_uri,
// invalid_code, …); the error must surface so the caller can decide how
// to react instead of silently failing closed.
func TestOAuthClient_ExchangeCode_OkFalseSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok": false, "error": "bad_redirect_uri"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_, err := c.ExchangeCode(context.Background(), "x", "https://astropods.com/cb")
	if err == nil || !strings.Contains(err.Error(), "bad_redirect_uri") {
		t.Errorf("expected bad_redirect_uri in error; got %v", err)
	}
}

// Missing identity is malformed — guard against persisting half-baked
// rows in slack_identity_mappings.
func TestOAuthClient_ExchangeCode_MissingIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok": true, "team": {"id":"T1"}}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_, err := c.ExchangeCode(context.Background(), "x", "https://astropods.com/cb")
	if err == nil {
		t.Fatal("expected error for missing authed_user.id")
	}
}

// 5xx propagates as a transport error.
func TestOAuthClient_ExchangeCode_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_, err := c.ExchangeCode(context.Background(), "x", "https://astropods.com/cb")
	if err == nil {
		t.Fatal("expected error")
	}
}
