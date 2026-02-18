package scaffold

import (
	"bytes"
	"path/filepath"
	"testing"
	"text/template"

	spec "github.com/postman/astro/packages/astro-spec"
)

// renderAstroYml renders the astro.yml template with the given config and returns the YAML string.
func renderAstroYml(t *testing.T, config ScaffoldConfig) string {
	t.Helper()
	paths, err := GetTemplatePaths("ts")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}
	tmplStr, err := GetTemplate(paths.AstroYml)
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	tmpl, err := template.New(filepath.Base(paths.AstroYml)).Parse(tmplStr)
	if err != nil {
		t.Fatalf("template.Parse: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		t.Fatalf("template.Execute: %v", err)
	}
	return buf.String()
}

func TestAstroYml_AnthropicUnderModels(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web"},
		Integrations:    []string{"anthropic"},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestion:       "none",
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if _, ok := s.Models["anthropic"]; !ok {
		t.Errorf("expected anthropic under models, got models=%v", s.Models)
	}
	if len(s.Integrations) != 0 {
		t.Errorf("expected no integrations, got %v", s.Integrations)
	}
}

func TestAstroYml_OpenAIUnderModels(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web"},
		Integrations:    []string{"openai"},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestion:       "none",
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if _, ok := s.Models["openai"]; !ok {
		t.Errorf("expected openai under models, got models=%v", s.Models)
	}
	if len(s.Integrations) != 0 {
		t.Errorf("expected no integrations, got %v", s.Integrations)
	}
}

func TestAstroYml_GitHubUnderTools(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web"},
		Integrations:    []string{"github"},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestion:       "none",
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if _, ok := s.Tools["github"]; !ok {
		t.Errorf("expected github under tools, got tools=%v", s.Tools)
	}
	if len(s.Integrations) != 0 {
		t.Errorf("expected no integrations, got %v", s.Integrations)
	}
}

func TestAstroYml_OllamaUnderModels(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web"},
		ModelProvider:   "ollama",
		Model:           "llama3",
		Integrations:    []string{},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestion:       "none",
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	m, ok := s.Models["default"]
	if !ok {
		t.Fatalf("expected 'default' under models, got models=%v", s.Models)
	}
	if m.Provider != "ollama" {
		t.Errorf("expected provider=ollama, got %q", m.Provider)
	}
	if m.Model != "llama3" {
		t.Errorf("expected model=llama3, got %q", m.Model)
	}
}

func TestAstroYml_KnowledgeMapping(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web"},
		Integrations:    []string{},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{"qdrant", "redis", "neo4j"},
		Ingestion:       "none",
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if _, ok := s.Knowledge["docs"]; !ok {
		t.Errorf("expected qdrant mapped to 'docs', got knowledge=%v", s.Knowledge)
	}
	if _, ok := s.Knowledge["cache"]; !ok {
		t.Errorf("expected redis mapped to 'cache', got knowledge=%v", s.Knowledge)
	}
	if _, ok := s.Knowledge["graph"]; !ok {
		t.Errorf("expected neo4j mapped to 'graph', got knowledge=%v", s.Knowledge)
	}
}

func TestAstroYml_FullInfrastructure(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web", "slack"},
		ModelProvider:   "ollama",
		Model:           "mistral",
		Integrations:    []string{"anthropic", "openai", "github"},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{"qdrant", "redis"},
		Ingestion:       "schedule",
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	// Models: ollama + anthropic + openai
	if len(s.Models) != 3 {
		t.Errorf("expected 3 models, got %d: %v", len(s.Models), s.Models)
	}
	if s.Models["default"].Provider != "ollama" {
		t.Errorf("expected default model provider=ollama, got %q", s.Models["default"].Provider)
	}
	if s.Models["anthropic"].Provider != "anthropic" {
		t.Errorf("expected anthropic model provider=anthropic, got %q", s.Models["anthropic"].Provider)
	}
	if s.Models["openai"].Provider != "openai" {
		t.Errorf("expected openai model provider=openai, got %q", s.Models["openai"].Provider)
	}

	// Tools: github
	if _, ok := s.Tools["github"]; !ok {
		t.Errorf("expected github under tools, got %v", s.Tools)
	}

	// Knowledge: qdrant + redis
	if len(s.Knowledge) != 2 {
		t.Errorf("expected 2 knowledge stores, got %d: %v", len(s.Knowledge), s.Knowledge)
	}

	// Integrations: none (all mapped to models/tools)
	if len(s.Integrations) != 0 {
		t.Errorf("expected no integrations, got %v", s.Integrations)
	}

	// Ingestion
	if len(s.Ingestion) != 1 {
		t.Errorf("expected 1 ingestion, got %d", len(s.Ingestion))
	}
}

func TestAstroYml_MinimalConfig(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "bare-agent",
		Description:     "minimal",
		Interfaces:      []string{"web"},
		Integrations:    []string{},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestion:       "none",
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if s.Name != "bare-agent" {
		t.Errorf("expected name=bare-agent, got %q", s.Name)
	}
	if len(s.Models) != 0 {
		t.Errorf("expected no models, got %v", s.Models)
	}
	if len(s.Knowledge) != 0 {
		t.Errorf("expected no knowledge, got %v", s.Knowledge)
	}
	if len(s.Tools) != 0 {
		t.Errorf("expected no tools, got %v", s.Tools)
	}
	if len(s.Integrations) != 0 {
		t.Errorf("expected no integrations, got %v", s.Integrations)
	}
}
