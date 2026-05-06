// Package slack provides a minimal Slack API client used to resolve a
// user's Slack identity from a Pipes-issued OAuth token.
//
// The only call we make is auth.test, which returns the (team_id, user_id)
// pair we need to persist a Slack ↔ WorkOS user mapping. Adding more Slack
// API calls (chat.postMessage, conversations.list, etc.) is a matter of
// adding methods to Client.
package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBase = "https://slack.com/api"

// Client calls the Slack Web API using a user OAuth token.
type Client struct {
	token      string
	httpClient *http.Client
}

// New returns a Client authenticated with the given OAuth token.
func New(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Identity is the subset of auth.test we surface to callers. The Slack
// workspace returns more (enterprise, bot_id, etc.) but only these fields
// are load-bearing for identity mapping.
//
// TeamID + UserID is the unique key for slack_identity_mappings. Team,
// TeamDomain, and User are display strings — we persist them for the
// settings UI ("Connected as @alice in Acme") and for audit.
type Identity struct {
	TeamID     string
	UserID     string
	Team       string // workspace display name (e.g. "Acme")
	TeamDomain string // *.slack.com subdomain (e.g. "acme")
	User       string // slack username (e.g. "alice")
}

// AuthTest calls https://slack.com/api/auth.test and returns the slack
// identity associated with the client's token. Returns an error when Slack
// reports ok=false (the token was revoked, scope insufficient, etc.) or
// when the HTTP round-trip fails.
func (c *Client) AuthTest(ctx context.Context) (Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/auth.test", http.NoBody)
	if err != nil {
		return Identity{}, fmt.Errorf("slack: build auth.test request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("slack: auth.test request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Identity{}, fmt.Errorf("slack: read auth.test body: %w", err)
	}
	// Slack returns 200 even for ok=false errors; the body's `ok` field is
	// the authoritative signal. A non-2xx is a real network/proxy issue.
	if resp.StatusCode >= 400 {
		return Identity{}, fmt.Errorf("slack: auth.test returned %d: %s", resp.StatusCode, body)
	}

	var raw struct {
		OK         bool   `json:"ok"`
		Error      string `json:"error,omitempty"`
		TeamID     string `json:"team_id"`
		UserID     string `json:"user_id"`
		Team       string `json:"team"`
		User       string `json:"user"`
		TeamDomain string `json:"team_domain,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Identity{}, fmt.Errorf("slack: decode auth.test: %w", err)
	}
	if !raw.OK {
		return Identity{}, fmt.Errorf("slack: auth.test failed: %s", raw.Error)
	}
	if raw.TeamID == "" || raw.UserID == "" {
		return Identity{}, fmt.Errorf("slack: auth.test missing team_id or user_id")
	}
	return Identity{
		TeamID:     raw.TeamID,
		UserID:     raw.UserID,
		Team:       raw.Team,
		TeamDomain: raw.TeamDomain,
		User:       raw.User,
	}, nil
}
