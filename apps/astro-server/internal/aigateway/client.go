// Package aigateway is astro-server's integration with the AI Gateway
// (Bifrost — see docs/plans/ai-gateway-astro-server.md). It mints per-account
// virtual keys against the gateway's governance API, KMS-encrypts the plaintext
// for storage, and surfaces the keys to the deployer for injection into tenant
// pod env vars.
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

// Client talks to Bifrost. Two endpoints are involved:
//   - baseURL: the public gateway URL written into tenant Secrets as the
//     OpenAI base_url (e.g. https://aig.astropod.ai). Data plane only.
//   - adminURL: the in-cluster governance API (e.g.
//     http://bifrost.bifrost.svc.cluster.local:8080), reached with HTTP Basic
//     auth. The public data host does not route /api, and the admin host is
//     WAF-gated, so key issuance always goes in-cluster.
type Client struct {
	baseURL    string
	adminURL   string
	adminAuth  string // pre-built Authorization header value, e.g. "Basic base64(admin:pass)"
	httpClient *http.Client
}

// NewClient constructs a Bifrost client. baseURL is the tenant-visible base_url;
// adminURL + adminAuth drive the governance API. adminAuth is the full
// Authorization header value (Basic base64(user:pass)), built by infra. If
// adminURL is empty it falls back to baseURL (single-URL/local-dev setups).
func NewClient(baseURL, adminURL, adminAuth string) *Client {
	admin := strings.TrimRight(adminURL, "/")
	if admin == "" {
		admin = strings.TrimRight(baseURL, "/")
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		adminURL:   admin,
		adminAuth:  adminAuth,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// URL returns the tenant-visible base_url (written into tenant Secrets).
func (c *Client) URL() string { return c.baseURL }

// KeyRequest is astro-server's provider-agnostic key-mint request. AccountID is
// the Astro account-id — it becomes the Bifrost customer_id and rides in the VK
// name, so per-tenant usage and budget roll up per account. Metadata mirrors the
// old LiteLLM metadata (kind, tags, cluster_id, actor_user_id) and is folded
// into the VK name/description. Duration, if set, becomes an expiry.
type KeyRequest struct {
	AccountID  string
	CustomerID string // resolved Bifrost customer id (the account's customer) — VK inherits its budget
	Metadata   map[string]any
	Duration   string // e.g. "8h"; empty = no expiry
}

// KeyResponse is the normalized reply. Key is the plaintext sk-bf-… (returned
// once); KeyID is the Bifrost virtual-key UUID, used for delete/rotate.
type KeyResponse struct {
	Key   string
	KeyID string
}

// monthlyBudgetUSD is the per-account (customer) spend cap applied to every
// virtual key, resetting monthly. Every VK is associated with the account as a
// Bifrost customer, so usage rolls up per account.
const monthlyBudgetUSD = 20.00

// bifrostVKRequest is the POST /api/governance/virtual-keys payload.
type bifrostVKRequest struct {
	Name            string                  `json:"name"`
	Description     string                  `json:"description,omitempty"`
	IsActive        bool                    `json:"is_active"`
	ExpiresAt       string                  `json:"expires_at,omitempty"` // RFC3339
	CustomerID      string                  `json:"customer_id,omitempty"`
	ProviderConfigs []bifrostProviderConfig `json:"provider_configs"`
}

type bifrostBudget struct {
	MaxLimit      float64 `json:"max_limit"`
	ResetDuration string  `json:"reset_duration"` // e.g. "1M" (monthly)
}

type bifrostProviderConfig struct {
	Provider      string   `json:"provider"`
	Weight        float64  `json:"weight"`
	KeyIDs        []string `json:"key_ids"`        // ["*"] = all provider keys
	AllowedModels []string `json:"allowed_models"` // ["*"] = all models on the provider
}

// bifrostVKResponse is the create reply. The plaintext value is only returned
// on creation; Bifrost stores it encrypted thereafter.
type bifrostVKResponse struct {
	VirtualKey struct {
		ID    string `json:"id"`
		Value string `json:"value"`
	} `json:"virtual_key"`
}

// GenerateKey mints a Bedrock-scoped virtual key. The account-id rides in the
// key name for attribution; grants use key_ids:["*"] (the field Bifrost honors).
func (c *Client) GenerateKey(ctx context.Context, req KeyRequest) (*KeyResponse, error) {
	if req.AccountID == "" {
		return nil, fmt.Errorf("ai gateway: KeyRequest.AccountID is required")
	}

	body := bifrostVKRequest{
		Name:        vkName(req),
		Description: vkDescription(req),
		IsActive:    true,
		// Attach to the account's Bifrost customer; the per-account budget lives
		// on the customer, so this VK inherits it (no per-VK budget).
		CustomerID: req.CustomerID,
		ProviderConfigs: []bifrostProviderConfig{{
			Provider:      "bedrock",
			Weight:        1.0,
			KeyIDs:        []string{"*"},
			AllowedModels: []string{"*"},
		}},
	}
	if req.Duration != "" {
		d, err := time.ParseDuration(req.Duration)
		if err != nil {
			return nil, fmt.Errorf("ai gateway: invalid Duration %q: %w", req.Duration, err)
		}
		body.ExpiresAt = time.Now().UTC().Add(d).Format(time.RFC3339)
	}

	var out bifrostVKResponse
	if err := c.do(ctx, http.MethodPost, "/api/governance/virtual-keys", body, &out); err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}
	if out.VirtualKey.Value == "" || out.VirtualKey.ID == "" {
		return nil, fmt.Errorf("ai gateway: virtual-key create returned empty id/value")
	}
	return &KeyResponse{Key: out.VirtualKey.Value, KeyID: out.VirtualKey.ID}, nil
}

// DeleteKey revokes a virtual key by its Bifrost UUID. Idempotent — a 404 for
// an already-deleted key is treated as success so retries are safe.
func (c *Client) DeleteKey(ctx context.Context, keyID string) error {
	if keyID == "" {
		return fmt.Errorf("ai gateway: DeleteKey: keyID is required")
	}
	if err := c.do(ctx, http.MethodDelete, "/api/governance/virtual-keys/"+keyID, nil, nil); err != nil {
		if isNotFoundErr(err) {
			return nil
		}
		return fmt.Errorf("delete key: %w", err)
	}
	return nil
}

// bifrostCustomerRequest is the POST /api/governance/customers payload. The
// customer id is server-generated (not caller-set); we look it up by name.
type bifrostCustomerRequest struct {
	Name    string          `json:"name"`
	Budgets []bifrostBudget `json:"budgets,omitempty"`
}

type bifrostCustomerResponse struct {
	Customer struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"customer"`
}

type bifrostCustomerList struct {
	Customers []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"customers"`
}

// CreateCustomer creates a Bifrost customer named accountID carrying the monthly
// per-account budget, and returns its server-generated id. Idempotent: if a
// customer with that name already exists (409), it looks up and returns the
// existing id (covers a prior create whose id wasn't persisted).
func (c *Client) CreateCustomer(ctx context.Context, accountID string) (string, error) {
	if accountID == "" {
		return "", fmt.Errorf("ai gateway: CreateCustomer: accountID is required")
	}
	body := bifrostCustomerRequest{
		Name:    accountID,
		Budgets: []bifrostBudget{{MaxLimit: monthlyBudgetUSD, ResetDuration: "1M"}},
	}
	var out bifrostCustomerResponse
	err := c.do(ctx, http.MethodPost, "/api/governance/customers", body, &out)
	if err == nil {
		if out.Customer.ID == "" {
			return "", fmt.Errorf("ai gateway: customer create returned empty id")
		}
		return out.Customer.ID, nil
	}
	var he *httpError
	if errors.As(err, &he) && he.Status == http.StatusConflict {
		return c.findCustomerByName(ctx, accountID)
	}
	return "", fmt.Errorf("create customer: %w", err)
}

func (c *Client) findCustomerByName(ctx context.Context, name string) (string, error) {
	var list bifrostCustomerList
	if err := c.do(ctx, http.MethodGet, "/api/governance/customers", nil, &list); err != nil {
		return "", fmt.Errorf("list customers: %w", err)
	}
	for _, cust := range list.Customers {
		if cust.Name == name {
			return cust.ID, nil
		}
	}
	return "", fmt.Errorf("ai gateway: customer %q exists but was not found in list", name)
}

// vkName builds a human-readable, attribution-bearing name. The account-id is
// always present; a deployment/dev discriminator is appended when available.
func vkName(req KeyRequest) string {
	if kind, _ := req.Metadata["kind"].(string); kind == "dev" {
		return "dev/" + req.AccountID
	}
	if tags, ok := req.Metadata["tags"].([]string); ok {
		for _, t := range tags {
			if strings.HasPrefix(t, "deployment:") {
				return req.AccountID + "/" + t
			}
		}
	}
	return req.AccountID
}

// vkDescription serializes the remaining metadata for admin-side visibility.
func vkDescription(req KeyRequest) string {
	if len(req.Metadata) == 0 {
		return ""
	}
	b, err := json.Marshal(req.Metadata)
	if err != nil {
		return ""
	}
	return string(b)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, c.adminURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", c.adminAuth)

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
