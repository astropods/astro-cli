package langfuse

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrNotFound is returned when Langfuse responds with 404 — i.e. the
// resource (trace, project, etc.) does not exist. Callers can use
// errors.Is to distinguish a missing resource from an upstream failure.
var ErrNotFound = errors.New("langfuse: not found")

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

// Observation represents a Langfuse observation (span / generation / event).
type Observation struct {
	ID                  string         `json:"id"`
	TraceID             string         `json:"traceId"`
	ParentObservationID string         `json:"parentObservationId"`
	Type                string         `json:"type"` // SPAN | GENERATION | EVENT
	Name                string         `json:"name"`
	StartTime           string         `json:"startTime"`
	EndTime             string         `json:"endTime"`
	Latency             float64        `json:"latency"` // seconds
	Model               string         `json:"model"`
	ModelParameters     map[string]any `json:"modelParameters"`
	Input               any            `json:"input"`
	Output              any            `json:"output"`
	Metadata            map[string]any `json:"metadata"`
	Level               string         `json:"level"` // DEBUG | DEFAULT | WARNING | ERROR
	StatusMessage       string         `json:"statusMessage"`
	Usage               *Usage         `json:"usage"`
	CalculatedTotalCost float64        `json:"calculatedTotalCost"`
}

// Usage represents token usage attached to a generation.
type Usage struct {
	Input  int    `json:"input"`
	Output int    `json:"output"`
	Total  int    `json:"total"`
	Unit   string `json:"unit"`
}

// Score represents a Langfuse evaluation score on a trace.
type Score struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Value         float64 `json:"value"`
	StringValue   string  `json:"stringValue"`
	DataType      string  `json:"dataType"` // NUMERIC | CATEGORICAL | BOOLEAN
	Comment       string  `json:"comment"`
	ObservationID string  `json:"observationId"`
	Source        string  `json:"source"`
	CreatedAt     string  `json:"createdAt"`
}

// TraceDetail is the response shape from GET /api/public/traces/{traceId}.
// Langfuse embeds observations and scores inline.
type TraceDetail struct {
	Trace
	Observations []Observation `json:"observations"`
	Scores       []Score       `json:"scores"`
	UserID       string        `json:"userId"`
	Release      string        `json:"release"`
	Version      string        `json:"version"`
	Environment  string        `json:"environment"`
	Bookmarked   bool          `json:"bookmarked"`
	ExternalID   string        `json:"externalId"`
}

// DailyMetricUsage holds per-model token usage within a daily metric.
type DailyMetricUsage struct {
	Model       string  `json:"model"`
	InputUsage  int     `json:"inputUsage"`
	OutputUsage int     `json:"outputUsage"`
	TotalUsage  int     `json:"totalUsage"`
	TotalCost   float64 `json:"totalCost"`
}

// DailyMetric holds daily aggregated metrics from Langfuse.
type DailyMetric struct {
	Date        string             `json:"date"`
	CountTraces int                `json:"countTraces"`
	TotalCost   float64            `json:"totalCost"`
	Usage       []DailyMetricUsage `json:"usage"`
}

// InputTokens returns the sum of input tokens across all models.
func (m DailyMetric) InputTokens() int {
	var total int
	for _, u := range m.Usage {
		total += u.InputUsage
	}
	return total
}

// OutputTokens returns the sum of output tokens across all models.
func (m DailyMetric) OutputTokens() int {
	var total int
	for _, u := range m.Usage {
		total += u.OutputUsage
	}
	return total
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

// GetTrace returns a single trace with its full observations and scores.
func (c *Client) GetTrace(traceID string) (*TraceDetail, error) {
	var result TraceDetail
	if err := c.doGet("/api/public/traces/"+url.PathEscape(traceID), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetDailyMetrics returns daily aggregated metrics filtered by deployment tag.
func (c *Client) GetDailyMetrics(deploymentID, startTime, endTime string) (*DailyMetricsResponse, error) {
	params := url.Values{}
	if deploymentID != "" {
		params.Set("tags", "deployment:"+deploymentID)
	}
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

// MetricsQuery is the structured query for GET /api/public/metrics.
type MetricsQuery struct {
	View          string              `json:"view"`
	Metrics       []MetricsQueryField `json:"metrics"`
	Dimensions    []MetricsDimension  `json:"dimensions,omitempty"`
	TimeDimension *TimeDimension      `json:"timeDimension,omitempty"`
	Filters       []MetricsFilter     `json:"filters,omitempty"`
	FromTimestamp string              `json:"fromTimestamp"`
	ToTimestamp   string              `json:"toTimestamp"`
}

// MetricsQueryField specifies a measure and aggregation.
type MetricsQueryField struct {
	Measure     string `json:"measure"`
	Aggregation string `json:"aggregation"`
}

// MetricsDimension specifies a grouping dimension.
type MetricsDimension struct {
	Field string `json:"field"`
}

// TimeDimension specifies the time bucketing granularity.
type TimeDimension struct {
	Granularity string `json:"granularity"` // "minute", "hour", "day", "week", "month", "auto"
}

// MetricsFilter specifies a filter condition.
type MetricsFilter struct {
	Type     string `json:"type"`
	Column   string `json:"column"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
	Key      string `json:"key,omitempty"`
}

// MetricsResponse is the response from GET /api/public/metrics.
type MetricsResponse struct {
	Data []map[string]any `json:"data"`
}

// GetMetrics calls the structured metrics API with configurable granularity.
func (c *Client) GetMetrics(q MetricsQuery) (*MetricsResponse, error) {
	queryJSON, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("langfuse: marshal metrics query: %w", err)
	}
	params := url.Values{}
	params.Set("query", string(queryJSON))

	var result MetricsResponse
	if err := c.doGet("/api/public/metrics", params, &result); err != nil {
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

	if resp.StatusCode == http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: %s", ErrNotFound, string(body))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("langfuse: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("langfuse: decode response: %w", err)
	}
	return nil
}
