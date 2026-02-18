package create

import "github.com/charmbracelet/bubbles/textinput"

type option struct {
	label    string
	value    string
	isHeader bool
}

func interfaceOptions() []option {
	return []option{
		{"Web (HTTP/SSE)", "web", false},
		{"Slack", "slack", false},
	}
}

func infrastructureOptions() []option {
	return []option{
		{"Models", "", true},
		{"Ollama (self-hosted)", "ollama", false},
		{"Anthropic", "anthropic", false},
		{"OpenAI", "openai", false},
		{"Knowledge", "", true},
		{"Qdrant (vector store)", "qdrant", false},
		{"Redis (key-value)", "redis", false},
		{"Neo4j (graph)", "neo4j", false},
		{"Tools", "", true},
		{"GitHub", "github", false},
	}
}

func ingestionOptions() []option {
	return []option{
		{"Scheduled (cron)", "schedule", false},
		{"Webhook", "webhook", false},
		{"Manual trigger", "manual", false},
		{"On startup", "startup", false},
		{"None", "none", false},
	}
}

// nextSelectable returns the next selectable index after idx, or idx if none.
func nextSelectable(opts []option, idx int) int {
	for i := idx + 1; i < len(opts); i++ {
		if !opts[i].isHeader {
			return i
		}
	}
	return idx
}

// prevSelectable returns the previous selectable index before idx, or idx if none.
func prevSelectable(opts []option, idx int) int {
	for i := idx - 1; i >= 0; i-- {
		if !opts[i].isHeader {
			return i
		}
	}
	return idx
}

// integrationKeyEnvVar maps key names to their environment variable names.
var integrationKeyEnvVar = map[string]string{
	"anthropic":       "ANTHROPIC_API_KEY",
	"openai":          "OPENAI_API_KEY",
	"github":          "GITHUB_TOKEN",
	"slack_bot_token": "SLACK_BOT_TOKEN",
	"slack_app_token": "SLACK_APP_TOKEN",
}

// integrationKeyLabel maps key names to display labels.
var integrationKeyLabel = map[string]string{
	"anthropic":       "Anthropic",
	"openai":          "OpenAI",
	"github":          "GitHub",
	"slack_bot_token": "Slack Bot Token",
	"slack_app_token": "Slack App Token",
}

func newKeyInput() textinput.Model {
	ti := textinput.New()
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	ti.Placeholder = ""
	ti.Width = 60
	return ti
}
