package deployment

import (
	"fmt"
	"strings"

	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/robfig/cron/v3"
)

// Validator validates agent specs and credentials
type Validator struct {
	cronParser cron.Parser
	// AIGatewayEnabled gates the agent.ai_gateway opt-in. When false, specs
	// that set agent.ai_gateway: true are rejected at admission. Set from
	// config.Deployment.AIGatewayURL != "" — empty URL means the gateway
	// provisioner is nil and no virtual keys can be minted.
	AIGatewayEnabled bool
}

// NewValidator creates a new validator with all toggles defaulted off. Use
// NewValidatorWithOptions when astro-server config needs to flow in.
func NewValidator() *Validator {
	return &Validator{
		cronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// ValidatorOptions carries deploy-time toggles that depend on server config.
type ValidatorOptions struct {
	AIGatewayEnabled bool
}

// NewValidatorWithOptions constructs a validator wired to server-config toggles.
func NewValidatorWithOptions(opts ValidatorOptions) *Validator {
	v := NewValidator()
	v.AIGatewayEnabled = opts.AIGatewayEnabled
	return v
}

// ValidateSpec validates the agent spec and variables.
// interfaces and schedules are deployment-time values (not in spec).
func (v *Validator) ValidateSpec(astroSpec *spec.AstroSpec, userVariables map[string]string, interfaces []string, schedules map[string]string) ValidationResult {
	result := ValidationResult{
		Valid:            true,
		Errors:           []ValidationError{},
		MissingVariables: []string{},
	}

	if astroSpec.Agent.Image == "" && astroSpec.Agent.Build == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "agent",
			Message: "must specify either image or build",
		})
	}

	for name, model := range astroSpec.Models {
		if model.Container != nil && model.Container.Image == "" && model.Container.Build == nil {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("models.%s", name),
				Message: "container must specify either image or build",
			})
		}
	}

	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.Container != nil && knowledge.Container.Image == "" && knowledge.Container.Build == nil {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("knowledge.%s", name),
				Message: "container must specify either image or build",
			})
		}
	}

	for name, integration := range astroSpec.Integrations {
		if integration.Container != nil && integration.Container.Image == "" && integration.Container.Build == nil {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("integrations.%s", name),
				Message: "container must specify either image or build",
			})
		}
	}

	for name, ingestion := range astroSpec.Ingestion {
		if ingestion.Container.Image == "" && ingestion.Container.Build == nil {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("ingestion.%s", name),
				Message: "container must specify either image or build",
			})
		}
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

	// Validate providers in models/knowledge/tools
	v.validateProviders(astroSpec, &result)

	// Collect required variables (provider credentials + interface tokens)
	requiredVars := v.collectRequiredCredentials(astroSpec, interfaces)

	// Check for missing variables
	for _, varKey := range requiredVars {
		if _, exists := userVariables[varKey]; !exists {
			result.Valid = false
			result.MissingVariables = append(result.MissingVariables, varKey)
			result.Errors = append(result.Errors, ValidationError{
				Field:   "variables." + varKey,
				Message: fmt.Sprintf("missing required variable: %s", varKey),
			})
		}
	}

	return result
}

// collectRequiredCredentials identifies all required (non-optional) credentials from the spec.
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

// validateProviders checks cloud providers in models/knowledge/tools and
// validates custom provider references are within scope.
func (v *Validator) validateProviders(astroSpec *spec.AstroSpec, result *ValidationResult) {
	// validateEntry checks a single provider reference and appends errors as needed.
	// section is the external-facing YAML section name (e.g. "integrations").
	// registrySection is the internal provider registry key (e.g. "tools").
	validateEntry := func(section, registrySection, entryName, provider string) {
		// Custom provider — validate scope.
		if cp, ok := astroSpec.Providers[provider]; ok {
			if !scopeContains(cp.Scope, section) {
				result.Valid = false
				result.Errors = append(result.Errors, ValidationError{
					Field:   fmt.Sprintf("%s.%s.provider", section, entryName),
					Message: fmt.Sprintf("provider %q does not allow scope %q", provider, section),
				})
			}
			return
		}
		// Built-in provider — must be known.
		if p, ok := spec.LookupBuiltin(registrySection, provider); !ok || p.Name == "" {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("%s.%s.provider", section, entryName),
				Message: fmt.Sprintf("unsupported provider %q", provider),
			})
		}
	}

	for name, model := range astroSpec.Models {
		if model.IsProviderMode() && model.Container == nil {
			validateEntry("models", "models", name, model.Provider)
		}
	}

	// AI Gateway: agent.ai_gateway: true is rejected at admission if the
	// gateway isn't enabled in this env. Failing here surfaces a clear
	// error vs. shipping an agent pod with empty ASTRO_GATEWAY_* env vars.
	if astroSpec.Agent.AIGateway && !v.AIGatewayEnabled {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "agent.ai_gateway",
			Message: "agent.ai_gateway is true but the AI Gateway is not enabled in this environment",
		})
	}
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.IsProviderMode() && knowledge.Container == nil {
			validateEntry("knowledge", "knowledge", name, knowledge.Provider)
		}
	}
	for name, tool := range astroSpec.Integrations {
		if tool.IsProviderMode() && tool.Container == nil {
			validateEntry("integrations", "integrations", name, tool.Provider)
		}
	}
}

// GetRequiredCredentials returns detailed information about required credentials.
// interfaces is the deployment-time list of enabled interfaces (e.g. ["slack","web"]).
func (v *Validator) GetRequiredCredentials(astroSpec *spec.AstroSpec, interfaces []string) []CredentialInfo {
	credMap := make(map[string]CredentialInfo)

	// Cloud credentials — names come from spec.CloudCredentialKeys, the single
	// source of truth shared with astro-cli's composeBuilder and the deployer's
	// spec_applier. Re-implementing §8.1 here would risk dev/prod divergence
	// (see docs/changelog/feat/ai-gateway-astro-server-* for the prior bug).
	//
	// Managed providers emit credential names too (so the deployer can inject
	// server-supplied values under the standard names), but they should NOT
	// surface as "user must supply" — filter them out here.
	for key, meta := range spec.CloudCredentialKeys(astroSpec) {
		section := categoryToSection(meta.Category)
		if section != "" && spec.IsManagedProvider(section, meta.Provider) {
			continue
		}
		credMap[key] = CredentialInfo{
			Key:         key,
			Provider:    meta.Provider,
			Category:    meta.Category,
			Description: meta.Description,
			Optional:    meta.Optional,
		}
	}

	// Scan custom providers referenced by components.
	// Delegates to spec.CustomProviderCredentialKeys which correctly constructs
	// {UPPER(provider)}_{varName} keys and handles duplicate-entry naming (§8.1/§5).
	for key, meta := range spec.CustomProviderCredentialKeys(astroSpec) {
		credMap[key] = CredentialInfo{
			Key:         key,
			Provider:    meta.Provider,
			Category:    meta.Category,
			Description: meta.Description,
			Optional:    meta.Optional,
		}
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

// categoryToSection maps the singular CredentialMeta.Category value
// ("model"/"knowledge"/"integration") onto the plural section name
// spec.IsManagedProvider expects ("models"/"knowledge"/"integrations").
func categoryToSection(category string) string {
	switch category {
	case "model":
		return "models"
	case "knowledge":
		return "knowledge"
	case "integration":
		return "integrations"
	}
	return ""
}

func scopeContains(scope []string, value string) bool {
	for _, s := range scope {
		if s == value {
			return true
		}
	}
	return false
}
