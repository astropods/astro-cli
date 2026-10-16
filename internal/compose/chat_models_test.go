package compose

import (
	"io"
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/stretchr/testify/assert"
)

func webSpec(models map[string]spec.Model) *spec.AstroSpec {
	return &spec.AstroSpec{
		Name:   "my-agent",
		Agent:  spec.Container{Image: "agent:latest"},
		Models: models,
		Dev:    &spec.Dev{Interfaces: &spec.DevInterfaces{Messaging: &spec.DevMessaging{Adapters: []string{"web"}}}},
	}
}

func TestTheLocalSidecarGetsTheLiveModels(t *testing.T) {
	s := webSpec(map[string]spec.Model{
		"claude":  {Provider: "anthropic", Models: []string{"claude-opus-4-7"}, Selection: spec.ModelSelectionLive},
		"default": {Provider: "gateway", Models: []string{"claude-sonnet-4-6", "gpt-4o"}, Selection: spec.ModelSelectionLive},
	})
	want := `{"default":{"entry":"default","model":"claude-sonnet-4-6"},"options":[` +
		`{"entry":"claude","model":"claude-opus-4-7"},{"entry":"default","model":"claude-sonnet-4-6"},{"entry":"default","model":"gpt-4o"}]}`

	env := buildMessagingEnvironment(s, map[string]string{}, io.Discard)

	assert.Equal(t, want, envVal(env, "CHAT_MODELS"), "the gateway entry's first option is its local MODEL_DEFAULT, so it is the default")
}

func TestAnEnvOverrideOfTheGatewayModelIsTheLocalDefault(t *testing.T) {
	s := webSpec(map[string]spec.Model{
		"default": {Provider: "gateway", Models: []string{"claude-sonnet-4-6", "gpt-4o"}, Selection: spec.ModelSelectionLive},
	})

	env := buildMessagingEnvironment(s, map[string]string{"MODEL_DEFAULT": "gpt-4o"}, io.Discard)

	assert.Contains(t, envVal(env, "CHAT_MODELS"), `"default":{"entry":"default","model":"gpt-4o"}`)
}

func TestByokOnlyLiveModelsDefaultToTheFirstOption(t *testing.T) {
	s := webSpec(map[string]spec.Model{
		"gpt":    {Provider: "openai", Models: []string{"gpt-5"}, Selection: spec.ModelSelectionLive},
		"claude": {Provider: "anthropic", Models: []string{"claude-opus-4-7"}, Selection: spec.ModelSelectionLive},
	})

	env := buildMessagingEnvironment(s, map[string]string{}, io.Discard)

	assert.Contains(t, envVal(env, "CHAT_MODELS"), `"default":{"entry":"claude","model":"claude-opus-4-7"}`)
}

func TestNoLiveModelsMeansNoCHAT_MODELS(t *testing.T) {
	s := webSpec(map[string]spec.Model{
		"default": {Provider: "gateway", Models: []string{"claude-sonnet-4-6", "gpt-4o"}},
	})

	env := buildMessagingEnvironment(s, map[string]string{}, io.Discard)

	assert.NotContains(t, env, "CHAT_MODELS")
}
