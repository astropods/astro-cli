package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro/apps/astro-cli/internal/scaffold"
)

// ── applyModelOverride ────────────────────────────────────────────────────────

func TestApplyModelOverride(t *testing.T) {
	tests := []struct {
		name            string
		override        string
		wantProvider    string
		wantModel       string
		wantIntegration string
	}{
		{"empty", "", "", "", ""},
		{"anthropic", "anthropic", "", "", "anthropic"},
		{"openai", "openai", "", "", "openai"},
		{"ollama no model", "ollama", "ollama", "", ""},
		{"ollama with model", "ollama/llama3.3:70b", "ollama", "llama3.3:70b", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := scaffold.ScaffoldConfig{IntegrationKeys: map[string]string{}}
			applyModelOverride(&cfg, tt.override)
			assert.Equal(t, tt.wantProvider, cfg.ModelProvider)
			assert.Equal(t, tt.wantModel, cfg.Model)
			if tt.wantIntegration != "" {
				assert.Contains(t, cfg.Integrations, tt.wantIntegration)
			} else {
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

	tests := []struct {
		toComplete   string
		wantLen      int
		wantContains []string // completion values (without description) that must appear
	}{
		// no prefix → top-level providers only
		{"", 3, []string{"anthropic", "openai", "ollama"}},
		// ollama prefix → full list (shell filters by prefix)
		{"ollama/", len(ollamaModelList), nil},
		{"ollama/llama3.", len(ollamaModelList), nil},
		// name + colon → only tags for that model name
		{"ollama/llama3.3:", 1, []string{"70b"}},
		{"ollama/llama3.1:", 2, []string{"8b", "70b"}},
		// partial tag narrows further
		{"ollama/llama3.1:7", 1, []string{"70b"}},
		{"ollama/llama3.1:8", 1, []string{"8b"}},
	}

	for _, tt := range tests {
		t.Run(tt.toComplete, func(t *testing.T) {
			got, _ := completionFn(cmd, nil, tt.toComplete)
			assert.Len(t, got, tt.wantLen)
			for _, want := range tt.wantContains {
				assert.Contains(t, names(got), want)
			}
		})
	}
}

// ── --model flag validation ───────────────────────────────────────────────────

func TestRunCreate_InvalidModelFlag(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		wantErr string
	}{
		{
			name:    "unknown provider",
			model:   "gpt4",
			wantErr: "unknown model provider",
		},
		{
			name:    "unknown ollama model",
			model:   "ollama/not-a-model",
			wantErr: "unknown ollama model",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { rootCmd.SetArgs(nil) })
			rootCmd.SetArgs([]string{"create", "my-agent", "--model", tt.model})
			err := rootCmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
