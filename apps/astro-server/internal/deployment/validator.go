package deployment

import (
	"fmt"
	"sort"
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

// ValidateSpec validates the agent spec and variables.
// interfaces and schedules are deployment-time values (not in spec).
func (v *Validator) ValidateSpec(astroSpec *spec.AstroSpec, userVariables map[string]string, interfaces []string, schedules map[string]string) ValidationResult {
	result := ValidationResult{
		Valid:            true,
		Errors:           []ValidationError{},
		MissingVariables: []string{},
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
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.IsProviderMode() && knowledge.Container == nil {
			validateEntry("knowledge", "knowledge", name, knowledge.Provider)
		}
	}
	for name, tool := range astroSpec.Tools {
		if tool.IsProviderMode() && tool.Container == nil {
			validateEntry("integrations", "tools", name, tool.Provider)
		}
	}
}

// GetRequiredCredentials returns detailed information about required credentials.
// interfaces is the deployment-time list of enabled interfaces (e.g. ["slack","web"]).
func (v *Validator) GetRequiredCredentials(astroSpec *spec.AstroSpec, interfaces []string) []CredentialInfo {
	credMap := make(map[string]CredentialInfo)

	// --- Cloud credentials: two-pass approach ---

	type cloudEntry struct {
		name     string
		provider string
		category string
		suffixes []spec.CredentialSuffix
	}

	providerGroups := make(map[string][]cloudEntry)

	// Pass 1: Collect cloud entries grouped by provider (skip custom providers).
	for name, model := range astroSpec.Models {
		if model.IsProviderMode() {
			if suffixes, ok := spec.GetCloudModelCredentials(model.Provider); ok {
				provider := strings.ToLower(model.Provider)
				providerGroups[provider] = append(providerGroups[provider], cloudEntry{
					name: name, provider: provider, category: "model", suffixes: suffixes,
				})
			}
		}
	}
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.IsProviderMode() {
			if suffixes, ok := spec.GetCloudKnowledgeCredentials(knowledge.Provider); ok {
				provider := strings.ToLower(knowledge.Provider)
				providerGroups[provider] = append(providerGroups[provider], cloudEntry{
					name: name, provider: provider, category: "knowledge", suffixes: suffixes,
				})
			}
		}
	}
	for name, tool := range astroSpec.Tools {
		if tool.IsProviderMode() {
			if suffixes, ok := spec.GetCloudToolCredentials(tool.Provider); ok {
				provider := strings.ToLower(tool.Provider)
				providerGroups[provider] = append(providerGroups[provider], cloudEntry{
					name: name, provider: provider, category: "tool", suffixes: suffixes,
				})
			}
		}
	}

	// addCred is a shorthand to insert a credential into the map.
	addCred := func(key string, entry cloudEntry, cs spec.CredentialSuffix) {
		credMap[key] = CredentialInfo{
			Key:         key,
			Provider:    entry.provider,
			Category:    entry.category,
			Description: cs.Description,
			Optional:    cs.Optional,
		}
	}

	// Pass 2: Generate keys with duplicate handling
	for _, entries := range providerGroups {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].name < entries[j].name
		})

		isDuplicate := len(entries) > 1
		basePrefix := strings.ToUpper(entries[0].provider)

		// Find which entry owns the bare key.
		bareOwnerIdx := 0
		if isDuplicate {
			for i, entry := range entries {
				if strings.EqualFold(entry.name, entry.provider) {
					bareOwnerIdx = i
					break
				}
			}
		}

		for i, entry := range entries {
			for _, cs := range entry.suffixes {
				if !isDuplicate {
					addCred(basePrefix+"_"+cs.Suffix, entry, cs)
				} else {
					if !strings.EqualFold(entry.name, entry.provider) {
						addCred(basePrefix+"_"+strings.ToUpper(SanitizeName(entry.name))+"_"+cs.Suffix, entry, cs)
					}
					if i == bareOwnerIdx {
						addCred(basePrefix+"_"+cs.Suffix, entry, cs)
					}
				}
			}
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

func scopeContains(scope []string, value string) bool {
	for _, s := range scope {
		if s == value {
			return true
		}
	}
	return false
}
