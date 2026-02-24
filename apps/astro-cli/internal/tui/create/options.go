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

// modelOptions returns multi-select options for the model screen (Ollama, Anthropic, OpenAI).
func modelOptions() []option {
	return []option{
		{"Ollama (self-hosted)", "ollama", false},
		{"Anthropic", "anthropic", false},
		{"OpenAI", "openai", false},
	}
}

// ollamaModelOptions returns the closed set of Ollama model names (radio/single-select).
func ollamaModelOptions() []option {
	return []option{
		{"llama3.2:1b", "llama3.2:1b", false},
		{"llama3.1:8b", "llama3.1:8b", false},
		{"mistral:7b", "mistral:7b", false},
		{"codellama:7b", "codellama:7b", false},
		{"phi3:3.8b", "phi3:3.8b", false},
		{"gemma2:2b", "gemma2:2b", false},
	}
}

func knowledgeOptions() []option {
	return []option{
		{"Qdrant (vector store)", "qdrant", false},
		{"Redis (key-value)", "redis", false},
		{"Neo4j (graph)", "neo4j", false},
	}
}

// toolsOptions returns multi-select options for integrations/tools (e.g. GitHub).
func toolsOptions() []option {
	return []option{
		{"GitHub", "github", false},
	}
}

func ingestionOptions() []option {
	return []option{
		{"Scheduled (cron)", "schedule", false},
		{"Webhook", "webhook", false},
		{"Manual trigger", "manual", false},
		{"On startup", "startup", false},
	}
}

// integrationKeyEnvVar maps key names to their environment variable names.
var integrationKeyEnvVar = map[string]string{ //nolint:gosec
	"anthropic":       "ANTHROPIC_API_KEY",
	"openai":          "OPENAI_API_KEY",
	"github":          "GITHUB_TOKEN",
	"slack_bot_token": "SLACK_BOT_TOKEN",
	"slack_app_token": "SLACK_APP_TOKEN",
}

// integrationKeyLabel maps key names to display labels.
var integrationKeyLabel = map[string]string{ //nolint:gosec
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
