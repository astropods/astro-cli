package scaffold

import (
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
func GenerateFiles(targetDir string, config ScaffoldConfig) error {
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
		path     string
		template string
	}{
		{filepath.Join(targetDir, "astro.yml"), astroYmlTemplate},
		{filepath.Join(targetDir, "Dockerfile"), dockerfileTemplate},
		{filepath.Join(targetDir, "package.json"), packageJsonTemplate},
		{filepath.Join(targetDir, "tsconfig.json"), tsconfigTemplate},
		{filepath.Join(targetDir, ".env.example"), envExampleTemplate},
		{filepath.Join(targetDir, ".gitignore"), gitignoreTemplate},
		{filepath.Join(targetDir, ".dockerignore"), dockerignoreTemplate},
		{filepath.Join(targetDir, "agent", "index.ts"), agentIndexTemplate},
		{filepath.Join(targetDir, "ingestion", "index.ts"), ingestionIndexTemplate},
	}

	// Add Dockerfile.ingestion if ingestion is enabled
	if config.Ingestion != "none" {
		files = append(files, struct {
			path     string
			template string
		}{filepath.Join(targetDir, "Dockerfile.ingestion"), dockerfileIngestionTemplate})
	}

	for _, f := range files {
		if err := writeTemplate(f.path, f.template, config); err != nil {
			return err
		}
	}

	return nil
}

func writeTemplate(path, tmplStr string, config ScaffoldConfig) error {
	tmpl, err := template.New("file").Parse(tmplStr)
	if err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return tmpl.Execute(file, config)
}
