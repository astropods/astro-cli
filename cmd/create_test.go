package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro-cli/internal/scaffold"
	spec "github.com/astropods/astro-spec"
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

// newCreateCmd builds a standalone create command. Running the shared rootCmd
// would leave its parsed flag values set for every later test in the package.
func newCreateCmd(out io.Writer, args ...string) *cobra.Command {
	cmd := &cobra.Command{Use: "create <name>", Args: exactValidProjectName, RunE: runCreate}
	registerCreateFlags(cmd)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SilenceUsage = true
	cmd.SetArgs(args)
	return cmd
}

func TestRunCreate_InvalidModelFlag(t *testing.T) {
	err := newCreateCmd(io.Discard, "my-agent", "--model", "gpt4").Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown model provider")
}

// ── description normalization ─────────────────────────────────────────────────

func TestNormalizeDescription(t *testing.T) {
	atLimit := strings.Repeat("a", spec.MaxDescriptionLength)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty stays empty", in: "", want: ""},
		{name: "whitespace only becomes empty", in: "  \n\t ", want: ""},
		{name: "trims surrounding whitespace", in: "  Summarise tech talks.  ", want: "Summarise tech talks."},
		{name: "collapses internal whitespace", in: "Summarise   tech\ttalks", want: "Summarise tech talks."},
		{name: "collapses newlines", in: "Summarise\ntech talks", want: "Summarise tech talks."},
		{name: "terminates an unpunctuated sentence", in: "Summarise tech talks", want: "Summarise tech talks."},
		{name: "keeps an existing period", in: "Summarise tech talks.", want: "Summarise tech talks."},
		{name: "keeps a question mark", in: "Which talks matter?", want: "Which talks matter?"},
		{name: "keeps an exclamation mark", in: "Summarise the talks!", want: "Summarise the talks!"},
		{name: "skips the period at the agent card limit", in: atLimit, want: atLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeDescription(tt.in))
		})
	}
}

func TestValidateDescription(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr error
	}{
		{name: "empty is allowed", in: ""},
		{name: "at the agent card limit", in: strings.Repeat("a", spec.MaxDescriptionLength)},
		{
			name:    "over the agent card limit",
			in:      strings.Repeat("a", spec.MaxDescriptionLength+1),
			wantErr: errDescriptionTooLong(spec.MaxDescriptionLength + 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDescription(tt.in)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantErr.Error())
		})
	}
}

// ── --description end to end ──────────────────────────────────────────────────

// runCreateInTempDir runs the create command non-interactively and returns the
// generated project directory alongside the command's own output.
func runCreateInTempDir(t *testing.T, name string, args ...string) (projectDir, output string) {
	t.Helper()
	parent := t.TempDir()
	var out bytes.Buffer

	require.NoError(t, newCreateCmd(&out, append([]string{name, "--path", parent, "--yes"}, args...)...).Execute())

	return filepath.Join(parent, name), out.String()
}

func findLine(t *testing.T, content, key string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, key) {
			return line
		}
	}
	require.FailNowf(t, "line not found", "no %q line in generated file", key)
	return ""
}

func readGenerated(t *testing.T, projectDir, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectDir, name))
	require.NoError(t, err)
	return string(content)
}

func TestRunCreate_DescriptionReachesGeneratedAgent(t *testing.T) {
	dir, out := runCreateInTempDir(t, "described-agent", "--description", "Summarise tech talks")

	assert.Contains(t, readGenerated(t, dir, "agent/index.ts"),
		"a helpful AI assistant. Summarise tech talks.",
		"the description should become part of the agent's instructions")
	assert.Contains(t, readGenerated(t, dir, "AGENT.md"), `description: "Summarise tech talks."`)
	assert.Contains(t, readGenerated(t, dir, "README.md"), "Summarise tech talks.")
	assert.Contains(t, out, msgPasteToCodingAgent(),
		"the coding-agent prompt should still be offered after the project is created")
	assert.Contains(t, out, "Summarise tech talks.",
		"the coding-agent prompt should carry the description")
}

func TestRunCreate_WithoutDescriptionKeepsPlaceholderOutOfInstructions(t *testing.T) {
	dir, _ := runCreateInTempDir(t, "plain-agent")

	index := readGenerated(t, dir, "agent/index.ts")
	instructions := findLine(t, index, "instructions:")
	assert.NotContains(t, instructions, scaffold.DescriptionPlaceholder,
		"the docs placeholder must never become an instruction to the model")
	assert.Equal(t, "  instructions: 'You are Plain Agent, a helpful AI assistant.',", instructions)
	assert.Contains(t, readGenerated(t, dir, "AGENT.md"), scaffold.DescriptionPlaceholder,
		"the agent card should still prompt the reader to fill a description in")
}

func TestRunCreate_DescriptionTooLong(t *testing.T) {
	tooLong := strings.Repeat("a", spec.MaxDescriptionLength+1)

	err := newCreateCmd(io.Discard, "my-agent", "--description", tooLong).Execute()
	require.EqualError(t, err, errDescriptionTooLong(spec.MaxDescriptionLength+1).Error())
}
