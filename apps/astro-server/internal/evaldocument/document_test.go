package evaldocument

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
)

func TestParseAcceptsAPresetOnlyDocument(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - ref: preset/user-sentiment
`, nil)
	require.NoError(t, err)
	require.Len(t, result.Evaluators, 2)
	assert.Equal(t, "exposed_pii", result.Evaluators[0].Key)
	assert.Equal(t, "user_sentiment", result.Evaluators[1].Key)
	assert.True(t, strings.HasPrefix(result.EvaluationRef, "agent/"))
}

func TestParseAcceptsACustomEvaluator(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    description: Flags credentials in the output.
    type: llm
    prompt: Determine whether the agent output exposes credentials.
    output:
      type: boolean
`, nil)
	require.NoError(t, err)
	require.Len(t, result.Evaluators, 1)
	assert.Equal(t, "has_secrets", result.Evaluators[0].Key)
	assert.Equal(t, "Flags credentials in the output.", result.Evaluators[0].Description)
	assert.Equal(t, "Flags credentials in the output.", result.Document.Evaluators[0].Description)
	assert.Equal(t, evaluator.OutputBoolean, result.Evaluators[0].Output.Type)
}

func TestParseInlinesAPromptFile(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: response_quality
    label: Response quality
    type: llm
    prompt_file: evaluation/response-quality.md
    output:
      type: number
      minimum: 0
      maximum: 1
`, map[string]string{
		"evaluation/response-quality.md": "Assess the overall quality of the response.",
	})
	require.NoError(t, err)
	require.Len(t, result.Evaluators, 1)
	assert.Equal(t, "Assess the overall quality of the response.", result.Evaluators[0].Prompt)
	assert.Equal(t, "Assess the overall quality of the response.", result.Document.Evaluators[0].Prompt)
}

func TestParseAllowsStepsContextOnACustomEvaluator(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: redundant_call
    label: Redundant tool call
    type: llm
    config:
      context:
        steps: true
        step_types:
          - tool
    prompt: Determine whether a tool call was redundant.
    output:
      type: boolean
`, nil)
	require.NoError(t, err)
	require.Len(t, result.Evaluators, 1)
	assert.True(t, result.Evaluators[0].Config.Context.Steps)
	assert.Equal(t, []string{"tool"}, result.Evaluators[0].Config.Context.StepTypes)
}

func TestParseRejectsStepTypesWithoutSteps(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: redundant_call
    label: Redundant tool call
    type: llm
    config:
      context:
        step_types:
          - tool
    prompt: Determine whether a tool call was redundant.
    output:
      type: boolean
`, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsWrongSchema(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v2
evaluators:
  - ref: preset/exposed-pii
`, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsTooFewEvaluators(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators: []
`, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsTooManyEvaluators(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("schema: evaluation/v1\nevaluators:\n")
	for i := 0; i < 11; i++ {
		sb.WriteString("  - ref: preset/exposed-pii\n")
	}
	_, err := Parse(sb.String(), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsDuplicateKeysAfterPresetResolution(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - key: exposed_pii
    label: Duplicate
    type: llm
    prompt: Some other prompt for this evaluator.
    output:
      type: boolean
`, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsAPresetRefWithExtraFields(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
    label: Overridden label
`, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsAnUnknownPresetRef(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/does-not-exist
`, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsAnUnknownTopLevelField(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
extra: not allowed
evaluators:
  - ref: preset/exposed-pii
`, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsAnUnknownEvaluatorField(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt: Determine whether the agent output exposes credentials.
    output:
      type: boolean
    extra: not allowed
`, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsBothPromptAndPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt: Inline prompt.
    prompt_file: evaluation/has-secrets.md
    output:
      type: boolean
`, map[string]string{"evaluation/has-secrets.md": "File prompt."})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsAMissingPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt_file: evaluation/has-secrets.md
    output:
      type: boolean
`, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsAnUnreferencedPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
`, map[string]string{"evaluation/unused.md": "Not referenced by anything."})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsANonMarkdownPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt_file: evaluation/has-secrets.txt
    output:
      type: boolean
`, map[string]string{"evaluation/has-secrets.txt": "File prompt."})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsAPathTraversalPromptFile(t *testing.T) {
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt_file: ../outside/has-secrets.md
    output:
      type: boolean
`, map[string]string{"../outside/has-secrets.md": "File prompt."})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestParseRejectsDocumentsOverTheSizeCap(t *testing.T) {
	huge := strings.Repeat("a", maxTotalBytes+1)
	_, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt_file: evaluation/has-secrets.md
    output:
      type: boolean
`, map[string]string{"evaluation/has-secrets.md": huge})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDocument))
}

func TestEvaluationRefIsStableAcrossFormattingChanges(t *testing.T) {
	a, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - ref: preset/user-sentiment
`, nil)
	require.NoError(t, err)

	b, err := Parse("schema: evaluation/v1\n# a comment\nevaluators:\n  - ref: preset/exposed-pii   # trailing comment\n  - ref: preset/user-sentiment\n", nil)
	require.NoError(t, err)

	assert.Equal(t, a.EvaluationRef, b.EvaluationRef)
}

func TestEvaluationRefChangesWithEvaluatorOrder(t *testing.T) {
	a, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - ref: preset/user-sentiment
`, nil)
	require.NoError(t, err)

	b, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/user-sentiment
  - ref: preset/exposed-pii
`, nil)
	require.NoError(t, err)

	assert.NotEqual(t, a.EvaluationRef, b.EvaluationRef)
}

func TestEvaluationRefDoesNotEmbedPresetContent(t *testing.T) {
	result, err := Parse(`
schema: evaluation/v1
evaluators:
  - ref: preset/exposed-pii
  - ref: preset/user-sentiment
`, nil)
	require.NoError(t, err)
	require.Len(t, result.Document.Evaluators, 2)
	assert.Equal(t, "preset/exposed-pii", result.Document.Evaluators[0].Ref)
	assert.Empty(t, result.Document.Evaluators[0].Key)
	assert.Nil(t, result.Document.Evaluators[0].Config)
	assert.Nil(t, result.Document.Evaluators[0].Output)
}

func TestEvaluationRefChangesWithCustomContent(t *testing.T) {
	a, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt: Determine whether the agent output exposes credentials.
    output:
      type: boolean
`, nil)
	require.NoError(t, err)

	b, err := Parse(`
schema: evaluation/v1
evaluators:
  - key: has_secrets
    label: Contains secrets
    type: llm
    prompt: Determine whether the agent output exposes an API key.
    output:
      type: boolean
`, nil)
	require.NoError(t, err)

	assert.NotEqual(t, a.EvaluationRef, b.EvaluationRef)
}
