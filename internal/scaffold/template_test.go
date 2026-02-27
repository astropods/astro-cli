package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// --- GetTemplatePaths tests ---

func TestGetTemplatePaths_MastraUsesOverridePaths(t *testing.T) {
	paths, err := GetTemplatePaths("ts", "mastra")
	if err != nil {
		t.Fatalf("GetTemplatePaths(ts, mastra): %v", err)
	}

	if !strings.Contains(paths.AgentIndex, "template-ts-mastra") {
		t.Errorf("AgentIndex = %q, want path containing template-ts-mastra", paths.AgentIndex)
	}
	if !strings.Contains(paths.PackageJson, "template-ts-mastra") {
		t.Errorf("PackageJson = %q, want path containing template-ts-mastra", paths.PackageJson)
	}
}

func TestGetTemplatePaths_UnsupportedTemplate(t *testing.T) {
	_, err := GetTemplatePaths("ts", "nonexistent")
	if err == nil {
		t.Fatal("expected error for unsupported template, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported template") {
		t.Errorf("error = %q, want message containing 'unsupported template'", err.Error())
	}
}

func TestGetTemplatePaths_UnsupportedLanguage(t *testing.T) {
	_, err := GetTemplatePaths("python", "mastra")
	if err == nil {
		t.Fatal("expected error for unsupported language, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported language") {
		t.Errorf("error = %q, want message containing 'unsupported language'", err.Error())
	}
}

// --- Embedded template content tests ---

func TestGetTemplatePaths_AllEmbeddedFilesExist(t *testing.T) {
	paths, err := GetTemplatePaths("ts", "mastra")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}

	allPaths := []string{
		paths.AstroYml, paths.Dockerfile, paths.DockerfileIngestion,
		paths.PackageJson, paths.Tsconfig,
		paths.Gitignore, paths.Dockerignore, paths.Npmrc,
		paths.AgentIndex, paths.IngestionIndex, paths.LlmMd,
		paths.Readme, paths.PostmanCollection, paths.IngestionWebhookIndex,
	}

	for _, p := range allPaths {
		content, err := GetTemplate(p)
		if err != nil {
			t.Errorf("GetTemplate(%q): %v", p, err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("GetTemplate(%q) returned empty content", p)
		}
	}
}

// --- Template rendering tests ---

// renderTemplate is a helper that renders a template path with the given config.
func renderTemplate(t *testing.T, templatePath string, config ScaffoldConfig) string {
	t.Helper()
	tmplStr, err := GetTemplate(templatePath)
	if err != nil {
		t.Fatalf("GetTemplate(%q): %v", templatePath, err)
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(templateFuncs).Parse(tmplStr)
	if err != nil {
		t.Fatalf("template.Parse: %v", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, config); err != nil {
		t.Fatalf("template.Execute: %v", err)
	}
	return buf.String()
}

var defaultConfig = ScaffoldConfig{
	Name:            "test-agent",
	Description:     "A test agent",
	Interfaces:      []string{"web"},
	Integrations:    []string{},
	IntegrationKeys: map[string]string{},
	Knowledge:       []string{},
	Ingestions:      []string{},
}

func TestMastraTemplate_AgentIndex_UsesMastraImports(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if !strings.Contains(content, `@mastra/core/agent`) {
		t.Error("mastra agent/index.ts should import from @mastra/core/agent")
	}
	if !strings.Contains(content, `@astropods/adapter-mastra`) {
		t.Error("mastra agent/index.ts should import from @astropods/adapter-mastra")
	}
	if !strings.Contains(content, `serve(agent)`) {
		t.Error("mastra agent/index.ts should call serve(agent)")
	}
}

func TestMastraTemplate_AgentIndex_DoesNotUseAstroAgent(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	// Strip adapter references, then check if the standalone @saswatds/astro-agent package remains
	withoutAdapters := strings.ReplaceAll(content, "@astropods/adapter-mastra", "")
	if strings.Contains(withoutAdapters, "@saswatds/astro-agent") {
		t.Error("mastra agent/index.ts should not import @saswatds/astro-agent")
	}
	if strings.Contains(content, "AstroAgent") {
		t.Error("mastra agent/index.ts should not reference AstroAgent class")
	}
	if strings.Contains(content, "MessagingClient") {
		t.Error("mastra agent/index.ts should not reference MessagingClient")
	}
}

func TestMastraTemplate_PackageJson_HasMastraDeps(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	content := renderTemplate(t, paths.PackageJson, defaultConfig)

	if !strings.Contains(content, `"@mastra/core"`) {
		t.Error("mastra package.json should depend on @mastra/core")
	}
	if !strings.Contains(content, `"@astropods/adapter-mastra"`) {
		t.Error("mastra package.json should depend on @astropods/adapter-mastra")
	}
}

func TestMastraTemplate_PackageJson_DoesNotHaveAstroAgentDeps(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	content := renderTemplate(t, paths.PackageJson, defaultConfig)

	if strings.Contains(content, `"@saswatds/astro-agent"`) {
		t.Error("mastra package.json should not depend on @saswatds/astro-agent")
	}
	if strings.Contains(content, `"@astropods/messaging"`) {
		t.Error("mastra package.json should not depend on @astropods/messaging")
	}
}

// --- Template variable substitution tests ---

func TestMastraTemplate_AgentIndex_IdAndName(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	cases := []struct {
		name     string
		wantID   string
		wantName string
	}{
		{"weather-agent", "weather-agent", "Weather Agent"},
		{"my-cool-bot", "my-cool-bot", "My Cool Bot"},
		{"simple", "simple", "Simple"},
	}
	for _, tc := range cases {
		config := defaultConfig
		config.Name = tc.name
		content := renderTemplate(t, paths.AgentIndex, config)
		if want := "id: '" + tc.wantID + "'"; !strings.Contains(content, want) {
			t.Errorf("name=%q: expected %q, got:\n%s", tc.name, want, content)
		}
		if want := "name: '" + tc.wantName + "'"; !strings.Contains(content, want) {
			t.Errorf("name=%q: expected %q, got:\n%s", tc.name, want, content)
		}
	}
}

func TestMastraTemplate_AgentIndex_SubstitutesName(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if !strings.Contains(content, `id: 'test-agent'`) {
		t.Errorf("mastra agent/index.ts should contain agent id, got:\n%s", content)
	}
	if !strings.Contains(content, `name: 'Test Agent'`) {
		t.Errorf("mastra agent/index.ts should contain human-readable agent name, got:\n%s", content)
	}
}

func TestMastraTemplate_AgentIndex_EscapesSingleQuotesInDescription(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	config := defaultConfig
	config.Description = "It's a helper that does O'Brien's work"
	content := renderTemplate(t, paths.AgentIndex, config)

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "instructions:") {
			if strings.Contains(line, "It's") || strings.Contains(line, "O'Brien") {
				t.Errorf("unescaped single quote in instructions line would break JS string literal:\n%s", line)
			}
			if !strings.Contains(line, `It\'s`) {
				t.Errorf("expected escaped single quote in instructions line:\n%s", line)
			}
			return
		}
	}
	t.Error("instructions: line not found in rendered template")
}

func TestMastraTemplate_AgentIndex_SubstitutesDescription(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if !strings.Contains(content, "A test agent") {
		t.Errorf("mastra agent/index.ts should contain description, got:\n%s", content)
	}
}

func TestMastraTemplate_AgentIndex_AnthropicModel(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	config := defaultConfig
	config.Integrations = []string{"anthropic"}
	content := renderTemplate(t, paths.AgentIndex, config)

	if !strings.Contains(content, "anthropic/claude") {
		t.Errorf("mastra agent with anthropic integration should use anthropic model, got:\n%s", content)
	}
}

func TestMastraTemplate_AgentIndex_OpenAIModel(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	config := defaultConfig
	config.Integrations = []string{"openai"}
	content := renderTemplate(t, paths.AgentIndex, config)

	if !strings.Contains(content, "openai/gpt") {
		t.Errorf("mastra agent with openai integration should use openai model, got:\n%s", content)
	}
}

func TestMastraTemplate_AgentIndex_CustomModelProvider(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	config := defaultConfig
	config.ModelProvider = "ollama"
	config.Model = "llama3"
	content := renderTemplate(t, paths.AgentIndex, config)

	if !strings.Contains(content, "ollama/llama3") {
		t.Errorf("mastra agent with custom model should use ollama/llama3, got:\n%s", content)
	}
}

func TestMastraTemplate_PackageJson_SubstitutesName(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	content := renderTemplate(t, paths.PackageJson, defaultConfig)

	if !strings.Contains(content, `"name": "test-agent"`) {
		t.Errorf("mastra package.json should contain agent name, got:\n%s", content)
	}
}

// --- GenerateFiles end-to-end tests ---

func TestGenerateFiles_MastraTemplate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-agent")

	err := GenerateFiles(target, defaultConfig, "ts", "mastra")
	if err != nil {
		t.Fatalf("GenerateFiles(mastra): %v", err)
	}

	// Verify agent/index.ts exists and has mastra content
	agentContent, err := os.ReadFile(filepath.Join(target, "agent", "index.ts"))
	if err != nil {
		t.Fatalf("read agent/index.ts: %v", err)
	}
	if !strings.Contains(string(agentContent), "@mastra/core/agent") {
		t.Error("generated agent/index.ts should contain @mastra/core/agent import")
	}
	if !strings.Contains(string(agentContent), "serve(agent)") {
		t.Error("generated agent/index.ts should call serve(agent)")
	}

	// Verify package.json has mastra deps
	pkgContent, err := os.ReadFile(filepath.Join(target, "package.json"))
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	if !strings.Contains(string(pkgContent), `"@mastra/core"`) {
		t.Error("generated package.json should depend on @mastra/core")
	}

	// Verify shared files exist
	for _, f := range []string{"astropods.yml", "Dockerfile", "tsconfig.json", ".env", ".gitignore", ".dockerignore", ".npmrc"} {
		if _, err := os.Stat(filepath.Join(target, f)); os.IsNotExist(err) {
			t.Errorf("expected file %q to exist", f)
		}
	}

	// Verify docs files exist
	for _, f := range []string{"CLAUDE.md", "AGENTS.md", "README.md"} {
		if _, err := os.Stat(filepath.Join(target, f)); os.IsNotExist(err) {
			t.Errorf("expected file %q to exist", f)
		}
	}

	// Verify messaging Postman collection exists and has expected content
	postmanPath := filepath.Join(target, "postman", "collections", "messaging.postman_collection.json")
	postmanData, err := os.ReadFile(postmanPath)
	if err != nil {
		t.Fatalf("expected postman/collections/messaging.postman_collection.json to exist: %v", err)
	}
	if !strings.Contains(string(postmanData), `"name": "Messaging Web API"`) {
		t.Error("messaging.postman_collection.json should contain Messaging Web API collection name")
	}

	// Verify webhook Postman collection is NOT generated without webhook ingestion
	webhookPostmanPath := filepath.Join(target, "postman", "collections", "webhook.postman_collection.json")
	if _, err := os.Stat(webhookPostmanPath); !os.IsNotExist(err) {
		t.Error("webhook.postman_collection.json should not be generated without webhook ingestion")
	}
}

func TestGenerateFiles_WebhookPostmanCollection(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-agent")

	config := defaultConfig
	config.Ingestions = []string{"webhook"}

	if err := GenerateFiles(target, config, "ts", "mastra"); err != nil {
		t.Fatalf("GenerateFiles(mastra, webhook): %v", err)
	}

	// messaging collection should always be present
	if _, err := os.Stat(filepath.Join(target, "postman", "collections", "messaging.postman_collection.json")); os.IsNotExist(err) {
		t.Error("expected messaging.postman_collection.json to exist")
	}

	// webhook collection should be generated when webhook ingestion is selected
	webhookData, err := os.ReadFile(filepath.Join(target, "postman", "collections", "webhook.postman_collection.json"))
	if err != nil {
		t.Fatalf("expected webhook.postman_collection.json to exist: %v", err)
	}
	if !strings.Contains(string(webhookData), `"name": "Webhook Ingestion"`) {
		t.Error("webhook.postman_collection.json should contain Webhook Ingestion collection name")
	}
	if !strings.Contains(string(webhookData), `/webhook"`) {
		t.Error("webhook.postman_collection.json should contain /webhook endpoint")
	}
}

func TestMastraTemplate_AgentIndex_DefaultModel(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	config := defaultConfig // no integrations, no custom model
	content := renderTemplate(t, paths.AgentIndex, config)

	if !strings.Contains(content, "anthropic/claude-sonnet-4-5") {
		t.Errorf("mastra agent with no integration should default to anthropic/claude-sonnet-4-5, got:\n%s", content)
	}
}

func TestMastraTemplate_AgentIndex_SubstitutesInstructions(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if !strings.Contains(content, "test-agent") {
		t.Errorf("mastra agent/index.ts instructions should reference agent name, got:\n%s", content)
	}
	if !strings.Contains(content, "A test agent") {
		t.Errorf("mastra agent/index.ts instructions should reference description, got:\n%s", content)
	}
}

func TestMastraTemplate_PackageJson_SubstitutesDescription(t *testing.T) {
	paths, _ := GetTemplatePaths("ts", "mastra")
	content := renderTemplate(t, paths.PackageJson, defaultConfig)

	if !strings.Contains(content, `"description": "A test agent"`) {
		t.Errorf("mastra package.json should contain description, got:\n%s", content)
	}
}

func TestGenerateFiles_UnsupportedTemplate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-agent")

	err := GenerateFiles(target, defaultConfig, "ts", "invalid")
	if err == nil {
		t.Fatal("expected error for unsupported template, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported template") {
		t.Errorf("error = %q, want message containing 'unsupported template'", err.Error())
	}
}
