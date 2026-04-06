// Package github provides a minimal GitHub API client for webhook and repo operations.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const apiBase = "https://api.github.com"

// Client calls the GitHub REST API using a user OAuth token.
type Client struct {
	token      string
	httpClient *http.Client
}

// New returns a Client authenticated with the given OAuth token.
func New(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Repo is a minimal GitHub repository representation.
type Repo struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// Commit holds the minimal commit metadata returned by GetCommit.
type Commit struct {
	Message string
	Author  string
}

// GetCommit returns the message and author name for a given commit SHA.
func (c *Client) GetCommit(ctx context.Context, repoFullName, sha string) (Commit, error) {
	var result struct {
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/commits/%s", repoFullName, sha), &result); err != nil {
		return Commit{}, fmt.Errorf("github: get commit: %w", err)
	}
	return Commit{
		Message: firstLine(result.Commit.Message),
		Author:  result.Commit.Author.Name,
	}, nil
}

// firstLine returns the first line of s, trimmed of whitespace.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// GetBranchHead returns the latest commit SHA on a branch.
func (c *Client) GetBranchHead(ctx context.Context, repoFullName, branch string) (string, error) {
	var result struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := c.get(ctx, fmt.Sprintf("/repos/%s/branches/%s", repoFullName, branch), &result); err != nil {
		return "", fmt.Errorf("github: get branch head: %w", err)
	}
	return result.Commit.SHA, nil
}

// ListRepos returns up to 100 repos the authenticated user has access to, sorted by recent push.
func (c *Client) ListRepos(ctx context.Context) ([]Repo, error) {
	var repos []Repo
	if err := c.get(ctx, "/user/repos?per_page=100&sort=pushed&affiliation=owner,collaborator", &repos); err != nil {
		return nil, fmt.Errorf("github: list repos: %w", err)
	}
	return repos, nil
}

// Webhook represents a GitHub repository webhook.
type Webhook struct {
	ID     int64    `json:"id"`
	Events []string `json:"events"`
	Active bool     `json:"active"`
}

// CreateWebhookInput holds parameters for creating a webhook.
type CreateWebhookInput struct {
	// RepoFullName is "owner/repo".
	RepoFullName string
	// PayloadURL is the HTTPS endpoint GitHub will POST to.
	PayloadURL string
	// Secret is the HMAC secret for payload signing.
	Secret string
}

// CreateWebhook installs a push-event webhook on the given repository.
// Returns the webhook ID to store for later removal.
func (c *Client) CreateWebhook(ctx context.Context, in CreateWebhookInput) (int64, error) {
	body := map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": map[string]string{
			"url":          in.PayloadURL,
			"content_type": "json",
			"secret":       in.Secret,
			"insecure_ssl": "0",
		},
	}

	var wh Webhook
	if err := c.postJSON(ctx, fmt.Sprintf("/repos/%s/hooks", in.RepoFullName), body, &wh); err != nil {
		return 0, fmt.Errorf("github: create webhook on %s: %w", in.RepoFullName, err)
	}
	return wh.ID, nil
}

// DeleteWebhook removes a webhook from a repository.
func (c *Client) DeleteWebhook(ctx context.Context, repoFullName string, webhookID int64) error {
	path := fmt.Sprintf("/repos/%s/hooks/%d", repoFullName, webhookID)
	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: delete webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("github: delete webhook returned %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("github: GET %s returned %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) postJSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("github: marshal: %w", err)
	}
	req, err := c.newRequest(ctx, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: POST %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("github: POST %s returned %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body *bytes.Reader) (*http.Request, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, apiBase+path, body)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, apiBase+path, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-Github-Api-Version", "2022-11-28")
	return req, nil
}
