package deployment

import (
	"fmt"

	"github.com/robfig/cron/v3"
)

// ResolveResult holds the outcome of validation and resolution.
type ResolveResult struct {
	Spec   *AstroDeploymentSpec
	Errors []string
}

// ValidateAndResolve validates a filled-in deployment spec and produces
// a fully resolved spec ready for translation.
func ValidateAndResolve(submitted *AstroDeploymentSpec) (*ResolveResult, error) {
	result := &ResolveResult{}
	var errs []string

	// 1. Basic validation (spec version, required fields) — already done by parser.
	//    Additional semantic validation below.

	// 2. Reference validation
	if submitted.Agent.Environment != nil {
		refs := ExtractAllReferences(submitted.Agent.Environment)
		refErrs := ValidateReferences(refs, submitted)
		errs = append(errs, refErrs...)
	}

	// Also validate references in interfaces environment
	if submitted.Interfaces != nil && submitted.Interfaces.Environment != nil {
		refs := ExtractAllReferences(submitted.Interfaces.Environment)
		refErrs := ValidateReferences(refs, submitted)
		errs = append(errs, refErrs...)
	}

	// 3. Check required variables non-empty. Refs count as provided after
	// resolution; configured markers count as opaque values during preflight.
	for key, v := range submitted.Variables {
		if !v.Optional && v.Value == "" && v.Ref == "" && !v.Configured {
			errs = append(errs, fmt.Sprintf("variables.%s.value: required variable is empty", key))
		}
		if v.Value != "" && v.Ref != "" {
			errs = append(errs, fmt.Sprintf("variables.%s: cannot set both value and ref", key))
		}
	}

	// 4. Validate ingestion
	cronParser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	for name, ing := range submitted.Ingestion {
		if ing.Image == "" {
			errs = append(errs, fmt.Sprintf("ingestion.%s.image: required", name))
		}
		if ing.Trigger.Type == "" {
			errs = append(errs, fmt.Sprintf("ingestion.%s.trigger.type: required", name))
		}
		if ing.Trigger.Type == "schedule" {
			if ing.Trigger.Schedule == "" {
				errs = append(errs, fmt.Sprintf("ingestion.%s.trigger.schedule: cron expression required for schedule trigger", name))
			} else if _, err := cronParser.Parse(ing.Trigger.Schedule); err != nil {
				errs = append(errs, fmt.Sprintf("ingestion.%s.trigger.schedule: invalid cron expression: %v", name, err))
			}
		}
		if ing.Trigger.Type == "webhook" && len(ing.Endpoints) == 0 {
			errs = append(errs, fmt.Sprintf("ingestion.%s.endpoints: required for webhook triggers", name))
		}
	}

	// 4b. Validate required fields on components
	if submitted.Agent.Image == "" {
		errs = append(errs, "agent.image: required")
	}
	if len(submitted.Agent.Endpoints) == 0 {
		errs = append(errs, "agent.endpoints: required (at least one endpoint)")
	}
	for name, m := range submitted.Models {
		if m.Image == "" {
			errs = append(errs, fmt.Sprintf("models.%s.image: required", name))
		}
		if len(m.Endpoints) == 0 {
			errs = append(errs, fmt.Sprintf("models.%s.endpoints: required (at least one endpoint)", name))
		}
	}
	for name, k := range submitted.Knowledge {
		if k.IsBound() {
			continue // bound entries have no container config — validated at deploy time
		}
		if k.Image == "" {
			errs = append(errs, fmt.Sprintf("knowledge.%s.image: required", name))
		}
		if len(k.Endpoints) == 0 {
			errs = append(errs, fmt.Sprintf("knowledge.%s.endpoints: required (at least one endpoint)", name))
		}
	}
	for name, t := range submitted.Integrations {
		if t.Image == "" {
			errs = append(errs, fmt.Sprintf("integrations.%s.image: required", name))
		}
		if len(t.Endpoints) == 0 {
			errs = append(errs, fmt.Sprintf("integrations.%s.endpoints: required (at least one endpoint)", name))
		}
	}

	// 5. Validate interface adapter names
	validAdapters := map[string]bool{"slack": true, "web": true}
	if submitted.Interfaces != nil {
		for _, adapter := range submitted.Interfaces.Adapters {
			if !validAdapters[adapter] {
				errs = append(errs, fmt.Sprintf("interfaces.adapters: unknown adapter %q", adapter))
			}
		}

		// 6. Re-derive interface variables for enabled adapters
		for _, adapter := range submitted.Interfaces.Adapters {
			if adapter == "slack" {
				ensureVariable(submitted, "SLACK_BOT_TOKEN", "Slack bot token for messaging", false, []string{"interface.slack"})
				ensureVariable(submitted, "SLACK_APP_TOKEN", "Slack app token for socket mode", false, []string{"interface.slack"})

				// Verify these variables have values
				if v, ok := submitted.Variables["SLACK_BOT_TOKEN"]; ok && v.Value == "" {
					errs = append(errs, "variables.SLACK_BOT_TOKEN.value: required for slack adapter")
				}
				if v, ok := submitted.Variables["SLACK_APP_TOKEN"]; ok && v.Value == "" {
					errs = append(errs, "variables.SLACK_APP_TOKEN.value: required for slack adapter")
				}
			}
		}
	}

	// 7. Validate agent.distributed / replicas rule
	if !submitted.Agent.Distributed && submitted.Agent.Replicas > 1 {
		errs = append(errs, "agent.replicas must be 1 when agent.distributed is false")
	}

	// 8. (reserved)

	if len(errs) > 0 {
		result.Errors = errs
		return result, nil
	}

	// 9. Apply defaults for any omitted optional fields
	resolved := applyDefaults(submitted)
	resolved.Spec = "deployment/v1"

	result.Spec = resolved
	return result, nil
}

func ensureVariable(ds *AstroDeploymentSpec, key, description string, optional bool, targets []string) {
	if ds.Variables == nil {
		ds.Variables = make(map[string]Variable)
	}
	if _, exists := ds.Variables[key]; !exists {
		ds.Variables[key] = Variable{
			Description: description,
			Optional:    optional,
			Secret:      true,
			Targets:     targets,
		}
	}
}

func applyDefaults(ds *AstroDeploymentSpec) *AstroDeploymentSpec {
	// Copy to avoid mutating input
	resolved := *ds

	if resolved.Agent.Replicas == 0 {
		resolved.Agent.Replicas = 1
	}
	if resolved.Agent.Update.Strategy == "" {
		resolved.Agent.Update = DefaultUpdateStrategy()
	}

	for name, m := range resolved.Models {
		if m.Replicas == 0 {
			m.Replicas = 1
		}
		if m.Update.Strategy == "" {
			m.Update = DefaultUpdateStrategy()
		}
		resolved.Models[name] = m
	}

	for name, k := range resolved.Knowledge {
		if k.IsBound() {
			continue // bound entries have no container config — no defaults to apply
		}
		if k.Replicas == 0 {
			k.Replicas = 1
		}
		if k.Update.Strategy == "" {
			k.Update = DefaultUpdateStrategy()
		}
		if k.Persistent && k.Storage == nil {
			defaultStorage := DefaultStorageConfig()
			k.Storage = &defaultStorage
		}
		resolved.Knowledge[name] = k
	}

	for name, t := range resolved.Integrations {
		if t.Replicas == 0 {
			t.Replicas = 1
		}
		if t.Update.Strategy == "" {
			t.Update = DefaultUpdateStrategy()
		}
		resolved.Integrations[name] = t
	}

	return &resolved
}
