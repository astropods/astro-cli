package cmd

import (
	"os/exec"
	"path/filepath"
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

// ── AGENT.md attribution ──────────────────────────────────────────────────────

func TestLocalCardAuthor(t *testing.T) {
	t.Run("uses the logged-in name and personal account", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		creds := accountTestCreds("acme-corp")
		creds.Profiles["default"].User.FirstName = "Jane"
		creds.Profiles["default"].User.LastName = "Doe"
		writeAccountTestCredentials(t, creds)

		author := localCardAuthor(filepath.Join(t.TempDir(), "scout"))

		assert.Equal(t, scaffold.CardAuthor{Name: "Jane Doe", Account: "alice"}, author)
	})

	t.Run("keeps the account when the profile carries no name", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		writeAccountTestCredentials(t, accountTestCreds("alice"))

		author := localCardAuthor(filepath.Join(t.TempDir(), "scout"))

		assert.Equal(t, scaffold.CardAuthor{Account: "alice"}, author)
	})

	t.Run("falls back to the git identity when nobody is logged in", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()
		require.NoError(t, exec.Command("git", "-C", dir, "init", "--quiet").Run())
		require.NoError(t, exec.Command("git", "-C", dir, "config", "user.name", "Jane Doe").Run())

		author := localCardAuthor(filepath.Join(dir, "scout"))

		assert.Equal(t, scaffold.CardAuthor{Name: "Jane Doe"}, author)
	})
}

func TestLocalCardRepository(t *testing.T) {
	t.Run("points at the origin that will hold the project", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, exec.Command("git", "-C", dir, "init", "--quiet").Run())
		require.NoError(t, exec.Command("git", "-C", dir, "remote", "add", "origin", "git@github.com:astropods/agents.git").Run())

		repo := localCardRepository(filepath.Join(dir, "scout"))

		assert.Equal(t, scaffold.CardRepository{URL: "https://github.com/astropods/agents", Directory: "scout"}, repo)
	})

	t.Run("stays empty outside a git repository", func(t *testing.T) {
		repo := localCardRepository(filepath.Join(t.TempDir(), "scout"))

		assert.Equal(t, scaffold.CardRepository{}, repo)
	})
}
