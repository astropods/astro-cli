package create

import (
	"slices"
	"testing"
)

// ── buildConfig ───────────────────────────────────────────────────────────────

func TestBuildConfig_OllamaModel(t *testing.T) {
	cfg := buildConfig(formInputs{
		name:        "my-agent",
		description: "desc",
		interfaces:  []string{"web"},
		models:      []string{"ollama"},
		ollamaModel: "mistral:7b",
	})

	if cfg.ModelProvider != "ollama" {
		t.Errorf("ModelProvider = %q, want ollama", cfg.ModelProvider)
	}
	if cfg.Model != "mistral:7b" {
		t.Errorf("Model = %q, want mistral:7b", cfg.Model)
	}
	if len(cfg.Integrations) != 0 {
		t.Errorf("Integrations = %v, want empty", cfg.Integrations)
	}
}

func TestBuildConfig_NoModel(t *testing.T) {
	cfg := buildConfig(formInputs{
		name:       "my-agent",
		interfaces: []string{"web"},
		models:     []string{},
	})

	if cfg.ModelProvider != "" {
		t.Errorf("ModelProvider = %q, want empty", cfg.ModelProvider)
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty", cfg.Model)
	}
}

func TestBuildConfig_AnthropicAndOpenAI(t *testing.T) {
	cfg := buildConfig(formInputs{
		name:         "my-agent",
		interfaces:   []string{"web"},
		models:       []string{"anthropic", "openai"},
		anthropicKey: "sk-ant",
		openaiKey:    "sk-oai",
	})

	if cfg.ModelProvider != "" {
		t.Errorf("ModelProvider = %q, want empty (cloud providers are integrations)", cfg.ModelProvider)
	}
	if len(cfg.Integrations) != 2 {
		t.Fatalf("Integrations = %v, want [anthropic openai]", cfg.Integrations)
	}
	if cfg.IntegrationKeys["anthropic"] != "sk-ant" {
		t.Errorf("anthropic key = %q, want sk-ant", cfg.IntegrationKeys["anthropic"])
	}
	if cfg.IntegrationKeys["openai"] != "sk-oai" {
		t.Errorf("openai key = %q, want sk-oai", cfg.IntegrationKeys["openai"])
	}
}

func TestBuildConfig_ToolsAddedToIntegrations(t *testing.T) {
	cfg := buildConfig(formInputs{
		name:        "my-agent",
		interfaces:  []string{"web"},
		models:      []string{},
		tools:       []string{"github"},
		githubToken: "ghp_tok",
	})

	if len(cfg.Integrations) != 1 || cfg.Integrations[0] != "github" {
		t.Errorf("Integrations = %v, want [github]", cfg.Integrations)
	}
	if cfg.IntegrationKeys["github"] != "ghp_tok" {
		t.Errorf("github token = %q, want ghp_tok", cfg.IntegrationKeys["github"])
	}
}

func TestBuildConfig_SlackKeys(t *testing.T) {
	cfg := buildConfig(formInputs{
		name:          "my-agent",
		interfaces:    []string{"slack"},
		models:        []string{},
		slackBotToken: "xoxb-bot",
		slackAppToken: "xapp-app",
	})

	if cfg.IntegrationKeys["slack_bot_token"] != "xoxb-bot" {
		t.Errorf("slack_bot_token = %q, want xoxb-bot", cfg.IntegrationKeys["slack_bot_token"])
	}
	if cfg.IntegrationKeys["slack_app_token"] != "xapp-app" {
		t.Errorf("slack_app_token = %q, want xapp-app", cfg.IntegrationKeys["slack_app_token"])
	}
}

func TestBuildConfig_EmptyKeysOmitted(t *testing.T) {
	cfg := buildConfig(formInputs{
		name:       "my-agent",
		interfaces: []string{"web"},
		models:     []string{"anthropic"},
		// no anthropicKey set
	})

	if _, ok := cfg.IntegrationKeys["anthropic"]; ok {
		t.Error("expected anthropic key to be absent when empty")
	}
}

func TestBuildConfig_OllamaAndAnthropicCombined(t *testing.T) {
	// Ollama → ModelProvider; Anthropic → Integrations. Both can coexist.
	cfg := buildConfig(formInputs{
		name:         "my-agent",
		interfaces:   []string{"web"},
		models:       []string{"ollama", "anthropic"},
		ollamaModel:  "llama3.2:1b",
		anthropicKey: "sk-ant",
	})

	if cfg.ModelProvider != "ollama" {
		t.Errorf("ModelProvider = %q, want ollama", cfg.ModelProvider)
	}
	if cfg.Model != "llama3.2:1b" {
		t.Errorf("Model = %q, want llama3.2:1b", cfg.Model)
	}
	if !slices.Contains(cfg.Integrations, "anthropic") {
		t.Errorf("Integrations = %v, want anthropic present", cfg.Integrations)
	}
	if cfg.IntegrationKeys["anthropic"] != "sk-ant" {
		t.Errorf("anthropic key = %q, want sk-ant", cfg.IntegrationKeys["anthropic"])
	}
}

func TestBuildConfig_OllamaAndToolsCombined(t *testing.T) {
	cfg := buildConfig(formInputs{
		name:        "my-agent",
		interfaces:  []string{"web"},
		models:      []string{"ollama"},
		ollamaModel: "llama3.2:1b",
		tools:       []string{"github"},
	})

	if cfg.ModelProvider != "ollama" {
		t.Errorf("ModelProvider = %q, want ollama", cfg.ModelProvider)
	}
	if len(cfg.Integrations) != 1 || cfg.Integrations[0] != "github" {
		t.Errorf("Integrations = %v, want [github]", cfg.Integrations)
	}
}

func TestBuildConfig_BasicFields(t *testing.T) {
	cfg := buildConfig(formInputs{
		name:        "agent-x",
		description: "does things",
		interfaces:  []string{"web", "slack"},
		knowledge:   []string{"qdrant"},
		ingestion:   []string{"schedule", "webhook"},
		models:      []string{},
	})

	if cfg.Name != "agent-x" {
		t.Errorf("Name = %q, want agent-x", cfg.Name)
	}
	if cfg.Description != "does things" {
		t.Errorf("Description = %q, want 'does things'", cfg.Description)
	}
	if len(cfg.Interfaces) != 2 {
		t.Errorf("Interfaces = %v, want [web slack]", cfg.Interfaces)
	}
	if len(cfg.Knowledge) != 1 || cfg.Knowledge[0] != "qdrant" {
		t.Errorf("Knowledge = %v, want [qdrant]", cfg.Knowledge)
	}
	if len(cfg.Ingestions) != 2 {
		t.Errorf("Ingestions = %v, want [schedule webhook]", cfg.Ingestions)
	}
}
