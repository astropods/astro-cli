package scaffold

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// Fs is the filesystem abstraction used by GenerateFiles.
type Fs interface {
	MkdirAll(path string, perm fs.FileMode) error
	WriteFile(path string, data []byte, perm fs.FileMode) error
}

// OsFs is the real filesystem implementation backed by the OS.
type OsFs struct{}

func (OsFs) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }
func (OsFs) WriteFile(path string, data []byte, perm fs.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// ScaffoldConfig holds the configuration for generating a new agent project.
type ScaffoldConfig struct {
	Name            string            // Agent name (required)
	Description     string            // Agent description
	Lang            string            // "ts" | "py" (set by GenerateFiles)
	Interfaces      []string          // ["web", "slack"]
	ModelProvider   string            // "ollama" | "huggingface" | "" for none
	Model           string            // Model name (e.g. "llama3", "mistral")
	Knowledge       []string          // ["qdrant", "redis", "neo4j"]
	Integrations    []string          // ["anthropic", "openai", "github"]
	IntegrationKeys map[string]string // integration name -> API key (optional, user-provided)
	Ingestions      []string          // e.g. ["schedule", "webhook"]
}

// HasKnowledge returns true if the given knowledge type is selected.
func (c ScaffoldConfig) HasKnowledge(k string) bool {
	for _, v := range c.Knowledge {
		if v == k {
			return true
		}
	}
	return false
}

// HasIngestion returns true if the given ingestion type is selected.
func (c ScaffoldConfig) HasIngestion(t string) bool {
	for _, v := range c.Ingestions {
		if v == t {
			return true
		}
	}
	return false
}

// HasIntegration returns true if the given integration is selected.
func (c ScaffoldConfig) HasIntegration(i string) bool {
	for _, v := range c.Integrations {
		if v == i {
			return true
		}
	}
	return false
}

// IntegrationKey returns the user-provided API key for an integration, or empty string.
func (c ScaffoldConfig) IntegrationKey(name string) string {
	if c.IntegrationKeys == nil {
		return ""
	}
	return c.IntegrationKeys[name]
}

// EnvVarInfo describes a single environment variable injected into the agent.
type EnvVarInfo struct {
	Key         string
	Description string
}

// AgentEnvVars returns the sorted list of environment variables that will be
// injected into the agent container based on the scaffold configuration.
// Descriptions use the form "injected by <provider> <component-type> [detail]".
func (c ScaffoldConfig) AgentEnvVars() []EnvVarInfo {
	vars := map[string]string{
		"GRPC_SERVER_ADDR": "injected by Astro messaging service",
	}
	if s, err := c.specFromTemplate(); err == nil {
		for k, meta := range spec.AllAgentAutoEnvKeys(s) {
			vars[k] = agentEnvDesc(k, meta)
		}
	}

	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]EnvVarInfo, 0, len(keys))
	for _, k := range keys {
		result = append(result, EnvVarInfo{Key: k, Description: vars[k]})
	}
	return result
}

// agentEnvDesc builds a human-readable description from AgentEnvMeta.
// Credential vars: "injected by <provider> <category>".
// Connection vars: same prefix + the key suffix as a detail (host/port/URL/…).
func agentEnvDesc(key string, meta spec.AgentEnvMeta) string {
	label := componentLabel(meta.Category)
	base := "injected by " + meta.Provider + " " + label
	if meta.Source != "connection" {
		return base
	}
	for _, s := range []struct{ suffix, detail string }{
		{"_BASE_URL", "base URL"},
		{"_HOST", "host"},
		{"_PORT", "port"},
		{"_URL", "URL"},
		{"_MODEL", "model name"},
	} {
		if strings.HasSuffix(key, s.suffix) {
			return base + " " + s.detail
		}
	}
	return base
}

// componentLabel maps a spec category to a readable component label.
func componentLabel(category string) string {
	if category == "knowledge" {
		return "knowledge store"
	}
	return category
}

