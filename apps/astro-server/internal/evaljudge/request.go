package evaljudge

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
)

const (
	// maxJudgeValueRunes is deliberately much larger than a normal trace field,
	// while bounding the combined target, prior-turn, and reaction context.
	maxJudgeValueRunes = 64_000
	truncationMarker   = "\n__[truncated]__\n"
)

type userPayload struct {
	Trace            tracePayload   `json:"trace"`
	PreviousTurns    []turnPayload  `json:"previous_turns,omitempty"`
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

type turnPayload struct {
	Input  any `json:"input"`
	Output any `json:"output"`
}

func buildUserMessage(input Input) (string, error) {
	input.TraceID = strings.TrimSpace(input.TraceID)
	if input.TraceID == "" {
		return "", fmt.Errorf("eval judge input: trace id is required")
	}
	signals := &signalPayload{
		NextUserText:   truncateRunes(strings.TrimSpace(input.NextUserText), maxJudgeValueRunes),
		ThumbsFeedback: normalizeThumbsFeedback(input.ThumbsFeedback),
	}
	if signals.NextUserText == "" && signals.ThumbsFeedback == "" {
		signals = nil
	}

	targetInput, err := compactJSONValue(input.TraceInput, maxJudgeValueRunes)
	if err != nil {
		return "", fmt.Errorf("eval judge input: target trace input: %w", err)
	}
	targetOutput, err := compactJSONValue(input.TraceOutput, maxJudgeValueRunes)
	if err != nil {
		return "", fmt.Errorf("eval judge input: target trace output: %w", err)
	}
	previousTurns := make([]turnPayload, 0, len(input.PreviousTurns))
	for index, turn := range input.PreviousTurns {
		turnInput, err := compactJSONValue(turn.Input, maxJudgeValueRunes)
		if err != nil {
			return "", fmt.Errorf("eval judge input: previous turn %d input: %w", index, err)
		}
		turnOutput, err := compactJSONValue(turn.Output, maxJudgeValueRunes)
		if err != nil {
			return "", fmt.Errorf("eval judge input: previous turn %d output: %w", index, err)
		}
		previousTurns = append(previousTurns, turnPayload{Input: turnInput, Output: turnOutput})
	}

	payload := userPayload{
		Trace: tracePayload{
			TraceID: input.TraceID,
			Input:   targetInput,
			Output:  targetOutput,
		},
		PreviousTurns:    previousTurns,
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
