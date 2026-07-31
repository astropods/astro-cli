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
	paths, err := GetTemplatePaths("mastra")
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
	_, err := GetTemplatePaths("nonexistent")
	if err == nil {
		t.Fatal("expected error for unsupported template, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported template") {
		t.Errorf("error = %q, want message containing 'unsupported template'", err.Error())
	}
}

// --- Embedded template content tests ---

func TestGetTemplatePaths_AllEmbeddedFilesExist(t *testing.T) {
	paths, err := GetTemplatePaths("mastra")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}

	allPaths := []string{
		paths.AstroYml, paths.Dockerfile, paths.DockerfileIngestion,
		paths.PackageJson, paths.Tsconfig,
		paths.Gitignore, paths.Dockerignore,
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
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if !strings.Contains(content, `@mastra/core/agent`) {
		t.Error("mastra agent/index.ts should import from @mastra/core/agent")
	}
	if !strings.Contains(content, `@mastra/core/mastra`) {
		t.Error("mastra agent/index.ts should import from @mastra/core/mastra")
	}
	if !strings.Contains(content, `@mastra/observability`) {
		t.Error("mastra agent/index.ts should import from @mastra/observability")
	}
	if !strings.Contains(content, `@mastra/otel-exporter`) {
		t.Error("mastra agent/index.ts should import from @mastra/otel-exporter")
	}
	if !strings.Contains(content, `@astropods/adapter-mastra`) {
		t.Error("mastra agent/index.ts should import from @astropods/adapter-mastra")
	}
	if !strings.Contains(content, `new Mastra({`) {
		t.Error("mastra agent/index.ts should instantiate Mastra runtime")
	}
	if !strings.Contains(content, `observability,`) {
		t.Error("mastra agent/index.ts should wire an observability object")
	}
	if !strings.Contains(content, `serve(agent)`) {
		t.Error("mastra agent/index.ts should call serve(agent)")
	}
}

func TestMastraTemplate_AgentIndex_IncludesTracingDefaults(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if !strings.Contains(content, "defaultOptions:") {
		t.Error("mastra agent/index.ts should include defaultOptions")
	}
	if !strings.Contains(content, "tracingOptions:") {
		t.Error("mastra agent/index.ts should include tracingOptions")
	}
	if !strings.Contains(content, "tags: ['astro', 'agent:test-agent']") {
		t.Error("mastra agent/index.ts should include astro tracing tags")
	}
}

func TestMastraTemplate_AgentIndex_DoesNotUseAstroAgent(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if strings.Contains(content, "AstroAgent") {
		t.Error("mastra agent/index.ts should not reference AstroAgent class")
	}
	if strings.Contains(content, "MessagingClient") {
		t.Error("mastra agent/index.ts should not reference MessagingClient")
	}
}

func TestMastraTemplate_PackageJson_HasMastraDeps(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.PackageJson, defaultConfig)

	if !strings.Contains(content, `"@mastra/core"`) {
		t.Error("mastra package.json should depend on @mastra/core")
	}
	if !strings.Contains(content, `"@mastra/observability"`) {
		t.Error("mastra package.json should depend on @mastra/observability")
	}
	if !strings.Contains(content, `"@mastra/otel-exporter"`) {
		t.Error("mastra package.json should depend on @mastra/otel-exporter")
	}
	if !strings.Contains(content, `"@opentelemetry/exporter-trace-otlp-proto"`) {
		t.Error("mastra package.json should depend on OTLP protobuf exporter")
	}
	if !strings.Contains(content, `"@astropods/adapter-mastra"`) {
		t.Error("mastra package.json should depend on @astropods/adapter-mastra")
	}
}

func TestMastraTemplate_PackageJson_DoesNotHaveAstroAgentDeps(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.PackageJson, defaultConfig)

	if strings.Contains(content, `"@astropods/messaging"`) {
		t.Error("mastra package.json should not depend on @astropods/messaging")
	}
}

// --- Template variable substitution tests ---

func TestMastraTemplate_AgentIndex_IdAndName(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
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
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if !strings.Contains(content, `id: 'test-agent'`) {
		t.Errorf("mastra agent/index.ts should contain agent id, got:\n%s", content)
	}
	if !strings.Contains(content, `name: 'Test Agent'`) {
		t.Errorf("mastra agent/index.ts should contain human-readable agent name, got:\n%s", content)
	}
}

func TestMastraTemplate_AgentIndex_EscapesSingleQuotesInDescription(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
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
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if !strings.Contains(content, "A test agent") {
		t.Errorf("mastra agent/index.ts should contain description, got:\n%s", content)
	}
}

