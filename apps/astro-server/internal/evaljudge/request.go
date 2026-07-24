package evaljudge

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
)

const (
	maxSupportingTextRunes = 500
	maxTraceInputRunes     = 1500
	maxTraceOutputRunes    = 2500
	truncationMarker       = "\n...[truncated]...\n"
)

type userPayload struct {
	Trace            tracePayload          `json:"trace"`
	Signals          *signalPayload        `json:"signals,omitempty"`
	RubricDimensions []string              `json:"rubric_dimensions"`
	PriorExamples    []priorExamplePayload `json:"prior_examples,omitempty"`
}

type tracePayload struct {
	TraceID string `json:"trace_id"`
	Input   any    `json:"input"`
	Output  any    `json:"output"`
}

type signalPayload struct {
	NextUserText   string `json:"next_user_text,omitempty"`
	ThumbsFeedback string `json:"thumbs_feedback,omitempty"`
}

type priorExamplePayload struct {
	TraceID  string             `json:"trace_id"`
	Input    any                `json:"input"`
	Output   any                `json:"output"`
	Verdict  string             `json:"verdict"`
	Criteria []criterionPayload `json:"criteria"`
}

type criterionPayload struct {
	DimensionKey   string  `json:"dimension_key"`
	DimensionValue float64 `json:"dimension_value"`
}

func buildUserMessage(input Input) (string, error) {
	input.TraceID = strings.TrimSpace(input.TraceID)
	if input.TraceID == "" {
		return "", fmt.Errorf("eval judge input: trace id is required")
	}
	signals := &signalPayload{
		NextUserText:   truncateRunes(strings.TrimSpace(input.NextUserText), maxSupportingTextRunes),
		ThumbsFeedback: normalizeThumbsFeedback(input.ThumbsFeedback),
	}
	if signals.NextUserText == "" && signals.ThumbsFeedback == "" {
		signals = nil
	}

	targetInput, err := compactJSONValue(input.TraceInput, maxTraceInputRunes)
	if err != nil {
		return "", fmt.Errorf("eval judge input: target trace input: %w", err)
	}
	targetOutput, err := compactJSONValue(input.TraceOutput, maxTraceOutputRunes)
	if err != nil {
		return "", fmt.Errorf("eval judge input: target trace output: %w", err)
	}

	selected, err := validatePriorExamples(input.TraceID, input.PriorExamples)
	if err != nil {
		return "", err
	}
	priors := make([]priorExamplePayload, 0, len(selected))
	for _, example := range selected {
		priorInput, err := compactJSONValue(example.Input, maxTraceInputRunes)
		if err != nil {
			return "", fmt.Errorf("eval judge input: prior trace %q input: %w", example.TraceID, err)
		}
		priorOutput, err := compactJSONValue(example.Output, maxTraceOutputRunes)
		if err != nil {
			return "", fmt.Errorf("eval judge input: prior trace %q output: %w", example.TraceID, err)
		}
		criteria, err := canonicalizePriorReasons(example.Verdict, example.Reasons)
		if err != nil {
			return "", fmt.Errorf("eval judge input: prior trace %q: %w", example.TraceID, err)
		}
		priors = append(priors, priorExamplePayload{
			TraceID:  example.TraceID,
			Input:    priorInput,
			Output:   priorOutput,
			Verdict:  string(example.Verdict),
			Criteria: criteria,
		})
	}

	payload := userPayload{
		Trace: tracePayload{
			TraceID: input.TraceID,
			Input:   targetInput,
			Output:  targetOutput,
		},
		Signals:          signals,
		RubricDimensions: criterionDimensionStrings(),
		PriorExamples:    priors,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("eval judge input: marshal context: %w", err)
	}
	return string(body), nil
}

func normalizeThumbsFeedback(feedback string) string {
	if feedback == "thumbs_up" || feedback == "thumbs_down" {
		return feedback
	}
	return ""
}

func validatePriorExamples(currentTraceID string, examples []PriorExample) ([]PriorExample, error) {
	if len(examples) > 5 {
		return nil, fmt.Errorf("eval judge input: expected at most 5 prior examples, got %d", len(examples))
	}
	remaining := map[judgmentstore.Verdict]int{
		judgmentstore.VerdictGood:    2,
		judgmentstore.VerdictBad:     2,
		judgmentstore.VerdictUnknown: 1,
	}
	seen := make(map[string]bool, len(examples))
	validated := make([]PriorExample, 0, len(examples))
	for _, example := range examples {
		example.TraceID = strings.TrimSpace(example.TraceID)
		if example.TraceID == "" {
			return nil, fmt.Errorf("eval judge input: prior trace id is required")
		}
		if example.TraceID == currentTraceID {
			return nil, fmt.Errorf("eval judge input: prior trace %q is the target trace", example.TraceID)
		}
		if !example.Verdict.Valid() {
			return nil, fmt.Errorf("eval judge input: prior trace %q has invalid verdict %q", example.TraceID, example.Verdict)
		}
		if seen[example.TraceID] {
			return nil, fmt.Errorf("eval judge input: duplicate prior trace %q", example.TraceID)
		}
		seen[example.TraceID] = true
		if remaining[example.Verdict] == 0 {
			return nil, fmt.Errorf("eval judge input: too many %q prior examples", example.Verdict)
		}
		remaining[example.Verdict]--
		validated = append(validated, example)
	}
	return validated, nil
}

func canonicalizePriorReasons(verdict judgmentstore.Verdict, reasons []judgmentstore.Reason) ([]criterionPayload, error) {
	if verdict == judgmentstore.VerdictUnknown && len(reasons) > 0 {
		return nil, fmt.Errorf("unknown verdict cannot have criteria")
	}
	byDimension := make(map[judgmentstore.CriterionDimension]float64, len(reasons))
	for _, reason := range reasons {
		if !reason.Dimension.Valid() {
			return nil, fmt.Errorf("invalid criterion dimension %q", reason.Dimension)
		}
		if math.IsNaN(reason.Value) || math.IsInf(reason.Value, 0) || reason.Value < -1 || reason.Value > 1 {
			return nil, fmt.Errorf("criterion %q value %v is outside [-1, 1]", reason.Dimension, reason.Value)
		}
		if _, exists := byDimension[reason.Dimension]; exists {
			return nil, fmt.Errorf("duplicate criterion dimension %q", reason.Dimension)
		}
		byDimension[reason.Dimension] = reason.Value
	}
	out := make([]criterionPayload, 0, len(reasons))
	for _, dimension := range judgmentstore.CriterionDimensions {
		if value, ok := byDimension[dimension]; ok {
			out = append(out, criterionPayload{DimensionKey: string(dimension), DimensionValue: value})
		}
	}
	return out, nil
}

func compactJSONValue(value any, maxRunes int) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	if len([]rune(string(raw))) <= maxRunes {
		return json.RawMessage(raw), nil
	}
	return truncateRunes(string(raw), maxRunes), nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	marker := []rune(truncationMarker)
	if limit <= len(marker) {
		return string(runes[:limit])
	}
	remaining := limit - len(marker)
	head := (remaining + 1) / 2
	tail := remaining - head
	return string(runes[:head]) + truncationMarker + string(runes[len(runes)-tail:])
}

func criterionDimensionStrings() []string {
	out := make([]string, len(judgmentstore.CriterionDimensions))
	for i, dimension := range judgmentstore.CriterionDimensions {
		out[i] = string(dimension)
	}
	return out
}
