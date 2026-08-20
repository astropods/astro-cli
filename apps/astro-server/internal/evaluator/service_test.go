package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
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

func TestEvaluateBuildsStructuredRequestAndReturnsValidatedResult(t *testing.T) {
	response := responseWithContent(`{"value":"negative","confidence":0.88,"explanation":"The next user message expresses dissatisfaction."}`)
	response.Usage = &aigateway.ChatCompletionUsage{PromptTokens: 120, CompletionTokens: 24, TotalTokens: 144}
	invoker := &recordingInvoker{response: response}

	evaluator := withContext(enumEvaluator(), ContextConfig{NextUserMessage: true})
	result, err := New(invoker).Evaluate(context.Background(), "judge-key", evaluator, fullContextInput())
	require.NoError(t, err)

	assert.Equal(t, "negative", result.Value)
	assert.InDelta(t, 0.88, result.Confidence, 1e-9)
	assert.Equal(t, "The next user message expresses dissatisfaction.", result.Explanation)
	require.NotNil(t, result.Usage)
	assert.Equal(t, 144, result.Usage.TotalTokens)

	assert.Equal(t, 1, invoker.calls)
	assert.Equal(t, "judge-key", invoker.apiKey)
	assert.Equal(t, Model, invoker.request.Model)
	require.Len(t, invoker.request.Messages, 2)
	assert.Equal(t, "system", invoker.request.Messages[0].Role)
	assert.Equal(t, systemInstruction, invoker.request.Messages[0].Content)
	assert.Equal(t, "user", invoker.request.Messages[1].Role)
	assert.Equal(t, responseFormat(evaluator.Output), invoker.request.ResponseFormat)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(invoker.request.Messages[1].Content), &payload))
	assert.Contains(t, payload, "next_user_message")
	assert.NotContains(t, payload, "user_feedback")
}

func TestEvaluateSendsOutputSchemaMatchingTheEvaluator(t *testing.T) {
	invoker := &recordingInvoker{response: responseWithContent(resultContent(`0.5`))}

	evaluator := numberEvaluator(float64Ptr(0), float64Ptr(1))
	_, err := New(invoker).Evaluate(context.Background(), "key", evaluator, fullContextInput())
	require.NoError(t, err)

	assert.Equal(t, responseFormat(evaluator.Output), invoker.request.ResponseFormat)
}

func TestEvaluateRejectsInvalidDefinitionBeforeInvoking(t *testing.T) {
	invoker := &recordingInvoker{response: responseWithContent(resultContent(`true`))}

	_, err := New(invoker).Evaluate(context.Background(), "key", keyed(booleanEvaluator(), "Bad Key"), fullContextInput())
	require.ErrorIs(t, err, ErrInvalidDefinition)
	assert.Zero(t, invoker.calls)
}

func TestEvaluateRejectsMissingTraceIDBeforeInvoking(t *testing.T) {
	invoker := &recordingInvoker{response: responseWithContent(resultContent(`true`))}

	_, err := New(invoker).Evaluate(context.Background(), "key", booleanEvaluator(), Input{})
	require.Error(t, err)
	assert.Zero(t, invoker.calls)
}

func TestEvaluateRequiresAPIKey(t *testing.T) {
	invoker := &recordingInvoker{response: responseWithContent(resultContent(`true`))}

	_, err := New(invoker).Evaluate(context.Background(), "  ", booleanEvaluator(), fullContextInput())
	require.Error(t, err)
	assert.Zero(t, invoker.calls)
}

func TestEvaluateRequiresInvoker(t *testing.T) {
	_, err := New(nil).Evaluate(context.Background(), "key", booleanEvaluator(), fullContextInput())
	require.Error(t, err)
}

func TestEvaluatePropagatesInvocationError(t *testing.T) {
	invocationErr := &aigateway.InvocationError{StatusCode: 429, Message: "slow down"}
	invoker := &recordingInvoker{err: invocationErr}

	_, err := New(invoker).Evaluate(context.Background(), "key", booleanEvaluator(), fullContextInput())
	require.Error(t, err)

	var target *aigateway.InvocationError
	require.True(t, errors.As(err, &target))
	assert.Equal(t, 429, target.StatusCode)
}

func TestEvaluateRejectsResponseFailingTheDeclaredOutput(t *testing.T) {
	invoker := &recordingInvoker{response: responseWithContent(resultContent(`"maybe"`))}

	_, err := New(invoker).Evaluate(context.Background(), "key", enumEvaluator(), fullContextInput())
	require.ErrorIs(t, err, ErrInvalidOutput)
}
