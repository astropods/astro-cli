package deployment

import (
	"fmt"
	"strings"

	"github.com/postman/astro/packages/astro-spec"
	"github.com/robfig/cron/v3"
)

// Validator validates agent specs and credentials
type Validator struct {
	cronParser cron.Parser
}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{
		cronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// ValidateSpec validates the agent spec and credentials
func (v *Validator) ValidateSpec(astroSpec *spec.AstroSpec, userCredentials map[string]string) ValidationResult {
	result := ValidationResult{
		Valid:              true,
		Errors:             []ValidationError{},
		MissingCredentials: []string{},
	}

	// Validate basic spec fields
	if astroSpec.Agent == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "agent",
			Message: "agent name is required",
		})
	}

	if astroSpec.Meta.Version == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "meta.version",
			Message: "version is required",
		})
	}

	// Validate container
	if astroSpec.Container.Image == "" && astroSpec.Container.Build == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "container",
			Message: "either image or build configuration is required",
		})
	}

	// Validate cron expressions in injections
	for name, injection := range astroSpec.Injections {
		if injection.Trigger.Type == "schedule" {
			if injection.Trigger.Cron == "" {
				result.Valid = false
				result.Errors = append(result.Errors, ValidationError{
					Field:   fmt.Sprintf("injections.%s.trigger.cron", name),
					Message: "cron expression is required for cron trigger",
				})
			} else {
				// Validate cron expression
				if _, err := v.cronParser.Parse(injection.Trigger.Cron); err != nil {
					result.Valid = false
					result.Errors = append(result.Errors, ValidationError{
						Field:   fmt.Sprintf("injections.%s.trigger.cron", name),
						Message: fmt.Sprintf("invalid cron expression: %v", err),
					})
				}
			}
		}
	}

	// Collect required credentials
	requiredCreds := v.collectRequiredCredentials(astroSpec)

	// Check for missing credentials
	for _, credKey := range requiredCreds {
		if _, exists := userCredentials[credKey]; !exists {
			result.Valid = false
			result.MissingCredentials = append(result.MissingCredentials, credKey)
		}
	}

	return result
}

// collectRequiredCredentials identifies all required credentials from the spec
func (v *Validator) collectRequiredCredentials(astroSpec *spec.AstroSpec) []string {
	credSet := make(map[string]bool)

	// Check integration models (cloud providers)
	for _, model := range astroSpec.Integrations.Models {
		credKey := v.getCredentialKeyForProvider(model.Provider)
		if credKey != "" {
			if model.Env != nil && model.Env.Prefix != "" {
				credKey = model.Env.Prefix + credKey
			}
			credSet[credKey] = true
		}
	}

	// Check integration knowledge stores (cloud providers)
	for _, knowledge := range astroSpec.Integrations.Knowledge {
		credKey := v.getCredentialKeyForProvider(knowledge.Provider)
		if credKey != "" {
			if knowledge.Env != nil && knowledge.Env.Prefix != "" {
				credKey = knowledge.Env.Prefix + credKey
			}
			credSet[credKey] = true
		}
	}

	// Check integration tools
	for _, tool := range astroSpec.Integrations.Tools {
		credKey := v.getCredentialKeyForProvider(tool.Provider)
		if credKey != "" {
			if tool.Env != nil && tool.Env.Prefix != "" {
				credKey = tool.Env.Prefix + credKey
			}
			credSet[credKey] = true
		}
	}

	// Check messaging interfaces
	for _, iface := range astroSpec.Interfaces {
		ifaceType := strings.ToLower(iface.Type)

		if ifaceType == "slack" || ifaceType == "messaging/slack" || strings.Contains(ifaceType, "slack") {
			credSet["SLACK_APP_TOKEN"] = true
			credSet["SLACK_BOT_TOKEN"] = true
		} else if ifaceType == "discord" || ifaceType == "messaging/discord" || strings.Contains(ifaceType, "discord") {
			credSet["DISCORD_BOT_TOKEN"] = true
		}
	}

	// Check injection sources
	for _, injection := range astroSpec.Injections {
		if injection.Source.Type == "github" {
			credSet["GITHUB_TOKEN"] = true
		} else if injection.Source.Type == "gitlab" {
			credSet["GITLAB_TOKEN"] = true
		}
	}

	// Convert set to slice
	var creds []string
	for cred := range credSet {
		creds = append(creds, cred)
	}

	return creds
}

