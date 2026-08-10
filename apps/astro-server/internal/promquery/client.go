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

// defaultQueryTimeout caps a single request. Callers whose work legitimately
// outlives it opt in per-call via QueryWithTimeout rather than raising it for
// everyone.
const defaultQueryTimeout = 10 * time.Second

// Client queries a Prometheus-compatible HTTP API.
type Client struct {
	baseURL    string
	cluster    string // Cluster label filter (e.g. EKS cluster name) — empty means no filtering
	httpClient *http.Client
}

// NewClient creates a Prometheus query client. Returns nil if baseURL is empty.
// The cluster parameter filters all queries to a specific cluster label value;
// pass "" to disable filtering (not recommended when environments share a Prometheus instance).
func NewClient(baseURL, cluster string) *Client {
	if baseURL == "" {
		return nil
	}
	// No http.Client.Timeout: it is a hard ceiling no caller can raise, which
	// leaves no way to run a query that legitimately takes minutes. Each request
	// is bounded by its context instead.
	return &Client{
		baseURL:    baseURL,
		cluster:    cluster,
		httpClient: &http.Client{},
	}
}

// Cluster returns the cluster label this client filters on (empty = no filter).
func (c *Client) Cluster() string {
	return c.cluster
}

// Sample is a single instant-query result vector element.
type Sample struct {
	Labels map[string]string
	Value  float64
}

// MatrixSample is a single series from a range-query matrix result.
type MatrixSample struct {
	Labels map[string]string
	Points []Point
}

// Point is one (timestamp, value) pair within a MatrixSample.
type Point struct {
	Timestamp time.Time
	Value     float64
}

// Query executes an instant PromQL query and returns the result vector.
func (c *Client) Query(ctx context.Context, promql string) ([]Sample, error) {
	return c.QueryWithTimeout(ctx, promql, defaultQueryTimeout)
}

// QueryWithTimeout is Query with an explicit per-request cap. The caller's own
// deadline still applies, so this can only tighten it, never extend it.
func (c *Client) QueryWithTimeout(ctx context.Context, promql string, timeout time.Duration) ([]Sample, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

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

// QueryRange executes a range PromQL query and returns the result matrix.
// step is the resolution between points (e.g. 30 * time.Second).
func (c *Client) QueryRange(ctx context.Context, promql string, start, end time.Time, step time.Duration) ([]MatrixSample, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	u, err := url.Parse(c.baseURL + "/api/v1/query_range")
	if err != nil {
		return nil, fmt.Errorf("promquery: bad url: %w", err)
	}
	q := u.Query()
	q.Set("query", promql)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", step.String())
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
	if promResp.Data.ResultType != "matrix" {
		return nil, fmt.Errorf("promquery: expected matrix, got %s", promResp.Data.ResultType)
	}

	series := make([]MatrixSample, 0, len(promResp.Data.Result))
	for _, r := range promResp.Data.Result {
		points := make([]Point, 0, len(r.Values))
		for _, v := range r.Values {
			if len(v) != 2 {
				continue
			}
			tsFloat, ok := v[0].(float64)
			if !ok {
				continue
			}
			valStr, ok := v[1].(string)
			if !ok {
				continue
			}
			val, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue
			}
			points = append(points, Point{
				Timestamp: time.Unix(int64(tsFloat), int64((tsFloat-float64(int64(tsFloat)))*1e9)),
				Value:     val,
			})
		}
		series = append(series, MatrixSample{
			Labels: r.Metric,
			Points: points,
		})
	}

	return series, nil
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
	Value  []any             `json:"value"`  // populated for instant queries (vector)
	Values [][]any           `json:"values"` // populated for range queries (matrix)
}
