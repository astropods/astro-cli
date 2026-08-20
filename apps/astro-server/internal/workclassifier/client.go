// Package workclassifier calls the Foundry work-classifier inference services,
// which label Claude Code prompts by purpose and topic.
package workclassifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Axis is one classifier head. Each is a separate InferenceService.
type Axis string

const (
	AxisPurpose Axis = "purpose"
	AxisTopic   Axis = "topic"
)

// The serving cluster also hosts a "task" head, not consumed here.
var Axes = []Axis{AxisPurpose, AxisTopic}

// Output space per axis, in chart stacking order. Source: astro-models label_schema.json.
var Labels = map[Axis][]string{
	AxisPurpose: {"work", "personal", "ambiguous"},
	AxisTopic: {
		"software-engineering", "data-analytics", "product", "design",
		"marketing", "sales", "customer-support", "operations-it",
		"hr-recruiting", "finance-legal", "research-learning",
		"creative-writing", "general-knowledge", "personal-life", "other",
	},
}

// Fallback is what to record when a prediction falls outside an axis's space:
// each axis's own "undetermined" member.
var Fallback = map[Axis]string{
	AxisPurpose: "ambiguous",
	AxisTopic:   "other",
}

const (
	// A call costs what its tokens cost, so the batch decides whether it fits in
	// defaultTimeout. Claude Code prompts nearly all reach the classifier's
	// truncation length, where a predictor scores about six a second: 256 took
	// 82 seconds and the deadline cancelled a request the server went on to
	// finish. 64 keeps a call near 11 seconds. Batching still pays below that,
	// because a call under about 8 texts is dominated by its own overhead.
	maxBatch       = 64
	defaultTimeout = 60 * time.Second

	readyTimeout = 5 * time.Second

	// A pod rolling under the call loses a batch whose inference is already
	// paid for, so an unavailable predictor is worth a second and third try.
	// Three attempts spaced this way outlast a rollout without holding the
	// day's deadline.
	maxAttempts  = 3
	retryBackoff = 2 * time.Second
)

// Client calls the work-classifier inference services.
type Client struct {
	baseURL      string
	modelVersion string
	httpClient   *http.Client
	retryWait    time.Duration
}

// Returns nil when baseURL is empty so classification degrades to a no-op.
// modelVersion is stamped onto results, not sent; it must track the
// work_classifier_versions pin in astro-infra.
func NewClient(baseURL, modelVersion string) *Client {
	if baseURL == "" {
		return nil
	}
	return &Client{
		baseURL:      baseURL,
		modelVersion: modelVersion,
		httpClient:   &http.Client{},
		retryWait:    retryBackoff,
	}
}

// ModelVersion returns the configured version stamp.
func (c *Client) ModelVersion() string {
	if c == nil {
		return ""
	}
	return c.modelVersion
}

// Prediction is one classification result.
type Prediction struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

type predictRequest struct {
	Instances []instance `json:"instances"`
}

type instance struct {
	Text string `json:"text"`
}

type predictResponse struct {
	Predictions []Prediction `json:"predictions"`
	Error       string       `json:"error"`
}

