// Package workclassifier calls the Foundry work-classifier inference services,
// which label Claude Code prompts by purpose and topic.
package workclassifier

import (
	"bytes"
	"context"
	"encoding/json"
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
	// Single-item requests cost ~480ms; batched, ~9ms each.
	maxBatch       = 256
	defaultTimeout = 60 * time.Second

	readyTimeout = 5 * time.Second
)

// Client calls the work-classifier inference services.
type Client struct {
	baseURL      string
	modelVersion string
	httpClient   *http.Client
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

// Classify returns predictions positionally aligned with texts.
func (c *Client) Classify(ctx context.Context, axis Axis, texts []string) ([]Prediction, error) {
	if c == nil {
		return nil, fmt.Errorf("workclassifier: no client configured")
	}
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([]Prediction, 0, len(texts))
	for start := 0; start < len(texts); start += maxBatch {
		end := min(start+maxBatch, len(texts))
		chunk, err := c.predict(ctx, axis, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
	}
	return out, nil
}

func (c *Client) predict(ctx context.Context, axis Axis, texts []string) ([]Prediction, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	instances := make([]instance, len(texts))
	for i, t := range texts {
		instances[i] = instance{Text: t}
	}
	body, err := json.Marshal(predictRequest{Instances: instances})
	if err != nil {
		return nil, fmt.Errorf("workclassifier: marshal: %w", err)
	}

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
		return nil, fmt.Errorf("workclassifier: %s: status %d: %s", axis, resp.StatusCode, truncate(string(raw), 200))
	}

	var parsed predictResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("workclassifier: %s: decode: %w", axis, err)
	}
	// The predictor returns 200 with an error body on tokenizer faults.
	if parsed.Error != "" {
		return nil, fmt.Errorf("workclassifier: %s: predictor error: %s", axis, truncate(parsed.Error, 200))
	}
	if len(parsed.Predictions) != len(texts) {
		return nil, fmt.Errorf("workclassifier: %s: got %d predictions for %d inputs", axis, len(parsed.Predictions), len(texts))
	}
	return parsed.Predictions, nil
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
