package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// apiCall makes an authenticated JSON API request to reqURL.
// body is marshalled to JSON and sent as the request body, or nil for no body.
// dest is a pointer to decode the JSON response into, or nil to ignore the body.
// Non-2xx responses are returned as an error with the status code; the caller
// is responsible for reading and interpreting any error body.
func apiCall(ctx context.Context, method, reqURL string, body any, token string, verbose bool, dest any) error { //nolint:unparam
	if ctx == nil {
		ctx = context.Background()
	}
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck,gosec

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}

// apiPath builds a full API URL: serverURL + /api/v1/ + operation + / + account + / + parts.
// e.g. apiPath(serverURL, "alice", "accounts", "variables", "MY_KEY") → "https://…/api/v1/accounts/alice/variables/MY_KEY"
func apiPath(serverURL, account string, operation string, parts ...string) string {
	base := strings.TrimSuffix(serverURL, "/") + "/api/v1/" + operation + "/" + account
	if len(parts) > 0 {
		base += "/" + strings.Join(parts, "/")
	}
	return base
}
