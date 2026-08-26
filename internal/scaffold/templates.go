package scaffold

import (
	"embed"
	"fmt"
)

//go:embed all:templates
var templateFS embed.FS

// TemplatePaths holds the paths to template files for a specific language
type TemplatePaths struct {
	AstroYml                 string
	Dockerfile               string
	DockerfileIngestion      string
	PackageJson              string
	Tsconfig                 string
	Gitignore                string
	Dockerignore             string
	AgentIndex               string
	AgentMain                string // Python: agent/main.py
	RequirementsTxt          string // Python: requirements.txt
	IngestionIndex           string
	IngestionMain            string // Python: ingestion/main.py
	IngestionWebhookPy       string // Python: ingestion/webhook.py
	IngestionRequirementsTxt string // Python: ingestion/<type>/requirements.txt
	LlmMd                    string
	AgentMd                  string
	Readme                   string
	PostmanCollection        string
	PostmanWebhookCollection string
	IngestionWebhookIndex    string
	// SrcTree is an embedded directory rendered file-for-file into the project,
	// for templates whose sources are a tree rather than a single entry point.
	SrcTree string
}

// templateToLang maps each template name to its language.
var templateToLang = map[string]string{
	"mastra":    "ts",
	"langchain": "py",
}

// LangForTemplate returns the language for the given template name.
func LangForTemplate(templateName string) (string, bool) {
	lang, ok := templateToLang[templateName]
	return lang, ok
}

// GetTemplatePaths returns the template paths for the specified template.
// The templateName selects which agent scaffold to use ("mastra" for ts, "langchain" for py).
// Shared files (astropods.yml, AGENT.md, agents.md) always come from template-ts/.
func GetTemplatePaths(templateName string) (*TemplatePaths, error) {
	switch templateName {
	case "mastra":
		return &TemplatePaths{
			AstroYml:                 "templates/template-ts/astropods.yml",
			Dockerfile:               "templates/template-ts-mastra/Dockerfile",
			DockerfileIngestion:      "templates/template-ts/Dockerfile.ingestion",
			Tsconfig:                 "templates/template-ts-mastra/tsconfig.json",
			Gitignore:                "templates/template-ts/gitignore.tmpl",
			Dockerignore:             "templates/template-ts/dockerignore.tmpl",
			IngestionIndex:           "templates/template-ts/ingestion/index.ts",
			LlmMd:                    "templates/template-ts/agents.md.tmpl",
			AgentMd:                  "templates/template-ts/AGENT.md.tmpl",
			Readme:                   "templates/template-ts/README.md.tmpl",
			PostmanCollection:        "templates/template-ts/postman/collections/messaging.postman_collection.json",
			PostmanWebhookCollection: "templates/template-ts/postman/collections/webhook.postman_collection.json",
			IngestionWebhookIndex:    "templates/template-ts/ingestion/webhook.ts",
			SrcTree:                  "templates/template-ts-mastra/src",
			PackageJson:              "templates/template-ts-mastra/package.json",
		}, nil

	case "langchain":
		return &TemplatePaths{
			AstroYml:                 "templates/template-ts/astropods.yml",
			Dockerfile:               "templates/template-py/Dockerfile",
			DockerfileIngestion:      "templates/template-py/Dockerfile.ingestion",
			Gitignore:                "templates/template-py/gitignore.tmpl",
			Dockerignore:             "templates/template-py/dockerignore.tmpl",
			LlmMd:                    "templates/template-ts/agents.md.tmpl",
			AgentMd:                  "templates/template-ts/AGENT.md.tmpl",
			Readme:                   "templates/template-py/README.md.tmpl",
			PostmanCollection:        "templates/template-ts/postman/collections/messaging.postman_collection.json",
			PostmanWebhookCollection: "templates/template-ts/postman/collections/webhook.postman_collection.json",
			IngestionMain:            "templates/template-py/ingestion/main.py",
			IngestionWebhookPy:       "templates/template-py/ingestion/webhook.py",
			IngestionRequirementsTxt: "templates/template-py/ingestion/requirements.txt",
			AgentMain:                "templates/template-py-langchain/agent/main.py",
			RequirementsTxt:          "templates/template-py-langchain/requirements.txt",
		}, nil

	default:
		// unreachable: templateToLang check above already caught unknowns
		return nil, fmt.Errorf("unsupported template: %s", templateName)
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
