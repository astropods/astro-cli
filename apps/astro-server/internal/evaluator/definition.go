package evaluator

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxLabelRunes  = 50
	maxPromptRunes = 8_000
	minEnumOptions = 2
	maxEnumOptions = 20
	maxOptionRunes = 50
	minStringLimit = 1
	maxStringLimit = 4_000
	// DefaultStringMaxLength applies when a string output omits max_length.
	DefaultStringMaxLength = 1_000
)

var (
	// ErrInvalidDefinition marks an evaluator the server refuses to execute.
	ErrInvalidDefinition = errors.New("invalid evaluator definition")

	keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

type Type string

const TypeLLM Type = "llm"

type OutputType string

const (
	OutputBoolean OutputType = "boolean"
	OutputEnum    OutputType = "enum"
	OutputNumber  OutputType = "number"
	OutputString  OutputType = "string"
)

type Evaluator struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Type        Type   `json:"type"`
	Config      Config `json:"config"`
	Prompt      string `json:"prompt"`
	Output      Output `json:"output"`
}

type Config struct {
	Context ContextConfig `json:"context"`
}

type ContextConfig struct {
	PreviousTurns   bool     `json:"previous_turns"`
	NextUserMessage bool     `json:"next_user_message"`
	UserFeedback    bool     `json:"user_feedback"`
	Steps           bool     `json:"steps"`
	StepTypes       []string `json:"step_types,omitempty"`
}

type Output struct {
	Type      OutputType `json:"type"`
	Options   []string   `json:"options,omitempty"`
	Minimum   *float64   `json:"minimum,omitempty"`
	Maximum   *float64   `json:"maximum,omitempty"`
	MaxLength *int       `json:"max_length,omitempty"`
}

// StringLimit resolves the effective max_length for a string output.
func (o Output) StringLimit() int {
	if o.MaxLength == nil {
		return DefaultStringMaxLength
	}
	return *o.MaxLength
}

// Validate reports whether the evaluator is executable under evaluation/v1.
func Validate(evaluator Evaluator) error {
	if !keyPattern.MatchString(evaluator.Key) {
		return invalidDefinition("key %q must match [a-z][a-z0-9_]{0,63}", evaluator.Key)
	}
	labelRunes := utf8.RuneCountInString(strings.TrimSpace(evaluator.Label))
	if labelRunes < 1 || labelRunes > maxLabelRunes {
		return invalidDefinition("label must contain 1 to %d characters, got %d", maxLabelRunes, labelRunes)
	}
	if evaluator.Type != TypeLLM {
		return invalidDefinition("type %q is not supported", evaluator.Type)
	}
	promptRunes := utf8.RuneCountInString(strings.TrimSpace(evaluator.Prompt))
	if promptRunes < 1 || promptRunes > maxPromptRunes {
		return invalidDefinition("prompt must contain 1 to %d characters, got %d", maxPromptRunes, promptRunes)
	}
	if err := validateContext(evaluator.Config.Context); err != nil {
		return err
	}
	return validateOutput(evaluator.Output)
}

func validateContext(config ContextConfig) error {
	if len(config.StepTypes) > 0 && !config.Steps {
		return invalidDefinition("context step_types requires steps")
	}
	for index, stepType := range config.StepTypes {
		if strings.TrimSpace(stepType) == "" {
			return invalidDefinition("context step_types entry %d must not be blank", index)
		}
	}
	return nil
}

func validateOutput(output Output) error {
	switch output.Type {
	case OutputBoolean:
		return rejectFields(output, "")
	case OutputEnum:
		if err := rejectFields(output, "options"); err != nil {
			return err
		}
		return validateEnumOptions(output.Options)
	case OutputNumber:
		if err := rejectFields(output, "minimum", "maximum"); err != nil {
			return err
		}
		return validateNumberBounds(output.Minimum, output.Maximum)
	case OutputString:
		if err := rejectFields(output, "max_length"); err != nil {
			return err
		}
		return validateStringLimit(output.MaxLength)
	default:
		return invalidDefinition("output type %q is not supported", output.Type)
	}
}

func validateEnumOptions(options []string) error {
	if len(options) < minEnumOptions || len(options) > maxEnumOptions {
		return invalidDefinition("enum output requires %d to %d options, got %d", minEnumOptions, maxEnumOptions, len(options))
	}
	seen := make(map[string]bool, len(options))
	for index, option := range options {
		optionRunes := utf8.RuneCountInString(option)
		if optionRunes < 1 || optionRunes > maxOptionRunes {
			return invalidDefinition("enum option %d must contain 1 to %d characters, got %d", index, maxOptionRunes, optionRunes)
		}
		if seen[option] {
			return invalidDefinition("enum option %q is duplicated", option)
		}
		seen[option] = true
	}
	return nil
}

func validateNumberBounds(minimum, maximum *float64) error {
	if minimum != nil && !isFinite(*minimum) {
		return invalidDefinition("number output minimum %v is not finite", *minimum)
	}
	if maximum != nil && !isFinite(*maximum) {
		return invalidDefinition("number output maximum %v is not finite", *maximum)
	}
	if minimum != nil && maximum != nil && *minimum >= *maximum {
		return invalidDefinition("number output minimum %v must be less than maximum %v", *minimum, *maximum)
	}
	return nil
}

func validateStringLimit(maxLength *int) error {
	if maxLength == nil {
		return nil
	}
	if *maxLength < minStringLimit || *maxLength > maxStringLimit {
		return invalidDefinition("string output max_length must be %d to %d, got %d", minStringLimit, maxStringLimit, *maxLength)
	}
	return nil
}

// rejectFields fails when the output carries a field that its declared type does
// not define, which is how the parser's unknown-field rule reaches type-specific
// configuration.
func rejectFields(output Output, allowed ...string) error {
	permitted := make(map[string]bool, len(allowed))
	for _, field := range allowed {
		permitted[field] = true
	}
	set := []struct {
		name    string
		present bool
	}{
		{"options", output.Options != nil},
		{"minimum", output.Minimum != nil},
		{"maximum", output.Maximum != nil},
		{"max_length", output.MaxLength != nil},
	}
	for _, field := range set {
		if field.present && !permitted[field.name] {
			return invalidDefinition("output type %q does not accept %s", output.Type, field.name)
		}
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func invalidDefinition(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidDefinition, fmt.Sprintf(format, args...))
}
