package langfuse

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client communicates with the Langfuse REST API for reading traces and metrics.
type Client struct {
	baseURL   string
	publicKey string
	secretKey string
	http      *http.Client
}

// NewClient creates a Langfuse REST API client.
func NewClient(baseURL, publicKey, secretKey string) *Client {
	return &Client{
		baseURL:   baseURL,
		publicKey: publicKey,
		secretKey: secretKey,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

// Trace represents a Langfuse trace.
type Trace struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Input     any            `json:"input"`
	Output    any            `json:"output"`
	SessionID string         `json:"sessionId"`
	Metadata  map[string]any `json:"metadata"`
	Tags      []string       `json:"tags"`
	Latency   float64        `json:"latency"` // seconds
	TotalCost float64        `json:"totalCost"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
}

// TracesResponse is the response from GET /api/public/traces.
type TracesResponse struct {
	Data []Trace `json:"data"`
	Meta struct {
		Page       int `json:"page"`
		Limit      int `json:"limit"`
		TotalItems int `json:"totalItems"`
		TotalPages int `json:"totalPages"`
	} `json:"meta"`
}

// DailyMetric holds daily aggregated metrics from Langfuse.
type DailyMetric struct {
	Date        string  `json:"date"`
	CountTraces int     `json:"countTraces"`
	TotalCost   float64 `json:"totalCost"`
	Usage       []any   `json:"usage"`
}

// DailyMetricsResponse is the response from GET /api/public/metrics/daily.
type DailyMetricsResponse struct {
	Data []DailyMetric `json:"data"`
	Meta struct {
		Page       int `json:"page"`
		Limit      int `json:"limit"`
		TotalItems int `json:"totalItems"`
		TotalPages int `json:"totalPages"`
	} `json:"meta"`
}

// GetTraces returns traces filtered by deployment ID tag.
func (c *Client) GetTraces(deploymentID, startTime, endTime string, limit, offset int) (*TracesResponse, error) {
	params := url.Values{}
	params.Set("tags", "deployment:"+deploymentID)
	if startTime != "" {
		params.Set("fromTimestamp", startTime)
	}
	if endTime != "" {
		params.Set("toTimestamp", endTime)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("page", fmt.Sprintf("%d", offset/max(limit, 1)+1))
	}

	var result TracesResponse
	if err := c.doGet("/api/public/traces", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDailyMetrics returns daily aggregated metrics.
func (c *Client) GetDailyMetrics(startTime, endTime string) (*DailyMetricsResponse, error) {
	params := url.Values{}
	if startTime != "" {
		params.Set("fromTimestamp", startTime)
	}
	if endTime != "" {
		params.Set("toTimestamp", endTime)
	}

	var result DailyMetricsResponse
	if err := c.doGet("/api/public/metrics/daily", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) doGet(path string, params url.Values, out any) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("langfuse: create request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(c.publicKey + ":" + c.secretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("langfuse: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("langfuse: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("langfuse: decode response: %w", err)
	}
	return nil
}