func TestMastraTemplate_AgentIndex_AnthropicModel(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	config := defaultConfig
	config.Integrations = []string{"anthropic"}
	content := renderTemplate(t, paths.AgentIndex, config)

	if !strings.Contains(content, "anthropic/claude") {
		t.Errorf("mastra agent with anthropic integration should use anthropic model, got:\n%s", content)
	}
}

func TestMastraTemplate_AgentIndex_OpenAIModel(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	config := defaultConfig
	config.Integrations = []string{"openai"}
	content := renderTemplate(t, paths.AgentIndex, config)

	if !strings.Contains(content, "openai/gpt") {
		t.Errorf("mastra agent with openai integration should use openai model, got:\n%s", content)
	}
}

func TestMastraTemplate_PackageJson_SubstitutesName(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.PackageJson, defaultConfig)

	if !strings.Contains(content, `"name": "test-agent"`) {
		t.Errorf("mastra package.json should contain agent name, got:\n%s", content)
	}
}

// --- GenerateFiles end-to-end tests ---

func TestGenerateFiles_MastraTemplate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-agent")

	err := GenerateFiles(target, defaultConfig, "mastra")
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
	for _, f := range []string{"astropods.yml", "Dockerfile", "tsconfig.json", ".gitignore", ".dockerignore"} {
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

	if err := GenerateFiles(target, config, "mastra"); err != nil {
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
	paths, _ := GetTemplatePaths("mastra")
	config := defaultConfig // no integrations, no custom model
	content := renderTemplate(t, paths.AgentIndex, config)

	if !strings.Contains(content, "anthropic/claude-sonnet-4-5") {
		t.Errorf("mastra agent with no integration should default to anthropic/claude-sonnet-4-5, got:\n%s", content)
	}
}

func TestMastraTemplate_AgentIndex_SubstitutesInstructions(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.AgentIndex, defaultConfig)

	if !strings.Contains(content, "test-agent") {
		t.Errorf("mastra agent/index.ts instructions should reference agent name, got:\n%s", content)
	}
	if !strings.Contains(content, "A test agent") {
		t.Errorf("mastra agent/index.ts instructions should reference description, got:\n%s", content)
	}
}

func TestMastraTemplate_PackageJson_SubstitutesDescription(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	content := renderTemplate(t, paths.PackageJson, defaultConfig)

	if !strings.Contains(content, `"description": "A test agent"`) {
		t.Errorf("mastra package.json should contain description, got:\n%s", content)
	}
}

func TestGenerateFiles_UnsupportedTemplate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-agent")

	err := GenerateFiles(target, defaultConfig, "invalid")
	if err == nil {
		t.Fatal("expected error for unsupported template, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported template") {
		t.Errorf("error = %q, want message containing 'unsupported template'", err.Error())
	}
}

// --- Python/langchain template path tests ---

func TestGetTemplatePaths_LangchainUsesOverridePaths(t *testing.T) {
	paths, err := GetTemplatePaths("langchain")
	if err != nil {
		t.Fatalf("GetTemplatePaths(py, langchain): %v", err)
	}

	if !strings.Contains(paths.AgentMain, "template-py-langchain") {
		t.Errorf("AgentMain = %q, want path containing template-py-langchain", paths.AgentMain)
	}
	if !strings.Contains(paths.RequirementsTxt, "template-py-langchain") {
		t.Errorf("RequirementsTxt = %q, want path containing template-py-langchain", paths.RequirementsTxt)
	}
}

