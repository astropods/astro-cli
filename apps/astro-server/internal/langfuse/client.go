package langfuse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// ErrNotFound is returned when Langfuse responds with 404 — i.e. the
// resource (trace, project, etc.) does not exist. Callers can use
// errors.Is to distinguish a missing resource from an upstream failure.
var ErrNotFound = errors.New("langfuse: not found")

// APIError is returned when Langfuse responds with a non-success HTTP status.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("langfuse: unexpected status %d: %s", e.StatusCode, e.Body)
}

// transport is the shared low-level HTTP layer: Basic-auth request building
// and response decoding. Used directly by Client for endpoints that don't
// differ between Langfuse versions (scores, datasets), and embedded in both
// traceReader implementations for the ones that do.
type transport struct {
	baseURL   string
	publicKey string
	secretKey string
	http      *http.Client
}

// traceReader is the versioned Langfuse read implementation backing Client.
// Selected once, in NewClient, based on LANGFUSE_USE_V4_API, not re-checked
// per call. This is what makes retiring v3 support later a clean removal
// (delete v3Reader, delete the branch in NewClient) rather than hunting down
// scattered per-call checks.
type traceReader interface {
	getTraces(ctx context.Context, f traceFilter) (*TracesResponse, error)
	getTraceDetail(ctx context.Context, id string) (*TraceDetail, error)
	getTraceCore(ctx context.Context, id string) (*TraceDetail, error)
	getObservation(ctx context.Context, id string) (*Observation, error)
	getMetrics(ctx context.Context, q MetricsQuery) (*MetricsResponse, error)
	getDailyMetrics(ctx context.Context, deploymentID, startTime, endTime string) ([]DailyMetric, error)
}

// Client communicates with the Langfuse REST API for reading traces and metrics.
type Client struct {
	*transport
	reader traceReader
}

