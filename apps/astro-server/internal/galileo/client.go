package galileo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client communicates with the Galileo API for observability data.
type Client struct {
	endpoint string
	apiKey   string
	project  string
	http     *http.Client
}

// NewClient creates a Galileo API client.
func NewClient(endpoint, apiKey, project string) *Client {
	return &Client{
		endpoint: endpoint,
		apiKey:   apiKey,
		project:  project,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

// LogStream represents a Galileo log stream.
type LogStream struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// MetricsBucket holds aggregated metrics for a time interval (Galileo bucketed_metrics).
type MetricsBucket struct {
	StartBucketTime string  `json:"start_bucket_time"`
	EndBucketTime   string  `json:"end_bucket_time"`
	RequestsCount   float64 `json:"requests_count"`
	FailuresCount   float64 `json:"failures_count"`
	AvgDurationNs   float64 `json:"average_duration_ns"`
	SumDurationNs   float64 `json:"sum_duration_ns"`
	InputTokens     float64 `json:"sum_num_input_tokens"`
	OutputTokens    float64 `json:"sum_num_output_tokens"`
	TotalTokens     float64 `json:"sum_num_total_tokens"`
	GroupBy         string  `json:"group_by,omitempty"`
}

// AggregateMetrics holds top-level aggregate metrics from Galileo.
type AggregateMetrics struct {
	RequestsCount float64 `json:"requests_count"`
	FailuresCount float64 `json:"failures_count"`
	AvgDurationNs float64 `json:"average_duration_ns"`
	SumDurationNs float64 `json:"sum_duration_ns"`
	InputTokens   float64 `json:"sum_num_input_tokens"`
	OutputTokens  float64 `json:"sum_num_output_tokens"`
	TotalTokens   float64 `json:"sum_num_total_tokens"`
}

// MetricsResponse is the response from Galileo's metrics/search endpoint.
type MetricsResponse struct {
	AggregateMetrics AggregateMetrics           `json:"aggregate_metrics"`
	BucketedMetrics  map[string][]MetricsBucket `json:"bucketed_metrics"`
}

// TraceMetrics holds per-trace metric values.
type TraceMetrics struct {
	DurationNs      float64 `json:"duration_ns"`
	TotalTokens     float64 `json:"num_total_tokens"`
	CostTotalTokens float64 `json:"cost_total_tokens"`
}

// TraceEntry represents a single trace record from Galileo.
type TraceEntry struct {
	ID         string       `json:"id"`
	TraceID    string       `json:"trace_id"`
	SessionID  string       `json:"session_id,omitempty"`
	Name       string       `json:"name"`
	Input      string       `json:"input"`
	Output     string       `json:"output"`
	StatusCode int          `json:"status_code"`
	CreatedAt  string       `json:"created_at"`
	UpdatedAt  string       `json:"updated_at,omitempty"`
	Metrics    TraceMetrics `json:"metrics"`
	Tags       []string     `json:"tags,omitempty"`
}

// TracesResponse is the response from Galileo's traces/search endpoint.
type TracesResponse struct {
	Records           []TraceEntry `json:"records"`
	NumRecords        int          `json:"num_records"`
	StartingToken     int          `json:"starting_token"`
	Limit             int          `json:"limit"`
	Paginated         bool         `json:"paginated"`
	NextStartingToken *int         `json:"next_starting_token"`
}

// SearchLogStreams searches for log streams with exact name match.
// POST /v2/projects/{project_id}/log_streams/search
func (c *Client) SearchLogStreams(projectID, agentName string) ([]LogStream, error) {
	return c.searchLogStreams(projectID, "eq", agentName)
}

// SearchLogStreamsContains searches for log streams whose names contain the query.
// POST /v2/projects/{project_id}/log_streams/search
func (c *Client) SearchLogStreamsContains(projectID, query string) ([]LogStream, error) {
	return c.searchLogStreams(projectID, "contains", query)
}

func (c *Client) searchLogStreams(projectID, operator, value string) ([]LogStream, error) {
	endpoint := fmt.Sprintf("%s/v2/projects/%s/log_streams/search", c.endpoint, url.PathEscape(projectID))

	body := map[string]any{
		"filters": []map[string]any{
			{
				"name":     "name",
				"operator": operator,
				"value":    value,
			},
		},
		"limit": 100,
	}

	var result struct {
		LogStreams []LogStream `json:"log_streams"`
	}
	if err := c.doPost(endpoint, body, &result); err != nil {
		return nil, err
	}
	return result.LogStreams, nil
}

// SearchMetrics retrieves aggregated metrics for a log stream.
// POST /v2/projects/{project_id}/metrics/search
func (c *Client) SearchMetrics(projectID, logStreamID, startTime, endTime string, intervalMinutes int) (*MetricsResponse, error) {
	endpoint := fmt.Sprintf("%s/v2/projects/%s/metrics/search", c.endpoint, url.PathEscape(projectID))

	body := map[string]any{
		"start_time":    startTime,
		"end_time":      endTime,
		"log_stream_id": logStreamID,
	}
	if intervalMinutes > 0 {
		body["interval"] = intervalMinutes
	}

	var result MetricsResponse
	if err := c.doPost(endpoint, body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SearchTraces retrieves paginated traces for a log stream.
// POST /v2/projects/{project_id}/traces/search
func (c *Client) SearchTraces(projectID, logStreamID, startTime, endTime string, limit, offset int, status string) (*TracesResponse, error) {
	endpoint := fmt.Sprintf("%s/v2/projects/%s/traces/search", c.endpoint, url.PathEscape(projectID))

	body := map[string]any{
		"log_stream_id": logStreamID,
	}
	if limit > 0 {
		body["limit"] = limit
	}
	if offset > 0 {
		body["starting_token"] = offset
	}

	// Time range filtering via filters array (Galileo uses "type" as discriminator)
	filters := []map[string]any{}
	if startTime != "" {
		filters = append(filters, map[string]any{
			"type":      "date",
			"column_id": "created_at",
			"operator":  "gte",
			"value":     startTime,
		})
	}
	if endTime != "" {
		filters = append(filters, map[string]any{
			"type":      "date",
			"column_id": "created_at",
			"operator":  "lte",
			"value":     endTime,
		})
	}
	if status != "" {
		filters = append(filters, map[string]any{
			"type":      "text",
			"column_id": "status_code",
			"operator":  "eq",
			"value":     status,
		})
	}
	if len(filters) > 0 {
		body["filters"] = filters
	}

	var result TracesResponse
	if err := c.doPost(endpoint, body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// doPost performs an authenticated POST request with a JSON body and decodes the response.
func (c *Client) doPost(rawURL string, body any, out any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("galileo: failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("galileo: failed to create request: %w", err)
	}
	req.Header.Set("Galileo-Api-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("galileo: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("galileo: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("galileo: failed to decode response: %w", err)
	}
	return nil
}
