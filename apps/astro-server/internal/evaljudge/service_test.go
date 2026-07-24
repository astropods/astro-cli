package evaljudge

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingInvoker struct {
	calls    int
	apiKey   string
	request  aigateway.ChatCompletionRequest
	response *aigateway.ChatCompletionResponse
	err      error
}

func (f *recordingInvoker) ChatCompletion(_ context.Context, apiKey string, request aigateway.ChatCompletionRequest) (*aigateway.ChatCompletionResponse, error) {
	f.calls++
	f.apiKey = apiKey
	f.request = request
	return f.response, f.err
}

func validResponse() *aigateway.ChatCompletionResponse {
	return responseWithContent(`{
		"verdict_score": -0.7,
		"confidence": 84,
		"explanation": "The response misses a required constraint.",
		"criteria": [
			{"dimension_key":"tone","dimension_value":-0.1},
			{"dimension_key":"accuracy","dimension_value":-0.9},
			{"dimension_key":"scope_clarity","dimension_value":-0.4},
			{"dimension_key":"completeness","dimension_value":-0.8},
			{"dimension_key":"instruction_following","dimension_value":-0.7}
		]
	}`)
}

func responseWithContent(content string) *aigateway.ChatCompletionResponse {
	return &aigateway.ChatCompletionResponse{Choices: []aigateway.ChatCompletionChoice{{
		Index:        0,
		Message:      aigateway.ChatMessage{Role: "assistant", Content: content},
		FinishReason: "stop",
	}}}
}

func responseWithFinishReason(content, finishReason string) *aigateway.ChatCompletionResponse {
	response := responseWithContent(content)
	response.Choices[0].FinishReason = finishReason
	return response
}