func TestGetTemplatePaths_Python_UnsupportedTemplate(t *testing.T) {
	_, err := GetTemplatePaths("nonexistent")
	if err == nil {
		t.Fatal("expected error for unsupported template, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported template") {
		t.Errorf("error = %q, want message containing 'unsupported template'", err.Error())
	}
}

func TestGetTemplatePaths_Python_AllEmbeddedFilesExist(t *testing.T) {
	paths, err := GetTemplatePaths("langchain")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}

	allPaths := []string{
		paths.AstroYml, paths.Dockerfile, paths.DockerfileIngestion,
		paths.Gitignore, paths.Dockerignore,
		paths.AgentMain, paths.RequirementsTxt,
		paths.IngestionMain, paths.IngestionWebhookPy, paths.IngestionRequirementsTxt,
		paths.LlmMd, paths.Readme, paths.PostmanCollection,
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

// --- Python/langchain template content tests ---

func TestLangchainTemplate_AgentMain_UsesLangchainImports(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	content := renderTemplate(t, paths.AgentMain, defaultConfig)

	if !strings.Contains(content, "langchain") {
		t.Error("langchain agent/main.py should import from langchain")
	}
	if !strings.Contains(content, "astropods_adapter_langchain") {
		t.Error("langchain agent/main.py should import from astropods_adapter_langchain")
	}
	if !strings.Contains(content, "serve(") {
		t.Error("langchain agent/main.py should call serve()")
	}
}

func TestLangchainTemplate_AgentMain_SubstitutesName(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	content := renderTemplate(t, paths.AgentMain, defaultConfig)

	if !strings.Contains(content, "test-agent") {
		t.Errorf("langchain agent/main.py should contain agent name, got:\n%s", content)
	}
	if !strings.Contains(content, "A test agent") {
		t.Errorf("langchain agent/main.py should contain description, got:\n%s", content)
	}
}

func TestLangchainTemplate_AgentMain_DefaultModel(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	config := defaultConfig // no integrations
	content := renderTemplate(t, paths.AgentMain, config)

	if !strings.Contains(content, "claude-sonnet-4-5") {
		t.Errorf("langchain agent with no integration should default to claude-sonnet-4-5, got:\n%s", content)
	}
}

func TestLangchainTemplate_AgentMain_AnthropicModel(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	config := defaultConfig
	config.Integrations = []string{"anthropic"}
	content := renderTemplate(t, paths.AgentMain, config)

	if !strings.Contains(content, "ChatAnthropic") {
		t.Errorf("langchain agent with anthropic integration should use ChatAnthropic, got:\n%s", content)
	}
}

func TestLangchainTemplate_AgentMain_OpenAIModel(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	config := defaultConfig
	config.Integrations = []string{"openai"}
	content := renderTemplate(t, paths.AgentMain, config)

	if !strings.Contains(content, "ChatOpenAI") {
		t.Errorf("langchain agent with openai integration should use ChatOpenAI, got:\n%s", content)
	}
}

func TestLangchainTemplate_RequirementsTxt_HasLangchainDeps(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	content := renderTemplate(t, paths.RequirementsTxt, defaultConfig)

	if !strings.Contains(content, "langchain") {
		t.Error("langchain requirements.txt should include langchain dependency")
	}
	if !strings.Contains(content, "astropods-adapter-langchain") {
		t.Error("langchain requirements.txt should include astropods-adapter-langchain dependency")
	}
}

func TestLangchainTemplate_RequirementsTxt_AnthropicDep(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	config := defaultConfig
	config.Integrations = []string{"anthropic"}
	content := renderTemplate(t, paths.RequirementsTxt, config)

	if !strings.Contains(content, "langchain-anthropic") {
		t.Errorf("requirements.txt with anthropic should include langchain-anthropic, got:\n%s", content)
	}
}

func TestLangchainTemplate_RequirementsTxt_OpenAIDep(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	config := defaultConfig
	config.Integrations = []string{"openai"}
	content := renderTemplate(t, paths.RequirementsTxt, config)

	if !strings.Contains(content, "langchain-openai") {
		t.Errorf("requirements.txt with openai should include langchain-openai, got:\n%s", content)
	}
	if strings.Contains(content, "langchain-anthropic") {
		t.Errorf("requirements.txt with only openai should not include langchain-anthropic, got:\n%s", content)
	}
}

func TestLangchainTemplate_AgentMain_EscapesDoubleQuotesInDescription(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	config := defaultConfig
	config.Description = `She said "hello" and it's fine`
	content := renderTemplate(t, paths.AgentMain, config)

	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "system_prompt") {
			if strings.Contains(line, `"hello"`) {
				t.Errorf("unescaped double quote in system_prompt would break Python string literal:\n%s", line)
			}
			if !strings.Contains(line, `\"hello\"`) {
				t.Errorf("expected escaped double quote in system_prompt line:\n%s", line)
			}
			return
		}
	}
	t.Error("system_prompt line not found in rendered template")
}

func TestLangchainTemplate_AgentMain_EnvVarsInDocstring(t *testing.T) {
	paths, _ := GetTemplatePaths("langchain")
	cfg := ScaffoldConfig{
		Name:            "test-agent",
		Description:     "A test agent",
		Integrations:    []string{"anthropic"},
		Knowledge:       []string{"qdrant"},
		IntegrationKeys: map[string]string{},
	}
	content := renderTemplate(t, paths.AgentMain, cfg)

	wantLines := []string{
		"GRPC_SERVER_ADDR - injected by Astro messaging service",
		"ANTHROPIC_API_KEY - injected by anthropic model",
		"QDRANT_HOST - injected by qdrant knowledge store host",
		"QDRANT_PORT - injected by qdrant knowledge store port",
	}
	for _, want := range wantLines {
		if !strings.Contains(content, want) {
			t.Errorf("rendered agent/main.py docstring should contain %q, got:\n%s", want, content)
		}
	}
}

