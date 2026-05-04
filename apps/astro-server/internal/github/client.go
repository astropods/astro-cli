// Package github provides a minimal GitHub API client for webhook and repo operations.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	Permissions   struct {
		Admin bool `json:"admin"`
	} `json:"permissions"`
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

// GetLogin returns the GitHub login of the authenticated user.
func (c *Client) GetLogin(ctx context.Context) (string, error) {
	var user struct {
		Login string `json:"login"`
	}
	if err := c.get(ctx, "/user", &user); err != nil {
		return "", fmt.Errorf("github: get user: %w", err)
	}
	return user.Login, nil
}

// SearchRepos searches the authenticated user's repositories via the GitHub Search API.
// login scopes results to user:{login}; if empty, GetLogin is called first.
// With an empty query it returns all repos sorted by push date; with a query it
// filters by name using the in:name qualifier.
// fork:true is included so forked repos (which the user owns) are not excluded by default.
// Note: the Search API does not return permissions objects reliably, so admin
// filtering is skipped — non-admin repos will fail at webhook installation time.
func (c *Client) SearchRepos(ctx context.Context, query, login string) ([]Repo, error) {
	if login == "" {
		var err error
		login, err = c.GetLogin(ctx)
		if err != nil {
			return nil, fmt.Errorf("github: get user login: %w", err)
		}
	}

	var q, sort string
	if query == "" {
		q = fmt.Sprintf("user:%s fork:true", login)
		sort = "pushed"
	} else {
		q = fmt.Sprintf("user:%s fork:true %s in:name", login, query)
		sort = "updated"
	}

	params := url.Values{
		"q":        {q},
		"sort":     {sort},
		"per_page": {"30"},
	}
	var result struct {
		Items []Repo `json:"items"`
	}
	if err := c.get(ctx, "/search/repositories?"+params.Encode(), &result); err != nil {
		return nil, fmt.Errorf("github: search repos: %w", err)
	}
	return result.Items, nil
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

// PathExists reports whether the given path exists in the repository at the
// specified ref (branch name or commit SHA). Uses the Contents API so only the
// path metadata is fetched, not the full tree. Returns false (no error) for a
// 404 response.
func (c *Client) PathExists(ctx context.Context, repoFullName, ref, path string) (bool, error) {
	u := &url.URL{
		Path:     fmt.Sprintf("/repos/%s/contents/%s", repoFullName, path),
		RawQuery: url.Values{"ref": {ref}}.Encode(),
	}
	req, err := c.newRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("github: check path: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("github: check path returned %d: %s", resp.StatusCode, body)
	}
	return true, nil
}

// GetSubtreeSHA returns the git tree SHA for subPath at the given ref.
// subPath is relative to the repo root, e.g. "svc" or "services/my-agent".
// Returns "", nil if the path doesn't exist at that ref.
func (c *Client) GetSubtreeSHA(ctx context.Context, repoBase, ref, subPath string) (string, error) {
	lastSlash := strings.LastIndex(subPath, "/")
	var parent, name string
	if lastSlash < 0 {
		parent = ""
		name = subPath
	} else {
		parent = subPath[:lastSlash]
		name = subPath[lastSlash+1:]
	}

	u := &url.URL{
		Path:     fmt.Sprintf("/repos/%s/contents/%s", repoBase, parent),
		RawQuery: url.Values{"ref": {ref}}.Encode(),
	}
	req, err := c.newRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github: get subtree sha: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github: get subtree sha returned %d: %s", resp.StatusCode, body)
	}
	var entries []struct {
		Name string `json:"name"`
		SHA  string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return "", fmt.Errorf("github: decode contents: %w", err)
	}
	for _, e := range entries {
		if e.Name == name {
			return e.SHA, nil
		}
	}
	return "", nil
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
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github: delete webhook returned %d: %s", resp.StatusCode, body)
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
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github: GET %s returned %d: %s", path, resp.StatusCode, body)
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
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github: POST %s returned %d: %s", path, resp.StatusCode, body)
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
