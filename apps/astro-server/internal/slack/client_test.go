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
// don't have to mock https://slack.com/api directly.
type redirectTransport struct {
	target *url.URL
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r2 := req.Clone(req.Context())
	r2.URL.Host = rt.target.Host
	r2.URL.Scheme = rt.target.Scheme
	return http.DefaultTransport.RoundTrip(r2)
}

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	c := New("test-token")
	c.httpClient = &http.Client{Transport: &redirectTransport{target: u}}
	return c
}

// AuthTest happy path: ok:true, fields populated → Identity returned.
func TestClient_AuthTest_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth.test" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{
			"ok": true,
			"team_id": "T123",
			"user_id": "U456",
			"team": "Acme",
			"user": "alice",
			"team_domain": "acme"
		}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	id, err := c.AuthTest(context.Background())
	if err != nil {
		t.Fatalf("auth.test: %v", err)
	}
	if id.TeamID != "T123" || id.UserID != "U456" {
		t.Errorf("identity keys: %+v", id)
	}
	if id.Team != "Acme" || id.TeamDomain != "acme" || id.User != "alice" {
		t.Errorf("identity display: %+v", id)
	}
}

// Slack returns 200 with ok:false on auth failures (revoked token,
// scope_insufficient, …). The error message must surface so callers can
// decide between user-actionable ("re-link") vs. operator-actionable.
func TestClient_AuthTest_OkFalseSurfacesSlackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok": false, "error": "invalid_auth"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_, err := c.AuthTest(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid_auth") {
		t.Errorf("error should surface slack code; got %v", err)
	}
}

// ok:true but missing keys is a malformed response — treat as error so we
// don't write half-baked rows to slack_identity_mappings.
func TestClient_AuthTest_MissingKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok": true, "team": "Acme"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_, err := c.AuthTest(context.Background())
	if err == nil {
		t.Fatal("expected error for missing team_id/user_id")
	}
}

// 5xx propagates as a transport error.
func TestClient_AuthTest_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_, err := c.AuthTest(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}
