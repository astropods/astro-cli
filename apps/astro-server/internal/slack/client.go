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
	authorizeURL      = "https://slack.com/oauth/v2/authorize"
	accessURL         = "https://slack.com/api/oauth.v2.access"
	maxUsersListPages = 50
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
// linker's slack user_id — the identity our mapping is keyed on. The
// access token is the user-token (xoxp-…) issued by user_scope and is
// what callers pass to TeamInfo / users.info / auth.test. Treat it as
// short-lived: we use it once during the link flow and discard.
type OAuthAuthedUser struct {
	ID          string
	Scope       string
	AccessToken string
}

// TeamInfo is a subset of the slack `team.info` response. Used to
// capture the workspace icon at link time so the settings UI can render
// it without an extra round-trip per render.
type TeamInfo struct {
	ID      string
	Name    string
	Domain  string
	IconURL string
}

// UserInfo is a subset of Slack's user objects from users.info/users.list.
// We capture enough profile metadata at link time to render connected
// settings rows and to seed the Insights Slack directory without per-message
// Slack API calls from deployed agents.
type UserInfo struct {
	ID          string
	Name        string // slack username (handle), e.g. "alice"
	DisplayName string // user-set display name, e.g. "Alice Cooper"
	RealName    string
	AvatarURL   string
	IsBot       bool
	Deleted     bool
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
			ID          string `json:"id"`
			Scope       string `json:"scope"`
			AccessToken string `json:"access_token"`
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
			ID:          raw.AuthedUser.ID,
			Scope:       raw.AuthedUser.Scope,
			AccessToken: raw.AuthedUser.AccessToken,
		},
	}, nil
}

