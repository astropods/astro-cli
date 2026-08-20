package evaluator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaxSteps is the most steps one evaluator receives, applied after its declared
// types narrow the list.
const MaxSteps = 20

const (
	// maxValueRunes is much larger than a normal trace field, while bounding the
	// combined target and context payload.
	maxValueRunes     = 64_000
	maxPreviousTurns  = 3
	truncationMarker  = "\n__[truncated]__\n"
	feedbackThumbsUp  = "thumbs_up"
	feedbackThumbsDwn = "thumbs_down"
)

type userPayload struct {
	Evaluator       evaluatorPayload `json:"evaluator"`
	Trace           tracePayload     `json:"trace"`
	PreviousTurns   []turnPayload    `json:"previous_turns,omitempty"`
	NextUserMessage string           `json:"next_user_message,omitempty"`
	UserFeedback    string           `json:"user_feedback,omitempty"`
	Steps           []stepPayload    `json:"steps,omitempty"`
}

type stepPayload struct {
	Name   string `json:"name"`
	Type   string `json:"type,omitempty"`
	Input  any    `json:"input"`
	Output any    `json:"output"`
	Error  string `json:"error,omitempty"`
}

type evaluatorPayload struct {
	Key    string `json:"key"`
	Prompt string `json:"prompt"`
	Output Output `json:"output"`
}

type tracePayload struct {
	TraceID string `json:"trace_id"`
	Input   any    `json:"input"`
	Output  any    `json:"output"`
}

type turnPayload struct {
	Input  any `json:"input"`
	Output any `json:"output"`
}

func buildUserMessage(evaluator Evaluator, input Input) (string, error) {
	traceID := strings.TrimSpace(input.TraceID)
	if traceID == "" {
		return "", fmt.Errorf("evaluator input: trace id is required")
	}

	traceInput, err := compactJSONValue(input.TraceInput, maxValueRunes)
	if err != nil {
		return "", fmt.Errorf("evaluator input: trace input: %w", err)
	}
	traceOutput, err := compactJSONValue(input.TraceOutput, maxValueRunes)
	if err != nil {
		return "", fmt.Errorf("evaluator input: trace output: %w", err)
	}

	payload := userPayload{
		Evaluator: evaluatorPayload{
			Key:    evaluator.Key,
			Prompt: evaluator.Prompt,
			Output: evaluator.Output,
		},
		Trace: tracePayload{
			TraceID: traceID,
			Input:   traceInput,
			Output:  traceOutput,
		},
	}
	if err := applyContext(&payload, evaluator.Config.Context, input); err != nil {
		return "", err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("evaluator input: marshal payload: %w", err)
	}
	return string(body), nil
}

// applyContext copies each context field the evaluator enables and drops the
// rest, so an evaluator never receives context it did not configure.
func applyContext(payload *userPayload, config ContextConfig, input Input) error {
	if config.PreviousTurns {
		turns, err := previousTurns(input.PreviousTurns)
		if err != nil {
			return err
		}
		payload.PreviousTurns = turns
	}
	if config.NextUserMessage {
		payload.NextUserMessage = truncateRunes(strings.TrimSpace(input.NextUserMessage), maxValueRunes)
	}
	if config.UserFeedback {
		payload.UserFeedback = normalizeUserFeedback(input.UserFeedback)
	}
	if config.Steps {
		steps, err := steps(filterStepTypes(input.Steps, config.StepTypes))
		if err != nil {
			return err
		}
		payload.Steps = steps
	}
	return nil
}

func filterStepTypes(list []Step, types []string) []Step {
	if len(types) == 0 {
		return list
	}
	permitted := make(map[string]bool, len(types))
	for _, stepType := range types {
		permitted[normalizeStepType(stepType)] = true
	}
	out := make([]Step, 0, len(list))
	for _, step := range list {
		if permitted[normalizeStepType(step.Type)] {
			out = append(out, step)
		}
	}
	return out
}

func normalizeStepType(stepType string) string {
	return strings.ToLower(strings.TrimSpace(stepType))
}

func steps(list []Step) ([]stepPayload, error) {
	if len(list) > MaxSteps {
		list = list[:MaxSteps]
	}
	out := make([]stepPayload, 0, len(list))
	for index, step := range list {
		stepInput, err := compactJSONValue(step.Input, maxValueRunes)
		if err != nil {
			return nil, fmt.Errorf("evaluator input: step %d input: %w", index, err)
		}
		stepOutput, err := compactJSONValue(step.Output, maxValueRunes)
		if err != nil {
			return nil, fmt.Errorf("evaluator input: step %d output: %w", index, err)
		}
		out = append(out, stepPayload{
			Name:   step.Name,
			Type:   step.Type,
			Input:  stepInput,
			Output: stepOutput,
			Error:  truncateRunes(strings.TrimSpace(step.Error), maxValueRunes),
		})
	}
	return out, nil
}

func previousTurns(turns []SessionTurn) ([]turnPayload, error) {
	if len(turns) > maxPreviousTurns {
		turns = turns[len(turns)-maxPreviousTurns:]
	}
	out := make([]turnPayload, 0, len(turns))
	for index, turn := range turns {
		turnInput, err := compactJSONValue(turn.Input, maxValueRunes)
		if err != nil {
			return nil, fmt.Errorf("evaluator input: previous turn %d input: %w", index, err)
		}
		turnOutput, err := compactJSONValue(turn.Output, maxValueRunes)
		if err != nil {
			return nil, fmt.Errorf("evaluator input: previous turn %d output: %w", index, err)
		}
		out = append(out, turnPayload{Input: turnInput, Output: turnOutput})
	}
	return out, nil
}

func normalizeUserFeedback(feedback string) string {
	if feedback == feedbackThumbsUp || feedback == feedbackThumbsDwn {
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

func truncatePrefixRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	const marker = "..."
	markerRunes := []rune(marker)
	if limit <= len(markerRunes) {
		return string(markerRunes[:limit])
	}
	return string(runes[:limit-len(markerRunes)]) + marker
}
