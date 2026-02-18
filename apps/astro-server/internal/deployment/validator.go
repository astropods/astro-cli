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

	// Validate providers in models/knowledge/tools and integrations
	v.validateProviders(astroSpec, &result)

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

// validateProviders checks cloud providers in models/knowledge/tools and
// validates that integrations have credentials.
func (v *Validator) validateProviders(astroSpec *spec.AstroSpec, result *ValidationResult) {
	// Validate model providers: cloud providers must be recognized
	for name, model := range astroSpec.Models {
		if model.IsProviderMode() && model.Container == nil {
			provider := model.Provider
			// Skip self-hosted providers (they have container registries)
			if _, ok := spec.GetCloudModelCredentials(provider); !ok {
				// Check if it's a self-hosted provider (has an image in the registry)
				p := spec.GetModelProvider(provider)
				if p.Image == "" {
					result.Valid = false
					result.Errors = append(result.Errors, ValidationError{
						Field:   fmt.Sprintf("models.%s.provider", name),
						Message: fmt.Sprintf("unsupported provider %q", provider),
					})
				}
			}
		}
	}

	// Validate knowledge providers
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.IsProviderMode() && knowledge.Container == nil {
			provider := knowledge.Provider
			if _, ok := spec.GetCloudKnowledgeCredentials(provider); !ok {
				p := spec.GetProvider(provider)
				if p.Image == "" {
					result.Valid = false
					result.Errors = append(result.Errors, ValidationError{
						Field:   fmt.Sprintf("knowledge.%s.provider", name),
						Message: fmt.Sprintf("unsupported provider %q", provider),
					})
				}
			}
		}
	}

	// Validate tool providers
	for name, tool := range astroSpec.Tools {
		if tool.IsProviderMode() && tool.Container == nil {
			provider := tool.Provider
			if _, ok := spec.GetCloudToolCredentials(provider); !ok {
				result.Valid = false
				result.Errors = append(result.Errors, ValidationError{
					Field:   fmt.Sprintf("tools.%s.provider", name),
					Message: fmt.Sprintf("unsupported provider %q", provider),
				})
			}
		}
	}

	// Validate integrations: must have credentials
	for name, integration := range astroSpec.Integrations {
		if len(integration.Credentials) == 0 {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("integrations.%s.credentials", name),
				Message: "integration requires at least one credential",
			})
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

	// Pass 1: Collect cloud entries grouped by provider
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
		// Prefer an entry whose name matches the provider (natural primary);
		// otherwise fall back to first alphabetically.
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
					// Single entry: bare provider key (e.g., ANTHROPIC_API_KEY)
					addCred(basePrefix+"_"+cs.Suffix, entry, cs)
				} else {
					// Name-qualified key for all entries, except when name == provider
					// (e.g., skip redundant ANTHROPIC_ANTHROPIC_API_KEY)
					if !strings.EqualFold(entry.name, entry.provider) {
						addCred(basePrefix+"_"+strings.ToUpper(SanitizeName(entry.name))+"_"+cs.Suffix, entry, cs)
					}
					// Bare key for the primary entry
					if i == bareOwnerIdx {
						addCred(basePrefix+"_"+cs.Suffix, entry, cs)
					}
				}
			}
		}
	}

	// Scan integrations
	for name, integration := range astroSpec.Integrations {
		for _, cc := range integration.Credentials {
			key := strings.ToUpper(name) + "_" + cc.Suffix
			credMap[key] = CredentialInfo{
				Key:         key,
				Provider:    "integration",
				Category:    "integration",
				Description: cc.Description,
				Optional:    cc.Optional,
			}
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
