package evaluator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fullContextInput() Input {
	return Input{
		TraceID:         "trace_123",
		TraceInput:      map[string]any{"question": "What is 2+2?"},
		TraceOutput:     map[string]any{"answer": "4"},
		PreviousTurns:   []SessionTurn{{Input: "earlier", Output: "reply"}},
		NextUserMessage: "that is wrong",
		UserFeedback:    "thumbs_down",
		Steps: []Step{
			{Name: "search", Type: "tool", Input: map[string]any{"q": "capital of France"}, Output: "Paris"},
		},
	}
}

func decodePayload(t *testing.T, evaluator Evaluator, input Input) map[string]any {
	t.Helper()
	message, err := buildUserMessage(evaluator, input)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(message), &payload))
	return payload
}

func withContext(evaluator Evaluator, config ContextConfig) Evaluator {
	evaluator.Config.Context = config
	return evaluator
}

func TestBuildUserMessageAlwaysIncludesEvaluatorAndTrace(t *testing.T) {
	payload := decodePayload(t, enumEvaluator(), fullContextInput())

	evaluator, ok := payload["evaluator"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "user_sentiment", evaluator["key"])
	assert.Equal(t, enumEvaluator().Prompt, evaluator["prompt"])

	output, ok := evaluator["output"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "enum", output["type"])
	assert.Equal(t, []any{"positive", "neutral", "negative", "unclear"}, output["options"])

	trace, ok := payload["trace"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "trace_123", trace["trace_id"])
	assert.Equal(t, map[string]any{"question": "What is 2+2?"}, trace["input"])
	assert.Equal(t, map[string]any{"answer": "4"}, trace["output"])

	assert.NotContains(t, evaluator, "label")
}

func TestBuildUserMessageOmitsContextByDefault(t *testing.T) {
	payload := decodePayload(t, booleanEvaluator(), fullContextInput())

	for _, field := range []string{"previous_turns", "next_user_message", "user_feedback", "steps"} {
		assert.NotContains(t, payload, field)
	}
}

func TestBuildUserMessageGatesEachContextFieldIndependently(t *testing.T) {
	cases := map[string]struct {
		config  ContextConfig
		present string
	}{
		"previous turns":    {ContextConfig{PreviousTurns: true}, "previous_turns"},
		"next user message": {ContextConfig{NextUserMessage: true}, "next_user_message"},
		"user feedback":     {ContextConfig{UserFeedback: true}, "user_feedback"},
		"steps":             {ContextConfig{Steps: true}, "steps"},
	}
	allFields := []string{"previous_turns", "next_user_message", "user_feedback", "steps"}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			payload := decodePayload(t, withContext(booleanEvaluator(), testCase.config), fullContextInput())
			for _, field := range allFields {
				if field == testCase.present {
					assert.Contains(t, payload, field)
					continue
				}
				assert.NotContains(t, payload, field)
			}
		})
	}
}

func TestBuildUserMessageIncludesAllEnabledContext(t *testing.T) {
	evaluator := withContext(enumEvaluator(), ContextConfig{
		PreviousTurns:   true,
		NextUserMessage: true,
		UserFeedback:    true,
		Steps:           true,
	})
	payload := decodePayload(t, evaluator, fullContextInput())

	assert.Equal(t, []any{map[string]any{"input": "earlier", "output": "reply"}}, payload["previous_turns"])
	assert.Equal(t, "that is wrong", payload["next_user_message"])
	assert.Equal(t, "thumbs_down", payload["user_feedback"])
	assert.Equal(t, []any{map[string]any{
		"name":   "search",
		"type":   "tool",
		"input":  map[string]any{"q": "capital of France"},
		"output": "Paris",
	}}, payload["steps"])
}

func TestBuildUserMessageDropsUnrecognizedUserFeedback(t *testing.T) {
	input := fullContextInput()
	input.UserFeedback = "shrug"
	evaluator := withContext(booleanEvaluator(), ContextConfig{UserFeedback: true})

	assert.NotContains(t, decodePayload(t, evaluator, input), "user_feedback")
}