// Classify returns predictions positionally aligned with texts. A prediction
// with an empty Label is one text the predictor refused; the caller records it
// under the axis fallback instead of paying for it again next tick. Batches
// that did complete come back alongside an error, so a late failure does not
// discard inference already spent. An error with no labels at all means the
// predictor refused everything, which is the cluster's fault, not a text's.
func (c *Client) Classify(ctx context.Context, axis Axis, texts []string) ([]Prediction, error) {
	if c == nil {
		return nil, fmt.Errorf("workclassifier: no client configured")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	s := &splitter{client: c, axis: axis}
	out := make([]Prediction, 0, len(texts))
	for start := 0; start < len(texts); start += maxBatch {
		end := min(start+maxBatch, len(texts))
		chunk, err := s.classify(ctx, texts[start:end])
		if err != nil {
			return append(out, chunk...), err
		}
		// A whole batch refused is a predictor refusing wholesale, not a
		// prompt it cannot read. Splitting the rest of the call would repeat
		// that refusal batch by batch, and keeping the blanks would label the
		// day out of a broken predictor, so the call stops here instead.
		if !anyLabelled(chunk) {
			return out, s.fault
		}
		out = append(out, chunk...)
	}
	return out, nil
}

// splitter narrows a refused batch down to the texts responsible for it. It
// keeps the first fault so a predictor refusing every text is still reported as
// a failure rather than returned as a batch of blanks.
type splitter struct {
	client *Client
	axis   Axis
	fault  error
}

func (s *splitter) classify(ctx context.Context, texts []string) ([]Prediction, error) {
	preds, err := s.client.predict(ctx, s.axis, texts)
	if err == nil {
		return preds, nil
	}
	if !isInputFault(err) {
		return nil, err
	}
	if s.fault == nil {
		s.fault = err
	}
	if len(texts) == 1 {
		return []Prediction{{}}, nil
	}
	half := len(texts) / 2
	left, err := s.classify(ctx, texts[:half])
	if err != nil {
		return left, err
	}
	right, err := s.classify(ctx, texts[half:])
	return append(left, right...), err
}

func anyLabelled(preds []Prediction) bool {
	for _, p := range preds {
		if p.Label != "" {
			return true
		}
	}
	return false
}

// predict retries a predictor that is unavailable and gives up immediately on
// one that rejected the input, which no later attempt changes.
func (c *Client) predict(ctx context.Context, axis Axis, texts []string) ([]Prediction, error) {
	instances := make([]instance, len(texts))
	for i, t := range texts {
		instances[i] = instance{Text: t}
	}
	body, err := json.Marshal(predictRequest{Instances: instances})
	if err != nil {
		return nil, fmt.Errorf("workclassifier: marshal: %w", err)
	}

	var lastErr error
	for attempt := 1; ; attempt++ {
		preds, err := c.attempt(ctx, axis, body, len(texts))
		if err == nil {
			return preds, nil
		}
		lastErr = err
		if attempt >= maxAttempts || isInputFault(err) {
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, lastErr
		case <-time.After(time.Duration(attempt) * c.retryWait):
		}
	}
}

func (c *Client) attempt(ctx context.Context, axis Axis, body []byte, want int) ([]Prediction, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/v1/models/%s:predict", c.baseURL, modelName(axis))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("workclassifier: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("workclassifier: %s: %w", axis, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("workclassifier: %s: read: %w", axis, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusErr := fmt.Errorf("workclassifier: %s: status %d: %s", axis, resp.StatusCode, truncate(string(raw), 200))
		if rejectsInput(resp.StatusCode) {
			return nil, &inputFault{err: statusErr}
		}
		return nil, statusErr
	}

	var parsed predictResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("workclassifier: %s: decode: %w", axis, err)
	}
	// The predictor returns 200 with an error body on tokenizer faults.
	if parsed.Error != "" {
		return nil, &inputFault{err: fmt.Errorf("workclassifier: %s: predictor error: %s", axis, truncate(parsed.Error, 200))}
	}
	if len(parsed.Predictions) != want {
		return nil, &inputFault{err: fmt.Errorf("workclassifier: %s: got %d predictions for %d inputs", axis, len(parsed.Predictions), want)}
	}
	return parsed.Predictions, nil
}

// inputFault marks a response the batch itself caused. Retrying repeats it;
// splitting the batch attributes it to a text.
type inputFault struct{ err error }

func (e *inputFault) Error() string { return e.err.Error() }
func (e *inputFault) Unwrap() error { return e.err }

func isInputFault(err error) bool {
	var fault *inputFault
	return errors.As(err, &fault)
}

// Timeout and rate limit are the two 4xx codes that describe the moment rather
// than the request, so they retry with everything else.
func rejectsInput(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return false
	default:
		return status >= 400 && status < 500
	}
}

// Ready reports whether an axis's InferenceService is serving.
func (c *Client) Ready(ctx context.Context, axis Axis) error {
	if c == nil {
		return fmt.Errorf("workclassifier: no client configured")
	}
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/v1/models/%s", c.baseURL, modelName(axis))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("workclassifier: request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("workclassifier: %s: %w", axis, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("workclassifier: %s: not ready (status %d)", axis, resp.StatusCode)
	}
	return nil
}

func modelName(axis Axis) string {
	return "work-classifier-" + string(axis)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