// CredentialInfo provides metadata about a required credential
type CredentialInfo struct {
	Key         string `json:"key"`
	Provider    string `json:"provider"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Optional    bool   `json:"optional"`
}

// GetRequiredCredentials returns detailed information about required credentials
func (v *Validator) GetRequiredCredentials(astroSpec *spec.AstroSpec) []CredentialInfo {
	credMap := make(map[string]CredentialInfo)

	// Check integration models (cloud providers)
	for _, model := range astroSpec.Integrations.Models {
		if info := v.getCredentialInfo(model.Provider, "model"); info.Key != "" {
			if model.Env != nil && model.Env.Prefix != "" {
				info.Key = model.Env.Prefix + info.Key
			}
			credMap[info.Key] = info
		}
	}

	// Check integration knowledge stores (cloud providers)
	for _, knowledge := range astroSpec.Integrations.Knowledge {
		if info := v.getCredentialInfo(knowledge.Provider, "knowledge"); info.Key != "" {
			if knowledge.Env != nil && knowledge.Env.Prefix != "" {
				info.Key = knowledge.Env.Prefix + info.Key
			}
			credMap[info.Key] = info
		}
	}

	// Check integration tools
	for _, tool := range astroSpec.Integrations.Tools {
		if info := v.getCredentialInfo(tool.Provider, "tool"); info.Key != "" {
			if tool.Env != nil && tool.Env.Prefix != "" {
				info.Key = tool.Env.Prefix + info.Key
			}
			credMap[info.Key] = info
		}
	}

	// Check messaging interfaces
	for _, iface := range astroSpec.Interfaces {
		ifaceType := strings.ToLower(iface.Type)

		if ifaceType == "slack" || ifaceType == "messaging/slack" || strings.Contains(ifaceType, "slack") {
			// Slack requires both app token and bot token
			credMap["SLACK_APP_TOKEN"] = CredentialInfo{
				Key:         "SLACK_APP_TOKEN",
				Provider:    "slack",
				Category:    "messaging",
				Description: "Slack app-level token for socket mode connections",
				Optional:    false,
			}
			credMap["SLACK_BOT_TOKEN"] = CredentialInfo{
				Key:         "SLACK_BOT_TOKEN",
				Provider:    "slack",
				Category:    "messaging",
				Description: "Slack bot token for API access and messaging",
				Optional:    false,
			}
		} else if ifaceType == "discord" || ifaceType == "messaging/discord" || strings.Contains(ifaceType, "discord") {
			credMap["DISCORD_BOT_TOKEN"] = CredentialInfo{
				Key:         "DISCORD_BOT_TOKEN",
				Provider:    "discord",
				Category:    "messaging",
				Description: "Discord bot token for messaging interface",
				Optional:    false,
			}
		}
	}

	// Check injection sources
	for _, injection := range astroSpec.Injections {
		if injection.Source.Type == "github" {
			credMap["GITHUB_TOKEN"] = CredentialInfo{
				Key:         "GITHUB_TOKEN",
				Provider:    "github",
				Category:    "injection",
				Description: "GitHub token for accessing repositories and issues",
				Optional:    true, // Optional for public repos
			}
		} else if injection.Source.Type == "gitlab" {
			credMap["GITLAB_TOKEN"] = CredentialInfo{
				Key:         "GITLAB_TOKEN",
				Provider:    "gitlab",
				Category:    "injection",
				Description: "GitLab token for accessing repositories and issues",
				Optional:    true, // Optional for public repos
			}
		}
	}

	// Convert map to slice
	var creds []CredentialInfo
	for _, info := range credMap {
		creds = append(creds, info)
	}

	return creds
}

// getCredentialInfo returns credential information for a given provider and category
func (v *Validator) getCredentialInfo(provider, category string) CredentialInfo {
	providerLower := strings.ToLower(provider)

	switch providerLower {
	case "anthropic":
		return CredentialInfo{
			Key:         "ANTHROPIC_API_KEY",
			Provider:    "anthropic",
			Category:    category,
			Description: "Anthropic API key for Claude models",
			Optional:    false,
		}
	case "openai":
		return CredentialInfo{
			Key:         "OPENAI_API_KEY",
			Provider:    "openai",
			Category:    category,
			Description: "OpenAI API key for GPT models",
			Optional:    false,
		}
	case "google", "gemini":
		return CredentialInfo{
			Key:         "GOOGLE_API_KEY",
			Provider:    "google",
			Category:    category,
			Description: "Google API key for Gemini models",
			Optional:    false,
		}
	case "cohere":
		return CredentialInfo{
			Key:         "COHERE_API_KEY",
			Provider:    "cohere",
			Category:    category,
			Description: "Cohere API key for language models",
			Optional:    false,
		}
	case "pinecone":
		return CredentialInfo{
			Key:         "PINECONE_API_KEY",
			Provider:    "pinecone",
			Category:    category,
			Description: "Pinecone API key for vector database",
			Optional:    false,
		}
	default:
		// For unknown providers, generate a generic key
		if providerLower != "" && providerLower != "self-hosted" {
			key := fmt.Sprintf("%s_API_KEY", strings.ToUpper(provider))
			return CredentialInfo{
				Key:         key,
				Provider:    providerLower,
				Category:    category,
				Description: fmt.Sprintf("API key for %s", provider),
				Optional:    false,
			}
		}
		return CredentialInfo{}
	}
}

// getCredentialKeyForProvider returns the credential key for a given provider
func (v *Validator) getCredentialKeyForProvider(provider string) string {
	providerLower := strings.ToLower(provider)

	switch providerLower {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "google", "gemini":
		return "GOOGLE_API_KEY"
	case "cohere":
		return "COHERE_API_KEY"
	case "github":
		return "GITHUB_TOKEN"
	case "gitlab":
		return "GITLAB_TOKEN"
	case "slack":
		return "SLACK_BOT_TOKEN"
	case "discord":
		return "DISCORD_BOT_TOKEN"
	case "pinecone":
		return "PINECONE_API_KEY"
	default:
		// For unknown providers, generate a generic key
		if providerLower != "" && providerLower != "self-hosted" {
			return fmt.Sprintf("%s_API_KEY", strings.ToUpper(provider))
		}
		return ""
	}
}