func TestBuildUserMessageKeepsThumbsUpFeedback(t *testing.T) {
	input := fullContextInput()
	input.UserFeedback = "thumbs_up"
	evaluator := withContext(booleanEvaluator(), ContextConfig{UserFeedback: true})

	assert.Equal(t, "thumbs_up", decodePayload(t, evaluator, input)["user_feedback"])
}

func TestBuildUserMessageOmitsEmptyEnabledContext(t *testing.T) {
	evaluator := withContext(booleanEvaluator(), ContextConfig{
		PreviousTurns:   true,
		NextUserMessage: true,
		UserFeedback:    true,
		Steps:           true,
	})
	payload := decodePayload(t, evaluator, Input{TraceID: "trace_123", TraceInput: "in", TraceOutput: "out"})

	for _, field := range []string{"previous_turns", "next_user_message", "user_feedback", "steps"} {
		assert.NotContains(t, payload, field)
	}
}

func TestBuildUserMessageCapsPreviousTurnsToMostRecent(t *testing.T) {
	input := fullContextInput()
	input.PreviousTurns = []SessionTurn{
		{Input: "oldest", Output: "a"},
		{Input: "second", Output: "b"},
		{Input: "third", Output: "c"},
		{Input: "newest", Output: "d"},
	}
	evaluator := withContext(booleanEvaluator(), ContextConfig{PreviousTurns: true})

	turns, ok := decodePayload(t, evaluator, input)["previous_turns"].([]any)
	require.True(t, ok)
	require.Len(t, turns, maxPreviousTurns)
	assert.Equal(t, map[string]any{"input": "second", "output": "b"}, turns[0])
	assert.Equal(t, map[string]any{"input": "newest", "output": "d"}, turns[2])
}

func TestBuildUserMessageIncludesStepErrorAndOmitsItWhenAbsent(t *testing.T) {
	input := fullContextInput()
	input.Steps = []Step{
		{Name: "lookup", Type: "tool", Input: "id-1", Error: "  upstream timed out  "},
		{Name: "search", Type: "retriever", Input: "q", Output: "hit"},
	}
	evaluator := withContext(booleanEvaluator(), ContextConfig{Steps: true})

	steps, ok := decodePayload(t, evaluator, input)["steps"].([]any)
	require.True(t, ok)
	require.Len(t, steps, 2)
	assert.Equal(t, "upstream timed out", steps[0].(map[string]any)["error"])
	assert.NotContains(t, steps[1].(map[string]any), "error")
	assert.Equal(t, "retriever", steps[1].(map[string]any)["type"])
}

func TestBuildUserMessageCapsSteps(t *testing.T) {
	input := fullContextInput()
	input.Steps = make([]Step, MaxSteps+5)
	for i := range input.Steps {
		input.Steps[i] = Step{Name: "search", Type: "tool"}
	}
	evaluator := withContext(booleanEvaluator(), ContextConfig{Steps: true})

	steps, ok := decodePayload(t, evaluator, input)["steps"].([]any)
	require.True(t, ok)
	assert.Len(t, steps, MaxSteps)
}

func TestBuildUserMessageKeepsEveryStepWhenNoTypesDeclared(t *testing.T) {
	input := fullContextInput()
	input.Steps = []Step{
		{Name: "marker", Type: "event"},
		{Name: "search", Type: "tool"},
		{Name: "embed", Type: "embedding"},
	}
	evaluator := withContext(booleanEvaluator(), ContextConfig{Steps: true})

	steps, ok := decodePayload(t, evaluator, input)["steps"].([]any)
	require.True(t, ok)
	assert.Len(t, steps, 3)
}

