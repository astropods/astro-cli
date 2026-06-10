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
			"authed_user": {
				"id": "U456",
				"scope": "users:read,team:read",
				"access_token": "xoxp-test-user-token"
			}
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
	// access_token must round-trip — TeamInfo / users.info need it.
	if r.AuthedUser.AccessToken != "xoxp-test-user-token" {
		t.Errorf("authed_user.access_token: %q", r.AuthedUser.AccessToken)
	}
}

// TeamInfo happy path: pulls icon URL from image_88, surfaces team
// display fields. Authorization header carries the user token verbatim.
func TestOAuthClient_TeamInfo_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/team.info" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer xoxp-test" {
			t.Errorf("auth header: %q", got)
		}
		_, _ = w.Write([]byte(`{
			"ok": true,
			"team": {
				"id": "T1",
				"name": "Acme",
				"domain": "acme",
				"icon": {
					"image_44": "https://example.com/44.png",
					"image_68": "https://example.com/68.png",
					"image_88": "https://example.com/88.png"
				}
			}
		}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	info, err := c.TeamInfo(context.Background(), "xoxp-test")
	if err != nil {
		t.Fatalf("team.info: %v", err)
	}
	if info.ID != "T1" || info.Name != "Acme" || info.Domain != "acme" {
		t.Errorf("identity: %+v", info)
	}
	if info.IconURL != "https://example.com/88.png" {
		t.Errorf("expected image_88 preferred; got %q", info.IconURL)
	}
}

// image_default=true means the workspace uses slack's generic icon —
// surface IconURL="" so the frontend renders our own svg instead of a
// placeholder slack-hash icon.
func TestOAuthClient_TeamInfo_DefaultIconReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"ok": true,
			"team": {
				"id": "T1", "name": "Acme", "domain": "acme",
				"icon": { "image_default": true, "image_88": "https://example.com/generic.png" }
			}
		}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	info, err := c.TeamInfo(context.Background(), "xoxp-test")
	if err != nil {
		t.Fatalf("team.info: %v", err)
	}
	if info.IconURL != "" {
		t.Errorf("expected empty icon for image_default=true; got %q", info.IconURL)
	}
}

// UserInfo happy path: returns username + display name via users.info.
// Authorization header carries the user token verbatim.
func TestOAuthClient_UserInfo_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users.info" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("user"); got != "U456" {
			t.Errorf("user query: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer xoxp-test" {
			t.Errorf("auth header: %q", got)
		}
		_, _ = w.Write([]byte(`{
			"ok": true,
			"user": {
				"id": "U456",
				"name": "alice",
				"real_name": "Alice Real",
				"is_bot": false,
				"deleted": true,
				"profile": {
					"display_name": "Alice Cooper",
					"real_name": "Alice Profile",
					"image_48": "https://example.com/alice-48.png",
					"image_72": "https://example.com/alice-72.png"
				}
			}
		}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	info, err := c.UserInfo(context.Background(), "xoxp-test", "U456")
	if err != nil {
		t.Fatalf("users.info: %v", err)
	}
	if info.ID != "U456" || info.Name != "alice" || info.DisplayName != "Alice Cooper" {
		t.Errorf("identity: %+v", info)
	}
	if info.RealName != "Alice Profile" || info.AvatarURL != "https://example.com/alice-72.png" || !info.Deleted {
		t.Errorf("profile metadata: %+v", info)
	}
}

// UsersList paginates through Slack's workspace directory and extracts the
// profile fields Insights needs for unlinked Slack users.
func TestOAuthClient_UsersList_PaginatesAndParsesProfiles(t *testing.T) {
	var cursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users.list" {
			t.Errorf("path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer xoxp-test" {
			t.Errorf("auth header: %q", got)
		}
		cursors = append(cursors, r.URL.Query().Get("cursor"))
		switch r.URL.Query().Get("cursor") {
		case "":
			_, _ = w.Write([]byte(`{
				"ok": true,
				"members": [
					{
						"id": "U111",
						"name": "jesse",
						"real_name": "Jesse Root",
						"profile": {
							"display_name": "Jesse Morgan",
							"real_name": "Jesse Profile",
							"image_48": "https://example.com/jesse-48.png",
							"image_72": "https://example.com/jesse-72.png"
						}
					}
				],
				"response_metadata": { "next_cursor": "page-2" }
			}`))
		case "page-2":
			_, _ = w.Write([]byte(`{
				"ok": true,
				"members": [
					{
						"id": "U222",
						"name": "botty",
						"is_bot": true,
						"deleted": true,
						"profile": {
							"real_name": "Botty Bot",
							"image_192": "https://example.com/botty-192.png"
						}
					}
				],
				"response_metadata": { "next_cursor": "" }
			}`))
		default:
			t.Fatalf("unexpected cursor: %q", r.URL.Query().Get("cursor"))
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	users, truncated, err := c.UsersList(context.Background(), "xoxp-test")
	if err != nil {
		t.Fatalf("users.list: %v", err)
	}
	if truncated {
		t.Fatalf("users.list unexpectedly truncated")
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if cursors[0] != "" || cursors[1] != "page-2" {
		t.Fatalf("unexpected cursor sequence: %+v", cursors)
	}
	if users[0].ID != "U111" || users[0].Name != "jesse" || users[0].DisplayName != "Jesse Morgan" {
		t.Errorf("first user identity: %+v", users[0])
	}
	if users[0].RealName != "Jesse Profile" || users[0].AvatarURL != "https://example.com/jesse-72.png" {
		t.Errorf("first user profile: %+v", users[0])
	}
	if users[1].ID != "U222" || users[1].DisplayName != "Botty Bot" || !users[1].IsBot || !users[1].Deleted {
		t.Errorf("second user metadata: %+v", users[1])
	}
	if users[1].AvatarURL != "https://example.com/botty-192.png" {
		t.Errorf("second user avatar fallback: %+v", users[1])
	}
}

func TestOAuthClient_UsersList_StopsAtPageCap(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/users.list" {
			t.Errorf("path: %s", r.URL.Path)
		}
		requests++
		_, _ = w.Write([]byte(`{
			"ok": true,
			"members": [{ "id": "U111", "name": "jesse" }],
			"response_metadata": { "next_cursor": "next-page" }
		}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	users, truncated, err := c.UsersList(context.Background(), "xoxp-test")
	if err != nil {
		t.Fatalf("users.list: %v", err)
	}
	if !truncated {
		t.Fatalf("expected users.list to report truncation")
	}
	if requests != maxUsersListPages {
		t.Fatalf("expected %d requests, got %d", maxUsersListPages, requests)
	}
	if len(users) != maxUsersListPages {
		t.Fatalf("expected one user per capped page, got %d", len(users))
	}
}

// users.info ok:false (e.g. user_not_found) surfaces as an error so the
// callback can log it and proceed without slack_username.
func TestOAuthClient_UserInfo_OkFalseSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok": false, "error": "user_not_found"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_, err := c.UserInfo(context.Background(), "xoxp-test", "U-missing")
	if err == nil || !strings.Contains(err.Error(), "user_not_found") {
		t.Errorf("expected user_not_found in error; got %v", err)
	}
}

// ok:false (e.g. missing_scope when team:read isn't granted) must surface
// so the callback handler can decide whether to log + continue without
// the icon vs. fail the link.
func TestOAuthClient_TeamInfo_OkFalseSurfacesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok": false, "error": "missing_scope"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_, err := c.TeamInfo(context.Background(), "xoxp-test")
	if err == nil || !strings.Contains(err.Error(), "missing_scope") {
		t.Errorf("expected missing_scope in error; got %v", err)
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
