package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/scaffold"
)

// ── applyModelOverride ────────────────────────────────────────────────────────

func TestApplyModelOverride(t *testing.T) {
	tests := []struct {
		name            string
		override        string
		startIntegs     []string // initial Integrations (mirrors DefaultConfig where relevant)
		wantIntegration string
		wantGateway     bool
		wantNoIntegs    bool // assert Integrations ends up empty
	}{
		{name: "empty", override: ""},
		{name: "anthropic", override: "anthropic", wantIntegration: "anthropic"},
		{name: "openai", override: "openai", wantIntegration: "openai"},
		// gateway opts in and drops the default anthropic integration so no key is required.
		{name: "gateway", override: "gateway", startIntegs: []string{"anthropic"}, wantGateway: true, wantNoIntegs: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := scaffold.ScaffoldConfig{IntegrationKeys: map[string]string{}, Integrations: tt.startIntegs}
			applyModelOverride(&cfg, tt.override)
			assert.Equal(t, tt.wantGateway, cfg.AIGateway)
			switch {
			case tt.wantNoIntegs:
				assert.Empty(t, cfg.Integrations)
			case tt.wantIntegration != "":
				assert.Contains(t, cfg.Integrations, tt.wantIntegration)
			default:
				assert.Empty(t, cfg.Integrations)
			}
		})
	}
}

// ── --model tab completion ────────────────────────────────────────────────────

func TestModelCompletion(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	registerCreateFlags(cmd)
	completionFn, ok := cmd.GetFlagCompletionFunc("model")
	require.True(t, ok)

	// cobra.CompletionWithDesc encodes as "value\tdescription"; strip the desc.
	names := func(completions []string) []string {
		out := make([]string, len(completions))
		for i, c := range completions {
			out[i], _, _ = strings.Cut(c, "\t")
		}
		return out
	}

	// The provider list is static — every invocation offers the same three.
	got, _ := completionFn(cmd, nil, "")
	assert.Equal(t, []string{"gateway", "anthropic", "openai"}, names(got))
}

// ── --model flag validation ───────────────────────────────────────────────────

func TestRunCreate_InvalidModelFlag(t *testing.T) {
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	rootCmd.SetArgs([]string{"create", "my-agent", "--model", "gpt4"})
	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown model provider")
}