func TestPredictBuildsStructuredRequestAndReturnsCanonicalPrediction(t *testing.T) {
	usage := &aigateway.ChatCompletionUsage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}
	response := validResponse()
	response.Usage = usage
	invoker := &recordingInvoker{response: response}

	result, err := New(invoker).Predict(context.Background(), "judge-key", Input{
		TraceID:        "target",
		TraceInput:     map[string]any{"question": "What is 2+2?"},
		TraceOutput:    map[string]any{"answer": "5"},
		NextUserText:   "No, that is incorrect.",
		ThumbsFeedback: "thumbs_down",
		PriorExamples: []PriorExample{
			{
				TraceID: "good-1",
				Input:   "input",
				Output:  "output",
				Verdict: judgmentstore.VerdictGood,
				Reasons: []judgmentstore.Reason{
					{Dimension: judgmentstore.DimensionTone, Value: 0.25},
					{Dimension: judgmentstore.DimensionAccuracy, Value: 1},
				},
			},
			{TraceID: "bad-1", Verdict: judgmentstore.VerdictBad},
			{TraceID: "good-2", Verdict: judgmentstore.VerdictGood},
			{TraceID: "bad-2", Verdict: judgmentstore.VerdictBad},
			{TraceID: "unknown", Verdict: judgmentstore.VerdictUnknown},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "claude-sonnet-4-6", EvalDatasetJudgeModel)
	assert.Equal(t, "1", EvalDatasetJudgeVersion)
	assert.Equal(t, 1, invoker.calls)
	assert.Equal(t, "judge-key", invoker.apiKey)
	assert.Equal(t, EvalDatasetJudgeModel, invoker.request.Model)
	require.Len(t, invoker.request.Messages, 2)
	assert.Equal(t, "system", invoker.request.Messages[0].Role)
	assert.Contains(t, invoker.request.Messages[0].Content, "dataset")
	assert.Contains(t, invoker.request.Messages[0].Content, criterionDimensionPromptList())
	assert.Contains(t, invoker.request.Messages[0].Content, "one complete sentence of at most 220 characters")
	assert.Contains(t, invoker.request.Messages[0].Content, "reaction inferred from the next user message")
	assert.Contains(t, invoker.request.Messages[0].Content, "prior examples")
	assert.Contains(t, invoker.request.Messages[0].Content, "Do not quote or restate any user or agent message")
	assert.Equal(t, "user", invoker.request.Messages[1].Role)

	var payload userPayload
	require.NoError(t, json.Unmarshal([]byte(invoker.request.Messages[1].Content), &payload))
	assert.Equal(t, "target", payload.Trace.TraceID)
	assert.Equal(t, map[string]any{"question": "What is 2+2?"}, payload.Trace.Input)
	assert.Equal(t, map[string]any{"answer": "5"}, payload.Trace.Output)
	require.NotNil(t, payload.Signals)
	assert.Equal(t, "No, that is incorrect.", payload.Signals.NextUserText)
	assert.Equal(t, "thumbs_down", payload.Signals.ThumbsFeedback)
	assert.Equal(t, criterionDimensionStrings(), payload.RubricDimensions)

	require.Len(t, payload.PriorExamples, 5)
	assert.Equal(t, []string{"good-1", "bad-1", "good-2", "bad-2", "unknown"}, priorIDs(payload.PriorExamples))
	require.Len(t, payload.PriorExamples[0].Criteria, 2)
	assert.Equal(t, "accuracy", payload.PriorExamples[0].Criteria[0].DimensionKey)
	assert.Equal(t, "tone", payload.PriorExamples[0].Criteria[1].DimensionKey)
	assert.Empty(t, payload.PriorExamples[4].Criteria)

	formatJSON, err := json.Marshal(invoker.request.ResponseFormat)
	require.NoError(t, err)
	var format map[string]any
	require.NoError(t, json.Unmarshal(formatJSON, &format))
	assert.Equal(t, "json_schema", format["type"])
	schemaEnvelope := format["json_schema"].(map[string]any)
	assert.Equal(t, true, schemaEnvelope["strict"])
	assert.Equal(t, "eval_dataset_judgment_prediction", schemaEnvelope["name"])
	schema := schemaEnvelope["schema"].(map[string]any)
	assert.Equal(t, false, schema["additionalProperties"])
	assert.ElementsMatch(t, []any{"verdict_score", "confidence", "explanation", "criteria"}, schema["required"])
	properties := schema["properties"].(map[string]any)
	criteriaSchema := properties["criteria"].(map[string]any)
	criterionSchema := criteriaSchema["items"].(map[string]any)
	assert.Equal(t, false, criterionSchema["additionalProperties"])
	criterionProperties := criterionSchema["properties"].(map[string]any)
	dimensionSchema := criterionProperties["dimension_key"].(map[string]any)
	assert.ElementsMatch(t, stringSliceToAny(criterionDimensionStrings()), dimensionSchema["enum"])

	assert.Equal(t, -0.7, result.Prediction.VerdictScore)
	assert.Equal(t, 84, result.Prediction.Confidence)
	assert.Equal(t, "1", result.Prediction.JudgeVersion)
	assert.Equal(t, usage, result.Usage)
	require.Len(t, result.Prediction.Criteria, 5)
	for i, dimension := range judgmentstore.CriterionDimensions {
		assert.Equal(t, dimension, result.Prediction.Criteria[i].Dimension)
	}
}

func stringSliceToAny(values []string) []any {
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}

func priorIDs(examples []priorExamplePayload) []string {
	out := make([]string, len(examples))
	for i, example := range examples {
		out[i] = example.TraceID
	}
	return out
}

func TestPredictOmitsEmptySignalsAndAllowsMissingUsage(t *testing.T) {
	invoker := &recordingInvoker{response: validResponse()}
	result, err := New(invoker).Predict(context.Background(), "key", Input{
		TraceID:     "trace-1",
		TraceInput:  "input",
		TraceOutput: nil,
	})
	require.NoError(t, err)
	assert.Nil(t, result.Usage)
	assert.NotContains(t, invoker.request.Messages[1].Content, `"signals"`)
	assert.NotContains(t, invoker.request.Messages[1].Content, `"prior_examples"`)
}

func TestPredictCompactsTargetSupportingAndPriorContent(t *testing.T) {
	targetInput := strings.Repeat("界", maxTraceInputRunes+100)
	targetOutput := strings.Repeat("🙂", maxTraceOutputRunes+100)
	priorInput := strings.Repeat("界", maxTraceInputRunes+100)
	priorOutput := strings.Repeat("🙂", maxTraceOutputRunes+100)
	supporting := strings.Repeat("é", maxSupportingTextRunes+100)
	invoker := &recordingInvoker{response: validResponse()}

	_, err := New(invoker).Predict(context.Background(), "key", Input{
		TraceID:      "target",
		TraceInput:   targetInput,
		TraceOutput:  targetOutput,
		NextUserText: supporting,
		PriorExamples: []PriorExample{{
			TraceID: "prior", Input: priorInput, Output: priorOutput,
			Verdict: judgmentstore.VerdictGood,
		}},
	})
	require.NoError(t, err)

	var payload userPayload
	require.NoError(t, json.Unmarshal([]byte(invoker.request.Messages[1].Content), &payload))
	compactTargetInput, ok := payload.Trace.Input.(string)
	require.True(t, ok)
	compactTargetOutput, ok := payload.Trace.Output.(string)
	require.True(t, ok)
	assert.Len(t, []rune(compactTargetInput), maxTraceInputRunes)
	assert.Len(t, []rune(compactTargetOutput), maxTraceOutputRunes)
	assert.Contains(t, compactTargetInput, truncationMarker)
	assert.Contains(t, compactTargetOutput, truncationMarker)
	assert.True(t, strings.HasPrefix(compactTargetInput, `"`))
	assert.True(t, strings.HasSuffix(compactTargetInput, `"`))
	require.NotNil(t, payload.Signals)
	assert.Len(t, []rune(payload.Signals.NextUserText), maxSupportingTextRunes)
	assert.Contains(t, payload.Signals.NextUserText, truncationMarker)
	require.Len(t, payload.PriorExamples, 1)
	compactInput, ok := payload.PriorExamples[0].Input.(string)
	require.True(t, ok)
	compactOutput, ok := payload.PriorExamples[0].Output.(string)
	require.True(t, ok)
	assert.Len(t, []rune(compactInput), maxTraceInputRunes)
	assert.Len(t, []rune(compactOutput), maxTraceOutputRunes)
	assert.Contains(t, compactInput, truncationMarker)
	assert.Contains(t, compactOutput, truncationMarker)
	assert.True(t, strings.HasPrefix(compactInput, `"`))
	assert.True(t, strings.HasSuffix(compactInput, `"`))
}

func TestPredictRejectsInvalidInputBeforeInvocation(t *testing.T) {
	tests := []struct {
		name  string
		input Input
		want  string
	}{
		{name: "trace id", input: Input{}, want: "trace id is required"},
		{name: "target input marshal", input: Input{TraceID: "t", TraceInput: func() {}}, want: "target trace input"},
		{name: "target output marshal", input: Input{TraceID: "t", TraceOutput: func() {}}, want: "target trace output"},
		{name: "prior marshal", input: Input{TraceID: "t", PriorExamples: []PriorExample{{TraceID: "p", Verdict: judgmentstore.VerdictGood, Input: math.NaN()}}}, want: "prior trace \"p\" input"},
		{name: "prior id", input: Input{TraceID: "t", PriorExamples: []PriorExample{{Verdict: judgmentstore.VerdictGood}}}, want: "prior trace id is required"},
		{name: "prior verdict", input: Input{TraceID: "t", PriorExamples: []PriorExample{{TraceID: "p", Verdict: "maybe"}}}, want: "invalid verdict"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoker := &recordingInvoker{response: validResponse()}
			_, err := New(invoker).Predict(context.Background(), "key", tt.input)
			require.ErrorContains(t, err, tt.want)
			assert.Zero(t, invoker.calls)
		})
	}

	invoker := &recordingInvoker{response: validResponse()}
	_, err := New(invoker).Predict(context.Background(), "", Input{TraceID: "t"})
	require.ErrorContains(t, err, "api key is required")
	assert.Zero(t, invoker.calls)

	_, err = New(nil).Predict(context.Background(), "key", Input{TraceID: "t"})
	require.ErrorContains(t, err, "invoker is required")
}

