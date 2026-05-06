// Package slack implements the slack OAuth client used to link a user's
// slack identity to their WorkOS user. Identity comes from the
// authed_user.id field of the oauth.v2.access response — there is no
// follow-up auth.test call.
//
// We do this raw (instead of via WorkOS Pipes) because Pipes' Slack
// template is bot-token-shaped: GetAccessToken returns xoxb-* which
// resolves to the *bot user* on auth.test, not the human installer. To
// get the human's slack_user_id we need the user token issued by slack's
// oauth.v2.access in the `authed_user.access_token` slot — and the
// `authed_user.id` it ships alongside is exactly the identity we map.
package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	authorizeURL = "https://slack.com/oauth/v2/authorize"
	accessURL    = "https://slack.com/api/oauth.v2.access"
)

// OAuthResponse is the subset of slack's oauth.v2.access response we
// persist. The full response carries bot-token fields too (access_token,
// scope), but we don't need them — slack identity is the only thing we
// take from this flow.
type OAuthResponse struct {
	OK         bool
	Error      string
	Team       OAuthTeam
	AuthedUser OAuthAuthedUser
}

// OAuthTeam corresponds to the `team` block in oauth.v2.access. Domain
// (the *.slack.com subdomain) is not always returned; we surface it when
// present so the settings UI can render "Connected as @alice in
// {team_domain}".
type OAuthTeam struct {
	ID     string
	Name   string
	Domain string
}

// OAuthAuthedUser corresponds to the `authed_user` block. ID is the
// linker's slack user_id — the identity our mapping is keyed on.
type OAuthAuthedUser struct {
	ID    string
	Scope string
}

// OAuthClient holds a slack app's client credentials and an HTTP client
// scoped for OAuth requests.
type OAuthClient struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

// NewOAuthClient returns a Client authenticated with the given slack app
// credentials. clientSecret is required only for ExchangeCode; the auth
// URL builder uses clientID alone.
func NewOAuthClient(clientID, clientSecret string) *OAuthClient {
	return &OAuthClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// BuildAuthorizeURL returns the slack.com/oauth/v2/authorize URL the
// browser navigates to.
//
// userScopes are slack USER scopes (passed via `user_scope=`), distinct
// from bot scopes (`scope=`). For identity link we only need
// `users:read` — minimum to identify the human. We pass `scope=` empty
// so no bot install happens.
//
// state is a CSRF nonce the caller writes to a cookie and verifies on
// the callback. redirectURI must exactly match one of the URLs listed
// under the slack app's OAuth & Permissions → Redirect URLs.
func (c *OAuthClient) BuildAuthorizeURL(redirectURI, state string, userScopes []string) string {
	q := url.Values{}
	q.Set("client_id", c.clientID)
	q.Set("user_scope", strings.Join(userScopes, ","))
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	return authorizeURL + "?" + q.Encode()
}

// ExchangeCode swaps an OAuth code for the response carrying the user's
// identity. redirectURI must match the value sent during BuildAuthorizeURL
// (slack rejects a mismatch).
//
// Slack returns 200 even for ok=false errors (invalid_code, bad_redirect,
// etc.); the body's `ok` field is the authoritative signal. A non-2xx is
// a real network/proxy issue.
func (c *OAuthClient) ExchangeCode(ctx context.Context, code, redirectURI string) (OAuthResponse, error) {
	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, accessURL, strings.NewReader(form.Encode()))
	if err != nil {
		return OAuthResponse{}, fmt.Errorf("slack oauth: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return OAuthResponse{}, fmt.Errorf("slack oauth: exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return OAuthResponse{}, fmt.Errorf("slack oauth: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return OAuthResponse{}, fmt.Errorf("slack oauth: returned %d: %s", resp.StatusCode, body)
	}

	var raw struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
		Team  struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Domain string `json:"domain,omitempty"`
		} `json:"team"`
		AuthedUser struct {
			ID    string `json:"id"`
			Scope string `json:"scope"`
		} `json:"authed_user"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return OAuthResponse{}, fmt.Errorf("slack oauth: decode response: %w", err)
	}
	if !raw.OK {
		return OAuthResponse{}, fmt.Errorf("slack oauth: exchange failed: %s", raw.Error)
	}
	if raw.AuthedUser.ID == "" || raw.Team.ID == "" {
		return OAuthResponse{}, fmt.Errorf("slack oauth: response missing team or authed_user identity")
	}
	return OAuthResponse{
		OK:    true,
		Error: raw.Error,
		Team: OAuthTeam{
			ID:     raw.Team.ID,
			Name:   raw.Team.Name,
			Domain: raw.Team.Domain,
		},
		AuthedUser: OAuthAuthedUser{
			ID:    raw.AuthedUser.ID,
			Scope: raw.AuthedUser.Scope,
		},
	}, nil
}
