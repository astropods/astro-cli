package evaluator

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func responseWithContent(content string) *aigateway.ChatCompletionResponse {
	return &aigateway.ChatCompletionResponse{Choices: []aigateway.ChatCompletionChoice{{
		Index:        0,
		Message:      aigateway.ChatMessage{Role: "assistant", Content: content},
		FinishReason: "stop",
	}}}
}

func resultContent(value string) string {
	return fmt.Sprintf(`{"value":%s,"confidence":0.9,"explanation":"The evidence supports this."}`, value)
}

func TestParseResultAcceptsEachOutputType(t *testing.T) {
	cases := map[string]struct {
		output  Output
		content string
		want    any
	}{
		"boolean true":     {Output{Type: OutputBoolean}, resultContent(`true`), true},
		"boolean false":    {Output{Type: OutputBoolean}, resultContent(`false`), false},
		"enum option":      {enumEvaluator().Output, resultContent(`"negative"`), "negative"},
		"number in range":  {Output{Type: OutputNumber, Minimum: float64Ptr(0), Maximum: float64Ptr(1)}, resultContent(`0.25`), 0.25},
		"number at bound":  {Output{Type: OutputNumber, Minimum: float64Ptr(0), Maximum: float64Ptr(1)}, resultContent(`1`), 1.0},
		"number unbounded": {Output{Type: OutputNumber}, resultContent(`-42.5`), -42.5},
		"string default":   {Output{Type: OutputString}, resultContent(`"a summary"`), "a summary"},
		"string empty":     {Output{Type: OutputString}, resultContent(`""`), ""},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := parseResult(responseWithContent(testCase.content), testCase.output)
			require.NoError(t, err)
			assert.Equal(t, testCase.want, result.Value)
			assert.InDelta(t, 0.9, result.Confidence, 1e-9)
			assert.Equal(t, "The evidence supports this.", result.Explanation)
		})
	}
}

func TestParseResultRejectsValuesFailingTheDeclaredOutput(t *testing.T) {
	bounded := Output{Type: OutputNumber, Minimum: float64Ptr(0), Maximum: float64Ptr(1)}

	cases := map[string]struct {
		output  Output
		content string
	}{
		"boolean given string":    {Output{Type: OutputBoolean}, resultContent(`"true"`)},
		"boolean given null":      {Output{Type: OutputBoolean}, resultContent(`null`)},
		"enum outside options":    {enumEvaluator().Output, resultContent(`"furious"`)},
		"enum given number":       {enumEvaluator().Output, resultContent(`1`)},
		"number below minimum":    {bounded, resultContent(`-0.1`)},
		"number above maximum":    {bounded, resultContent(`1.5`)},
		"number given string":     {Output{Type: OutputNumber}, resultContent(`"0.5"`)},
		"string given boolean":    {Output{Type: OutputString}, resultContent(`true`)},
		"string over max_length":  {Output{Type: OutputString, MaxLength: intPtr(5)}, resultContent(`"far too long"`)},
		"unsupported output type": {Output{Type: "code"}, resultContent(`true`)},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseResult(responseWithContent(testCase.content), testCase.output)
			require.ErrorIs(t, err, ErrInvalidOutput)
		})
	}
}

func TestValidateValueRejectsNullForEveryOutputType(t *testing.T) {
	outputs := map[string]Output{
		"boolean": {Type: OutputBoolean},
		"enum":    enumEvaluator().Output,
		"number":  {Type: OutputNumber},
		"string":  {Type: OutputString},
	}

	for name, output := range outputs {
		t.Run(name, func(t *testing.T) {
			for _, raw := range []string{`null`, ` null `} {
				_, err := ValidateValue(output, json.RawMessage(raw))
				require.ErrorIs(t, err, ErrInvalidOutput)
			}
		})
	}
}

func TestParseResultAppliesDefaultStringLimit(t *testing.T) {
	content := resultContent(fmt.Sprintf("%q", strings.Repeat("x", DefaultStringMaxLength+1)))

	_, err := parseResult(responseWithContent(content), Output{Type: OutputString})
	require.ErrorIs(t, err, ErrInvalidOutput)
}

func TestParseResultRejectsInvalidMetadata(t *testing.T) {
	cases := map[string]string{
		"confidence above one":  `{"value":true,"confidence":1.4,"explanation":"ok"}`,
		"confidence below zero": `{"value":true,"confidence":-0.1,"explanation":"ok"}`,
		"confidence as string":  `{"value":true,"confidence":"0.9","explanation":"ok"}`,
		"explanation empty":     `{"value":true,"confidence":0.9,"explanation":""}`,
		"explanation blank":     `{"value":true,"confidence":0.9,"explanation":"   "}`,
		"missing value":         `{"confidence":0.9,"explanation":"ok"}`,
		"missing confidence":    `{"value":true,"explanation":"ok"}`,
		"missing explanation":   `{"value":true,"confidence":0.9}`,
		"unknown field":         `{"value":true,"confidence":0.9,"explanation":"ok","verdict":"good"}`,
		"multiple JSON values":  `{"value":true,"confidence":0.9,"explanation":"ok"} {"value":false}`,
		"not JSON":              `sure, the answer is true`,
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseResult(responseWithContent(content), Output{Type: OutputBoolean})
			require.ErrorIs(t, err, ErrInvalidOutput)
		})
	}
}

func TestParseResultTruncatesLongExplanation(t *testing.T) {
	explanation := strings.Repeat("e", maxExplanationRunes+50)
	content := fmt.Sprintf(`{"value":true,"confidence":0.5,"explanation":%q}`, explanation)

	result, err := parseResult(responseWithContent(content), Output{Type: OutputBoolean})
	require.NoError(t, err)
	assert.Len(t, []rune(result.Explanation), maxExplanationRunes)
	assert.True(t, strings.HasSuffix(result.Explanation, "..."))
}

func TestParseResultRejectsUnusableEnvelope(t *testing.T) {
	valid := resultContent(`true`)

	noChoices := responseWithContent(valid)
	noChoices.Choices = nil

	twoChoices := responseWithContent(valid)
	twoChoices.Choices = append(twoChoices.Choices, twoChoices.Choices[0])

	wrongRole := responseWithContent(valid)
	wrongRole.Choices[0].Message.Role = "user"

	truncated := responseWithContent(valid)
	truncated.Choices[0].FinishReason = "length"

	cases := map[string]*aigateway.ChatCompletionResponse{
		"nil response":  nil,
		"no choices":    noChoices,
		"two choices":   twoChoices,
		"wrong role":    wrongRole,
		"not stopped":   truncated,
		"empty content": responseWithContent("   "),
	}

	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseResult(response, Output{Type: OutputBoolean})
			require.ErrorIs(t, err, ErrInvalidOutput)
		})
	}
}

func TestParseResultReturnsUsage(t *testing.T) {
	response := responseWithContent(resultContent(`true`))
	response.Usage = &aigateway.ChatCompletionUsage{PromptTokens: 40, CompletionTokens: 8, TotalTokens: 48}

	result, err := parseResult(response, Output{Type: OutputBoolean})
	require.NoError(t, err)
	require.NotNil(t, result.Usage)
	assert.Equal(t, 48, result.Usage.TotalTokens)
}
