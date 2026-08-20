package evaluator

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func booleanEvaluator() Evaluator {
	return Evaluator{
		Key:    "accuracy",
		Label:  "Accuracy",
		Type:   TypeLLM,
		Prompt: "Determine whether the agent output is factually correct.",
		Output: Output{Type: OutputBoolean},
	}
}

func enumEvaluator() Evaluator {
	evaluator := booleanEvaluator()
	evaluator.Key = "user_sentiment"
	evaluator.Label = "User sentiment"
	evaluator.Output = Output{
		Type:    OutputEnum,
		Options: []string{"positive", "neutral", "negative", "unclear"},
	}
	return evaluator
}

func numberEvaluator(minimum, maximum *float64) Evaluator {
	evaluator := booleanEvaluator()
	evaluator.Key = "response_quality"
	evaluator.Label = "Response quality"
	evaluator.Output = Output{Type: OutputNumber, Minimum: minimum, Maximum: maximum}
	return evaluator
}

func stringEvaluator(maxLength *int) Evaluator {
	evaluator := booleanEvaluator()
	evaluator.Key = "summary"
	evaluator.Label = "Summary"
	evaluator.Output = Output{Type: OutputString, MaxLength: maxLength}
	return evaluator
}

func float64Ptr(value float64) *float64 { return &value }

func intPtr(value int) *int { return &value }

func TestValidateAcceptsEachOutputType(t *testing.T) {
	cases := map[string]Evaluator{
		"boolean":            booleanEvaluator(),
		"enum":               enumEvaluator(),
		"number unbounded":   numberEvaluator(nil, nil),
		"number bounded":     numberEvaluator(float64Ptr(0), float64Ptr(1)),
		"number minimum":     numberEvaluator(float64Ptr(-5), nil),
		"string default":     stringEvaluator(nil),
		"string with limit":  stringEvaluator(intPtr(4_000)),
		"key at max length":  keyed(booleanEvaluator(), "a"+strings.Repeat("b", 63)),
		"label at max":       labeled(booleanEvaluator(), strings.Repeat("l", 50)),
		"prompt at max":      prompted(booleanEvaluator(), strings.Repeat("p", 8_000)),
		"enum at max option": enumWithOptions(strings.Repeat("o", 50), "other"),
	}

	for name, evaluator := range cases {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, Validate(evaluator))
		})
	}
}

func TestValidateRejectsInvalidDefinitions(t *testing.T) {
	tooManyOptions := make([]string, 0, 21)
	for i := 0; i < 21; i++ {
		tooManyOptions = append(tooManyOptions, string(rune('a'+i)))
	}

	cases := map[string]Evaluator{
		"empty key":               keyed(booleanEvaluator(), ""),
		"key starts with digit":   keyed(booleanEvaluator(), "1accuracy"),
		"key with uppercase":      keyed(booleanEvaluator(), "Accuracy"),
		"key with dash":           keyed(booleanEvaluator(), "user-sentiment"),
		"key too long":            keyed(booleanEvaluator(), "a"+strings.Repeat("b", 64)),
		"empty label":             labeled(booleanEvaluator(), ""),
		"label too long":          labeled(booleanEvaluator(), strings.Repeat("l", 51)),
		"unsupported type":        typed(booleanEvaluator(), "classifier"),
		"empty type":              typed(booleanEvaluator(), ""),
		"empty prompt":            prompted(booleanEvaluator(), ""),
		"whitespace only prompt":  prompted(booleanEvaluator(), "   \n\t "),
		"whitespace only label":   labeled(booleanEvaluator(), "  "),
		"prompt too long":         prompted(booleanEvaluator(), strings.Repeat("p", 8_001)),
		"unsupported output type": outputTyped(booleanEvaluator(), "code"),
		"enum with one option":    enumWithOptions("only"),
		"enum with no options":    enumWithOptions(),
		"enum too many options":   enumWithOptionSlice(tooManyOptions),
		"enum duplicate option":   enumWithOptions("same", "same"),
		"enum empty option":       enumWithOptions("", "other"),
		"enum option too long":    enumWithOptions(strings.Repeat("o", 51), "other"),
		"number inverted bounds":  numberEvaluator(float64Ptr(1), float64Ptr(0)),
		"number equal bounds":     numberEvaluator(float64Ptr(1), float64Ptr(1)),
		"number NaN minimum":      numberEvaluator(float64Ptr(math.NaN()), nil),
		"number infinite maximum": numberEvaluator(nil, float64Ptr(math.Inf(1))),
		"string limit zero":       stringEvaluator(intPtr(0)),
		"string limit negative":   stringEvaluator(intPtr(-1)),
		"string limit too large":  stringEvaluator(intPtr(4_001)),
		"step types without steps": withContext(booleanEvaluator(), ContextConfig{
			StepTypes: []string{"tool"},
		}),
		"blank step type": withContext(booleanEvaluator(), ContextConfig{
			Steps:     true,
			StepTypes: []string{"tool", "  "},
		}),
	}

	for name, evaluator := range cases {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, Validate(evaluator), ErrInvalidDefinition)
		})
	}
}

func TestValidateAcceptsStepTypesAlongsideSteps(t *testing.T) {
	evaluator := withContext(booleanEvaluator(), ContextConfig{
		Steps:     true,
		StepTypes: []string{"tool"},
	})

	require.NoError(t, Validate(evaluator))
}

func TestValidateRejectsFieldsForeignToOutputType(t *testing.T) {
	boolWithOptions := booleanEvaluator()
	boolWithOptions.Output.Options = []string{"yes", "no"}

	enumWithMinimum := enumEvaluator()
	enumWithMinimum.Output.Minimum = float64Ptr(0)

	numberWithMaxLength := numberEvaluator(nil, nil)
	numberWithMaxLength.Output.MaxLength = intPtr(100)

	stringWithMaximum := stringEvaluator(nil)
	stringWithMaximum.Output.Maximum = float64Ptr(1)

	cases := map[string]Evaluator{
		"boolean with options":   boolWithOptions,
		"enum with minimum":      enumWithMinimum,
		"number with max_length": numberWithMaxLength,
		"string with maximum":    stringWithMaximum,
	}

	for name, evaluator := range cases {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, Validate(evaluator), ErrInvalidDefinition)
		})
	}
}

func TestStringLimitDefaultsWhenMaxLengthOmitted(t *testing.T) {
	assert.Equal(t, DefaultStringMaxLength, Output{Type: OutputString}.StringLimit())
	assert.Equal(t, 25, Output{Type: OutputString, MaxLength: intPtr(25)}.StringLimit())
}

func keyed(evaluator Evaluator, key string) Evaluator {
	evaluator.Key = key
	return evaluator
}

func labeled(evaluator Evaluator, label string) Evaluator {
	evaluator.Label = label
	return evaluator
}

func typed(evaluator Evaluator, value Type) Evaluator {
	evaluator.Type = value
	return evaluator
}

func prompted(evaluator Evaluator, prompt string) Evaluator {
	evaluator.Prompt = prompt
	return evaluator
}

func outputTyped(evaluator Evaluator, value OutputType) Evaluator {
	evaluator.Output = Output{Type: value}
	return evaluator
}

func enumWithOptions(options ...string) Evaluator {
	return enumWithOptionSlice(options)
}

func enumWithOptionSlice(options []string) Evaluator {
	evaluator := enumEvaluator()
	evaluator.Output.Options = options
	return evaluator
}
