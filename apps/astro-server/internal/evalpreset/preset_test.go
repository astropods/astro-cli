package evalpreset

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEveryPresetIsExecutable(t *testing.T) {
	for _, ref := range EvaluatorRefs() {
		t.Run(ref, func(t *testing.T) {
			preset, err := Lookup(ref)
			require.NoError(t, err)
			require.NoError(t, evaluator.Validate(preset))
		})
	}
}

func TestPresetKeysAreUnique(t *testing.T) {
	seen := make(map[string]string, len(presets))
	for _, ref := range EvaluatorRefs() {
		preset, err := Lookup(ref)
		require.NoError(t, err)
		if other, exists := seen[preset.Key]; exists {
			t.Fatalf("key %q used by both %s and %s", preset.Key, other, ref)
		}
		seen[preset.Key] = ref
	}
}

func TestPresetDefinitions(t *testing.T) {
	cases := map[string]struct {
		key     string
		label   string
		output  evaluator.OutputType
		options []string
		context evaluator.ContextConfig
	}{
		RefExposedPII: {
			key: "exposed_pii", label: "Exposed PII", output: evaluator.OutputBoolean,
		},
		RefLeakedCredentials: {
			key: "leaked_credentials", label: "Leaked credentials", output: evaluator.OutputBoolean,
		},
		RefDisclosedSystemInstructions: {
			key: "disclosed_system_instructions", label: "Disclosed system instructions", output: evaluator.OutputBoolean,
		},
		RefUnnecessaryToolCall: {
			key: "unnecessary_tool_call", label: "Unnecessary tool call", output: evaluator.OutputBoolean,
			context: evaluator.ContextConfig{Steps: true, StepTypes: []string{"tool"}},
		},
		RefClaimGrounding: {
			key: "claim_grounding", label: "Claim grounding", output: evaluator.OutputEnum,
			options: []string{"grounded", "unsupported", "contradicted", "no_claims"},
			context: evaluator.ContextConfig{Steps: true},
		},
		RefUserSentiment: {
			key: "user_sentiment", label: "User sentiment", output: evaluator.OutputEnum,
			options: []string{"positive", "neutral", "negative", "unclear"},
			context: evaluator.ContextConfig{
				PreviousTurns:   true,
				NextUserMessage: true,
				UserFeedback:    true,
			},
		},
	}

	for ref, want := range cases {
		t.Run(ref, func(t *testing.T) {
			preset, err := Lookup(ref)
			require.NoError(t, err)
			assert.Equal(t, want.key, preset.Key)
			assert.Equal(t, want.label, preset.Label)
			assert.Equal(t, evaluator.TypeLLM, preset.Type)
			assert.Equal(t, want.output, preset.Output.Type)
			assert.Equal(t, want.options, preset.Output.Options)
			assert.Equal(t, want.context, preset.Config.Context)
			assert.NotEmpty(t, preset.Prompt)
		})
	}
}

func TestLookupRejectsUnknownAndSetRefs(t *testing.T) {
	for _, ref := range []string{"", "preset/nope", "exposed_pii", RefDefaultSet} {
		_, err := Lookup(ref)
		require.ErrorIs(t, err, ErrUnknownRef)
	}
}

func TestEvaluatorRefsMatchRegistryAndAreCopied(t *testing.T) {
	refs := EvaluatorRefs()
	require.Len(t, refs, len(presets))
	for _, ref := range refs {
		assert.True(t, IsEvaluatorRef(ref))
	}

	refs[0] = "mutated"
	assert.Equal(t, RefExposedPII, EvaluatorRefs()[0])
}

func TestIsEvaluatorRefRejectsSetRef(t *testing.T) {
	assert.False(t, IsEvaluatorRef(RefDefaultSet))
	assert.False(t, IsEvaluatorRef("preset/nope"))
}
