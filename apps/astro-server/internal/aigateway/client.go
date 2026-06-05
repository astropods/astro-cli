// Package aigateway is astro-server's integration with the AI Gateway
// (LiteLLM proxy fleet — see docs/plans/ai-gateway-astro-server.md). It mints
// per-account virtual keys against the gateway's admin API, KMS-encrypts the
// plaintext for storage, and surfaces the keys to the deployer for injection
// into tenant pod env vars.
package aigateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to the LiteLLM proxy's admin API. URL is the LiteLLM endpoint
// (the AI_GATEWAY_URL env var — see config.go); MasterKey is the LiteLLM
// master key (AI_GATEWAY_MASTER_KEY).
type Client struct {
	url        string
	masterKey  string
	httpClient *http.Client
}

// NewClient constructs a LiteLLM admin client. The URL is used as-is — both
// for /key/* admin calls and as the base URL written into tenant Secrets.
func NewClient(url, masterKey string) *Client {
	return &Client{
		url:        strings.TrimRight(url, "/"),
		masterKey:  masterKey,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// URL returns the configured gateway URL (used as the tenant-visible base_url too).
func (c *Client) URL() string { return c.url }

// KeyRequest is the payload sent to LiteLLM's /key/generate.
//
// UserID and TeamID are non-optional and MUST be the Astro account-id. OpenMeter
// rolls per-tenant spend up on `subject` which LiteLLM derives from
// metadata.user_api_key_user_id at /key/generate time; any drift here silently
// corrupts the chargeback ledger.
type KeyRequest struct {
	UserID string `json:"user_id"`
	TeamID string `json:"team_id"`
	// Metadata holds the per-key LiteLLM metadata. Values are arbitrary JSON
	// so we can pass a []string for the "tags" key (which LiteLLM's Langfuse
	// callback reads and stamps onto every trace logged under this key).
	Metadata map[string]any `json:"metadata,omitempty"`
	Duration string         `json:"duration,omitempty"` // e.g. "60d"; empty = no expiry
}

// KeyResponse is LiteLLM's reply from /key/generate.
type KeyResponse struct {
	Key   string `json:"key"`   // plaintext sk-…; only returned once
	KeyID string `json:"token"` // stable LiteLLM identifier; used for /key/delete and /key/info
	Note  string `json:"key_alias,omitempty"`
}

// GenerateKey mints a new virtual key. Plaintext is returned in the response and
// is the only chance the caller has to capture it — the gateway stores a hash
// thereafter. The caller is responsible for KMS-encrypting before persisting.
func (c *Client) GenerateKey(ctx context.Context, req KeyRequest) (*KeyResponse, error) {
	if req.UserID == "" {
		return nil, fmt.Errorf("ai gateway: KeyRequest.UserID is required (must be the account-id)")
	}
	if req.TeamID == "" {
		return nil, fmt.Errorf("ai gateway: KeyRequest.TeamID is required (must be the account-id)")
	}

	var out KeyResponse
	if err := c.post(ctx, "/key/generate", req, &out); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	return &out, nil
}

// DeleteKey revokes a key by its LiteLLM key ID. Idempotent — a 404 for an
// already-deleted key is treated as success so retries are safe.
func (c *Client) DeleteKey(ctx context.Context, keyID string) error {
	if keyID == "" {
		return fmt.Errorf("ai gateway: DeleteKey: keyID is required")
	}
	payload := map[string]any{"keys": []string{keyID}}
	if err := c.post(ctx, "/key/delete", payload, nil); err != nil {
		if isNotFoundErr(err) {
			return nil
		}
		return fmt.Errorf("delete key: %w", err)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+path, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.masterKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return &httpError{Status: resp.StatusCode, Body: string(respBody)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("unmarshal response: %w (body=%q)", err, string(respBody))
	}
	return nil
}

type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("ai gateway: HTTP %d: %s", e.Status, e.Body)
}

func isNotFoundErr(err error) bool {
	var he *httpError
	if !errors.As(err, &he) {
		return false
	}
	return he.Status == http.StatusNotFound
}
