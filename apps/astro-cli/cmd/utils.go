package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Table layout constants shared by list commands.
const (
	tableTimeFmt    = "2006-01-02T15:04:05"
	tableTimeWidth  = len(tableTimeFmt)
	tableBuildWidth = 8
)

// truncate clips s to at most width runes, appending "…" if trimmed.
func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
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

// apiCall makes an authenticated JSON API request to reqURL.
// body is marshalled to JSON and sent as the request body, or nil for no body.
// dest is a pointer to decode the JSON response into for 2xx responses, or nil to ignore.
//
// Returns (statusCode, error):
//   - Network or marshalling failures: (-1, err)
//   - Non-2xx response: (statusCode, err) — err contains the status code and response body
//   - 2xx response: (statusCode, nil) — dest is populated if non-nil
func apiCall(ctx context.Context, method, reqURL string, body any, token string, verbose bool, dest any) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var bodyReader *bytes.Reader
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return -1, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyBytes = b
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "%s%s %s%s\n", colorDim, method, reqURL, colorReset)
		if len(bodyBytes) > 0 {
			fmt.Fprintf(os.Stderr, "%sbody: %s%s\n", colorDim, string(bodyBytes), colorReset)
		}
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

	if verbose {
		fmt.Fprintf(os.Stderr, "%s→ %d%s\n", colorDim, resp.StatusCode, colorReset)
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

// apiStream makes an authenticated GET request and returns the response body for streaming.
// The caller must close the returned ReadCloser. Returns (statusCode, body, error).
func apiStream(ctx context.Context, reqURL string, token string, verbose bool) (int, io.ReadCloser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "%sGET %s%s\n", colorDim, reqURL, colorReset)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return -1, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return -1, nil, fmt.Errorf("request failed: %w", err)
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "%s→ %d%s\n", colorDim, resp.StatusCode, colorReset)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck,gosec
		return resp.StatusCode, nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}
	return resp.StatusCode, resp.Body, nil
}

// writeJSON encodes v as indented JSON to w.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v) //nolint:errcheck,gosec
}

// cmdAuth returns the current account token and the verbose flag for a command.
func cmdAuth(cmd *cobra.Command) (AccountToken, bool, error) {
	at, err := getCurrentAccountToken(cmd.Context())
	if err != nil {
		return AccountToken{}, false, err
	}
	verbose, _ := cmd.Root().PersistentFlags().GetBool("verbose")
	return at, verbose, nil
}
