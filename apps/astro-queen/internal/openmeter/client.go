package openmeter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is a simple REST client for the OpenMeter API.
type Client struct {
	base   string // e.g. "https://meter.astropod.ai"
	apiKey string
	http   *http.Client
}

// New creates an OpenMeter REST client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		base:   strings.TrimRight(baseURL, "/"),
		apiKey: apiKey,
		http:   &http.Client{},
	}
}

// APIError is returned when the server responds with a non-2xx status.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	// Try to parse RFC 7807 problem detail JSON.
	var problem struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
	}
	if json.Unmarshal([]byte(e.Body), &problem) == nil && problem.Title != "" {
		if problem.Detail != "" {
			return fmt.Sprintf("%d %s: %s", e.Status, problem.Title, problem.Detail)
		}
		return fmt.Sprintf("%d %s", e.Status, problem.Title)
	}
	return fmt.Sprintf("openmeter: %d — %s", e.Status, e.Body)
}

// ─── Meters ──────────────────────────────────────────────────────────────────

func (c *Client) ListMeters() (json.RawMessage, error) {
	return c.get("/api/v1/meters")
}

func (c *Client) CreateMeter(body json.RawMessage) (json.RawMessage, error) {
	return c.post("/api/v1/meters", body)
}

func (c *Client) GetMeter(idOrSlug string) (json.RawMessage, error) {
	return c.get("/api/v1/meters/" + idOrSlug)
}

func (c *Client) DeleteMeter(idOrSlug string) error {
	return c.del("/api/v1/meters/" + idOrSlug)
}

func (c *Client) QueryMeter(idOrSlug string, body json.RawMessage) (json.RawMessage, error) {
	return c.post("/api/v1/meters/"+idOrSlug+"/query", body)
}

func (c *Client) UpdateMeter(idOrSlug string, body json.RawMessage) (json.RawMessage, error) {
	return c.put("/api/v1/meters/"+idOrSlug, body)
}

func (c *Client) ListMeterGroupByValues(idOrSlug, groupByKey, params string) (json.RawMessage, error) {
	path := "/api/v1/meters/" + idOrSlug + "/group-by/" + groupByKey + "/values"
	if params != "" {
		path += "?" + params
	}
	return c.get(path)
}

// ─── Events ──────────────────────────────────────────────────────────────────

// IngestEvent sends a CloudEvents-formatted event via POST /api/v1/events.
func (c *Client) IngestEvent(body json.RawMessage) error {
	reader := bytes.NewReader(body)
	req, err := http.NewRequest(http.MethodPost, c.base+"/api/v1/events", reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/cloudevents+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{Status: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

func (c *Client) ListEvents(params string) (json.RawMessage, error) {
	path := "/api/v1/events"
	if params != "" {
		path += "?" + params
	}
	return c.get(path)
}

func (c *Client) ListEventsV2(params string) (json.RawMessage, error) {
	path := "/api/v2/events"
	if params != "" {
		path += "?" + params
	}
	return c.get(path)
}

// ─── Customers ───────────────────────────────────────────────────────────────

func (c *Client) ListCustomers() (json.RawMessage, error) {
	return c.get("/api/v1/customers")
}

func (c *Client) GetCustomer(id string) (json.RawMessage, error) {
	return c.get("/api/v1/customers/" + id)
}

func (c *Client) UpdateCustomer(id string, body json.RawMessage) (json.RawMessage, error) {
	return c.put("/api/v1/customers/"+id, body)
}

func (c *Client) DeleteCustomer(id string) error {
	return c.del("/api/v1/customers/" + id)
}

func (c *Client) GetCustomerAccess(id string) (json.RawMessage, error) {
	return c.get("/api/v1/customers/" + id + "/access")
}

func (c *Client) ListCustomerApps(id string) (json.RawMessage, error) {
	return c.get("/api/v1/customers/" + id + "/apps")
}

func (c *Client) ListCustomerEntitlements(id string) (json.RawMessage, error) {
	return c.get("/api/v2/customers/" + id + "/entitlements")
}

func (c *Client) CreateCustomerEntitlement(id string, body json.RawMessage) (json.RawMessage, error) {
	return c.post("/api/v2/customers/"+id+"/entitlements", body)
}

func (c *Client) DeleteCustomerEntitlement(custID, entID string) error {
	return c.del("/api/v2/customers/" + custID + "/entitlements/" + entID)
}

func (c *Client) GetEntitlementValue(custID, entID string) (json.RawMessage, error) {
	return c.get("/api/v2/customers/" + custID + "/entitlements/" + entID + "/value")
}

func (c *Client) ListEntitlementGrants(custID, entID string) (json.RawMessage, error) {
	return c.get("/api/v2/customers/" + custID + "/entitlements/" + entID + "/grants")
}

func (c *Client) CreateEntitlementGrant(custID, entID string, body json.RawMessage) (json.RawMessage, error) {
	return c.post("/api/v2/customers/"+custID+"/entitlements/"+entID+"/grants", body)
}

func (c *Client) ResetEntitlement(custID, entID string, body json.RawMessage) (json.RawMessage, error) {
	return c.post("/api/v2/customers/"+custID+"/entitlements/"+entID+"/reset", body)
}

// ─── Features ────────────────────────────────────────────────────────────────

func (c *Client) ListFeatures(includeArchived bool) (json.RawMessage, error) {
	path := "/api/v1/features"
	if includeArchived {
		path += "?includeArchived=true"
	}
	return c.get(path)
}

func (c *Client) CreateFeature(body json.RawMessage) (json.RawMessage, error) {
	return c.post("/api/v1/features", body)
}

func (c *Client) GetFeature(id string) (json.RawMessage, error) {
	return c.get("/api/v1/features/" + id)
}

func (c *Client) DeleteFeature(id string) error {
	return c.del("/api/v1/features/" + id)
}

// ─── internal helpers ────────────────────────────────────────────────────────

func (c *Client) get(path string) (json.RawMessage, error) {
	return c.do("GET", path, nil)
}

func (c *Client) post(path string, body json.RawMessage) (json.RawMessage, error) {
	return c.do("POST", path, body)
}

func (c *Client) put(path string, body json.RawMessage) (json.RawMessage, error) {
	return c.do("PUT", path, body)
}

func (c *Client) del(path string) error {
	_, err := c.do("DELETE", path, nil)
	return err
}

func (c *Client) do(method, path string, body json.RawMessage) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Body: string(respBody)}
	}

	if len(respBody) == 0 {
		return nil, nil
	}
	return json.RawMessage(respBody), nil
}
