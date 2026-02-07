package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// ScaffoldConfig holds the configuration for generating a new agent project.
type ScaffoldConfig struct {
	Name        string   // Agent name (required)
	Description string   // Agent description
	Interfaces  []string // ["web", "slack"]
	Model       string   // "anthropic" | "openai" | "none"
	ModelApiKey string   // Optional API key for the selected model (written to .env)
	Knowledge   string   // "vector" | "kv" | "both" | "none"
	Tools       []string // ["github"]
	Ingestion   string   // "schedule" | "manual" | "none"
}

// DefaultConfig returns a ScaffoldConfig with default values.
func DefaultConfig(name string) ScaffoldConfig {
	return ScaffoldConfig{
		Name:        name,
		Description: "An AI-powered agent",
		Interfaces:  []string{"web"},
		Model:       "openai",
		Knowledge:   "none",
		Tools:       []string{},
		Ingestion:   "none",
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
		{filepath.Join(targetDir, "astro.yml"), paths.AstroYml},
		{filepath.Join(targetDir, "Dockerfile"), paths.Dockerfile},
		{filepath.Join(targetDir, "package.json"), paths.PackageJson},
		{filepath.Join(targetDir, "tsconfig.json"), paths.Tsconfig},
		{filepath.Join(targetDir, ".env"), paths.EnvExample},
		{filepath.Join(targetDir, ".gitignore"), paths.Gitignore},
		{filepath.Join(targetDir, ".dockerignore"), paths.Dockerignore},
		{filepath.Join(targetDir, "agent", "index.ts"), paths.AgentIndex},
		{filepath.Join(targetDir, "ingestion", "index.ts"), paths.IngestionIndex},
		{filepath.Join(targetDir, "CLAUDE.md"), paths.LlmMd},
		{filepath.Join(targetDir, "AGENTS.md"), paths.LlmMd},
	}

	// Add Dockerfile.ingestion if ingestion is enabled
	if config.Ingestion != "none" {
		files = append(files, struct {
			path         string
			templatePath string
		}{filepath.Join(targetDir, "Dockerfile.ingestion"), paths.DockerfileIngestion})
	}

	for _, f := range files {
		if err := writeTemplateFromEmbed(f.path, f.templatePath, config); err != nil {
			return err
		}
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
