package scaffold

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"
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

// DefaultConfig returns a ScaffoldConfig with default values.
func DefaultConfig(name string) ScaffoldConfig {
	return ScaffoldConfig{
		Name:            name,
		Description:     "An AI-powered agent",
		Interfaces:      []string{"web"},
		ModelProvider:   "",
		Model:           "",
		Knowledge:       []string{},
		Integrations:    []string{},
		IntegrationKeys: map[string]string{},
		Ingestions:      []string{},
	}
}

// GenerateFiles creates all project files in the target directory using the OS filesystem.
func GenerateFiles(targetDir string, config ScaffoldConfig, lang string) error {
	return generateFiles(OsFs{}, targetDir, config, lang)
}

func generateFiles(fsys Fs, targetDir string, config ScaffoldConfig, lang string) error {
	// Get template paths for the specified language
	paths, err := GetTemplatePaths(lang)
	if err != nil {
		return err
	}

	// Create directory structure
	dirs := []string{
		targetDir,
		filepath.Join(targetDir, "agent"),
		filepath.Join(targetDir, "postman", "collections"),
	}
	for _, ing := range config.Ingestions {
		dirs = append(dirs, filepath.Join(targetDir, "ingestion", ing))
	}

	for _, dir := range dirs {
		if err := fsys.MkdirAll(dir, 0755); err != nil {
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

	// Add per-ingestion-type files: ingestion/<type>/Dockerfile and ingestion/<type>/index.ts
	for _, ing := range config.Ingestions {
		ingestionTemplate := paths.IngestionIndex
		if ing == "webhook" {
			ingestionTemplate = paths.IngestionWebhookIndex
		}
		if err := writeIngestionDockerfile(fsys, filepath.Join(targetDir, "ingestion", ing, "Dockerfile"), paths.DockerfileIngestion, config, ing); err != nil {
			return err
		}
		files = append(files, struct {
			path         string
			templatePath string
		}{filepath.Join(targetDir, "ingestion", ing, "index.ts"), ingestionTemplate})
	}

	for _, f := range files {
		if err := writeTemplateFromEmbed(fsys, f.path, f.templatePath, config); err != nil {
			return err
		}
	}

	// Copy static Postman collection (no templating)
	if err := copyStaticFile(fsys, filepath.Join(targetDir, "postman", "collections", "Astro-API.postman_collection.json"), paths.PostmanCollection); err != nil {
		return err
	}

	return nil
}

// RenderIngestionDockerfile renders the ingestion Dockerfile template for a specific type.
func RenderIngestionDockerfile(templatePath string, config ScaffoldConfig, ingType string) (string, error) {
	tmplStr, err := GetTemplate(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %s: %w", templatePath, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ingestionDockerfileData{config, ingType}); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templatePath, err)
	}
	return buf.String(), nil
}

// RenderTemplate renders the named template with config and returns the result as a string.
// This is the same logic used when generating files; tests use it to get content without writing to disk.
func RenderTemplate(templatePath string, config ScaffoldConfig) (string, error) {
	tmplStr, err := GetTemplate(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template %s: %w", templatePath, err)
	}
	tmpl, err := template.New(filepath.Base(templatePath)).Parse(tmplStr)
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
