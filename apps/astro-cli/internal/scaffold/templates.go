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
}

// GetTemplatePaths returns the template paths for the specified language and template.
// The templateName selects which agent scaffold to use ("mastra" for ts, "langchain" for py).
// Shared files (astropods.yml, AGENT.md, agents.md) always come from template-ts/.
func GetTemplatePaths(lang string, templateName string) (*TemplatePaths, error) {
	switch lang {
	case "ts":
		paths := &TemplatePaths{
			AstroYml:                 "templates/template-ts/astropods.yml",
			Dockerfile:               "templates/template-ts/Dockerfile",
			DockerfileIngestion:      "templates/template-ts/Dockerfile.ingestion",
			Tsconfig:                 "templates/template-ts/tsconfig.json",
			Gitignore:                "templates/template-ts/gitignore.tmpl",
			Dockerignore:             "templates/template-ts/dockerignore.tmpl",
			IngestionIndex:           "templates/template-ts/ingestion/index.ts",
			LlmMd:                    "templates/template-ts/agents.md.tmpl",
			AgentMd:                  "templates/template-ts/AGENT.md.tmpl",
			Readme:                   "templates/template-ts/README.md.tmpl",
			PostmanCollection:        "templates/template-ts/postman/collections/messaging.postman_collection.json",
			PostmanWebhookCollection: "templates/template-ts/postman/collections/webhook.postman_collection.json",
			IngestionWebhookIndex:    "templates/template-ts/ingestion/webhook.ts",
		}
		switch templateName {
		case "mastra":
			paths.AgentIndex = "templates/template-ts-mastra/agent/index.ts"
			paths.PackageJson = "templates/template-ts-mastra/package.json"
		default:
			return nil, fmt.Errorf("unsupported template: %s (supported: mastra)", templateName)
		}
		return paths, nil

	case "py":
		paths := &TemplatePaths{
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
		}
		switch templateName {
		case "langchain":
			paths.AgentMain = "templates/template-py-langchain/agent/main.py"
			paths.RequirementsTxt = "templates/template-py-langchain/requirements.txt"
		default:
			return nil, fmt.Errorf("unsupported template: %s (supported: langchain)", templateName)
		}
		return paths, nil

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