func TestPredictIgnoresNonThumbsFeedback(t *testing.T) {
	invoker := &recordingInvoker{response: validResponse()}
	_, err := New(invoker).Predict(context.Background(), "key", Input{
		TraceID:        "trace",
		ThumbsFeedback: "comment",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, invoker.calls)
	assert.NotContains(t, invoker.request.Messages[1].Content, `"signals"`)
}

func TestPredictValidatesPriorReasonsBeforeInvocation(t *testing.T) {
	tests := []struct {
		name    string
		verdict judgmentstore.Verdict
		reasons []judgmentstore.Reason
		want    string
	}{
		{
			name:    "invalid dimension",
			verdict: judgmentstore.VerdictGood,
			reasons: []judgmentstore.Reason{{Dimension: "speed", Value: 1}},
			want:    "invalid criterion dimension",
		},
		{
			name:    "duplicate",
			verdict: judgmentstore.VerdictBad,
			reasons: []judgmentstore.Reason{{Dimension: judgmentstore.DimensionTone, Value: -1}, {Dimension: judgmentstore.DimensionTone, Value: -0.5}},
			want:    "duplicate criterion dimension",
		},
		{
			name:    "out of range",
			verdict: judgmentstore.VerdictGood,
			reasons: []judgmentstore.Reason{{Dimension: judgmentstore.DimensionAccuracy, Value: 1.1}},
			want:    "outside [-1, 1]",
		},
		{
			name:    "non-finite",
			verdict: judgmentstore.VerdictGood,
			reasons: []judgmentstore.Reason{{Dimension: judgmentstore.DimensionAccuracy, Value: math.Inf(1)}},
			want:    "outside [-1, 1]",
		},
		{
			name:    "unknown with reasons",
			verdict: judgmentstore.VerdictUnknown,
			reasons: []judgmentstore.Reason{{Dimension: judgmentstore.DimensionAccuracy, Value: 0}},
			want:    "unknown verdict cannot have criteria",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoker := &recordingInvoker{response: validResponse()}
			_, err := New(invoker).Predict(context.Background(), "key", Input{
				TraceID: "target",
				PriorExamples: []PriorExample{{
					TraceID: "prior", Verdict: tt.verdict, Reasons: tt.reasons,
				}},
			})
			require.ErrorContains(t, err, tt.want)
			assert.Zero(t, invoker.calls)
		})
	}
}