// TeamInfo calls slack.com/api/team.info with the supplied user token
// and returns the workspace's display fields including its icon URL. The
// caller passes the OAuth response's authed_user.access_token; the token
// needs `team:read` scope.
//
// We pick image_88 for the icon — sharp at 2x for the small size we
// render (~16-20px), no point fetching the larger sizes the API also
// returns.
func (c *OAuthClient) TeamInfo(ctx context.Context, userToken string) (TeamInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/team.info", nil)
	if err != nil {
		return TeamInfo{}, fmt.Errorf("slack team.info: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+userToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TeamInfo{}, fmt.Errorf("slack team.info: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TeamInfo{}, fmt.Errorf("slack team.info: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return TeamInfo{}, fmt.Errorf("slack team.info: returned %d: %s", resp.StatusCode, body)
	}

	var raw struct {
		OK    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
		Team  struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Domain string `json:"domain"`
			Icon   struct {
				Image34      string `json:"image_34,omitempty"`
				Image44      string `json:"image_44,omitempty"`
				Image68      string `json:"image_68,omitempty"`
				Image88      string `json:"image_88,omitempty"`
				ImageDefault bool   `json:"image_default,omitempty"`
			} `json:"icon"`
		} `json:"team"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return TeamInfo{}, fmt.Errorf("slack team.info: decode: %w", err)
	}
	if !raw.OK {
		return TeamInfo{}, fmt.Errorf("slack team.info: %s", raw.Error)
	}

	// Prefer image_88 (sharp at 2x); fall back through smaller sizes.
	// image_default=true means the workspace has the generic Slack icon —
	// we leave IconURL empty in that case so the frontend uses our svg.
	icon := raw.Team.Icon.Image88
	if icon == "" {
		icon = raw.Team.Icon.Image68
	}
	if icon == "" {
		icon = raw.Team.Icon.Image44
	}
	if raw.Team.Icon.ImageDefault {
		icon = ""
	}

	return TeamInfo{
		ID:      raw.Team.ID,
		Name:    raw.Team.Name,
		Domain:  raw.Team.Domain,
		IconURL: icon,
	}, nil
}

// UserInfo calls slack.com/api/users.info?user=<id> with the supplied
// user token and returns the user's display fields. Requires `users:read`
// scope (which we already request).
//
// Used at link time to capture the slack username so the settings UI
// can render "@alice" instead of "@U0ALENLUWBG". Best-effort: callers
// log and continue when this fails — identity is already captured by
// the OAuth response.
func (c *OAuthClient) UserInfo(ctx context.Context, userToken, userID string) (UserInfo, error) {
	q := url.Values{}
	q.Set("user", userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/users.info?"+q.Encode(), nil)
	if err != nil {
		return UserInfo{}, fmt.Errorf("slack users.info: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+userToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return UserInfo{}, fmt.Errorf("slack users.info: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UserInfo{}, fmt.Errorf("slack users.info: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return UserInfo{}, fmt.Errorf("slack users.info: returned %d: %s", resp.StatusCode, body)
	}

	var raw struct {
		OK    bool         `json:"ok"`
		Error string       `json:"error,omitempty"`
		User  slackUserRaw `json:"user"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return UserInfo{}, fmt.Errorf("slack users.info: decode: %w", err)
	}
	if !raw.OK {
		return UserInfo{}, fmt.Errorf("slack users.info: %s", raw.Error)
	}
	return userInfoFromRaw(raw.User), nil
}

// UsersList calls slack.com/api/users.list with the supplied user token.
// Requires users:read, which the account-link OAuth flow already requests.
// The bool return is true when pagination hit the safety cap and the returned
// user slice is a partial workspace directory.
//
// This is used once at account-connect time to seed slack_observed_users with
// the workspace directory. That keeps Insights deterministic for unlinked
// Slack users without asking every deployed agent to perform per-message
// profile lookups.
func (c *OAuthClient) UsersList(ctx context.Context, userToken string) ([]UserInfo, bool, error) {
	var users []UserInfo
	cursor := ""
	for page := 0; page < maxUsersListPages; page++ {
		q := url.Values{}
		q.Set("limit", "200")
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/users.list?"+q.Encode(), nil)
		if err != nil {
			return nil, false, fmt.Errorf("slack users.list: build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+userToken)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, false, fmt.Errorf("slack users.list: request: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return nil, false, fmt.Errorf("slack users.list: read body: %w", readErr)
		}
		if closeErr != nil {
			return nil, false, fmt.Errorf("slack users.list: close body: %w", closeErr)
		}
		if resp.StatusCode >= 400 {
			return nil, false, fmt.Errorf("slack users.list: returned %d: %s", resp.StatusCode, body)
		}

		var raw struct {
			OK               bool           `json:"ok"`
			Error            string         `json:"error,omitempty"`
			Members          []slackUserRaw `json:"members"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor,omitempty"`
			} `json:"response_metadata"`
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, false, fmt.Errorf("slack users.list: decode: %w", err)
		}
		if !raw.OK {
			return nil, false, fmt.Errorf("slack users.list: %s", raw.Error)
		}
		for _, member := range raw.Members {
			info := userInfoFromRaw(member)
			if info.ID != "" {
				users = append(users, info)
			}
		}
		cursor = raw.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
		if page == maxUsersListPages-1 {
			return users, true, nil
		}
	}
	return users, false, nil
}

type slackUserRaw struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RealName string `json:"real_name,omitempty"`
	IsBot    bool   `json:"is_bot,omitempty"`
	Deleted  bool   `json:"deleted,omitempty"`
	Profile  struct {
		DisplayName string `json:"display_name,omitempty"`
		RealName    string `json:"real_name,omitempty"`
		Image48     string `json:"image_48,omitempty"`
		Image72     string `json:"image_72,omitempty"`
		Image192    string `json:"image_192,omitempty"`
		Image512    string `json:"image_512,omitempty"`
	} `json:"profile"`
}

func userInfoFromRaw(raw slackUserRaw) UserInfo {
	return UserInfo{
		ID:          raw.ID,
		Name:        raw.Name,
		DisplayName: firstNonEmpty(raw.Profile.DisplayName, raw.Profile.RealName, raw.RealName),
		RealName:    firstNonEmpty(raw.Profile.RealName, raw.RealName),
		AvatarURL:   firstNonEmpty(raw.Profile.Image72, raw.Profile.Image48, raw.Profile.Image192, raw.Profile.Image512),
		IsBot:       raw.IsBot,
		Deleted:     raw.Deleted,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
