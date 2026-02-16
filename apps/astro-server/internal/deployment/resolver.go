package deployment

import (
	"fmt"

	"github.com/postman/astro/packages/astro-spec"
	"github.com/robfig/cron/v3"
)

// ResolveResult holds the outcome of validation and resolution.
type ResolveResult struct {
	Spec   *spec.AstroDeploymentSpec
	Errors []string
}

// ValidateAndResolve validates a filled-in deployment spec and produces
// a fully resolved spec ready for translation.
func ValidateAndResolve(submitted *spec.AstroDeploymentSpec) (*ResolveResult, error) {
	result := &ResolveResult{}
	var errs []string

	// 1. Basic validation (spec version, required fields) — already done by parser.
	//    Additional semantic validation below.

	// 2. Reference validation
	if submitted.Agent.Environment != nil {
		refs := spec.ExtractAllReferences(submitted.Agent.Environment)
		refErrs := spec.ValidateReferences(refs, submitted)
		errs = append(errs, refErrs...)
	}

	// Also validate references in interfaces environment
	if submitted.Interfaces != nil && submitted.Interfaces.Environment != nil {
		refs := spec.ExtractAllReferences(submitted.Interfaces.Environment)
		refErrs := spec.ValidateReferences(refs, submitted)
		errs = append(errs, refErrs...)
	}

	// 3. Check required credentials non-empty
	for key, cred := range submitted.Credentials {
		if !cred.Optional && cred.Value == "" {
			errs = append(errs, fmt.Sprintf("credentials.%s.value: required credential is empty", key))
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
		if ing.Trigger.Type == "webhook" && ing.Port == 0 {
			errs = append(errs, fmt.Sprintf("ingestion.%s.port: required for webhook triggers", name))
		}
	}

	// 4b. Validate required fields on components
	if submitted.Agent.Image == "" {
		errs = append(errs, "agent.image: required")
	}
	if submitted.Agent.Port == 0 {
		errs = append(errs, "agent.port: required")
	}
	for name, m := range submitted.Models {
		if m.Image == "" {
			errs = append(errs, fmt.Sprintf("models.%s.image: required", name))
		}
		if m.Port == 0 {
			errs = append(errs, fmt.Sprintf("models.%s.port: required", name))
		}
	}
	for name, k := range submitted.Knowledge {
		if k.Image == "" {
			errs = append(errs, fmt.Sprintf("knowledge.%s.image: required", name))
		}
		if k.Port == 0 {
			errs = append(errs, fmt.Sprintf("knowledge.%s.port: required", name))
		}
	}
	for name, t := range submitted.Tools {
		if t.Image == "" {
			errs = append(errs, fmt.Sprintf("tools.%s.image: required", name))
		}
		if t.Port == 0 {
			errs = append(errs, fmt.Sprintf("tools.%s.port: required", name))
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

		// 6. Re-derive interface credentials for enabled adapters
		for _, adapter := range submitted.Interfaces.Adapters {
			if adapter == "slack" {
				ensureCredential(submitted, "SLACK_BOT_TOKEN", "Slack bot token for messaging", false)
				ensureCredential(submitted, "SLACK_APP_TOKEN", "Slack app token for socket mode", false)

				// Verify these credentials have values
				if c, ok := submitted.Credentials["SLACK_BOT_TOKEN"]; ok && c.Value == "" {
					errs = append(errs, "credentials.SLACK_BOT_TOKEN.value: required for slack adapter")
				}
				if c, ok := submitted.Credentials["SLACK_APP_TOKEN"]; ok && c.Value == "" {
					errs = append(errs, "credentials.SLACK_APP_TOKEN.value: required for slack adapter")
				}
			}
		}
	}

	// 7. Validate target namespace
	if submitted.Target.Namespace == "" {
		errs = append(errs, "target.namespace: namespace is required")
	}

	if len(errs) > 0 {
		result.Errors = errs
		return result, nil
	}

	// 8. Apply defaults for any omitted optional fields
	resolved := applyDefaults(submitted)

	// 9. Strip editable field (template-only)
	resolved.Editable = nil

	result.Spec = resolved
	return result, nil
}

func ensureCredential(ds *spec.AstroDeploymentSpec, key, description string, optional bool) {
	if ds.Credentials == nil {
		ds.Credentials = make(map[string]spec.DeploymentCredential)
	}
	if _, exists := ds.Credentials[key]; !exists {
		ds.Credentials[key] = spec.DeploymentCredential{
			Description: description,
			Optional:    optional,
		}
	}
}

func applyDefaults(ds *spec.AstroDeploymentSpec) *spec.AstroDeploymentSpec {
	// Copy to avoid mutating input
	resolved := *ds

	if resolved.Agent.Port == 0 {
		resolved.Agent.Port = 8080
	}
	if resolved.Agent.Replicas == 0 {
		resolved.Agent.Replicas = 1
	}
	if resolved.Agent.Update.Strategy == "" {
		resolved.Agent.Update = spec.DefaultUpdateStrategy()
	}

	for name, m := range resolved.Models {
		if m.Replicas == 0 {
			m.Replicas = 1
		}
		if m.Update.Strategy == "" {
			m.Update = spec.DefaultUpdateStrategy()
		}
		resolved.Models[name] = m
	}

	for name, k := range resolved.Knowledge {
		if k.Replicas == 0 {
			k.Replicas = 1
		}
		if k.Update.Strategy == "" {
			k.Update = spec.DefaultUpdateStrategy()
		}
		if k.Persistent && k.Storage == nil {
			defaultStorage := spec.DefaultStorageConfig()
			k.Storage = &defaultStorage
		}
		resolved.Knowledge[name] = k
	}

	for name, t := range resolved.Tools {
		if t.Replicas == 0 {
			t.Replicas = 1
		}
		if t.Update.Strategy == "" {
			t.Update = spec.DefaultUpdateStrategy()
		}
		resolved.Tools[name] = t
	}

	return &resolved
}
