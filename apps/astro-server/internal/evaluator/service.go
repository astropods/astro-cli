package evaluator

import (
	"context"
	"fmt"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
)

// Model is the model Astro uses for every LLM evaluator. Builders cannot
// configure it.
const Model = "claude-sonnet-4-6"

// Invoker is the boundary between evaluator behavior and Bifrost's
// OpenAI-compatible model transport.
type Invoker interface {
	ChatCompletion(context.Context, string, aigateway.ChatCompletionRequest) (*aigateway.ChatCompletionResponse, error)
}

var _ Invoker = (*aigateway.InvocationClient)(nil)

type Service struct {
	invoker Invoker
}

func New(invoker Invoker) *Service {
	return &Service{invoker: invoker}
}

// Input is the trace context available to an evaluator. Fields beyond the target
// trace reach the model only when the evaluator's context configuration enables
// them.
type Input struct {
	TraceID         string
	TraceInput      any
	TraceOutput     any
	PreviousTurns   []SessionTurn
	NextUserMessage string
	UserFeedback    string
	Steps           []Step
}

// SessionTurn is one completed trace from before the target trace.
type SessionTurn struct {
	Input  any
	Output any
}

// Step is one observation the agent produced while handling the target trace: a
// tool call, a retrieval, a nested model call, or a sub-agent invocation. Type
// carries the observation type verbatim so a prompt can narrow to the kinds it
// cares about. Error is set when the step failed, so an evaluator can tell a
// missing result from an empty one.
type Step struct {
	Name   string
	Type   string
	Input  any
	Output any
	Error  string
}

// Result is one evaluator's validated output.
type Result struct {
	Value       any
	Confidence  float64
	Explanation string
	Usage       *aigateway.ChatCompletionUsage
}

// Evaluate runs one evaluator against one trace and validates the model's value
// against the evaluator's declared output.
func (s *Service) Evaluate(ctx context.Context, apiKey string, evaluator Evaluator, input Input) (Result, error) {
	if s == nil || s.invoker == nil {
		return Result{}, fmt.Errorf("evaluator: invoker is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return Result{}, fmt.Errorf("evaluator: api key is required")
	}
	if err := Validate(evaluator); err != nil {
		return Result{}, err
	}

	userMessage, err := buildUserMessage(evaluator, input)
	if err != nil {
		return Result{}, err
	}

	response, err := s.invoker.ChatCompletion(ctx, apiKey, aigateway.ChatCompletionRequest{
		Model: Model,
		Messages: []aigateway.ChatMessage{
			{Role: "system", Content: systemInstruction},
			{Role: "user", Content: userMessage},
		},
		ResponseFormat: responseFormat(evaluator.Output),
	})
	if err != nil {
		return Result{}, fmt.Errorf("evaluator invoke: %w", err)
	}
	return parseResult(response, evaluator.Output)
}
