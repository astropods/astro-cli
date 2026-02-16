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

// ValidateSpec validates the agent spec and credentials.
// interfaces and schedules are deployment-time values (not in spec).
func (v *Validator) ValidateSpec(astroSpec *spec.AstroSpec, userCredentials map[string]string, interfaces []string, schedules map[string]string) ValidationResult {
	result := ValidationResult{
		Valid:              true,
		Errors:             []ValidationError{},
		MissingCredentials: []string{},
	}

	// Validate basic spec fields
	if astroSpec.Name == "" {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "agent",
			Message: "agent name is required",
		})
	}

	// Validate container
	if astroSpec.Agent.Image == "" && astroSpec.Agent.Build == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "container",
			Message: "either image or build configuration is required",
		})
	}

	// Validate ingestion triggers
	validTriggerTypes := map[string]bool{"schedule": true, "manual": true, "startup": true, "webhook": true}
	for name, ingestion := range astroSpec.Ingestion {
		triggerType := ingestion.Trigger.Type
		if !validTriggerTypes[triggerType] {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("ingestion.%s.trigger.type", name),
				Message: fmt.Sprintf("invalid trigger type %q: must be one of schedule, manual, startup, webhook", triggerType),
			})
			continue
		}

		if triggerType == "schedule" {
			schedule := schedules[name]
			if schedule == "" {
				result.Valid = false
				result.Errors = append(result.Errors, ValidationError{
					Field:   fmt.Sprintf("ingestion.%s.trigger.schedule", name),
					Message: "schedule expression is required for schedule trigger",
				})
			} else {
				if _, err := v.cronParser.Parse(schedule); err != nil {
					result.Valid = false
					result.Errors = append(result.Errors, ValidationError{
						Field:   fmt.Sprintf("ingestion.%s.trigger.schedule", name),
						Message: fmt.Sprintf("invalid cron expression: %v", err),
					})
				}
			}
		}
	}

	// Validate integration providers
	v.validateIntegrationProviders(astroSpec, &result)

	// Collect required credentials
	requiredCreds := v.collectRequiredCredentials(astroSpec, interfaces)

	// Check for missing credentials
	for _, credKey := range requiredCreds {
		if _, exists := userCredentials[credKey]; !exists {
			result.Valid = false
			result.MissingCredentials = append(result.MissingCredentials, credKey)
			result.Errors = append(result.Errors, ValidationError{
				Field:   "credentials." + credKey,
				Message: fmt.Sprintf("missing required credential: %s", credKey),
			})
		}
	}

	return result
}

// collectRequiredCredentials identifies all required (non-optional) credentials from the spec.
// It derives from GetRequiredCredentials to ensure consistency between validation and config endpoint.
func (v *Validator) collectRequiredCredentials(astroSpec *spec.AstroSpec, interfaces []string) []string {
	allCreds := v.GetRequiredCredentials(astroSpec, interfaces)

	var required []string
	for _, cred := range allCreds {
		if !cred.Optional {
			required = append(required, cred.Key)
		}
	}

	return required
}

// CredentialInfo provides metadata about a required credential
type CredentialInfo struct {
	Key         string `json:"key"`
	Provider    string `json:"provider"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Optional    bool   `json:"optional"`
}

// CredentialSuffix describes one credential a provider requires
type CredentialSuffix struct {
	Suffix      string
	Description string
	Optional    bool
}

// supportedProviders maps each supported provider to its credential suffixes.
var supportedProviders = map[string][]CredentialSuffix{
	"anthropic": {{Suffix: "API_KEY", Description: "Anthropic API key for Claude models"}},
	"openai":    {{Suffix: "API_KEY", Description: "OpenAI API key for GPT models"}},
	"google":    {{Suffix: "API_KEY", Description: "Google API key for Gemini models"}},
	"gemini":    {{Suffix: "API_KEY", Description: "Google API key for Gemini models"}},
	"cohere":    {{Suffix: "API_KEY", Description: "Cohere API key for language models"}},
	"pinecone":  {{Suffix: "API_KEY", Description: "Pinecone API key for vector database"}},
	"github":    {{Suffix: "TOKEN", Description: "GitHub token for API access"}},
	"gitlab":    {{Suffix: "TOKEN", Description: "GitLab token for API access"}},
	"slack": {
		{Suffix: "BOT_TOKEN", Description: "Slack bot token for API access"},
		{Suffix: "APP_TOKEN", Description: "Slack app-level token for socket mode"},
	},
}

// getProviderCredentialSuffixes returns the credential suffixes for a provider.
// Returns nil and false if the provider is not supported.
func (v *Validator) getProviderCredentialSuffixes(provider string) ([]CredentialSuffix, bool) {
	suffixes, ok := supportedProviders[strings.ToLower(provider)]
	return suffixes, ok
}

// validateIntegrationProviders checks that all integration providers are supported.
func (v *Validator) validateIntegrationProviders(astroSpec *spec.AstroSpec, result *ValidationResult) {
	for name, integration := range astroSpec.Integrations {
		if strings.ToLower(integration.Provider) == "custom" {
			if len(integration.Credentials) == 0 {
				result.Valid = false
				result.Errors = append(result.Errors, ValidationError{
					Field:   fmt.Sprintf("integrations.%s.credentials", name),
					Message: "custom provider requires at least one credential suffix",
				})
			}
			continue
		}
		if _, ok := v.getProviderCredentialSuffixes(integration.Provider); !ok {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("integrations.%s.provider", name),
				Message: fmt.Sprintf("unsupported provider %q", integration.Provider),
			})
		}
	}
}

// GetRequiredCredentials returns detailed information about required credentials.
// interfaces is the deployment-time list of enabled interfaces (e.g. ["slack","web"]).
func (v *Validator) GetRequiredCredentials(astroSpec *spec.AstroSpec, interfaces []string) []CredentialInfo {
	credMap := make(map[string]CredentialInfo)

	addCreds := func(name, provider, category string, customCreds []spec.CustomCredential) {
		if strings.ToLower(provider) == "custom" {
			for _, cc := range customCreds {
				key := strings.ToUpper(name) + "_" + cc.Suffix
				credMap[key] = CredentialInfo{
					Key:         key,
					Provider:    "custom",
					Category:    category,
					Description: cc.Description,
					Optional:    cc.Optional,
				}
			}
			return
		}
		suffixes, ok := v.getProviderCredentialSuffixes(provider)
		if !ok {
			return
		}
		for _, cs := range suffixes {
			key := strings.ToUpper(name) + "_" + cs.Suffix
			credMap[key] = CredentialInfo{
				Key:         key,
				Provider:    strings.ToLower(provider),
				Category:    category,
				Description: cs.Description,
				Optional:    cs.Optional,
			}
		}
	}

	for name, integration := range astroSpec.Integrations {
		category := integration.Type
		if category == "" {
			category = "integration"
		}
		addCreds(name, integration.Provider, category, integration.Credentials)
	}

	// Check messaging interfaces (deployment-time values)
	for _, name := range interfaces {
		ifaceType := strings.ToLower(name)

		if ifaceType == "slack" || ifaceType == "messaging/slack" || strings.Contains(ifaceType, "slack") {
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
		}
	}

	// Convert map to slice
	var creds []CredentialInfo
	for _, info := range credMap {
		creds = append(creds, info)
	}

	return creds
}

