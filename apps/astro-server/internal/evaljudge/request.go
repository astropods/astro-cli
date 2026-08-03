package evaljudge

import (
	"encoding/json"
	"fmt"
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
	Trace            tracePayload   `json:"trace"`
	Signals          *signalPayload `json:"signals,omitempty"`
	RubricDimensions []string       `json:"rubric_dimensions"`
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

	payload := userPayload{
		Trace: tracePayload{
			TraceID: input.TraceID,
			Input:   targetInput,
			Output:  targetOutput,
		},
		Signals:          signals,
		RubricDimensions: criterionDimensionStrings(),
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
