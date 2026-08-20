package evalpreset

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/evaluator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSetOrder(t *testing.T) {
	refs, err := SetRefs(RefDefaultSet)
	require.NoError(t, err)
	assert.Equal(t, []string{
		RefExposedPII,
		RefLeakedCredentials,
		RefDisclosedSystemInstructions,
		RefUnnecessaryToolCall,
		RefClaimGrounding,
		RefUserSentiment,
	}, refs)
}

func TestResolveSetReturnsExecutableEvaluatorsInOrder(t *testing.T) {
	resolved, err := ResolveSet(RefDefaultSet)
	require.NoError(t, err)
	require.Len(t, resolved, 6)

	keys := make([]string, 0, len(resolved))
	for _, ev := range resolved {
		require.NoError(t, evaluator.Validate(ev))
		keys = append(keys, ev.Key)
	}
	assert.Equal(t, []string{
		"exposed_pii",
		"leaked_credentials",
		"disclosed_system_instructions",
		"unnecessary_tool_call",
		"claim_grounding",
		"user_sentiment",
	}, keys)
}

func TestResolvedSetKeysAreUnique(t *testing.T) {
	resolved, err := ResolveSet(RefDefaultSet)
	require.NoError(t, err)

	seen := make(map[string]bool, len(resolved))
	for _, ev := range resolved {
		require.False(t, seen[ev.Key], "duplicate key %q after resolution", ev.Key)
		seen[ev.Key] = true
	}
}

func TestSetRefsRejectsUnknownAndEvaluatorRefs(t *testing.T) {
	for _, ref := range []string{"", "preset/nope", RefExposedPII} {
		_, err := SetRefs(ref)
		require.ErrorIs(t, err, ErrUnknownRef)

		_, err = ResolveSet(ref)
		require.ErrorIs(t, err, ErrUnknownRef)
	}
}

func TestSetRefsAreCopied(t *testing.T) {
	refs, err := SetRefs(RefDefaultSet)
	require.NoError(t, err)
	refs[0] = "mutated"

	again, err := SetRefs(RefDefaultSet)
	require.NoError(t, err)
	assert.Equal(t, RefExposedPII, again[0])
}

func TestIsSetRefRejectsEvaluatorRef(t *testing.T) {
	assert.True(t, IsSetRef(RefDefaultSet))
	assert.False(t, IsSetRef(RefExposedPII))
	assert.False(t, IsSetRef("preset/nope"))
}
