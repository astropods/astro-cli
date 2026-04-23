package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// apiCall makes an authenticated JSON API request to reqURL.
// body is marshalled to JSON and sent as the request body, or nil for no body.
// dest is a pointer to decode the JSON response into for 2xx responses, or nil to ignore.
//
// Returns (statusCode, error):
//   - Network or marshalling failures: (-1, err)
//   - Non-2xx response: (statusCode, err) — err contains the status code and response body
//   - 2xx response: (statusCode, nil) — dest is populated if non-nil
func apiCall(ctx context.Context, method, reqURL string, body any, token string, verbose bool, dest any) (int, error) { //nolint:unparam
	if ctx == nil {
		ctx = context.Background()
	}
	var bodyReader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return -1, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return -1, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return -1, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck,gosec

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
	}

	if dest != nil {
		if err := json.Unmarshal(respBody, dest); err != nil {
			return resp.StatusCode, fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
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
