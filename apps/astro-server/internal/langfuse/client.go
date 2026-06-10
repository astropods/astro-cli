package langfuse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/sync/errgroup"
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

// APIError is returned when Langfuse responds with a non-success HTTP status.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("langfuse: unexpected status %d: %s", e.StatusCode, e.Body)
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
	UserID    string         `json:"userId"`
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
func (c *Client) GetTraces(ctx context.Context, deploymentID, startTime, endTime string, limit, offset int) (*TracesResponse, error) {
	return c.getTraces(ctx, deploymentID, startTime, endTime, limit, offset, "", "")
}

// GetDatasetTraces returns the trace fields needed to build dataset items.
// Orders ascending by timestamp so offset paging over a frozen
// [fromTimestamp, toTimestamp] window is stable across requests.
func (c *Client) GetDatasetTraces(ctx context.Context, deploymentID, startTime, endTime string, limit, offset int) (*TracesResponse, error) {
	return c.getTraces(ctx, deploymentID, startTime, endTime, limit, offset, "core,io", "timestamp.asc")
}

func (c *Client) getTraces(ctx context.Context, deploymentID, startTime, endTime string, limit, offset int, fields, orderBy string) (*TracesResponse, error) {
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
	if fields != "" {
		params.Set("fields", fields)
	}
	if orderBy != "" {
		params.Set("orderBy", orderBy)
	}

	var result TracesResponse
	if err := c.doGet(ctx, "/api/public/traces", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTrace returns a single trace with its observations and scores.
//
// Two parallel Langfuse requests are made to avoid the slow ClickHouse query
// that fetches all observation I/O in a single scan:
//   - fields=core,io,scores,metrics  → trace-level input/output/metadata; no observation join
//   - fields=core,observations        → observation tree skeleton; no I/O columns
//
// The results are merged before returning.
func (c *Client) GetTrace(ctx context.Context, traceID string) (*TraceDetail, error) {
	escaped := url.PathEscape(traceID)

	var traceIO TraceDetail
	var treeSkeleton TraceDetail

	// Capture errors independently so we can prefer ErrNotFound over
	// context.Canceled when choosing which error to surface. WithContext
	// cancels the sibling goroutine on first failure; without independent
	// capture that cancellation error could mask the original ErrNotFound
	// and flip the caller's HTTP response from 404 to 502.
	var errs [2]error
	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		params := url.Values{}
		params.Set("fields", "core,io,scores,metrics")
		errs[0] = c.doGet(gctx, "/api/public/traces/"+escaped, params, &traceIO)
		return errs[0]
	})

	g.Go(func() error {
		params := url.Values{}
		params.Set("fields", "core,observations")
		errs[1] = c.doGet(gctx, "/api/public/traces/"+escaped, params, &treeSkeleton)
		return errs[1]
	})

	_ = g.Wait()

	// Prefer ErrNotFound over context cancellation so callers get the right
	// HTTP status regardless of which goroutine the errgroup cancelled.
	for _, err := range errs {
		if errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	traceIO.Observations = treeSkeleton.Observations
	return &traceIO, nil
}

// GetTraceCore fetches only the core trace metadata (tags, name, timestamps, cost)
// in a single lightweight ClickHouse lookup — no observations or I/O columns.
// Use this when only the tags are needed, e.g. for ownership verification.
func (c *Client) GetTraceCore(ctx context.Context, traceID string) (*TraceDetail, error) {
	params := url.Values{}
	params.Set("fields", "core")
	var result TraceDetail
	if err := c.doGet(ctx, "/api/public/traces/"+url.PathEscape(traceID), params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetObservation returns a single observation by ID with full input/output/metadata.
func (c *Client) GetObservation(ctx context.Context, observationID string) (*Observation, error) {
	var result Observation
	if err := c.doGet(ctx, "/api/public/observations/"+url.PathEscape(observationID), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// maxDailyMetricsPages caps the pagination loop in GetDailyMetrics to prevent
// a runaway loop if Langfuse ever returns inconsistent TotalPages metadata.
// 100 pages × default limit covers ~2700 days (~7.5 years) of daily metrics.
const maxDailyMetricsPages = 100

// GetDailyMetrics returns all daily aggregated metrics filtered by deployment tag.
// It paginates through every page so callers always receive the full dataset.
func (c *Client) GetDailyMetrics(ctx context.Context, deploymentID, startTime, endTime string) ([]DailyMetric, error) {
	var all []DailyMetric
	for page := 1; page <= maxDailyMetricsPages; page++ {
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
		params.Set("page", fmt.Sprintf("%d", page))

		var result DailyMetricsResponse
		if err := c.doGet(ctx, "/api/public/metrics/daily", params, &result); err != nil {
			return nil, err
		}
		all = append(all, result.Data...)
		if page >= result.Meta.TotalPages || result.Meta.TotalPages == 0 {
			break
		}
	}
	return all, nil
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
func (c *Client) GetMetrics(ctx context.Context, q MetricsQuery) (*MetricsResponse, error) {
	queryJSON, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("langfuse: marshal metrics query: %w", err)
	}
	params := url.Values{}
	params.Set("query", string(queryJSON))

	var result MetricsResponse
	if err := c.doGet(ctx, "/api/public/metrics", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DatasetItemInput is the payload for upserting a single dataset item.
type DatasetItemInput struct {
	ID                  string `json:"id,omitempty"`
	DatasetName         string `json:"datasetName"`
	Input               any    `json:"input"`
	ExpectedOutput      any    `json:"expectedOutput"`
	Metadata            any    `json:"metadata,omitempty"`
	SourceTraceID       string `json:"sourceTraceId,omitempty"`
	SourceObservationID string `json:"sourceObservationId,omitempty"`
}

// DatasetItem is a single entry in a Langfuse dataset.
type DatasetItem struct {
	ID                  string `json:"id"`
	DatasetName         string `json:"datasetName"`
	Input               any    `json:"input"`
	ExpectedOutput      any    `json:"expectedOutput"`
	Metadata            any    `json:"metadata"`
	SourceTraceID       string `json:"sourceTraceId"`
	SourceObservationID string `json:"sourceObservationId"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

// DatasetItemsResponse is the response from GET /api/public/dataset-items.
type DatasetItemsResponse struct {
	Data []DatasetItem `json:"data"`
	Meta struct {
		Page       int `json:"page"`
		Limit      int `json:"limit"`
		TotalItems int `json:"totalItems"`
		TotalPages int `json:"totalPages"`
	} `json:"meta"`
}

// CreateDataset creates a new dataset in Langfuse.
func (c *Client) CreateDataset(ctx context.Context, name, description string) error {
	return c.doPost(ctx, "/api/public/v2/datasets", map[string]string{
		"name":        name,
		"description": description,
	})
}

// UpsertDatasetItem inserts or updates a single dataset item.
func (c *Client) UpsertDatasetItem(ctx context.Context, item DatasetItemInput) error {
	return c.doPost(ctx, "/api/public/dataset-items", item)
}

// GetDatasetItems returns a page of items from a Langfuse dataset.
func (c *Client) GetDatasetItems(ctx context.Context, datasetName string, page, limit int) (*DatasetItemsResponse, error) {
	params := url.Values{}
	params.Set("datasetName", datasetName)
	if page > 0 {
		params.Set("page", fmt.Sprintf("%d", page))
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	var result DatasetItemsResponse
	if err := c.doGet(ctx, "/api/public/dataset-items", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) doPost(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("langfuse: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("langfuse: create request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(c.publicKey + ":" + c.secretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("langfuse: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(b)}
	}
	return nil
}

func (c *Client) doGet(ctx context.Context, path string, params url.Values, out any) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("langfuse: create request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(c.publicKey + ":" + c.secretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
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
		return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("langfuse: decode response: %w", err)
	}
	return nil
}
