package scaffold

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	spec "github.com/astropods/astro-spec"
)

// promptSite locates the line that becomes the model's system prompt in each
// agent template, so both templates get the same assertions. wantEscapes and
// headerWant name the forms hostileDescription must take once escaped for that
// language's string literal and file header.
type promptSite struct {
	name        string
	agentFile   func(*TemplatePaths) string
	promptKey   string
	closer      string
	wantEscapes []string
	headerWant  string
}

var promptSites = []promptSite{
	{
		name:        "mastra",
		agentFile:   func(p *TemplatePaths) string { return p.AgentIndex },
		promptKey:   "instructions:",
		closer:      "',",
		wantEscapes: []string{`it\'s`, `C:\\path`},
		headerWant:  `*\/`,
	},
	{
		name:        "langchain",
		agentFile:   func(p *TemplatePaths) string { return p.AgentMain },
		promptKey:   "system_prompt",
		closer:      `"`,
		wantEscapes: []string{`\"hi\"`, `C:\\path`},
		headerWant:  `C:\\path`,
	},
}

func (s promptSite) renderAgent(t *testing.T, config ScaffoldConfig) string {
	t.Helper()
	paths, err := GetTemplatePaths(s.name)
	require.NoError(t, err)
	content, err := RenderTemplate(s.agentFile(paths), config)
	require.NoError(t, err)
	return content
}

func (s promptSite) render(t *testing.T, config ScaffoldConfig) string {
	t.Helper()
	for _, line := range strings.Split(s.renderAgent(t, config), "\n") {
		if strings.Contains(line, s.promptKey) {
			return line
		}
	}
	require.FailNowf(t, "prompt line not found", "no %q line in rendered %s template", s.promptKey, s.name)
	return ""
}

func TestDefaultConfig_LeavesDescriptionUnset(t *testing.T) {
	assert.Empty(t, DefaultConfig("test-agent").Description,
		"a default description would be interpolated into the agent's system prompt")
}

func TestDocDescription(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{name: "unset falls back to the placeholder", description: "", want: DescriptionPlaceholder},
		{name: "set is returned as written", description: "Summarise tech talks.", want: "Summarise tech talks."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ScaffoldConfig{Description: tt.description}.DocDescription())
		})
	}
}

func TestAgentPrompt_UnsetDescriptionAddsNothing(t *testing.T) {
	for _, site := range promptSites {
		t.Run(site.name, func(t *testing.T) {
			line := site.render(t, DefaultConfig("test-agent"))

			assert.NotContains(t, line, DescriptionPlaceholder,
				"the docs placeholder must never become an instruction to the model")
			assert.Contains(t, line, "You are Test Agent, a helpful AI assistant.",
				"the prompt should still introduce the agent by name")
			assert.NotContains(t, line, "assistant. ",
				"an unset description should leave no trailing space in the prompt")
		})
	}
}

func TestAgentPrompt_IncludesDescription(t *testing.T) {
	for _, site := range promptSites {
		t.Run(site.name, func(t *testing.T) {
			config := DefaultConfig("test-agent")
			config.Description = "Summarise tech talks."

			assert.Contains(t, site.render(t, config), "a helpful AI assistant. Summarise tech talks.")
		})
	}
}

func TestTemplateEscapers(t *testing.T) {
	tests := []struct {
		name string
		fn   string
		in   string
		want string
	}{
		{name: "jsStr escapes a single quote", fn: "jsStr", in: "it's", want: `it\'s`},
		{name: "jsStr escapes a backslash", fn: "jsStr", in: `C:\path`, want: `C:\\path`},
		{name: "jsStr escapes a newline", fn: "jsStr", in: "one\ntwo", want: `one\ntwo`},
		{name: "dqStr escapes a double quote", fn: "dqStr", in: `say "hi"`, want: `say \"hi\"`},
		{name: "dqStr escapes a backslash", fn: "dqStr", in: `C:\path`, want: `C:\\path`},
		{name: "dqStr escapes a windows newline", fn: "dqStr", in: "one\r\ntwo", want: `one\ntwo`},
		{name: "jsComment neutralizes a block terminator", fn: "jsComment", in: "ends */ here", want: `ends *\/ here`},
		{name: "pyDoc escapes a backslash", fn: "pyDoc", in: `C:\path`, want: `C:\\path`},
		{name: "pyDoc escapes a docstring terminator", fn: "pyDoc", in: `a """ b`, want: `a \"\"\" b`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, ok := templateFuncs[tt.fn].(func(string) string)
			require.Truef(t, ok, "template func %q should be a func(string) string", tt.fn)
			assert.Equal(t, tt.want, fn(tt.in))
		})
	}
}

// hostileDescription carries one character that would break out of each context
// a description is interpolated into.
const hostileDescription = `Say "hi", it's C:\path */ """ done.`

func TestGeneratedFiles_ParseWithHostileDescription(t *testing.T) {
	config := DefaultConfig("test-agent")
	config.Description = hostileDescription

	t.Run("package.json stays valid JSON", func(t *testing.T) {
		paths, err := GetTemplatePaths("mastra")
		require.NoError(t, err)
		content, err := RenderTemplate(paths.PackageJson, config)
		require.NoError(t, err)

		var pkg struct {
			Description string `json:"description"`
		}
		require.NoError(t, json.Unmarshal([]byte(content), &pkg))
		assert.Equal(t, hostileDescription, pkg.Description,
			"the description should survive JSON escaping unchanged")
	})

	t.Run("AGENT.md keeps its placeholder", func(t *testing.T) {
		paths, err := GetTemplatePaths("mastra")
		require.NoError(t, err)
		content, err := RenderTemplate(paths.AgentMd, config)
		require.NoError(t, err)

		card := spec.ParseAgentCard(content)
		assert.Empty(t, card.Warnings)
		assert.Equal(t, DescriptionPlaceholder, card.Description,
			"the card is a published listing, so the scaffold never fills it in")
	})

	t.Run("agent prompt stays inside its string literal", func(t *testing.T) {
		for _, site := range promptSites {
			t.Run(site.name, func(t *testing.T) {
				line := site.render(t, config)
				for _, want := range site.wantEscapes {
					assert.Containsf(t, line, want,
						"unescaped input would terminate the %s prompt literal early", site.name)
				}
				assert.True(t, strings.HasSuffix(strings.TrimRight(line, " \t"), site.closer),
					"prompt line should still end with its closing delimiter: %s", line)
			})
		}
	})

	t.Run("file header stays inside its comment", func(t *testing.T) {
		for _, site := range promptSites {
			t.Run(site.name, func(t *testing.T) {
				header := strings.SplitN(site.renderAgent(t, config), "\n", 3)[1]
				assert.Containsf(t, header, site.headerWant,
					"unescaped input would break out of the %s file header comment", site.name)
			})
		}
	})
}

func TestDescriptionLength_WithinAgentCardLimit(t *testing.T) {
	assert.LessOrEqual(t, len([]rune(DescriptionPlaceholder)), spec.MaxDescriptionLength,
		"the placeholder itself must not be truncated by the agent card parser")
}
