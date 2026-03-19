package promquery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client queries a Prometheus-compatible HTTP API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a Prometheus query client. Returns nil if baseURL is empty.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		return nil
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Sample is a single instant-query result vector element.
type Sample struct {
	Labels map[string]string
	Value  float64
}

// Query executes an instant PromQL query and returns the result vector.
func (c *Client) Query(ctx context.Context, promql string) ([]Sample, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		return nil, fmt.Errorf("promquery: bad url: %w", err)
	}
	q := u.Query()
	q.Set("query", promql)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("promquery: request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("promquery: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("promquery: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("promquery: status %d: %s", resp.StatusCode, body)
	}

	var promResp promResponse
	if err := json.Unmarshal(body, &promResp); err != nil {
		return nil, fmt.Errorf("promquery: decode: %w", err)
	}
	if promResp.Status != "success" {
		return nil, fmt.Errorf("promquery: prometheus error: %s", promResp.Status)
	}
	if promResp.Data.ResultType != "vector" {
		return nil, fmt.Errorf("promquery: expected vector, got %s", promResp.Data.ResultType)
	}

	samples := make([]Sample, 0, len(promResp.Data.Result))
	for _, r := range promResp.Data.Result {
		if len(r.Value) != 2 {
			continue
		}
		valStr, ok := r.Value[1].(string)
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		samples = append(samples, Sample{
			Labels: r.Metric,
			Value:  val,
		})
	}

	return samples, nil
}

// promResponse is the Prometheus /api/v1/query JSON envelope.
type promResponse struct {
	Status string   `json:"status"`
	Data   promData `json:"data"`
}

type promData struct {
	ResultType string       `json:"resultType"`
	Result     []promResult `json:"result"`
}

type promResult struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}
