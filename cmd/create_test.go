package cmd

import (
	"testing"

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
