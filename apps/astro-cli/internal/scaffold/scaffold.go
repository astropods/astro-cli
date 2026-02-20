package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// ScaffoldConfig holds the configuration for generating a new agent project.
type ScaffoldConfig struct {
	Name         string   // Agent name (required)
	Description  string   // Agent description
	Interfaces   []string // ["web", "slack"]
	ModelProvider string  // "ollama" | "huggingface" | "" for none
	Model         string  // Model name (e.g. "llama3", "mistral")
	Knowledge    []string // ["qdrant", "redis", "neo4j"]
	Integrations    []string          // ["anthropic", "openai", "github"]
	IntegrationKeys map[string]string // integration name -> API key (optional, user-provided)
	Ingestion       string            // "schedule" | "webhook" | "manual" | "startup" | "none"
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

// DefaultConfig returns a ScaffoldConfig with default values.
func DefaultConfig(name string) ScaffoldConfig {
	return ScaffoldConfig{
		Name:         name,
		Description:  "An AI-powered agent",
		Interfaces:   []string{"web"},
		ModelProvider: "",
		Model:         "",
		Knowledge:    []string{},
		Integrations:    []string{},
		IntegrationKeys: map[string]string{},
		Ingestion:       "none",
	}
}

// GenerateFiles creates all project files in the target directory.
func GenerateFiles(targetDir string, config ScaffoldConfig, lang string) error {
	// Get template paths for the specified language
	paths, err := GetTemplatePaths(lang)
	if err != nil {
		return err
	}

	// Create directory structure
	dirs := []string{
		targetDir,
		filepath.Join(targetDir, "agent"),
		filepath.Join(targetDir, "ingestion"),
		filepath.Join(targetDir, ".postman", "collections"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	// Generate files from templates
	files := []struct {
		path         string
		templatePath string
	}{
		{filepath.Join(targetDir, "astroai.yml"), paths.AstroYml},
		{filepath.Join(targetDir, "Dockerfile"), paths.Dockerfile},
		{filepath.Join(targetDir, "package.json"), paths.PackageJson},
		{filepath.Join(targetDir, "tsconfig.json"), paths.Tsconfig},
		{filepath.Join(targetDir, ".env"), paths.EnvExample},
		{filepath.Join(targetDir, ".gitignore"), paths.Gitignore},
		{filepath.Join(targetDir, ".dockerignore"), paths.Dockerignore},
		{filepath.Join(targetDir, ".npmrc"), paths.Npmrc},
		{filepath.Join(targetDir, "agent", "index.ts"), paths.AgentIndex},
		{filepath.Join(targetDir, "CLAUDE.md"), paths.LlmMd},
		{filepath.Join(targetDir, "AGENTS.md"), paths.LlmMd},
		{filepath.Join(targetDir, "README.md"), paths.Readme},
	}

	// Add ingestion files if ingestion is enabled
	if config.Ingestion != "none" {
		files = append(files, struct {
			path         string
			templatePath string
		}{filepath.Join(targetDir, "Dockerfile.ingestion"), paths.DockerfileIngestion})

		ingestionTemplate := paths.IngestionIndex
		if config.Ingestion == "webhook" {
			ingestionTemplate = paths.IngestionWebhookIndex
		}
		files = append(files, struct {
			path         string
			templatePath string
		}{filepath.Join(targetDir, "ingestion", "index.ts"), ingestionTemplate})
	}

	for _, f := range files {
		if err := writeTemplateFromEmbed(f.path, f.templatePath, config); err != nil {
			return err
		}
	}

	// Copy static Postman collection (no templating)
	if err := copyStaticFile(filepath.Join(targetDir, ".postman", "collections", "Astro-API.postman_collection.json"), paths.PostmanCollection); err != nil {
		return err
	}

	return nil
}

func writeTemplateFromEmbed(outputPath, templatePath string, config ScaffoldConfig) error {
	// Read template from embedded filesystem
	tmplStr, err := GetTemplate(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}

	tmpl, err := template.New(filepath.Base(templatePath)).Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	return tmpl.Execute(file, config)
}

func copyStaticFile(outputPath, embedPath string) error {
	data, err := GetTemplate(embedPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", embedPath, err)
	}
	return os.WriteFile(outputPath, []byte(data), 0644)
}