func TestBuildUserMessageNarrowsStepsToDeclaredTypes(t *testing.T) {
	input := fullContextInput()
	input.Steps = []Step{
		{Name: "marker", Type: "event"},
		{Name: "search", Type: "tool"},
		{Name: "fetch", Type: "retriever"},
		{Name: "answer", Type: "generation"},
	}
	evaluator := withContext(booleanEvaluator(), ContextConfig{
		Steps:     true,
		StepTypes: []string{"tool", "retriever"},
	})

	steps, ok := decodePayload(t, evaluator, input)["steps"].([]any)
	require.True(t, ok)
	require.Len(t, steps, 2)
	assert.Equal(t, "search", steps[0].(map[string]any)["name"])
	assert.Equal(t, "fetch", steps[1].(map[string]any)["name"])
}

func TestBuildUserMessageMatchesStepTypesCaseInsensitively(t *testing.T) {
	input := fullContextInput()
	input.Steps = []Step{{Name: "answer", Type: "GENERATION"}}
	evaluator := withContext(booleanEvaluator(), ContextConfig{
		Steps:     true,
		StepTypes: []string{"generation"},
	})

	steps, ok := decodePayload(t, evaluator, input)["steps"].([]any)
	require.True(t, ok)
	require.Len(t, steps, 1)
	assert.Equal(t, "answer", steps[0].(map[string]any)["name"])
}

func TestBuildUserMessageNarrowsStepsBeforeCapping(t *testing.T) {
	input := fullContextInput()
	input.Steps = make([]Step, 0, MaxSteps+2)
	for i := 0; i < MaxSteps; i++ {
		input.Steps = append(input.Steps, Step{Name: "marker", Type: "event"})
	}
	input.Steps = append(input.Steps,
		Step{Name: "search", Type: "tool"},
		Step{Name: "lookup", Type: "tool"},
	)
	evaluator := withContext(booleanEvaluator(), ContextConfig{
		Steps:     true,
		StepTypes: []string{"tool"},
	})

	steps, ok := decodePayload(t, evaluator, input)["steps"].([]any)
	require.True(t, ok)
	require.Len(t, steps, 2)
	assert.Equal(t, "search", steps[0].(map[string]any)["name"])
}

func TestBuildUserMessageOmitsStepsWhenNoneMatchDeclaredTypes(t *testing.T) {
	input := fullContextInput()
	input.Steps = []Step{{Name: "marker", Type: "event"}}
	evaluator := withContext(booleanEvaluator(), ContextConfig{
		Steps:     true,
		StepTypes: []string{"tool"},
	})

	assert.NotContains(t, decodePayload(t, evaluator, input), "steps")
}

func TestBuildUserMessageRejectsUnmarshalableStepValue(t *testing.T) {
	input := fullContextInput()
	input.Steps = []Step{{Name: "search", Type: "tool", Input: make(chan int)}}
	evaluator := withContext(booleanEvaluator(), ContextConfig{Steps: true})

	_, err := buildUserMessage(evaluator, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step 0 input")
}

func TestBuildUserMessageTruncatesOversizedTraceValues(t *testing.T) {
	input := fullContextInput()
	input.TraceOutput = strings.Repeat("x", maxValueRunes+500)

	trace, ok := decodePayload(t, booleanEvaluator(), input)["trace"].(map[string]any)
	require.True(t, ok)
	output, ok := trace["output"].(string)
	require.True(t, ok)
	assert.LessOrEqual(t, len([]rune(output)), maxValueRunes)
	assert.Contains(t, output, "__[truncated]__")
}

func TestBuildUserMessageRequiresTraceID(t *testing.T) {
	for _, traceID := range []string{"", "   "} {
		input := fullContextInput()
		input.TraceID = traceID

		_, err := buildUserMessage(booleanEvaluator(), input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trace id is required")
	}
}

func TestBuildUserMessageTrimsTraceID(t *testing.T) {
	input := fullContextInput()
	input.TraceID = "  trace_123  "

	trace, ok := decodePayload(t, booleanEvaluator(), input)["trace"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "trace_123", trace["trace_id"])
}

func TestBuildUserMessageRejectsUnmarshalableTraceValue(t *testing.T) {
	input := fullContextInput()
	input.TraceInput = make(chan int)

	_, err := buildUserMessage(booleanEvaluator(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace input")
}
