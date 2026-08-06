package evaljudge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/judgmentstore"
)

const (
	// EvalDatasetJudgeModel is the model used for dataset-admission predictions.
	EvalDatasetJudgeModel = "claude-sonnet-4-6"
	// EvalDatasetJudgeVersion is incremented whenever judge behavior changes.
	EvalDatasetJudgeVersion = "1"
)

var ErrInvalidOutput = errors.New("invalid eval judge output")

// Invoker is the production boundary between judge behavior and Bifrost's
// OpenAI-compatible model transport. *aigateway.InvocationClient implements it.
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

// Input contains the product-owned context for one dataset-admission
// prediction.
// TODO: Wire the prediction endpoint to load these values and the judge API key.
type Input struct {
	TraceID        string
	TraceInput     any
	TraceOutput    any
	PreviousTurns  []SessionTurn
	NextUserText   string
	ThumbsFeedback string
}

// SessionTurn is one completed trace from before the prediction target.
type SessionTurn struct {
	Input  any
	Output any
}

type Result struct {
	Prediction judgmentstore.Prediction
	Usage      *aigateway.ChatCompletionUsage
}

// Predict builds one bounded, structured prompt and validates the model's
// response into the storage model owned by judgmentstore.
func (s *Service) Predict(ctx context.Context, apiKey string, input Input) (Result, error) {
	if s == nil || s.invoker == nil {
		return Result{}, fmt.Errorf("eval judge: invoker is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return Result{}, fmt.Errorf("eval judge: api key is required")
	}

	userMessage, err := buildUserMessage(input)
	if err != nil {
		return Result{}, err
	}

	response, err := s.invoker.ChatCompletion(ctx, apiKey, aigateway.ChatCompletionRequest{
		Model: EvalDatasetJudgeModel,
		Messages: []aigateway.ChatMessage{
			{Role: "system", Content: systemInstruction},
			{Role: "user", Content: userMessage},
		},
		ResponseFormat: responseFormat(),
	})
	if err != nil {
		return Result{}, fmt.Errorf("eval judge invoke: %w", err)
	}

	prediction, err := parsePrediction(response)
	if err != nil {
		return Result{}, err
	}
	return Result{Prediction: prediction, Usage: response.Usage}, nil
}