func TestValidatePriorExamplesPreservesSelectedOrder(t *testing.T) {
	selected, err := validatePriorExamples("target", []PriorExample{
		{TraceID: "g1", Verdict: judgmentstore.VerdictGood},
		{TraceID: "b1", Verdict: judgmentstore.VerdictBad},
		{TraceID: "g2", Verdict: judgmentstore.VerdictGood},
	})
	require.NoError(t, err)
	require.Len(t, selected, 3)
	assert.Equal(t, []string{"g1", "b1", "g2"}, []string{selected[0].TraceID, selected[1].TraceID, selected[2].TraceID})
}

func TestValidatePriorExamplesRejectsInvalidSets(t *testing.T) {
	tests := []struct {
		name     string
		examples []PriorExample
		want     string
	}{
		{name: "too many total", examples: []PriorExample{{}, {}, {}, {}, {}, {}}, want: "at most 5"},
		{name: "target included", examples: []PriorExample{{TraceID: "target", Verdict: judgmentstore.VerdictGood}}, want: "is the target trace"},
		{name: "duplicate", examples: []PriorExample{{TraceID: "same", Verdict: judgmentstore.VerdictGood}, {TraceID: "same", Verdict: judgmentstore.VerdictBad}}, want: "duplicate prior trace"},
		{name: "too many good", examples: []PriorExample{{TraceID: "g1", Verdict: judgmentstore.VerdictGood}, {TraceID: "g2", Verdict: judgmentstore.VerdictGood}, {TraceID: "g3", Verdict: judgmentstore.VerdictGood}}, want: `too many "good"`},
		{name: "too many bad", examples: []PriorExample{{TraceID: "b1", Verdict: judgmentstore.VerdictBad}, {TraceID: "b2", Verdict: judgmentstore.VerdictBad}, {TraceID: "b3", Verdict: judgmentstore.VerdictBad}}, want: `too many "bad"`},
		{name: "too many unknown", examples: []PriorExample{{TraceID: "u1", Verdict: judgmentstore.VerdictUnknown}, {TraceID: "u2", Verdict: judgmentstore.VerdictUnknown}}, want: `too many "unknown"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validatePriorExamples("target", tt.examples)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestPredictTruncatesExplanationByUnicodeRunes(t *testing.T) {
	long := strings.Repeat("🙂", maxExplanationRunes+20)
	content := strings.Replace(validResponse().Choices[0].Message.Content, "The response misses a required constraint.", long, 1)
	invoker := &recordingInvoker{response: responseWithContent(content)}

	result, err := New(invoker).Predict(context.Background(), "key", Input{TraceID: "trace"})
	require.NoError(t, err)
	assert.Len(t, []rune(result.Prediction.Explanation), maxExplanationRunes)
	assert.Equal(t, strings.Repeat("🙂", maxExplanationRunes), result.Prediction.Explanation)
}

func TestPredictRejectsInvalidModelOutput(t *testing.T) {
	valid := validResponse().Choices[0].Message.Content
	tests := []struct {
		name     string
		response *aigateway.ChatCompletionResponse
		want     string
	}{
		{name: "nil response", response: nil, want: "missing response"},
		{name: "no choices", response: &aigateway.ChatCompletionResponse{}, want: "exactly one choice"},
		{name: "multiple choices", response: &aigateway.ChatCompletionResponse{Choices: []aigateway.ChatCompletionChoice{{}, {}}}, want: "exactly one choice"},
		{name: "wrong role", response: &aigateway.ChatCompletionResponse{Choices: []aigateway.ChatCompletionChoice{{Message: aigateway.ChatMessage{Role: "user", Content: valid}}}}, want: "want assistant"},
		{name: "missing finish reason", response: responseWithFinishReason(valid, ""), want: `finish_reason is "", want "stop"`},
		{name: "length finish reason", response: responseWithFinishReason(valid, "length"), want: `finish_reason is "length", want "stop"`},
		{name: "content filter finish reason", response: responseWithFinishReason(valid, "content_filter"), want: `finish_reason is "content_filter", want "stop"`},
		{name: "tool calls finish reason", response: responseWithFinishReason(valid, "tool_calls"), want: `finish_reason is "tool_calls", want "stop"`},
		{name: "function call finish reason", response: responseWithFinishReason(valid, "function_call"), want: `finish_reason is "function_call", want "stop"`},
		{name: "unknown finish reason", response: responseWithFinishReason(valid, "other"), want: `finish_reason is "other", want "stop"`},
		{name: "empty", response: responseWithContent(" "), want: "content is empty"},
		{name: "malformed", response: responseWithContent("{"), want: "decode"},
		{name: "trailing", response: responseWithContent(valid + `{}`), want: "multiple JSON values"},
		{name: "unknown field", response: responseWithContent(strings.Replace(valid, `"confidence": 84,`, `"confidence": 84,"extra":true,`, 1)), want: "unknown field"},
		{name: "missing", response: responseWithContent(strings.Replace(valid, `"confidence": 84,`, "", 1)), want: "missing required field"},
		{name: "fractional confidence", response: responseWithContent(strings.Replace(valid, `"confidence": 84`, `"confidence": 84.5`, 1)), want: "cannot unmarshal number"},
		{name: "low verdict", response: responseWithContent(strings.Replace(valid, `"verdict_score": -0.7`, `"verdict_score": -1.1`, 1)), want: "outside [-1, 1]"},
		{name: "high confidence", response: responseWithContent(strings.Replace(valid, `"confidence": 84`, `"confidence": 101`, 1)), want: "outside [0, 100]"},
		{name: "too few criteria", response: responseWithContent(strings.Replace(valid, `{"dimension_key":"tone","dimension_value":-0.1},`, "", 1)), want: "expected 5 criteria"},
		{name: "duplicate criterion", response: responseWithContent(strings.Replace(valid, `"dimension_key":"tone"`, `"dimension_key":"accuracy"`, 1)), want: "duplicate criterion"},
		{name: "unknown criterion", response: responseWithContent(strings.Replace(valid, `"dimension_key":"tone"`, `"dimension_key":"speed"`, 1)), want: "invalid criterion"},
		{name: "criterion missing field", response: responseWithContent(strings.Replace(valid, `,"dimension_value":-0.1`, "", 1)), want: "missing a required field"},
		{name: "criterion out of range", response: responseWithContent(strings.Replace(valid, `"dimension_value":-0.1`, `"dimension_value":-2`, 1)), want: "outside [-1, 1]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoker := &recordingInvoker{response: tt.response}
			_, err := New(invoker).Predict(context.Background(), "key", Input{TraceID: "trace"})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidOutput)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestPredictPreservesInvocationErrors(t *testing.T) {
	invocationErr := &aigateway.InvocationError{StatusCode: 429, Code: "rate_limit", Message: "slow down"}
	invoker := &recordingInvoker{err: invocationErr}
	_, err := New(invoker).Predict(context.Background(), "key", Input{TraceID: "trace"})
	require.Error(t, err)
	var got *aigateway.InvocationError
	require.True(t, errors.As(err, &got))
	assert.Same(t, invocationErr, got)
}

type contextInvoker struct{}

func (contextInvoker) ChatCompletion(ctx context.Context, _ string, _ aigateway.ChatCompletionRequest) (*aigateway.ChatCompletionResponse, error) {
	return nil, ctx.Err()
}

func TestPredictPreservesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(contextInvoker{}).Predict(ctx, "key", Input{TraceID: "trace"})
	require.ErrorIs(t, err, context.Canceled)
}