// CollectEnvVars returns the user-provided API keys from IntegrationKeys mapped to
// their proper env var names (e.g. "anthropic" → "ANTHROPIC_API_KEY"). The mapping
// is derived from the spec that would be generated for this config, keeping it in
// sync with AllCredentialKeys automatically. Empty values are omitted.
func (c ScaffoldConfig) CollectEnvVars() map[string]string {
	vars := make(map[string]string)
	s, err := c.specFromTemplate()
	if err != nil {
		return vars
	}
	for envKey, meta := range spec.AllCredentialKeys(s) {
		if val := c.IntegrationKey(meta.Provider); val != "" {
			vars[envKey] = val
		}
	}
	if v := c.IntegrationKey("slack_bot_token"); v != "" {
		vars["SLACK_BOT_TOKEN"] = v
	}
	if v := c.IntegrationKey("slack_app_token"); v != "" {
		vars["SLACK_APP_TOKEN"] = v
	}
	return vars
}

// specFromTemplate renders the spec template and parses it into an
// AstroSpec. This is the single source of truth for what the spec will look
// like at runtime, so AgentEnvVars never drifts from the actual generated file.
func (c ScaffoldConfig) specFromTemplate() (*spec.AstroSpec, error) {
	templateName := "mastra"
	if c.Lang == "py" {
		templateName = "langchain"
	}
	paths, err := GetTemplatePaths(templateName)
	if err != nil {
		return nil, err
	}
	yaml, err := RenderTemplate(paths.AstroYml, c)
	if err != nil {
		return nil, err
	}
	return spec.ParseString(yaml)
}

// DefaultConfig returns a ScaffoldConfig with default values.
func DefaultConfig(name string) ScaffoldConfig {
	return ScaffoldConfig{
		Name:            name,
		Description:     "Describe what your agent does in one sentence.",
		Interfaces:      []string{"web"},
		ModelProvider:   "",
		Model:           "",
		Knowledge:       []string{},
		Integrations:    []string{"anthropic"},
		IntegrationKeys: map[string]string{},
		Ingestions:      []string{},
	}
}

// GenerateFiles creates all project files in the target directory using the OS filesystem.
// The templateName selects which agent scaffold to use ("mastra", "langchain").
func GenerateFiles(targetDir string, config ScaffoldConfig, templateName string) error {
	return generateFiles(OsFs{}, targetDir, config, templateName)
}

func generateFiles(fsys Fs, targetDir string, config ScaffoldConfig, templateName string) error {
	lang, _ := LangForTemplate(templateName)
	// Set lang in config so templates can reference {{.Lang}}
	config.Lang = lang

	paths, err := GetTemplatePaths(templateName)
	if err != nil {
		return err
	}

	// Create directory structure
	dirs := []string{
		targetDir,
		filepath.Join(targetDir, "agent"),
		filepath.Join(targetDir, "postman", "collections"),
	}
	if len(config.Ingestions) > 0 {
		dirs = append(dirs, filepath.Join(targetDir, "ingestion"))
		for _, ing := range config.Ingestions {
			dirs = append(dirs, filepath.Join(targetDir, "ingestion", ing))
		}
	}
	for _, dir := range dirs {
		if err := fsys.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Files common to all languages
	files := []struct {
		path         string
		templatePath string
	}{
		{filepath.Join(targetDir, "astropods.yml"), paths.AstroYml},
		{filepath.Join(targetDir, "Dockerfile"), paths.Dockerfile},
		{filepath.Join(targetDir, ".gitignore"), paths.Gitignore},
		{filepath.Join(targetDir, ".dockerignore"), paths.Dockerignore},
		{filepath.Join(targetDir, "CLAUDE.md"), paths.LlmMd},
		{filepath.Join(targetDir, "AGENTS.md"), paths.LlmMd},
		{filepath.Join(targetDir, spec.AgentReadmeFilename), paths.AgentMd},
		{filepath.Join(targetDir, "README.md"), paths.Readme},
	}

	// Language-specific files
	switch lang {
	case "ts":
		files = append(files,
			struct{ path, templatePath string }{filepath.Join(targetDir, "package.json"), paths.PackageJson},
			struct{ path, templatePath string }{filepath.Join(targetDir, "tsconfig.json"), paths.Tsconfig},
			struct{ path, templatePath string }{filepath.Join(targetDir, "agent", "index.ts"), paths.AgentIndex},
		)
		for _, ing := range config.Ingestions {
			ingestionTemplate := paths.IngestionIndex
			if ing == "webhook" {
				ingestionTemplate = paths.IngestionWebhookIndex
			}
			if err := writeIngestionDockerfile(fsys, filepath.Join(targetDir, "ingestion", ing, "Dockerfile"), paths.DockerfileIngestion, config, ing); err != nil {
				return err
			}
			files = append(files, struct{ path, templatePath string }{
				filepath.Join(targetDir, "ingestion", ing, "index.ts"), ingestionTemplate,
			})
		}
	case "py":
		files = append(files,
			struct{ path, templatePath string }{filepath.Join(targetDir, "requirements.txt"), paths.RequirementsTxt},
			struct{ path, templatePath string }{filepath.Join(targetDir, "agent", "main.py"), paths.AgentMain},
		)
		for _, ing := range config.Ingestions {
			ingestionTemplate := paths.IngestionMain
			if ing == "webhook" {
				ingestionTemplate = paths.IngestionWebhookPy
			}
			if err := writeIngestionDockerfile(fsys, filepath.Join(targetDir, "ingestion", ing, "Dockerfile"), paths.DockerfileIngestion, config, ing); err != nil {
				return err
			}
			files = append(files, struct{ path, templatePath string }{
				filepath.Join(targetDir, "ingestion", ing, "main.py"), ingestionTemplate,
			})
			files = append(files, struct{ path, templatePath string }{
				filepath.Join(targetDir, "ingestion", ing, "requirements.txt"), paths.IngestionRequirementsTxt,
			})
		}
	}

	for _, f := range files {
		if err := writeTemplateFromEmbed(fsys, f.path, f.templatePath, config); err != nil {
			return err
		}
	}

	// Copy static Postman collections (no templating)
	if err := copyStaticFile(fsys, filepath.Join(targetDir, "postman", "collections", "messaging.postman_collection.json"), paths.PostmanCollection); err != nil {
		return err
	}
	if config.HasIngestion("webhook") {
		if err := copyStaticFile(fsys, filepath.Join(targetDir, "postman", "collections", "webhook.postman_collection.json"), paths.PostmanWebhookCollection); err != nil {
			return err
		}
	}

	return nil
}

// RenderIngestionDockerfile renders the ingestion Dockerfile template for a specific type.
func RenderIngestionDockerfile(templatePath string, config ScaffoldConfig, ingType string) (string, error) {
	tmplStr, err := GetTemplate(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(templateFuncs).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ingestionDockerfileData{config, ingType}); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templatePath, err)
	}
	return buf.String(), nil
}

