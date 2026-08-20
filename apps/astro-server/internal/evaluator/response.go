package evaluator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
)

const maxExplanationRunes = 240

// ErrInvalidOutput marks a model response the server refuses to store.
var ErrInvalidOutput = errors.New("invalid evaluator output")

type modelResult struct {
	Value       *json.RawMessage `json:"value"`
	Confidence  *float64         `json:"confidence"`
	Explanation *string          `json:"explanation"`
}

func parseResult(response *aigateway.ChatCompletionResponse, output Output) (Result, error) {
	content, err := assistantContent(response)
	if err != nil {
		return Result{}, err
	}

	model, err := decodeResult(content)
	if err != nil {
		return Result{}, err
	}
	if model.Value == nil || model.Confidence == nil || model.Explanation == nil {
		return Result{}, invalidOutput("missing required field")
	}

	value, err := validateValue(output, *model.Value)
	if err != nil {
		return Result{}, err
	}
	confidence := *model.Confidence
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		return Result{}, invalidOutput("confidence %v is outside [0, 1]", confidence)
	}
	explanation := strings.TrimSpace(*model.Explanation)
	if explanation == "" {
		return Result{}, invalidOutput("explanation is empty")
	}

	return Result{
		Value:       value,
		Confidence:  confidence,
		Explanation: truncatePrefixRunes(explanation, maxExplanationRunes),
		Usage:       response.Usage,
	}, nil
}

func assistantContent(response *aigateway.ChatCompletionResponse) (string, error) {
	if response == nil {
		return "", invalidOutput("missing response")
	}
	if len(response.Choices) != 1 {
		return "", invalidOutput("expected exactly one choice, got %d", len(response.Choices))
	}
	choice := response.Choices[0]
	if choice.Message.Role != "assistant" {
		return "", invalidOutput("choice role is %q, want assistant", choice.Message.Role)
	}
	if choice.FinishReason != "stop" {
		return "", invalidOutput("choice finish_reason is %q, want %q", choice.FinishReason, "stop")
	}
	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		return "", invalidOutput("choice content is empty")
	}
	return content, nil
}

func decodeResult(content string) (modelResult, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(content))
	decoder.DisallowUnknownFields()
	var model modelResult
	if err := decoder.Decode(&model); err != nil {
		return modelResult{}, invalidOutput("decode: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return modelResult{}, invalidOutput("multiple JSON values")
		}
		return modelResult{}, invalidOutput("trailing content: %v", err)
	}
	return model, nil
}

func validateValue(output Output, raw json.RawMessage) (any, error) {
	switch output.Type {
	case OutputBoolean:
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, invalidOutput("value is not a boolean: %v", err)
		}
		return value, nil
	case OutputEnum:
		value, err := decodeString(raw)
		if err != nil {
			return nil, err
		}
		for _, option := range output.Options {
			if value == option {
				return value, nil
			}
		}
		return nil, invalidOutput("value %q is not a configured enum option", value)
	case OutputNumber:
		var value float64
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, invalidOutput("value is not a number: %v", err)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, invalidOutput("value %v is not finite", value)
		}
		if output.Minimum != nil && value < *output.Minimum {
			return nil, invalidOutput("value %v is below minimum %v", value, *output.Minimum)
		}
		if output.Maximum != nil && value > *output.Maximum {
			return nil, invalidOutput("value %v is above maximum %v", value, *output.Maximum)
		}
		return value, nil
	case OutputString:
		value, err := decodeString(raw)
		if err != nil {
			return nil, err
		}
		limit := output.StringLimit()
		if length := utf8.RuneCountInString(value); length > limit {
			return nil, invalidOutput("value length %d exceeds max_length %d", length, limit)
		}
		return value, nil
	default:
		return nil, invalidOutput("output type %q is not supported", output.Type)
	}
}

func decodeString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", invalidOutput("value is not a string: %v", err)
	}
	return value, nil
}

func invalidOutput(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidOutput, fmt.Sprintf(format, args...))
}
