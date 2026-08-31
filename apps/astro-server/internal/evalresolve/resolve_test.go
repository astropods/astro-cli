package evalresolve

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro/apps/astro-server/internal/evalagentstore"
	"github.com/astropods/astro/apps/astro-server/internal/evaldefinitionstore"
	"github.com/astropods/astro/apps/astro-server/internal/evalpreset"
)

type fakeAgentStore struct {
	ae  *evalagentstore.AgentEvaluation
	err error
}

func (f *fakeAgentStore) Get(context.Context, string, string) (*evalagentstore.AgentEvaluation, error) {
	return f.ae, f.err
}

type fakeDefinitionStore struct {
	def *evaldefinitionstore.Definition
	err error
}

func (f *fakeDefinitionStore) Get(context.Context, string) (*evaldefinitionstore.Definition, error) {
	return f.def, f.err
}

func TestActiveRefFallsBackToDefault(t *testing.T) {
	resolver := NewResolver(&fakeAgentStore{}, &fakeDefinitionStore{})
	ref, err := resolver.ActiveRef(context.Background(), "account-1", "agent-1")
	require.NoError(t, err)
	assert.Equal(t, evalpreset.RefDefaultSet, ref)
}

func TestActiveRefUsesTheStoredOverride(t *testing.T) {
	resolver := NewResolver(&fakeAgentStore{ae: &evalagentstore.AgentEvaluation{
		AccountID: "account-1", AgentName: "agent-1", EvaluationRef: "agent/abc123",
	}}, &fakeDefinitionStore{})
	ref, err := resolver.ActiveRef(context.Background(), "account-1", "agent-1")
	require.NoError(t, err)
	assert.Equal(t, "agent/abc123", ref)
}

func TestActiveRefPropagatesStoreErrors(t *testing.T) {
	resolver := NewResolver(&fakeAgentStore{err: errors.New("boom")}, &fakeDefinitionStore{})
	_, err := resolver.ActiveRef(context.Background(), "account-1", "agent-1")
	require.Error(t, err)
}

func TestSetResolvesAPresetRef(t *testing.T) {
	resolver := NewResolver(&fakeAgentStore{}, &fakeDefinitionStore{})
	set, err := resolver.Set(context.Background(), evalpreset.RefDefaultSet)
	require.NoError(t, err)
	assert.NotEmpty(t, set)
}

func TestSetResolvesACustomDefinition(t *testing.T) {
	definitionJSON, err := json.Marshal(map[string]any{
		"schema": "evaluation/v1",
		"evaluators": []map[string]any{
			{
				"key":    "has_secrets",
				"label":  "Contains secrets",
				"type":   "llm",
				"prompt": "Determine whether the agent output exposes credentials.",
				"output": map[string]any{"type": "boolean"},
			},
			{"ref": "preset/user-sentiment"},
		},
	})
	require.NoError(t, err)

	resolver := NewResolver(&fakeAgentStore{}, &fakeDefinitionStore{
		def: &evaldefinitionstore.Definition{EvaluationRef: "agent/abc123", DefinitionJSON: definitionJSON},
	})
	set, err := resolver.Set(context.Background(), "agent/abc123")
	require.NoError(t, err)
	require.Len(t, set, 2)
	assert.Equal(t, "has_secrets", set[0].Key)
	assert.Equal(t, "user_sentiment", set[1].Key)
}

func TestSetReturnsAnErrorForAnUnknownRef(t *testing.T) {
	resolver := NewResolver(&fakeAgentStore{}, &fakeDefinitionStore{})
	_, err := resolver.Set(context.Background(), "agent/does-not-exist")
	require.Error(t, err)
}