// --- Python GenerateFiles end-to-end tests ---

func TestGenerateFiles_LangchainTemplate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-py-agent")

	err := GenerateFiles(target, defaultConfig, "langchain")
	if err != nil {
		t.Fatalf("GenerateFiles(langchain): %v", err)
	}

	// agent/main.py exists with langchain content
	agentContent, err := os.ReadFile(filepath.Join(target, "agent", "main.py"))
	if err != nil {
		t.Fatalf("read agent/main.py: %v", err)
	}
	if !strings.Contains(string(agentContent), "astropods_adapter_langchain") {
		t.Error("generated agent/main.py should import astropods_adapter_langchain")
	}
	if !strings.Contains(string(agentContent), "serve(") {
		t.Error("generated agent/main.py should call serve()")
	}

	// requirements.txt exists with langchain deps
	reqsContent, err := os.ReadFile(filepath.Join(target, "requirements.txt"))
	if err != nil {
		t.Fatalf("read requirements.txt: %v", err)
	}
	if !strings.Contains(string(reqsContent), "astropods-adapter-langchain") {
		t.Error("generated requirements.txt should include astropods-adapter-langchain")
	}

	// TypeScript-specific files must not exist
	for _, tsFile := range []string{"package.json", "tsconfig.json", "agent/index.ts"} {
		if _, err := os.Stat(filepath.Join(target, tsFile)); !os.IsNotExist(err) {
			t.Errorf("TypeScript file %q should not exist in Python scaffold", tsFile)
		}
	}

	// Shared files exist
	for _, f := range []string{"astropods.yml", "Dockerfile", ".gitignore", ".dockerignore"} {
		if _, err := os.Stat(filepath.Join(target, f)); os.IsNotExist(err) {
			t.Errorf("expected shared file %q to exist", f)
		}
	}

	// Docs files exist
	for _, f := range []string{"CLAUDE.md", "AGENTS.md", "README.md"} {
		if _, err := os.Stat(filepath.Join(target, f)); os.IsNotExist(err) {
			t.Errorf("expected file %q to exist", f)
		}
	}
}

func TestGenerateFiles_LangchainTemplate_WithIngestion(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-py-agent")

	config := defaultConfig
	config.Ingestions = []string{"webhook", "startup"}

	err := GenerateFiles(target, config, "langchain")
	if err != nil {
		t.Fatalf("GenerateFiles(langchain, ingestion): %v", err)
	}

	for _, ingType := range []string{"webhook", "startup"} {
		mainPy := filepath.Join(target, "ingestion", ingType, "main.py")
		reqsTxt := filepath.Join(target, "ingestion", ingType, "requirements.txt")
		dockerfile := filepath.Join(target, "ingestion", ingType, "Dockerfile")

		if _, err := os.Stat(mainPy); os.IsNotExist(err) {
			t.Errorf("expected ingestion/%s/main.py to exist", ingType)
		}
		if _, err := os.Stat(reqsTxt); os.IsNotExist(err) {
			t.Errorf("expected ingestion/%s/requirements.txt to exist", ingType)
		}
		if _, err := os.Stat(dockerfile); os.IsNotExist(err) {
			t.Errorf("expected ingestion/%s/Dockerfile to exist", ingType)
		}

		// Dockerfile must install from requirements.txt and set PYTHONUNBUFFERED
		dfContent, err := os.ReadFile(dockerfile)
		if err != nil {
			t.Fatalf("read ingestion/%s/Dockerfile: %v", ingType, err)
		}
		if !strings.Contains(string(dfContent), "pip install") {
			t.Errorf("ingestion/%s/Dockerfile should install pip dependencies", ingType)
		}
		if !strings.Contains(string(dfContent), "PYTHONUNBUFFERED=1") {
			t.Errorf("ingestion/%s/Dockerfile should set PYTHONUNBUFFERED=1", ingType)
		}
	}
}

func TestGenerateFiles_Python_UnsupportedTemplate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-py-agent")

	err := GenerateFiles(target, defaultConfig, "invalid")
	if err == nil {
		t.Fatal("expected error for unsupported template, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported template") {
		t.Errorf("error = %q, want message containing 'unsupported template'", err.Error())
	}
}