// NewClient creates a Langfuse REST API client. The read implementation is
// selected once here, per environment. See
// docs/06-plan/langfuse-v4-migration.md.
func NewClient(baseURL, publicKey, secretKey string) *Client {
	t := &transport{
		baseURL:   baseURL,
		publicKey: publicKey,
		secretKey: secretKey,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
	var reader traceReader
	if useV4API() {
		reader = &v4Reader{transport: t}
	} else {
		reader = &v3Reader{transport: t}
	}
	return &Client{transport: t, reader: reader}
}

// useV4API reports whether to read through the v4 API. An environment whose
// Langfuse writes in `legacy` mode has no populated v4 ClickHouse tables, so
// v4 requires an explicit opt-in.
func useV4API() bool {
	v, err := strconv.ParseBool(os.Getenv("LANGFUSE_USE_V4_API"))
	if err != nil {
		return false
	}
	return v
}

// Trace represents a Langfuse trace.
type Trace struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Timestamp string         `json:"timestamp"`
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

// CreateScoreRequest is the payload for POST /api/public/scores.
type CreateScoreRequest struct {
	ID            string `json:"id,omitempty"`
	TraceID       string `json:"traceId,omitempty"`
	ObservationID string `json:"observationId,omitempty"`
	SessionID     string `json:"sessionId,omitempty"`
	Name          string `json:"name"`
	Value         any    `json:"value"`
	DataType      string `json:"dataType,omitempty"`
	Comment       string `json:"comment,omitempty"`
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

// traceFilter narrows a /api/public/traces query. The deployment tag is always
// applied; userID and sessionID are optional additional filters.
type traceFilter struct {
	deploymentID string
	userID       string
	sessionID    string
	startTime    string
	endTime      string
	limit        int
	offset       int
	fields       string
	orderBy      string
	filters      []TraceFilter
}

// TraceFilter is one condition accepted by Langfuse's JSON-encoded trace
// filter parameter. Value is omitted for null predicates.
type TraceFilter struct {
	Type     string `json:"type"`
	Column   string `json:"column"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

// GetTraces returns traces filtered by deployment ID tag.
func (c *Client) GetTraces(ctx context.Context, deploymentID, startTime, endTime string, limit, offset int) (*TracesResponse, error) {
	return c.GetTracesOrdered(ctx, deploymentID, startTime, endTime, limit, offset, "")
}

// GetTracesOrdered returns one trace page using a Langfuse-supported order.
func (c *Client) GetTracesOrdered(ctx context.Context, deploymentID, startTime, endTime string, limit, offset int, orderBy string) (*TracesResponse, error) {
	return c.reader.getTraces(ctx, traceFilter{
		deploymentID: deploymentID,
		startTime:    startTime,
		endTime:      endTime,
		limit:        limit,
		offset:       offset,
		fields:       "core,metrics",
		orderBy:      orderBy,
	})
}

// GetUserTracesOrdered returns one ordered trace page for a deployment user.
func (c *Client) GetUserTracesOrdered(ctx context.Context, deploymentID, userID, startTime, endTime string, limit, offset int, orderBy string) (*TracesResponse, error) {
	return c.reader.getTraces(ctx, traceFilter{
		deploymentID: deploymentID,
		userID:       userID,
		startTime:    startTime,
		endTime:      endTime,
		limit:        limit,
		offset:       offset,
		fields:       "core,metrics",
		orderBy:      orderBy,
	})
}

// GetTracesFilteredOrdered returns one trace page using Langfuse's advanced
// filter contract and caller-selected field groups.
func (c *Client) GetTracesFilteredOrdered(
	ctx context.Context,
	deploymentID, startTime, endTime string,
	filters []TraceFilter,
	fields string,
	limit, offset int,
	orderBy string,
) (*TracesResponse, error) {
	if fields == "" {
		fields = "core,metrics"
	}
	return c.reader.getTraces(ctx, traceFilter{
		deploymentID: deploymentID,
		startTime:    startTime,
		endTime:      endTime,
		limit:        limit,
		offset:       offset,
		fields:       fields,
		orderBy:      orderBy,
		filters:      filters,
	})
}

// GetQueueTraces returns the trace fields needed to render the judgment queue.
func (c *Client) GetQueueTraces(ctx context.Context, deploymentID, startTime, endTime string, limit, offset int) (*TracesResponse, error) {
	return c.reader.getTraces(ctx, traceFilter{
		deploymentID: deploymentID,
		startTime:    startTime,
		endTime:      endTime,
		limit:        limit,
		offset:       offset,
		fields:       "core,io",
		orderBy:      "timestamp.desc",
	})
}

// GetSessionTraces returns every trace in one conversation (Langfuse session),
// scoped to a single user, ordered oldest-first with trace input/output so the
// chat history can be reconstructed turn-by-turn. The deployment tag plus the
// user filter ensure a caller only ever reads their own conversation.
func (c *Client) GetSessionTraces(
	ctx context.Context,
	deploymentID, userID, sessionID string,
	limit int,
	orderBy string,
) (*TracesResponse, error) {
	if orderBy == "" {
		orderBy = "timestamp.asc"
	}
	return c.reader.getTraces(ctx, traceFilter{
		deploymentID: deploymentID,
		userID:       userID,
		sessionID:    sessionID,
		limit:        limit,
		fields:       "core,io",
		orderBy:      orderBy,
	})
}

// GetPreviousSessionTraces returns the traces immediately before the target in
// the same deployment, user, and session. Results are ordered oldest-first so
// callers can present them as conversational context without exposing later
// turns to the consumer.
func (c *Client) GetPreviousSessionTraces(
	ctx context.Context,
	deploymentID, userID, sessionID, targetTraceID, targetTimestamp string,
	limit int,
) ([]Trace, error) {
	if userID == "" || sessionID == "" || targetTimestamp == "" || limit <= 0 {
		return nil, nil
	}
	targetTime, err := time.Parse(time.RFC3339Nano, targetTimestamp)
	if err != nil {
		return nil, fmt.Errorf("langfuse: invalid target timestamp %q: %w", targetTimestamp, err)
	}

	response, err := c.reader.getTraces(ctx, traceFilter{
		deploymentID: deploymentID,
		userID:       userID,
		sessionID:    sessionID,
		endTime:      targetTimestamp,
		limit:        limit + 1, // The inclusive time window normally includes the target.
		fields:       "core,io",
		orderBy:      "timestamp.desc",
	})
	if err != nil {
		return nil, err
	}

	previous := make([]Trace, 0, limit)
	for _, trace := range response.Data {
		if trace.ID == targetTraceID {
			continue
		}
		traceTime, parseErr := time.Parse(time.RFC3339Nano, trace.Timestamp)
		if parseErr != nil || !traceTime.Before(targetTime) {
			continue
		}
		previous = append(previous, trace)
		if len(previous) == limit {
			break
		}
	}
	for left, right := 0, len(previous)-1; left < right; left, right = left+1, right-1 {
		previous[left], previous[right] = previous[right], previous[left]
	}
	return previous, nil
}

// GetNextSessionTrace returns the trace immediately after the target in the
// same deployment, user, and session. Langfuse's fromTimestamp filters the
// trace event timestamp (not createdAt), so a small ascending window is scanned
// for the target and its successor.
func (c *Client) GetNextSessionTrace(
	ctx context.Context,
	deploymentID, userID, sessionID, targetTraceID, targetTimestamp string,
) (*Trace, error) {
	if userID == "" || sessionID == "" || targetTimestamp == "" {
		return nil, nil
	}
	response, err := c.reader.getTraces(ctx, traceFilter{
		deploymentID: deploymentID,
		userID:       userID,
		sessionID:    sessionID,
		startTime:    targetTimestamp,
		limit:        10,
		fields:       "core,io",
		orderBy:      "timestamp.asc",
	})
	if err != nil {
		return nil, err
	}
	for i := 0; i+1 < len(response.Data); i++ {
		if response.Data[i].ID == targetTraceID {
			return &response.Data[i+1], nil
		}
	}
	return nil, nil
}

// GetTrace returns a single trace with its observations and scores.
func (c *Client) GetTrace(ctx context.Context, traceID string) (*TraceDetail, error) {
	return c.reader.getTraceDetail(ctx, traceID)
}

// GetTraceCore fetches only the core trace metadata (tags, name, timestamps, cost)
// in a single lightweight lookup — no observations or I/O columns. Use this
// when only the tags are needed, e.g. for ownership verification.
func (c *Client) GetTraceCore(ctx context.Context, traceID string) (*TraceDetail, error) {
	return c.reader.getTraceCore(ctx, traceID)
}

// GetObservation returns a single observation by ID with full input/output/metadata.
func (c *Client) GetObservation(ctx context.Context, observationID string) (*Observation, error) {
	return c.reader.getObservation(ctx, observationID)
}

// CreateScore attaches a score to a trace, observation, or session. Confirmed
// live: unchanged between v3.221.1 and v4.11.0 — /api/public/v3/scores
// returns 405 (read-only), and this legacy write endpoint carries no
// _deprecation notice, so there's no non-deprecated path to move to.
func (c *Client) CreateScore(ctx context.Context, req CreateScoreRequest) error {
	return c.doPost(ctx, "/api/public/scores", req)
}

// GetDailyMetrics returns all daily aggregated metrics filtered by deployment tag.
func (c *Client) GetDailyMetrics(ctx context.Context, deploymentID, startTime, endTime string) ([]DailyMetric, error) {
	return c.reader.getDailyMetrics(ctx, deploymentID, startTime, endTime)
}

// MetricsQuery is the structured query for GET /api/public/metrics.
type MetricsQuery struct {
	View          string              `json:"view"`
	Metrics       []MetricsQueryField `json:"metrics"`
	Dimensions    []MetricsDimension  `json:"dimensions,omitempty"`
	TimeDimension *TimeDimension      `json:"timeDimension,omitempty"`
	Filters       []MetricsFilter     `json:"filters,omitempty"`
	// OrderBy and Config are unused on v3 (the legacy /api/public/metrics
	// ignores them if set) but required on v4 when grouping by a
	// high-cardinality dimension — see getMetricsV4's doc comment. Left as
	// caller-settable rather than always auto-filled so a caller with a
	// deliberate order/limit isn't silently overridden.
	OrderBy       []MetricsOrderBy `json:"orderBy,omitempty"`
	Config        *MetricsConfig   `json:"config,omitempty"`
	FromTimestamp string           `json:"fromTimestamp"`
	ToTimestamp   string           `json:"toTimestamp"`
}

// MetricsOrderBy specifies the sort applied to a structured metrics query.
// Field is the response row key (e.g. "count_count", "sum_totalCost" —
// aggregation_measure), not the bare measure name.
type MetricsOrderBy struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

// MetricsConfig carries query-level options for GET /api/public/metrics.
type MetricsConfig struct {
	RowLimit int `json:"row_limit,omitempty"`
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
	return c.reader.getMetrics(ctx, q)
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

// CreateDataset creates a new dataset in Langfuse. Confirmed live: unchanged
// between v3.221.1 and v4.11.0.
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

// DeleteDatasetItem deletes a single dataset item by ID.
func (c *Client) DeleteDatasetItem(ctx context.Context, id string) error {
	return c.doDelete(ctx, "/api/public/dataset-items/"+url.PathEscape(id))
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

func (t *transport) doPost(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("langfuse: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("langfuse: create request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(t.publicKey + ":" + t.secretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := t.http.Do(req)
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

func (t *transport) doDelete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, t.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("langfuse: create request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(t.publicKey + ":" + t.secretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return fmt.Errorf("langfuse: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: %s", ErrNotFound, string(body))
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	return nil
}

func (t *transport) doGet(ctx context.Context, path string, params url.Values, out any) error {
	u := t.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("langfuse: create request: %w", err)
	}

	auth := base64.StdEncoding.EncodeToString([]byte(t.publicKey + ":" + t.secretKey))
	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Accept", "application/json")

	resp, err := t.http.Do(req)
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

// ============================================================================
// v3Reader — Langfuse's legacy (pre-v4) REST API.
// ============================================================================

// v3Reader implements traceReader against /api/public/traces,
// /api/public/observations/{id}, and /api/public/metrics.
type v3Reader struct {
	*transport
}

func (r *v3Reader) getTraces(ctx context.Context, f traceFilter) (*TracesResponse, error) {
	params := url.Values{}
	params.Set("tags", "deployment:"+f.deploymentID)
	if f.userID != "" {
		params.Set("userId", f.userID)
	}
	if f.sessionID != "" {
		params.Set("sessionId", f.sessionID)
	}
	if f.startTime != "" {
		params.Set("fromTimestamp", f.startTime)
	}
	if f.endTime != "" {
		params.Set("toTimestamp", f.endTime)
	}
	if f.limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", f.limit))
	}
	if f.offset > 0 {
		params.Set("page", fmt.Sprintf("%d", f.offset/max(f.limit, 1)+1))
	}
	if f.fields != "" {
		params.Set("fields", f.fields)
	}
	if f.orderBy != "" {
		params.Set("orderBy", f.orderBy)
	}
	if len(f.filters) > 0 {
		encoded, err := json.Marshal(f.filters)
		if err != nil {
			return nil, fmt.Errorf("langfuse: marshal trace filters: %w", err)
		}
		params.Set("filter", string(encoded))
	}

	var result TracesResponse
	if err := r.doGet(ctx, "/api/public/traces", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// getTraceDetail fetches via two parallel Langfuse requests to avoid the slow
// ClickHouse query that fetches all observation I/O in a single scan:
//   - fields=core,io,scores,metrics  → trace-level input/output/metadata; no observation join
//   - fields=core,observations        → observation tree skeleton; no I/O columns
//
// The results are merged before returning.
func (r *v3Reader) getTraceDetail(ctx context.Context, traceID string) (*TraceDetail, error) {
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
		errs[0] = r.doGet(gctx, "/api/public/traces/"+escaped, params, &traceIO)
		return errs[0]
	})

	g.Go(func() error {
		params := url.Values{}
		params.Set("fields", "core,observations")
		errs[1] = r.doGet(gctx, "/api/public/traces/"+escaped, params, &treeSkeleton)
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

func (r *v3Reader) getTraceCore(ctx context.Context, traceID string) (*TraceDetail, error) {
	params := url.Values{}
	params.Set("fields", "core")
	var result TraceDetail
	if err := r.doGet(ctx, "/api/public/traces/"+url.PathEscape(traceID), params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *v3Reader) getObservation(ctx context.Context, observationID string) (*Observation, error) {
	var result Observation
	if err := r.doGet(ctx, "/api/public/observations/"+url.PathEscape(observationID), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *v3Reader) getMetrics(ctx context.Context, q MetricsQuery) (*MetricsResponse, error) {
	queryJSON, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("langfuse: marshal metrics query: %w", err)
	}
	params := url.Values{}
	params.Set("query", string(queryJSON))

	var result MetricsResponse
	if err := r.doGet(ctx, "/api/public/metrics", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// maxDailyMetricsPages caps the pagination loop to prevent a runaway loop if
// Langfuse ever returns inconsistent TotalPages metadata. 100 pages ×
// default limit covers ~2700 days (~7.5 years) of daily metrics.
const maxDailyMetricsPages = 100

// getDailyMetrics paginates through every page so callers always receive the
// full dataset.
func (r *v3Reader) getDailyMetrics(ctx context.Context, deploymentID, startTime, endTime string) ([]DailyMetric, error) {
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
		if err := r.doGet(ctx, "/api/public/metrics/daily", params, &result); err != nil {
			return nil, err
		}
		all = append(all, result.Data...)
		if page >= result.Meta.TotalPages || result.Meta.TotalPages == 0 {
			break
		}
	}
	return all, nil
}

// ============================================================================
// v4Reader — Langfuse v4's REST API (/v2/observations, /v2/metrics,
// /v3/scores). See docs/06-plan/langfuse-v4-migration.md for the full record
// of what was tested live and why each choice below is made the way it is.
// Delete this whole block, v4Reader's branch in NewClient, and the
// traceReader interface (folding v3Reader's methods directly onto Client)
// once every environment has cut over to v4 and v3Reader is retired.
// ============================================================================

// v4Reader implements traceReader against Langfuse v4's REST API.
type v4Reader struct {
	*transport
}

// ObservationV2 is one row from GET /api/public/v2/observations. Confirmed
// live against a Langfuse v4.11.0 instance and cross-checked against
// Langfuse's Fern API spec: fields= is additive (fields=basic,io returns
// core-equivalent fields AND input/output together in one call) — initial
// testing with single group values in isolation wrongly concluded fields=
// was a mutually-exclusive view selector; see the migration doc for that
// correction.
type ObservationV2 struct {
	ID                string   `json:"id"`
	TraceID           string   `json:"traceId"`
	StartTime         string   `json:"startTime"`
	EndTime           string   `json:"endTime"`
	ParentObsID       string   `json:"parentObservationId"`
	Type              string   `json:"type"`
	Name              string   `json:"name"`
	UserID            string   `json:"userId"`
	SessionID         string   `json:"sessionId"`
	IsRootObservation bool     `json:"isRootObservation"`
	Latency           float64  `json:"latency"`
	Input             any      `json:"input"`
	Output            any      `json:"output"`
	Level             string   `json:"level"`
	StatusMessage     string   `json:"statusMessage"`
	Tags              []string `json:"tags"`
	TraceName         string   `json:"traceName"`
	Release           string   `json:"release"`
	Environment       string   `json:"environment"`
	Version           string   `json:"version"`

	Metadata        map[string]any `json:"metadata"`
	Model           string         `json:"model"`
	ModelParameters map[string]any `json:"modelParameters"`
	// Nullable upstream. A JSON null decodes to the zero value rather than
	// failing, which is the wanted behavior: a span with no cost reads as 0.
	TotalCost   float64 `json:"totalCost"`
	InputUsage  int     `json:"inputUsage"`
	OutputUsage int     `json:"outputUsage"`
	TotalUsage  int     `json:"totalUsage"`
}

// observationsV2Response is the response from GET /api/public/v2/observations.
// Confirmed live: pagination is an opaque cursor (Meta.Cursor / cursor=
// request param), not v3's page numbers.
type observationsV2Response struct {
	Data []ObservationV2 `json:"data"`
	Meta struct {
		Cursor string `json:"cursor"`
	} `json:"meta"`
}

// v3ScoresResponse is the response from GET /api/public/v3/scores — the
// stable, non-deprecated read replacement for the scores embedded in v3's
// GET /api/public/traces/{id}. Confirmed live: v3/scores accepts a traceId=
// filter and returns id/name/value/dataType/createdAt; observationId and
// stringValue were not observed on a live response and may need further
// verification before relying on them.
type v3ScoresResponse struct {
	Data []Score `json:"data"`
}

// countFamilyMeasures are measures that double-count spans within a trace
// unless the query is scoped to one span per trace, which the
// isRootObservation filter does.
var countFamilyMeasures = map[string]bool{
	"count": true, "uniqueUserIds": true, "uniqueSessionIds": true, "traceId": true,
}

// defaultHighCardinalityRowLimit is the row_limit auto-filled for a
// userId-grouped v4 metrics query. 1000 is the server-enforced maximum,
// confirmed live ("Too big: expected number to be <=1000") — there is no
// larger value to ask for. See getMetrics's doc comment: this is a hard
// ceiling on result-set completeness, not a tunable default.
const defaultHighCardinalityRowLimit = 1000

// v4ObservationFilters builds the deployment/user/session filter= array
// shared by every v2/observations call. Always routed through the JSON
// filter= array, never through dedicated query params — confirmed live that
// tags=, fromTimestamp=, and toTimestamp= are silently ignored by this
// endpoint (no-op instead of erroring), which would otherwise leak other
// deployments' traces into the result set.
// v4RootFilter selects the one observation per trace that starts it.
//
// It must travel in the filter= array. As a dedicated query param,
// isRootObservation is silently ignored, which is what made the listing return
// every child span as though each were its own trace.
//
// Prefer this over a null filter on parentObservationId. Langfuse resolves the
// column to (parent_span_id = ” OR is_app_root), so it also matches a trace
// whose root carries a parent that was never ingested, the case where the root
// span lives in another service. A parentless-only filter misses those.
var v4RootFilter = TraceFilter{Type: "boolean", Column: "isRootObservation", Operator: "=", Value: true}

// v4TraceFromRoot converts a trace's root observation into a Trace.
//
// The ID is the trace ID, not the root span's own ID. Callers round-trip it
// back through getTraceDetail and getTraceCore, and traceId is the identifier
// the v4 filter contract can look a trace up by.
//
// CreatedAt is set as well as Timestamp because callers render CreatedAt as
// the trace timestamp; leaving it empty renders an invalid date.
func v4TraceFromRoot(o ObservationV2) Trace {
	name := o.TraceName
	if name == "" {
		name = o.Name
	}
	return Trace{
		ID:        o.TraceID,
		Name:      name,
		Timestamp: o.StartTime,
		CreatedAt: o.StartTime,
		UpdatedAt: o.EndTime,
		Input:     o.Input,
		Output:    o.Output,
		SessionID: o.SessionID,
		UserID:    o.UserID,
		Latency:   o.Latency,
		Tags:      o.Tags,
		Metadata:  o.Metadata,
		// TotalCost stays unset here. Cost sits on the child GENERATION spans
		// and does not roll up onto the root, so only a caller holding the
		// whole tree can total it (see v4SumCost).
	}
}

// v4ObservationFromV2 converts a span to the version-independent shape.
func v4ObservationFromV2(o ObservationV2) Observation {
	return Observation{
		ID:                  o.ID,
		TraceID:             o.TraceID,
		ParentObservationID: o.ParentObsID,
		Type:                o.Type,
		Name:                o.Name,
		StartTime:           o.StartTime,
		EndTime:             o.EndTime,
		Latency:             o.Latency,
		Level:               o.Level,
		StatusMessage:       o.StatusMessage,
		Model:               o.Model,
		ModelParameters:     o.ModelParameters,
		Metadata:            o.Metadata,
		Input:               o.Input,
		Output:              o.Output,
		CalculatedTotalCost: o.TotalCost,
		Usage: &Usage{
			Input:  o.InputUsage,
			Output: o.OutputUsage,
			Total:  o.TotalUsage,
		},
	}
}

// v4SumCost totals a trace's cost across its spans. The root carries none of
// its children's cost, so summing is the only way to report a trace total.
//
// Every span counts, including the root. Langfuse's own events_traces view
// sums with parent_span_id != ” instead, which drops the root's own cost and
// so reports nothing for a single-span trace whose only span is a generation.
// Including the root costs nothing when it is a plain span, because its cost is
// then zero anyway. fillTraceCost aggregates the same way, so a trace's cost
// matches between the listing and its detail.
func v4SumCost(spans []ObservationV2) float64 {
	var total float64
	for _, o := range spans {
		total += o.TotalCost
	}
	return total
}

// v4ObservationFields is the field-group set every read requests. The groups
// map to response fields as follows, taken from Langfuse's own group table
// rather than inferred:
//
//	basic         name, level, statusMessage, environment, version
//	metrics       latency, timeToFirstToken
//	trace_context tags, release, traceName
//	metadata      the metadata map
//	model         model, modelParameters, internalModelId
//	usage         totalCost, inputUsage, outputUsage, totalUsage, costDetails
//
// trace_context matters most: it carries the tags callers use to confirm a
// trace belongs to the deployment they asked about. Without it that check sees
// no tags and rejects every trace.
const v4ObservationFields = "basic,metrics,trace_context,metadata,model,usage"

func v4ObservationFilters(f traceFilter) []TraceFilter {
	filters := append([]TraceFilter{}, f.filters...)
	filters = append(filters, TraceFilter{Type: "stringOptions", Column: "tags", Operator: "any of", Value: []string{"deployment:" + f.deploymentID}})
	if f.userID != "" {
		filters = append(filters, TraceFilter{Type: "stringOptions", Column: "userId", Operator: "any of", Value: []string{f.userID}})
	}
	if f.sessionID != "" {
		filters = append(filters, TraceFilter{Type: "stringOptions", Column: "sessionId", Operator: "any of", Value: []string{f.sessionID}})
	}
	return filters
}

// getTraces is the v4 equivalent of v3Reader.getTraces, built on
// /api/public/v2/observations.
func (r *v4Reader) getTraces(ctx context.Context, f traceFilter) (*TracesResponse, error) {
	encoded, err := json.Marshal(append(v4ObservationFilters(f), v4RootFilter))
	if err != nil {
		return nil, fmt.Errorf("langfuse: marshal v4 observation filters: %w", err)
	}

	params := url.Values{}
	params.Set("filter", string(encoded))
	if f.startTime != "" {
		params.Set("fromStartTime", f.startTime)
	}
	if f.endTime != "" {
		params.Set("toStartTime", f.endTime)
	}
	// "io" is added only when the caller's v3-style fields value asked for it,
	// to avoid always paying for input/output payload on calls that don't need it.
	v4Fields := v4ObservationFields
	if strings.Contains(f.fields, "io") {
		v4Fields += ",io"
	}
	params.Set("fields", v4Fields)

	limit := f.limit
	if limit <= 0 {
		limit = 50 // page size used for the cursor walk below; v3 left this to Langfuse's own default when unset.
	}

	// v2/observations uses opaque cursor pagination, not v3's page numbers
	// (confirmed live). There is no way to compute a cursor for an arbitrary
	// f.offset without walking from the start, so that's what this does —
	// correct but O(offset/limit) requests for a caller far into a result set.
	cursor := ""
	remaining := f.offset
	for remaining > 0 {
		step := min(limit, remaining)
		walkParams := url.Values{}
		maps.Copy(walkParams, params)
		walkParams.Set("limit", fmt.Sprintf("%d", step))
		if cursor != "" {
			walkParams.Set("cursor", cursor)
		}
		var walkResult observationsV2Response
		if err := r.doGet(ctx, "/api/public/v2/observations", walkParams, &walkResult); err != nil {
			return nil, fmt.Errorf("langfuse: paginate v4 observations: %w", err)
		}
		if walkResult.Meta.Cursor == "" {
			return &TracesResponse{}, nil // ran out of data before reaching the requested offset
		}
		cursor = walkResult.Meta.Cursor
		remaining -= step
	}

	params.Set("limit", fmt.Sprintf("%d", limit))
	if cursor != "" {
		params.Set("cursor", cursor)
	}

	var result observationsV2Response
	if err := r.doGet(ctx, "/api/public/v2/observations", params, &result); err != nil {
		return nil, err
	}

	out := &TracesResponse{Data: make([]Trace, 0, len(result.Data))}
	for _, o := range result.Data {
		out.Data = append(out.Data, v4TraceFromRoot(o))
	}

	if err := r.fillTraceCost(ctx, f, out.Data); err != nil {
		return nil, err
	}

	// orderBy is accepted but silently ignored by v2/observations (confirmed
	// live in both v3's flat format and a JSON-array format) — sort
	// client-side instead so a caller-requested order is actually honored.
	if f.orderBy != "" {
		desc := strings.HasSuffix(f.orderBy, "desc")
		sort.SliceStable(out.Data, func(i, j int) bool {
			if desc {
				return out.Data[i].Timestamp > out.Data[j].Timestamp
			}
			return out.Data[i].Timestamp < out.Data[j].Timestamp
		})
	}

	return out, nil
}

// fillTraceCost totals each listed trace's cost with one /v2/metrics call.
//
// A listing returns root spans, and cost sits on the child GENERATION spans,
// so a root's own totalCost is 0 and reporting it would price every trace at
// nothing. Langfuse aggregates the same way internally: its events_traces view
// groups the events table by trace_id and sums the spans' cost. That view is
// declared for the v2 API but deliberately withheld from the public one
// ("Public v2 API views - excludes traces"), so the aggregate has to be asked
// for through view:"observations" grouped by traceId instead.
//
// The error is returned rather than swallowed. A zero cost is indistinguishable
// from a free trace on the page, which is the failure this method exists to fix.
func (r *v4Reader) fillTraceCost(ctx context.Context, f traceFilter, traces []Trace) error {
	if len(traces) == 0 {
		return nil
	}

	ids := make([]string, 0, len(traces))
	for _, t := range traces {
		ids = append(ids, t.ID)
	}

	// traceId is a high-cardinality dimension, so v4 requires orderBy and
	// row_limit. One page never exceeds the server's 1000 ceiling.
	resp, err := r.getMetrics(ctx, MetricsQuery{
		View: "observations",
		Metrics: []MetricsQueryField{
			{Measure: "totalCost", Aggregation: "sum"},
		},
		Dimensions: []MetricsDimension{{Field: "traceId"}},
		Filters: []MetricsFilter{
			{Type: "stringOptions", Column: "traceId", Operator: "any of", Value: ids},
			{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: []string{"deployment:" + f.deploymentID}},
		},
		FromTimestamp: f.startTime,
		ToTimestamp:   f.endTime,
		OrderBy:       []MetricsOrderBy{{Field: "sum_totalCost", Direction: "desc"}},
		Config:        &MetricsConfig{RowLimit: min(len(ids), defaultHighCardinalityRowLimit)},
	})
	if err != nil {
		return fmt.Errorf("langfuse: total v4 trace cost: %w", err)
	}

	costByTrace := make(map[string]float64, len(resp.Data))
	for _, row := range resp.Data {
		id, _ := row["traceId"].(string)
		if id == "" {
			continue
		}
		cost, _ := row["sum_totalCost"].(float64)
		costByTrace[id] = cost
	}
	for i := range traces {
		traces[i].TotalCost = costByTrace[traces[i].ID]
	}
	return nil
}

// getTraceDetail resolves a trace by trace ID, the identifier getTraces
// returns as Trace.ID. One /v2/observations call fetches the whole span tree,
// and the root span (the one with no parent) supplies the trace-level fields.
// Scores come from the non-deprecated /v3/scores.
func (r *v4Reader) getTraceDetail(ctx context.Context, id string) (*TraceDetail, error) {
	traceFilterJSON, err := json.Marshal([]TraceFilter{{Type: "stringOptions", Column: "traceId", Operator: "any of", Value: []string{id}}})
	if err != nil {
		return nil, fmt.Errorf("langfuse: marshal v4 trace filter: %w", err)
	}

	var spans observationsV2Response
	var scores v3ScoresResponse
	{
		g, gctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			p := url.Values{}
			p.Set("filter", string(traceFilterJSON))
			p.Set("fields", v4ObservationFields+",io")
			p.Set("limit", "100")
			return r.doGet(gctx, "/api/public/v2/observations", p, &spans)
		})
		g.Go(func() error {
			p := url.Values{}
			p.Set("traceId", id)
			return r.doGet(gctx, "/api/public/v3/scores", p, &scores)
		})
		if err := g.Wait(); err != nil {
			return nil, err
		}
	}
	if len(spans.Data) == 0 {
		return nil, ErrNotFound
	}

	rootObs := v4RootOf(spans.Data)
	children := make([]Observation, 0, len(spans.Data))
	for _, o := range spans.Data {
		if o.ID == rootObs.ID {
			continue
		}
		children = append(children, v4ObservationFromV2(o))
	}

	trace := v4TraceFromRoot(rootObs)
	trace.TotalCost = v4SumCost(spans.Data)

	return &TraceDetail{
		Trace:        trace,
		Observations: children,
		Scores:       scores.Data,
		UserID:       rootObs.UserID,
		Release:      rootObs.Release,
		Version:      rootObs.Version,
		Environment:  rootObs.Environment,
	}, nil
}

// v4RootOf returns the span with no parent, falling back to the first row so a
// trace whose root is outside the page still resolves rather than 404s.
func v4RootOf(spans []ObservationV2) ObservationV2 {
	for _, o := range spans {
		if o.ParentObsID == "" {
			return o
		}
	}
	return spans[0]
}

// getTraceCore is the lightweight, no-I/O variant of getTraceDetail — a
// single request, used for ownership checks that only need tags/name/times.
func (r *v4Reader) getTraceCore(ctx context.Context, id string) (*TraceDetail, error) {
	filter, err := json.Marshal([]TraceFilter{
		{Type: "stringOptions", Column: "traceId", Operator: "any of", Value: []string{id}},
		v4RootFilter,
	})
	if err != nil {
		return nil, fmt.Errorf("langfuse: marshal v4 root filter: %w", err)
	}
	params := url.Values{}
	params.Set("filter", string(filter))
	params.Set("fields", v4ObservationFields)

	var result observationsV2Response
	if err := r.doGet(ctx, "/api/public/v2/observations", params, &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, ErrNotFound
	}
	root := result.Data[0]
	return &TraceDetail{
		Trace:       v4TraceFromRoot(root),
		UserID:      root.UserID,
		Release:     root.Release,
		Environment: root.Environment,
	}, nil
}

// getObservation fetches a single observation by ID, requesting both core
// fields and input/output in one call (fields=basic,io).
func (r *v4Reader) getObservation(ctx context.Context, observationID string) (*Observation, error) {
	filter, err := json.Marshal([]TraceFilter{{Type: "stringOptions", Column: "id", Operator: "any of", Value: []string{observationID}}})
	if err != nil {
		return nil, fmt.Errorf("langfuse: marshal v4 observation filter: %w", err)
	}

	var result observationsV2Response
	p := url.Values{}
	p.Set("filter", string(filter))
	p.Set("fields", v4ObservationFields+",io")
	if err := r.doGet(ctx, "/api/public/v2/observations", p, &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, ErrNotFound
	}
	o := v4ObservationFromV2(result.Data[0])
	return &o, nil
}

// getMetrics is the v4 equivalent of v3Reader.getMetrics, targeting
// /api/public/v2/metrics. view:"traces" doesn't exist on v2 (confirmed live:
// "must be one of observations, scores-numeric, scores-categorical"; also
// confirmed via Langfuse's 2025-12-17 changelog, which states the traces view
// was removed entirely), so view:"traces" queries are rewritten to
// view:"observations". The root filter that replicates v3's per-trace dedup is
// added ONLY when every requested measure is a count-family one (see
// countFamilyMeasures): a real child GENERATION span carrying real usage
// (100/50/150 tokens, cost 0.000045) does NOT roll those numbers up onto the
// root, so a root-scoped sum(inputTokens)/sum(totalCost) comes back zero
// despite the data being present in ClickHouse. Cost/token measures are left
// unfiltered: summing across all spans is already correct there, since
// non-generation spans contribute zero. A query mixing both measure families
// in one call can't be correctly satisfied by a single filter choice — that's
// a known, documented limitation, not silently papered over.
//
// Separately: v4 rejects grouping by a high-cardinality dimension unless the
// query supplies both orderBy and config.row_limit — confirmed live (a
// userId-grouped query 400'd: "High cardinality dimension(s) 'userId'
// require both config.row_limit and orderBy with direction 'desc' on a
// measure field"). v3 imposed no such limit. This method auto-fills both
// when userId is grouped on and the caller hasn't already set them.
//
// UNRESOLVED RISK, not papered over by the auto-fill: row_limit caps the
// result set to top-N by whatever it's sorted on. insights-rollup-spec.md's
// design assumes a complete per-user result set every day ("v1 imposes no
// high-cardinality limits... an ETL can read complete result sets") — for
// any account with more than defaultHighCardinalityRowLimit active users in
// a day, this silently drops the long tail instead of erroring. That's a
// real product-level gap once an environment cuts over to v4, not just a
// parameter to fill in — see docs/06-plan/langfuse-v4-migration.md.
func (r *v4Reader) getMetrics(ctx context.Context, q MetricsQuery) (*MetricsResponse, error) {
	if q.View == "traces" {
		q.View = "observations"
		allCountFamily := len(q.Metrics) > 0
		for _, m := range q.Metrics {
			if !countFamilyMeasures[m.Measure] {
				allCountFamily = false
				break
			}
		}
		if allCountFamily {
			q.Filters = append(append([]MetricsFilter{}, q.Filters...), MetricsFilter{Type: "boolean", Column: "isRootObservation", Operator: "=", Value: true})
		}
	}

	if len(q.OrderBy) == 0 {
		for _, d := range q.Dimensions {
			if d.Field == "userId" && len(q.Metrics) > 0 {
				m := q.Metrics[0]
				q.OrderBy = []MetricsOrderBy{{Field: m.Aggregation + "_" + m.Measure, Direction: "desc"}}
				if q.Config == nil {
					q.Config = &MetricsConfig{RowLimit: defaultHighCardinalityRowLimit}
				}
				break
			}
		}
	}

	queryJSON, err := json.Marshal(q)
	if err != nil {
		return nil, fmt.Errorf("langfuse: marshal v4 metrics query: %w", err)
	}
	params := url.Values{}
	params.Set("query", string(queryJSON))

	var result MetricsResponse
	if err := r.doGet(ctx, "/api/public/v2/metrics", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// getDailyMetrics replaces the deprecated GET /api/public/metrics/daily
// (confirmed live: still returns 200 in v4.11.0, but carries an explicit
// _deprecation notice pointing to /v2/metrics — avoided on principle, not
// because it's broken). Count and cost/token measures need different
// filters (see getMetrics's doc comment), so this issues two separate
// /v2/metrics queries — one root-filtered for the trace count, one
// unfiltered grouped by model for cost/usage — and merges them by date.
func (r *v4Reader) getDailyMetrics(ctx context.Context, deploymentID, startTime, endTime string) ([]DailyMetric, error) {
	var tagFilter []MetricsFilter
	if deploymentID != "" {
		tagFilter = append(tagFilter, MetricsFilter{Type: "arrayOptions", Column: "tags", Operator: "any of", Value: []string{"deployment:" + deploymentID}})
	}

	countQuery := MetricsQuery{
		View:          "observations",
		Metrics:       []MetricsQueryField{{Measure: "count", Aggregation: "count"}},
		TimeDimension: &TimeDimension{Granularity: "day"},
		Filters:       append(append([]MetricsFilter{}, tagFilter...), MetricsFilter{Type: "boolean", Column: "isRootObservation", Operator: "=", Value: true}),
		FromTimestamp: startTime,
		ToTimestamp:   endTime,
	}
	// providedModelName as a grouping dimension is inferred from the same
	// pattern confirmed live for isRootObservation (dimension field name
	// echoed verbatim as a response key) — not independently re-verified with
	// this specific dimension. The live smoke test returned a plausible
	// gpt-4o-mini row, which is reassuring but not the same as testing the
	// dimension in isolation.
	usageQuery := MetricsQuery{
		View: "observations",
		Metrics: []MetricsQueryField{
			{Measure: "inputTokens", Aggregation: "sum"},
			{Measure: "outputTokens", Aggregation: "sum"},
			{Measure: "totalTokens", Aggregation: "sum"},
			{Measure: "totalCost", Aggregation: "sum"},
		},
		Dimensions:    []MetricsDimension{{Field: "providedModelName"}},
		TimeDimension: &TimeDimension{Granularity: "day"},
		Filters:       tagFilter,
		FromTimestamp: startTime,
		ToTimestamp:   endTime,
	}

	var countResp, usageResp *MetricsResponse
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		countResp, err = r.getMetrics(gctx, countQuery)
		return err
	})
	g.Go(func() error {
		var err error
		usageResp, err = r.getMetrics(gctx, usageQuery)
		return err
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	byDate := make(map[string]*DailyMetric)
	for _, row := range countResp.Data {
		date, _ := row["time_dimension"].(string)
		if date == "" {
			continue
		}
		m, ok := byDate[date]
		if !ok {
			m = &DailyMetric{Date: date}
			byDate[date] = m
		}
		count, _ := row["count_count"].(float64)
		m.CountTraces += int(count)
	}
	for _, row := range usageResp.Data {
		date, _ := row["time_dimension"].(string)
		if date == "" {
			continue
		}
		m, ok := byDate[date]
		if !ok {
			m = &DailyMetric{Date: date}
			byDate[date] = m
		}
		cost, _ := row["sum_totalCost"].(float64)
		inputTokens, _ := row["sum_inputTokens"].(float64)
		outputTokens, _ := row["sum_outputTokens"].(float64)
		totalTokens, _ := row["sum_totalTokens"].(float64)
		model, _ := row["providedModelName"].(string)
		m.TotalCost += cost
		m.Usage = append(m.Usage, DailyMetricUsage{
			Model:       model,
			InputUsage:  int(inputTokens),
			OutputUsage: int(outputTokens),
			TotalUsage:  int(totalTokens),
			TotalCost:   cost,
		})
	}

	out := make([]DailyMetric, 0, len(byDate))
	for _, m := range byDate {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

// ============================================================================
// End v4Reader
// ============================================================================
