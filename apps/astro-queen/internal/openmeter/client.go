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

func (c *Client) CreateCustomer(body json.RawMessage) (json.RawMessage, error) {
	return c.post("/api/v1/customers", body)
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
	defer resp.Body.Close()

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