// templateFuncs returns the FuncMap available to all scaffold templates.
var templateFuncs = template.FuncMap{
	// jsStr escapes a string for safe embedding in a JS/TS single-quoted literal.
	"jsStr": func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `'`, `\'`)
		return s
	},
	// pyStr escapes a string for safe embedding in a Python double-quoted string literal.
	"pyStr": func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		return s
	},
	// humanName converts a kebab-case name to title-cased words (e.g. "my-agent" → "My Agent").
	"humanName": func(s string) string {
		parts := strings.Split(s, "-")
		for i, p := range parts {
			if len(p) > 0 {
				parts[i] = strings.ToUpper(p[:1]) + p[1:]
			}
		}
		return strings.Join(parts, " ")
	},
}

// RenderTemplate renders the named template with config and returns the result as a string.
// This is the same logic used when generating files; tests use it to get content without writing to disk.
func RenderTemplate(templatePath string, config ScaffoldConfig) (string, error) {
	tmplStr, err := GetTemplate(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Funcs(templateFuncs).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templatePath, err)
	}
	return buf.String(), nil
}

// ingestionDockerfileData extends ScaffoldConfig with the specific ingestion type
// so that Dockerfile.ingestion templates can reference {{.IngestionType}}.
type ingestionDockerfileData struct {
	ScaffoldConfig
	IngestionType string
}

func writeTemplateFromEmbed(fsys Fs, outputPath, templatePath string, config ScaffoldConfig) error {
	content, err := RenderTemplate(templatePath, config)
	if err != nil {
		return err
	}
	return fsys.WriteFile(outputPath, []byte(content), 0644)
}

func writeIngestionDockerfile(fsys Fs, outputPath, templatePath string, config ScaffoldConfig, ingType string) error {
	content, err := RenderIngestionDockerfile(templatePath, config, ingType)
	if err != nil {
		return err
	}
	return fsys.WriteFile(outputPath, []byte(content), 0644)
}

func copyStaticFile(fsys Fs, outputPath, embedPath string) error {
	data, err := GetTemplate(embedPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", embedPath, err)
	}
	return fsys.WriteFile(outputPath, []byte(data), 0644)
}
