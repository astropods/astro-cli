// Package pipes wraps the WorkOS Pipes API.
// GetAccessToken delegates to the official SDK (workos-go/v6/pkg/pipes).
// GetAuthorizationURL is a direct HTTP call — the SDK does not yet expose it.
package pipes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	sdkpipes "github.com/workos/workos-go/v6/pkg/pipes"
)

// Re-export SDK types so callers only import this package.
type AccessToken = sdkpipes.AccessToken

// ErrNeedsReauthorization is returned when the user must re-authorize via OAuth.
var ErrNeedsReauthorization error = sdkpipes.NeedsReauthorization

// ErrNotInstalled is returned when the user has not connected the provider.
var ErrNotInstalled error = sdkpipes.NotInstalled

// Client wraps WorkOS Pipes functionality.
type Client struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
	sdk        *sdkpipes.Client
}

// New returns a Client using the given WorkOS API key.
func New(apiKey string) *Client {
	return &Client{
		apiKey:   apiKey,
		endpoint: "https://api.workos.com",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		sdk: &sdkpipes.Client{APIKey: apiKey},
	}
}

// GetAccessTokenInput holds parameters for retrieving a provider access token.
type GetAccessTokenInput struct {
	Provider       string
	UserID         string
	OrganizationID string
}

// GetAccessToken retrieves the current access token for a connected provider.
// Delegates to the WorkOS SDK. Returns ErrNeedsReauthorization or ErrNotInstalled
// when the token cannot be served.
func (c *Client) GetAccessToken(ctx context.Context, in GetAccessTokenInput) (*AccessToken, error) {
	tok, err := c.sdk.GetAccessToken(ctx, sdkpipes.GetAccessTokenOpts{
		Provider:       in.Provider,
		UserID:         in.UserID,
		OrganizationID: in.OrganizationID,
	})
	if err != nil {
		var pipeErr sdkpipes.GetAccessTokenError
		if errors.As(err, &pipeErr) {
			switch pipeErr {
			case sdkpipes.NeedsReauthorization:
				return nil, ErrNeedsReauthorization
			case sdkpipes.NotInstalled:
				return nil, ErrNotInstalled
			}
		}
		return nil, err
	}
	return &tok, nil
}

// GetAuthorizationURLInput holds parameters for generating a Pipes OAuth URL.
type GetAuthorizationURLInput struct {
	Provider       string
	UserID         string
	OrganizationID string
	ReturnTo       string
}

// DeleteConnectionInput holds parameters for revoking a provider OAuth connection.
type DeleteConnectionInput struct {
	Provider       string
	UserID         string
	OrganizationID string
}

// DeleteConnection revokes the OAuth connection for a provider on behalf of a user.
// Calls DELETE /data-integrations/:slug/connections — not yet in the SDK.
func (c *Client) DeleteConnection(ctx context.Context, in DeleteConnectionInput) error {
	body := map[string]string{
		"user_id": in.UserID,
	}
	if in.OrganizationID != "" {
		body["organization_id"] = in.OrganizationID
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("pipes: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/data-integrations/%s/connections", c.endpoint, in.Provider)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("pipes: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("pipes: delete connection request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= 400 {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body) //nolint:errcheck
		return fmt.Errorf("pipes: delete connection returned %d: %s", resp.StatusCode, buf.String())
	}
	return nil
}

// GetAuthorizationURL returns the OAuth URL to redirect the user to.
// Calls POST /data-integrations/:slug/authorize — not yet in the SDK.
func (c *Client) GetAuthorizationURL(ctx context.Context, in GetAuthorizationURLInput) (string, error) {
	body := map[string]string{
		"user_id":   in.UserID,
		"return_to": in.ReturnTo,
	}
	if in.OrganizationID != "" {
		body["organization_id"] = in.OrganizationID
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("pipes: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/data-integrations/%s/authorize", c.endpoint, in.Provider)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("pipes: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("pipes: authorize request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return "", fmt.Errorf("pipes: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("pipes: authorize returned %d: %s", resp.StatusCode, buf.String())
	}

	var result struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		return "", fmt.Errorf("pipes: decode authorize response: %w", err)
	}
	return result.URL, nil
}
