package scaffold

import (
	"embed"
	"fmt"
)

//go:embed templates/*
var templateFS embed.FS

// TemplatePaths holds the paths to template files for a specific language
type TemplatePaths struct {
	AstroYml            string
	Dockerfile          string
	DockerfileIngestion string
	PackageJson         string
	Tsconfig            string
	EnvExample          string
	Gitignore           string
	Dockerignore        string
	AgentIndex          string
	IngestionIndex      string
}

// GetTemplatePaths returns the template paths for the specified language
func GetTemplatePaths(lang string) (*TemplatePaths, error) {
	switch lang {
	case "ts":
		return &TemplatePaths{
			AstroYml:            "templates/template-ts/astro.yml",
			Dockerfile:          "templates/template-ts/Dockerfile",
			DockerfileIngestion: "templates/template-ts/Dockerfile.ingestion",
			PackageJson:         "templates/template-ts/package.json",
			Tsconfig:            "templates/template-ts/tsconfig.json",
			EnvExample:          "templates/template-ts/env.example.tmpl",
			Gitignore:           "templates/template-ts/gitignore.tmpl",
			Dockerignore:        "templates/template-ts/dockerignore.tmpl",
			AgentIndex:          "templates/template-ts/agent/index.ts",
			IngestionIndex:      "templates/template-ts/ingestion/index.ts",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported language: %s", lang)
	}
}

// GetTemplate reads a template from the embedded filesystem
func GetTemplate(path string) (string, error) {
	data, err := templateFS.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
