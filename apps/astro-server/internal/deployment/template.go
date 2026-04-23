package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/robfig/cron/v3"

	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
)

// providerEnvKey returns the env-var key for a provider entry.
// When isDuplicate is false, it returns basePrefix+"_"+suffix (e.g. "QDRANT_HOST").
// When isDuplicate is true, it returns basePrefix+"_"+NAME+"_"+suffix for all entries,
// and additionally basePrefix+"_"+suffix for the first entry (alphabetically).
func providerEnvKeys(basePrefix, name, suffix string, isDuplicate, isFirst bool) []string {
	if !isDuplicate {
		return []string{basePrefix + "_" + suffix}
	}
	keys := []string{basePrefix + "_" + spec.SanitizeEnvName(name) + "_" + suffix}
	if isFirst {
		keys = append(keys, basePrefix+"_"+suffix)
	}
	return keys
}

// TemplateInput holds the parameters needed to generate a deployment template.
type TemplateInput struct {
	Spec              *spec.AstroSpec
	AgentName         string // canonical agent name from the registry (used in DeploymentSource and image fallback)
	Account           string // account name (display, used in DeploymentSource)
	ECRNamespace      string // where this version's images physically live in ECR
	BuildID           string
	RegistryURL       string
	ProxyRegistryHost string // Host of the tenant's private image registry (e.g. "registry.astropods.ai")
	Environment       string // Environment prefix for ECR tenant repos (e.g. "prod", "preview")
}

// GenerateDeploymentTemplate creates a deployment spec template from a registered astro-spec.
// The template has placeholder values for user-fillable fields and ${} references
// for component wiring. The spec version is "deployment-template/v1".
func GenerateDeploymentTemplate(input TemplateInput) (*spec.AstroDeploymentSpec, error) {
	astroSpec := input.Spec
	if astroSpec == nil {
		return nil, fmt.Errorf("astro spec is required")
	}

	ds := &spec.AstroDeploymentSpec{
		Spec: "deployment-template/v1",
		Source: spec.DeploymentSource{
			Account:  input.Account,
			Name:     input.AgentName,
			Build:    input.BuildID,
			Registry: input.RegistryURL,
		},
		Target: spec.DeploymentTarget{
			Runtime: "kubernetes",
		},
		Observability: spec.DeploymentObservability{
			Enabled:   true,
			Provider:  "langfuse",
			Image:     resolveImage("astropods/collector:latest", input),
			Port:      4318,
			Resources: spec.CollectorResources,
		},
	}

	// Build agent environment with ${} references
	agentEnv := make(map[string]string)

	// Process models
	if len(astroSpec.Models) > 0 {
		// Count provider occurrences among self-hosted models
		modelProviderCount := make(map[string]int)
		for _, model := range astroSpec.Models {
			if model.IsProviderMode() && model.DeploysContainer(astroSpec.Providers) {
				if prov := spec.GetModelProvider(model.Provider); prov.EnvPrefix != "" {
					modelProviderCount[prov.EnvPrefix]++
				}
			}
		}

		// Sort model names for deterministic iteration
		modelNames := make([]string, 0, len(astroSpec.Models))
		for name := range astroSpec.Models {
			modelNames = append(modelNames, name)
		}
		sort.Strings(modelNames)

		// Track which provider prefix we've seen first
		modelProviderFirst := make(map[string]bool)

		for _, name := range modelNames {
			model := astroSpec.Models[name]
			if !model.DeploysContainer(astroSpec.Providers) {
				continue
			}

			if ds.Models == nil {
				ds.Models = make(map[string]spec.DeploymentModel)
			}
			dm := buildDeploymentModel(model, input)
			ds.Models[name] = dm

			// Wire references into agent environment
			// Determine primary endpoint name for port/url refs
			primaryEp := primaryEndpointName(dm.Endpoints)

			if model.IsProviderMode() {
				// Provider-specific env vars (e.g., OLLAMA_BASE_URL, OLLAMA_MODEL)
				prov := spec.GetModelProvider(model.Provider)
				if prov.EnvPrefix != "" {
					isDup := modelProviderCount[prov.EnvPrefix] > 1
					isFirst := !modelProviderFirst[prov.EnvPrefix]
					modelProviderFirst[prov.EnvPrefix] = true

					for _, key := range providerEnvKeys(prov.EnvPrefix, name, "HOST", isDup, isFirst) {
						agentEnv[key] = fmt.Sprintf("${models.%s.host}", name)
					}
					for _, key := range providerEnvKeys(prov.EnvPrefix, name, "PORT", isDup, isFirst) {
						agentEnv[key] = fmt.Sprintf("${models.%s.%s.port}", name, primaryEp)
					}
					for _, key := range providerEnvKeys(prov.EnvPrefix, name, "URL", isDup, isFirst) {
						agentEnv[key] = fmt.Sprintf("${models.%s.%s.url}", name, primaryEp)
					}
					for _, key := range providerEnvKeys(prov.EnvPrefix, name, "BASE_URL", isDup, isFirst) {
						agentEnv[key] = fmt.Sprintf("${models.%s.%s.url}", name, primaryEp) + "/api"
					}
					if models := model.ResolvedModels(); len(models) > 0 {
						joined := strings.Join(models, ",")
						for _, key := range providerEnvKeys(prov.EnvPrefix, name, "MODEL", isDup, isFirst) {
							agentEnv[key] = joined
						}
					}
				}
			} else {
				// Generic env vars for container-mode models
				envPrefix := fmt.Sprintf("MODEL_%s", spec.SanitizeEnvName(name))
				agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${models.%s.host}", name)
				agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${models.%s.%s.port}", name, primaryEp)
				agentEnv[envPrefix+"_URL"] = fmt.Sprintf("${models.%s.%s.url}", name, primaryEp)
			}
		}
	}

	// Process knowledge
	type knowledgeCredVar struct{ key, target string }
	var knowledgeCredVars []knowledgeCredVar
	if len(astroSpec.Knowledge) > 0 {
		// Count provider occurrences among self-hosted knowledge stores
		knowledgeProviderCount := make(map[string]int)
		for _, knowledge := range astroSpec.Knowledge {
			if knowledge.IsProviderMode() && knowledge.DeploysContainer(astroSpec.Providers) {
				if prov := spec.GetProvider(knowledge.Provider); prov.EnvPrefix != "" {
					knowledgeProviderCount[prov.EnvPrefix]++
				}
			}
		}

		// Sort knowledge names for deterministic iteration
		knowledgeNames := make([]string, 0, len(astroSpec.Knowledge))
		for name := range astroSpec.Knowledge {
			knowledgeNames = append(knowledgeNames, name)
		}
		sort.Strings(knowledgeNames)

		// Track which provider prefix we've seen first
		knowledgeProviderFirst := make(map[string]bool)

		for _, name := range knowledgeNames {
			knowledge := astroSpec.Knowledge[name]
			if !knowledge.DeploysContainer(astroSpec.Providers) {
				continue
			}

			if ds.Knowledge == nil {
				ds.Knowledge = make(map[string]spec.DeploymentKnowledge)
			}
			dk := buildDeploymentKnowledge(knowledge, input)
			ds.Knowledge[name] = dk

			primaryEp := primaryEndpointName(dk.Endpoints)

			// Wire references — use provider env prefix when available
			if knowledge.IsProviderMode() {
				prov := spec.GetProvider(knowledge.Provider)
				if prov.EnvPrefix != "" {
					isDup := knowledgeProviderCount[prov.EnvPrefix] > 1
					isFirst := !knowledgeProviderFirst[prov.EnvPrefix]
					knowledgeProviderFirst[prov.EnvPrefix] = true

					for _, key := range providerEnvKeys(prov.EnvPrefix, name, "HOST", isDup, isFirst) {
						agentEnv[key] = fmt.Sprintf("${knowledge.%s.host}", name)
					}
					for _, key := range providerEnvKeys(prov.EnvPrefix, name, "PORT", isDup, isFirst) {
						agentEnv[key] = fmt.Sprintf("${knowledge.%s.%s.port}", name, primaryEp)
					}
					// For postgres, inject auto-derived database name into the agent.
					if knowledge.Provider == "postgres" {
						dbName := spec.SanitizeDBName(input.AgentName)
						for _, key := range providerEnvKeys(prov.EnvPrefix, name, "DB", isDup, isFirst) {
							agentEnv[key] = dbName
						}
					}
					if prov.URLScheme != "" {
						for _, key := range providerEnvKeys(prov.EnvPrefix, name, "URL", isDup, isFirst) {
							agentEnv[key] = fmt.Sprintf("${knowledge.%s.%s.url}", name, primaryEp)
						}
					}

					// Collect credential variables for self-hosted providers
					// (injected into the variables map after it's created below).
					for _, cred := range prov.BindCredentials {
						for _, key := range providerEnvKeys(prov.EnvPrefix, name, strings.ToUpper(cred.Attr), isDup, isFirst) {
							knowledgeCredVars = append(knowledgeCredVars, knowledgeCredVar{
								key: key, target: "knowledge." + name,
							})
						}
						// Wire credential refs into agent environment so the agent
						// can connect to the knowledge store with proper per-name keys.
						// Skip "database" — already covered by explicit POSTGRES_DB injection above.
						if cred.Attr != "database" {
							for _, key := range providerEnvKeys(prov.EnvPrefix, name, strings.ToUpper(cred.Attr), isDup, isFirst) {
								agentEnv[key] = fmt.Sprintf("${knowledge.%s.credentials.%s}", name, cred.Attr)
							}
						}
					}
				} else {
					envPrefix := fmt.Sprintf("KNOWLEDGE_%s", spec.SanitizeEnvName(name))
					agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${knowledge.%s.host}", name)
					agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${knowledge.%s.%s.port}", name, primaryEp)
				}
			} else {
				envPrefix := fmt.Sprintf("KNOWLEDGE_%s", spec.SanitizeEnvName(name))
				agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${knowledge.%s.host}", name)
				agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${knowledge.%s.%s.port}", name, primaryEp)
			}
		}
	}

	// Process tools
	if len(astroSpec.Integrations) > 0 {
		for name, tool := range astroSpec.Integrations {
			if !tool.DeploysContainer(astroSpec.Providers) {
				continue
			}

			if ds.Integrations == nil {
				ds.Integrations = make(map[string]spec.DeploymentIntegration)
			}
			dt := buildDeploymentIntegration(tool, input)
			ds.Integrations[name] = dt

			primaryEp := primaryEndpointName(dt.Endpoints)
			envPrefix := fmt.Sprintf("INTEGRATION_%s", spec.SanitizeEnvName(name))
			agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${integrations.%s.host}", name)
			agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${integrations.%s.%s.port}", name, primaryEp)
			agentEnv[envPrefix+"_URL"] = fmt.Sprintf("${integrations.%s.%s.url}", name, primaryEp)
		}
	}

	// Build variables: merge provider credentials + user inputs
	variables := make(map[string]spec.Variable)

	// Extract credentials (cloud providers + custom provider secrets) → variables with secret:true
	validator := NewValidator()
	credInfos := validator.GetRequiredCredentials(astroSpec, nil)
	for _, ci := range credInfos {
		variables[ci.Key] = spec.Variable{
			Description: ci.Description,
			Optional:    ci.Optional,
			Secret:      true,
			Targets:     []string{"agent"},
		}
		// Wire credential references into agent environment
		agentEnv[ci.Key] = fmt.Sprintf("${variables.%s}", ci.Key)
	}

	// Inject self-hosted provider credentials collected during the knowledge loop.
	for _, cv := range knowledgeCredVars {
		variables[cv.key] = spec.Variable{
			Secret:  true,
			Targets: []string{cv.target},
		}
	}

	// Collect inputs from all sources into variables map
	collectVariablesFromInputs(astroSpec, ds, agentEnv, variables)

	if len(variables) > 0 {
		ds.Variables = variables
	}

	// Platform metadata
	agentEnv["ASTRO_AGENT_NAME"] = "${source.name}"
	agentEnv["ASTRO_AGENT_BUILD"] = "${source.build}"

	// Build agent block
	agentImage := resolveImage(astroSpec.Agent.Image, input)
	if agentImage == "" {
		return nil, fmt.Errorf("agent image is not set in spec")
	}
	agentEndpoints := map[string]spec.Endpoint{
		"http": {Port: 8080, Protocol: "http"},
	}
	// When the agent declares a frontend, expose its HTTP endpoint for ingress
	if astroSpec.Agent.HasFrontend() {
		agentEndpoints["http"] = spec.Endpoint{
			Port: 80, Protocol: "http",
			Expose: &spec.EndpointExpose{Enabled: true},
		}
	}
	ds.Agent = spec.DeploymentAgent{
		Image:       agentImage,
		Endpoints:   agentEndpoints,
		Replicas:    1,
		Resources:   spec.StandardResources,
		Environment: agentEnv,
		Healthcheck: astroSpec.Agent.Healthcheck,
		Update:      spec.DefaultUpdateStrategy(),
	}

	// Process ingestion
	if len(astroSpec.Ingestion) > 0 {
		ds.Ingestion = make(map[string]spec.DeploymentIngestion, len(astroSpec.Ingestion))
		for name, ingestion := range astroSpec.Ingestion {
			ds.Ingestion[name] = buildDeploymentIngestion(ingestion, input)
		}
	}

	// Interfaces block — only emitted when the agent supports messaging
	if astroSpec.Agent.HasMessaging() {
		ds.Interfaces = &spec.DeploymentInterfaces{
			Adapters:  []string{},
			Image:     resolveImage("astropods/messaging:latest", input),
			Resources: spec.MessagingResources,
			Endpoints: map[string]spec.Endpoint{
				"grpc": {Port: 9090, Protocol: "grpc"},
				"http": {Port: 8080, Protocol: "http", Expose: &spec.EndpointExpose{Enabled: false}},
			},
			Auth: &spec.DeploymentInterfacesAuth{
				Web: &spec.DeploymentWebAuth{Type: "oidc"},
			},
		}

		// All Slack-related variables are forced to targets: ["interface.slack"] so they
		// group under the Slack toggle in the deploy UI. We merge into existing
		// variables (preserving Value/Default from inputs) rather than overwriting.
		if ds.Variables == nil {
			ds.Variables = make(map[string]spec.Variable)
		}
		mergeSlackVar := func(key string, v spec.Variable) {
			if existing, ok := ds.Variables[key]; ok {
				existing.Targets = v.Targets
				existing.Description = v.Description
				existing.Label = v.Label
				existing.Placeholder = v.Placeholder
				existing.HelpURL = v.HelpURL
				ds.Variables[key] = existing
			} else {
				ds.Variables[key] = v
			}
		}
		mergeSlackVar("SLACK_BOT_TOKEN", spec.Variable{
			Description: "Slack bot token for API access and messaging",
			Label:       "Slack Bot Token",
			Placeholder: "xoxb-...",
			HelpURL:     "https://docs.slack.dev/authentication/tokens/",
			Optional:    true,
			Secret:      true,
			Targets:     []string{"interface.slack"},
		})
		mergeSlackVar("SLACK_APP_TOKEN", spec.Variable{
			Description: "Slack app-level token for socket mode connections",
			Label:       "Slack App Token",
			Placeholder: "xapp-...",
			HelpURL:     "https://docs.slack.dev/authentication/tokens/",
			Optional:    true,
			Secret:      true,
			Targets:     []string{"interface.slack"},
		})

		slackCfgDefault := slackConfigDefault(astroSpec)
		ds.Variables["SLACK_CONFIG"] = spec.Variable{
			Description: "Slack adapter configuration",
			Label:       "Slack Configuration",
			Datatype:    "object",
			Optional:    true,
			Secret:      false,
			Targets:     []string{"interface.slack"},
			Value:       slackCfgDefault,
			Default:     slackCfgDefault,
			Fields: map[string]spec.VariableField{
				"actionable_reactions": {
					Label:       "Actionable Reactions",
					Description: "Emoji names the bot acts on",
					Placeholder: "ticket, bug",
					Datatype:    "csv",
					Optional:    true,
				},
				"allowed_channel_ids": {
					Label:       "Allowed Channel IDs",
					Description: "Restrict to specific channels",
					Placeholder: "C12345, C67890",
					Datatype:    "csv",
					Optional:    true,
				},
				"allowed_user_ids": {
					Label:       "Allowed User IDs",
					Description: "Restrict to specific users",
					Placeholder: "U12345, U67890",
					Datatype:    "csv",
					Optional:    true,
				},
			},
		}

		wireInterfaceEnvironment(ds)
	}

	// Editable fields
	ds.Editable = defaultEditableFields()

	return ds, nil
}

// ShapeOptions carries optional dependencies for binding resolution in ShapeTemplate.
// nil is safe — binding shaping is simply skipped.
type ShapeOptions struct {
	KnowledgeStore *knowledgestore.Store
	AccountID      string
}

// ShapeTemplate applies deploy-time inputs (adapters, variables, bindings) to a base template
// and returns a TemplateResponse with the shaped template, variable schema, and validation.
func ShapeTemplate(ctx context.Context, base *spec.AstroDeploymentSpec, req *spec.TemplateRequest, opts *ShapeOptions) *spec.TemplateResponse {
	// Deep-copy via JSON round-trip so mutations don't affect the base.
	shaped := deepCopySpec(base)

	// --- Interface shaping ---
	if req.Interfaces != nil && shaped.Interfaces != nil {
		shaped.Interfaces.Adapters = req.Interfaces.Adapters
		if req.Interfaces.Auth != nil {
			shaped.Interfaces.Auth = req.Interfaces.Auth
		}
		// When web is selected, expose the HTTP endpoint for ingress.
		// (expose is editable, so this doesn't need to live in ApplyAdapterShaping)
		if slices.Contains(req.Interfaces.Adapters, "web") {
			if ep, ok := shaped.Interfaces.Endpoints["http"]; ok {
				if ep.Expose == nil {
					ep.Expose = &spec.EndpointExpose{}
				}
				ep.Expose.Enabled = true
				shaped.Interfaces.Endpoints["http"] = ep
			}
		}
	}

	// Apply all adapter-dependent mutations that touch server-owned fields.
	// Shared with the deploy handler so the two endpoints can't diverge.
	if req.Interfaces != nil {
		ApplyAdapterShaping(shaped, req.Interfaces.Adapters)
	}

	// --- Binding shaping ---
	var errs []spec.ValidationError
	var resolvedBindings *spec.ResolvedBindings
	if opts != nil && opts.KnowledgeStore != nil && req.Bindings != nil && len(req.Bindings.Knowledge) > 0 {
		resolved, bindingErrs := ResolveBindings(
			ctx, opts.KnowledgeStore, opts.AccountID,
			shaped.Knowledge, req.Bindings.Knowledge,
		)
		errs = append(errs, bindingErrs...)

		if len(resolved) > 0 {
			resolvedBindings = &spec.ResolvedBindings{
				Knowledge: make(map[string]spec.KnowledgeBindingInfo, len(resolved)),
			}
			// Build set of bound entry names for variable/editable filtering.
			boundNames := make(map[string]bool, len(resolved))
			for name, rb := range resolved {
				boundNames[name] = true
				// Zero container fields but preserve binding ARN and provider
				// so reference resolution can look up provider endpoints.
				shaped.Knowledge[name] = spec.DeploymentKnowledge{
					Binding:  rb.ARN,
					Provider: shaped.Knowledge[name].Provider,
				}
				resolvedBindings.Knowledge[name] = spec.KnowledgeBindingInfo{
					ARN: rb.ARN, Name: rb.Name, Provider: rb.Provider, Status: rb.Status,
				}
			}

			// Remove credential variables targeting bound entries.
			for key, v := range shaped.Variables {
				for _, t := range v.Targets {
					if strings.HasPrefix(t, "knowledge.") {
						entryName := strings.TrimPrefix(t, "knowledge.")
						if boundNames[entryName] {
							delete(shaped.Variables, key)
							break
						}
					}
				}
			}

			// Remove editable fields for bound entries.
			filtered := shaped.Editable[:0]
			for _, field := range shaped.Editable {
				exclude := false
				for name := range boundNames {
					if strings.HasPrefix(field, "knowledge."+name+".") {
						exclude = true
						break
					}
				}
				if !exclude {
					filtered = append(filtered, field)
				}
			}
			shaped.Editable = filtered
		}
	}

	// --- Variable filling ---
	for key, input := range req.Variables {
		if v, ok := shaped.Variables[key]; ok {
			if input.Value != "" {
				v.Value = input.Value
				v.Ref = ""
			} else if input.Ref != "" {
				v.Ref = input.Ref
				v.Value = ""
			}
			shaped.Variables[key] = v
		}
	}

	// --- Schedule shaping ---
	if len(req.Schedules) > 0 {
		for name, cron := range req.Schedules {
			if ing, ok := shaped.Ingestion[name]; ok && ing.Trigger.Type == "schedule" {
				ing.Trigger.Schedule = cron
				shaped.Ingestion[name] = ing
			}
		}
	}

	// --- Build response ---
	// Root Variables = full schema (from shaped copy, includes descriptions etc.)
	schemaVars := make(map[string]spec.Variable, len(shaped.Variables))
	maps.Copy(schemaVars, shaped.Variables)

	// Root Editable = promoted from template
	editable := shaped.Editable

	// Template = deployment/v1 ready: strip template-only fields
	shaped.Spec = "deployment/v1"
	shaped.Editable = nil
	for key, v := range shaped.Variables {
		v.Description = ""
		v.Label = ""
		v.Placeholder = ""
		v.HelpURL = ""
		v.Datatype = ""
		v.DisplayAs = ""
		v.Options = nil
		v.Fields = nil
		v.Default = ""
		shaped.Variables[key] = v
	}

	// --- Validation ---
	// Required variables (adapter shaping already flipped optionality,
	// so slack tokens are caught here when slack is selected).
	for key, v := range schemaVars {
		if !v.Optional && v.Value == "" && v.Ref == "" {
			errs = append(errs, spec.ValidationError{
				Field:   "variables." + key,
				Message: "required variable is empty",
			})
		}
	}

	// Ingestion cron validation
	for name, ing := range shaped.Ingestion {
		if ing.Trigger.Type == "schedule" {
			if ing.Trigger.Schedule == "" {
				errs = append(errs, spec.ValidationError{
					Field:   "ingestion." + name + ".trigger.schedule",
					Message: "cron expression required for schedule trigger",
				})
			} else if !isValidCron(ing.Trigger.Schedule) {
				errs = append(errs, spec.ValidationError{
					Field:   "ingestion." + name + ".trigger.schedule",
					Message: "invalid cron expression",
				})
			}
		}
	}

	// Sort errors for deterministic output
	sort.Slice(errs, func(i, j int) bool { return errs[i].Field < errs[j].Field })

	// Promote the user-editable interface config to the response root.
	respInterfaces := spec.TemplateInterfaces{Adapters: []string{}}
	if shaped.Interfaces != nil {
		if len(shaped.Interfaces.Adapters) > 0 {
			respInterfaces.Adapters = shaped.Interfaces.Adapters
		}
		respInterfaces.Auth = shaped.Interfaces.Auth
	}

	// Promote ingestion schedules to the response root.
	respSchedules := make(map[string]string)
	for name, ing := range shaped.Ingestion {
		if ing.Trigger.Type == "schedule" {
			respSchedules[name] = ing.Trigger.Schedule
		}
	}

	return &spec.TemplateResponse{
		Spec:       "deployment-template/v1",
		Template:   *shaped,
		Variables:  schemaVars,
		Editable:   editable,
		Interfaces: respInterfaces,
		Schedules:  respSchedules,
		Bindings:   resolvedBindings,
		Validation: spec.TemplateValidation{
			Valid:  len(errs) == 0,
			Errors: errs,
		},
	}
}

// deepCopySpec creates a deep copy of an AstroDeploymentSpec via JSON round-trip.
func deepCopySpec(s *spec.AstroDeploymentSpec) *spec.AstroDeploymentSpec {
	b, err := json.Marshal(s)
	if err != nil {
		// Should never happen with a well-formed spec.
		panic("deployment: failed to marshal spec for deep copy: " + err.Error())
	}
	var copy spec.AstroDeploymentSpec
	if err := json.Unmarshal(b, &copy); err != nil {
		panic("deployment: failed to unmarshal spec for deep copy: " + err.Error())
	}
	return &copy
}

// isValidCron checks whether a cron expression parses successfully.
func isValidCron(expr string) bool {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	return err == nil
}

// portNameToProtocol maps a provider-defined port name to one of the valid spec protocols
// (http, grpc, tcp). Unknown names default to tcp.
func portNameToProtocol(name string) string {
	switch name {
	case "http":
		return "http"
	case "grpc":
		return "grpc"
	default:
		return "tcp"
	}
}

// primaryEndpointName returns the name of the primary endpoint for env-var ref generation.
// Prefers "http"; otherwise first alphabetically.
func primaryEndpointName(endpoints map[string]spec.Endpoint) string {
	if _, ok := endpoints["http"]; ok {
		return "http"
	}
	names := make([]string, 0, len(endpoints))
	for name := range endpoints {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 0 {
		return names[0]
	}
	return "http"
}

// ApplyAdapterShaping normalises a deployment spec for the given adapter selection.
// It must be the single source of truth for every adapter-dependent mutation that
// touches server-owned fields, so that the POST template endpoint and the deploy
// handler stay in sync.  Both call sites pass the same adapter list and get the
// same result — no field-by-field patching in the deploy handler.
//
// Mutations applied:
//  1. Strip variables that belong exclusively to non-selected adapters, plus their
//     ${variables.KEY} references in interfaces.environment.
//  2. Flip slack token optionality based on whether "slack" is selected.
func ApplyAdapterShaping(ds *spec.AstroDeploymentSpec, selectedAdapters []string) {
	if ds.Interfaces == nil {
		return
	}
	selectedSet := make(map[string]bool, len(selectedAdapters))
	for _, a := range selectedAdapters {
		selectedSet[a] = true
	}

	// 1. Strip variables belonging exclusively to non-selected adapters.
	for key, v := range ds.Variables {
		if len(v.Targets) == 0 {
			continue
		}
		allInterface := true
		anySelected := false
		for _, t := range v.Targets {
			if !strings.HasPrefix(t, "interface.") {
				allInterface = false
				break
			}
			adapter := strings.TrimPrefix(t, "interface.")
			if selectedSet[adapter] {
				anySelected = true
			}
		}
		if allInterface && !anySelected {
			delete(ds.Variables, key)
			// Also remove the corresponding ${variables.KEY} reference from
			// interfaces.environment so it doesn't fail reference validation.
			delete(ds.Interfaces.Environment, key)
		}
	}

	// 2. Slack token optionality: required when slack is selected, optional otherwise.
	slackSelected := selectedSet["slack"]
	for _, key := range []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"} {
		if v, ok := ds.Variables[key]; ok {
			v.Optional = !slackSelected
			ds.Variables[key] = v
		}
	}
}

// RestoreBindingsFromSpec extracts knowledge binding ARNs from a stored
// deployment spec JSON. Returns nil if no bound entries are found (or on
// parse error). Used by the template handler to seed the TemplateRequest
// when the client opens the configure panel for an existing deployment.
func RestoreBindingsFromSpec(specJSON string) *spec.TemplateBindings {
	if specJSON == "" {
		return nil
	}
	var stored spec.AstroDeploymentSpec
	if err := json.Unmarshal([]byte(specJSON), &stored); err != nil {
		return nil
	}
	restored := make(map[string]string)
	for name, k := range stored.Knowledge {
		if k.IsBound() {
			restored[name] = k.Binding
		}
	}
	if len(restored) == 0 {
		return nil
	}
	return &spec.TemplateBindings{Knowledge: restored}
}

// ApplyBindingShaping adjusts a template so that knowledge entries whose
// submitted counterparts carry a binding ARN are zeroed to match the shape
// the client originally received from ShapeTemplate. Without this the
// EnforceEditable check would compare a full (unshaped) template against the
// shaped submitted spec and reject the server-owned fields.
func ApplyBindingShaping(template *spec.AstroDeploymentSpec, submitted *spec.AstroDeploymentSpec) {
	boundNames := make(map[string]bool)
	for name, k := range submitted.Knowledge {
		if k.IsBound() {
			boundNames[name] = true
			// Zero container fields but preserve binding + provider.
			template.Knowledge[name] = spec.DeploymentKnowledge{
				Binding:  k.Binding,
				Provider: template.Knowledge[name].Provider,
			}
		}
	}
	if len(boundNames) == 0 {
		return
	}

	// Remove credential variables targeting bound entries.
	for key, v := range template.Variables {
		for _, t := range v.Targets {
			if entryName, ok := strings.CutPrefix(t, "knowledge."); ok {
				if boundNames[entryName] {
					delete(template.Variables, key)
					break
				}
			}
		}
	}

	// Remove editable fields for bound entries.
	filtered := template.Editable[:0]
	for _, field := range template.Editable {
		exclude := false
		for name := range boundNames {
			if strings.HasPrefix(field, "knowledge."+name+".") {
				exclude = true
				break
			}
		}
		if !exclude {
			filtered = append(filtered, field)
		}
	}
	template.Editable = filtered
}

func buildDeploymentModel(model spec.Model, input TemplateInput) spec.DeploymentModel {
	container := model.ResolvedContainer()
	port := container.Port
	if port == 0 {
		port = 8080
	}

	dm := spec.DeploymentModel{
		Image:       resolveImage(container.Image, input),
		Endpoints:   spec.SingleEndpoint("http", port, "http"),
		Replicas:    1,
		Resources:   spec.StandardResources,
		Healthcheck: container.Healthcheck,
		Update:      spec.DefaultUpdateStrategy(),
	}

	// Provider-mode auto-configuration
	if model.IsProviderMode() {
		prov := spec.GetModelProvider(model.Provider)
		dm.Provider = model.Provider

		// Provider port overrides container port
		if prov.DefaultPort != 0 {
			dm.Endpoints = spec.SingleEndpoint("http", prov.DefaultPort, "http")
		}

		// Multi-port providers (e.g. qdrant: http + grpc)
		if len(prov.ExtraPorts) > 0 {
			for _, ep := range prov.ExtraPorts {
				dm.Endpoints[ep.Name] = spec.Endpoint{Port: ep.Port, Protocol: portNameToProtocol(ep.Name)}
			}
		}

		// Auto-enable GPU for providers that require it
		if prov.GPU {
			dm.Resources = spec.GPUResources
			dm.GPU = &spec.DeploymentGPU{
				Runtime: "cuda",
				Count:   1,
			}
			dm.Update = spec.UpdateStrategy{Strategy: "recreate"}
		}

		// Set model name(s) for pull
		if models := model.ResolvedModels(); len(models) > 0 {
			dm.Model = strings.Join(models, ",")
			dm.Persistent = true
		}

		// Model-aware readiness: verify model is pulled, not just server up
		if dm.Healthcheck == nil {
			if models := model.ResolvedModels(); len(models) > 0 {
				// Build compound grep check: each model must be present
				var checks []string
				for _, m := range models {
					checks = append(checks, fmt.Sprintf("ollama list | grep -q '%s'", m))
				}
				dm.Healthcheck = &spec.Healthcheck{
					Test: []string{"sh", "-c",
						strings.Join(checks, " && "),
					},
					Interval: "15s",
					Timeout:  "5s",
					Retries:  40,
				}
			} else if prov.HealthPath != "" {
				dm.Healthcheck = &spec.Healthcheck{Path: prov.HealthPath}
			} else if len(prov.HealthCheck) > 0 {
				dm.Healthcheck = &spec.Healthcheck{Test: prov.HealthCheck}
			}
		}
	}

	// Container-mode GPU (explicit gpu block in the spec)
	if container.HasGPU() {
		dm.Resources = spec.GPUResources
		dm.GPU = &spec.DeploymentGPU{
			VRAM:    container.GPU.VRAM,
			Runtime: container.GPU.Runtime,
			Count:   1,
		}
		if dm.GPU.Runtime == "" {
			dm.GPU.Runtime = "cuda"
		}
		dm.Update = spec.UpdateStrategy{Strategy: "recreate"}
	}

	if len(container.Environment) > 0 {
		dm.Environment = container.Environment
	}
	return dm
}

func buildDeploymentKnowledge(knowledge spec.Knowledge, input TemplateInput) spec.DeploymentKnowledge {
	container := knowledge.ResolvedContainer()
	port := container.Port
	if port == 0 {
		port = 8080
	}

	dk := spec.DeploymentKnowledge{
		Image:       resolveImage(container.Image, input),
		Endpoints:   spec.SingleEndpoint("http", port, "http"),
		Replicas:    1,
		Resources:   spec.StandardResources,
		Persistent:  container.Persistent,
		Healthcheck: container.Healthcheck,
		Update:      spec.DefaultUpdateStrategy(),
		Provider:    knowledge.Provider,
	}

	// Provider-specific port and multi-port
	if knowledge.IsProviderMode() {
		prov := spec.GetProvider(knowledge.Provider)
		if prov.DefaultPort != 0 {
			dk.Endpoints = spec.SingleEndpoint("http", prov.DefaultPort, "http")
		}
		if len(prov.ExtraPorts) > 0 {
			for _, ep := range prov.ExtraPorts {
				dk.Endpoints[ep.Name] = spec.Endpoint{Port: ep.Port, Protocol: portNameToProtocol(ep.Name)}
			}
		}

		// Provider-specific healthcheck
		if dk.Healthcheck == nil {
			if prov.HealthCheck != nil {
				dk.Healthcheck = &spec.Healthcheck{Test: prov.HealthCheck}
			} else if prov.HealthPath != "" {
				dk.Healthcheck = &spec.Healthcheck{Path: prov.HealthPath}
			}
		}
	}

	if dk.Persistent {
		dk.Storage = &spec.StorageConfig{
			Size:       "10Gi",
			AccessMode: "ReadWriteOnce",
		}
		// Persistent stores default to recreate strategy
		dk.Update = spec.UpdateStrategy{Strategy: "recreate"}
	}

	// Resolve volume mount path: container.volume > provider mount path
	if container.Volume != "" {
		dk.Volume = container.Volume
	} else if prov := spec.GetProvider(knowledge.Provider); prov.MountPath != "" {
		dk.Volume = prov.MountPath
	}
	if len(container.Environment) > 0 {
		dk.Environment = container.Environment
	}

	// Inject provider default env vars
	if knowledge.IsProviderMode() {
		prov := spec.GetProvider(knowledge.Provider)
		if len(prov.DefaultEnv) > 0 {
			if dk.Environment == nil {
				dk.Environment = make(map[string]string)
			}
			for k, v := range prov.DefaultEnv {
				if _, exists := dk.Environment[k]; !exists {
					dk.Environment[k] = v
				}
			}
		}
	}

	// For postgres, inject auto-derived database name into the knowledge container
	// so the entrypoint creates the database on first init.
	if knowledge.Provider == "postgres" {
		if dk.Environment == nil {
			dk.Environment = make(map[string]string)
		}
		if _, exists := dk.Environment["POSTGRES_DB"]; !exists {
			dk.Environment["POSTGRES_DB"] = spec.SanitizeDBName(input.AgentName)
		}
	}

	return dk
}

func buildDeploymentIntegration(tool spec.Integration, input TemplateInput) spec.DeploymentIntegration {
	port := 8080
	dt := spec.DeploymentIntegration{
		Replicas:  1,
		Resources: spec.StandardResources,
		Update:    spec.DefaultUpdateStrategy(),
	}
	if tool.Container != nil {
		dt.Image = resolveImage(tool.Container.Image, input)
		if tool.Container.Port != 0 {
			port = tool.Container.Port
		}
		dt.Healthcheck = tool.Container.Healthcheck
		if len(tool.Container.Environment) > 0 {
			dt.Environment = tool.Container.Environment
		}
	}
	dt.Endpoints = spec.SingleEndpoint("http", port, "http")
	return dt
}

func buildDeploymentIngestion(ingestion spec.Ingestion, input TemplateInput) spec.DeploymentIngestion {
	image := resolveImage(ingestion.Container.Image, input)
	di := spec.DeploymentIngestion{
		Image:     image,
		Resources: spec.StandardResources,
		Trigger: spec.DeploymentTrigger{
			Type: ingestion.Trigger.Type,
		},
		Healthcheck: ingestion.Container.Healthcheck,
	}
	if len(ingestion.Container.Environment) > 0 {
		di.Environment = ingestion.Container.Environment
	}
	// Webhook triggers expose a port via endpoints
	if ingestion.Container.Port > 0 {
		di.Endpoints = spec.SingleEndpoint("http", ingestion.Container.Port, "http")
	}
	// Schedule triggers get an empty placeholder
	if ingestion.Trigger.Type == "schedule" {
		di.Trigger.Schedule = ""
	}
	return di
}

// resolveImage maps an image reference to its final pull path:
//   - Tenant images (hosted on ProxyRegistryHost) → ECR tenant repo: {ecrHost}/{env}-tenant-{account}/{image}
//   - Public images (bare Docker Hub reference, no registry host) → ECR pull-through cache: {ecrHost}/dockerhub/{image}
//     Official library images (no org prefix) are placed under "library/".
//   - Third-party images (explicit registry host such as gcr.io, ghcr.io) → unchanged.
func resolveImage(image string, input TemplateInput) string {
	if image == "" {
		return image
	}

	// 1. Tenant image → ECR tenant repo
	if input.ProxyRegistryHost != "" && input.RegistryURL != "" && strings.HasPrefix(image, input.ProxyRegistryHost+"/") {
		pathWithTag := strings.TrimPrefix(image, input.ProxyRegistryHost+"/")
		parts := strings.SplitN(pathWithTag, "/", 2)
		if len(parts) >= 2 {
			// Use ECRNamespace (frozen at push time) instead of the account name
			// parsed from the image path, so transferred agents resolve correctly.
			ns := input.ECRNamespace
			if ns == "" {
				ns = parts[0]
			}
			return fmt.Sprintf("%s/%s-tenant-%s/%s", stripScheme(input.RegistryURL), input.Environment, ns, parts[1])
		}
		return image
	}

	// 2. Public image (no registry host in first segment) → ECR pull-through cache.
	// In local environments images are pulled directly from Docker Hub (or are
	// already present in the local daemon), so we skip the rewrite.
	if input.RegistryURL != "" && input.Environment != "local" {
		name := image
		if i := strings.LastIndex(image, ":"); i >= 0 {
			name = image[:i]
		}
		firstSegment := name
		if i := strings.Index(name, "/"); i >= 0 {
			firstSegment = name[:i]
		}
		if !strings.Contains(firstSegment, ".") && !strings.Contains(firstSegment, ":") {
			imageName, tag := image, ""
			if i := strings.LastIndex(image, ":"); i >= 0 {
				imageName, tag = image[:i], image[i:]
			}
			if !strings.Contains(imageName, "/") {
				imageName = "library/" + imageName
			}
			return fmt.Sprintf("%s/dockerhub/%s%s", stripScheme(input.RegistryURL), imageName, tag)
		}
	}

	// 3. Third-party image (explicit registry host) → unchanged
	return image
}

func stripScheme(url string) string {
	if idx := strings.Index(url, "://"); idx >= 0 {
		url = url[idx+3:]
	}
	return strings.TrimRight(url, "/")
}

// wireInterfaceEnvironment populates interfaces.environment with ${variables.KEY}
// references for every non-secret variable that targets an interface adapter.
// Secrets are excluded because they flow through the k8s Secret, not the ConfigMap.
func wireInterfaceEnvironment(ds *spec.AstroDeploymentSpec) {
	if ds.Interfaces == nil || len(ds.Variables) == 0 {
		return
	}
	for key, v := range ds.Variables {
		if v.Secret {
			continue
		}
		for _, t := range v.Targets {
			if strings.HasPrefix(t, "interface.") {
				if ds.Interfaces.Environment == nil {
					ds.Interfaces.Environment = make(map[string]string)
				}
				ds.Interfaces.Environment[key] = fmt.Sprintf("${variables.%s}", key)
				break
			}
		}
	}
}

func slackConfigDefault(s *spec.AstroSpec) string {
	cfg := s.Dev.SlackConfig()
	if cfg == nil {
		return ""
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return string(b)
}

func defaultEditableFields() []string {
	return []string{
		"agent.replicas",
		"agent.resources",
		"agent.environment",
		"agent.healthcheck",
		"agent.update",
		"agent.endpoints.*.expose",
		"models.*.replicas",
		"models.*.resources",
		"models.*.gpu",
		"models.*.environment",
		"models.*.healthcheck",
		"models.*.update",
		"knowledge.*.replicas",
		"knowledge.*.resources",
		"knowledge.*.storage",
		"knowledge.*.environment",
		"knowledge.*.healthcheck",
		"knowledge.*.update",
		"integrations.*.replicas",
		"integrations.*.resources",
		"integrations.*.environment",
		"integrations.*.healthcheck",
		"integrations.*.update",
		"ingestion.*.resources",
		"ingestion.*.trigger.schedule",
		"ingestion.*.environment",
		"interfaces.adapters",
		"interfaces.resources",
		"interfaces.endpoints.*.expose",
		"variables.*.value",
		"variables.*.targets",
		"observability.enabled",
		"observability.resources",
		"observability.environment",
	}
}

// collectVariablesFromInputs gathers all Input declarations from the astro spec into
// the variables map and injects default values into the relevant container environments.
func collectVariablesFromInputs(astroSpec *spec.AstroSpec, ds *spec.AstroDeploymentSpec, agentEnv map[string]string, variables map[string]spec.Variable) {
	addVariable := func(input spec.Input, targets []string) {
		v := spec.Variable{
			Datatype:    input.Datatype,
			Secret:      input.Secret,
			Description: input.Description,
			DisplayAs:   input.DisplayAs,
			Options:     input.Options,
			Default:     input.Default,
			Optional:    input.Optional,
			Targets:     targets,
		}
		if input.Default != "" {
			v.Value = input.Default
		}
		// Merge: if the variable already exists (e.g. from credentials), preserve
		// its secret/targets but fill in Default/Value from the input so that
		// user-specified defaults are not lost.
		if existing, exists := variables[input.Name]; exists {
			if existing.Default == "" && input.Default != "" {
				existing.Default = input.Default
				existing.Value = input.Default
				variables[input.Name] = existing
			}
		} else {
			variables[input.Name] = v
		}
	}

	// Top-level inputs → agent + ingestion
	for _, inp := range astroSpec.Inputs {
		addVariable(inp, []string{"agent", "ingestion"})
		if !inp.Secret {
			agentEnv[inp.Name] = fmt.Sprintf("${variables.%s}", inp.Name)
		}
	}

	// Agent inputs → agent only
	for _, inp := range astroSpec.Agent.Inputs {
		addVariable(inp, []string{"agent"})
		if !inp.Secret {
			agentEnv[inp.Name] = fmt.Sprintf("${variables.%s}", inp.Name)
		}
	}

	// Model inputs — inject defaults into model environment directly (not in variables)
	for name, model := range astroSpec.Models {
		for _, inp := range model.Inputs {
			if inp.Default != "" && ds.Models != nil {
				if dm, ok := ds.Models[name]; ok {
					if dm.Environment == nil {
						dm.Environment = make(map[string]string)
					}
					dm.Environment[inp.Name] = inp.Default
					ds.Models[name] = dm
				}
			}
		}
	}

	// Knowledge inputs — inject defaults into knowledge environment directly
	for name, knowledge := range astroSpec.Knowledge {
		for _, inp := range knowledge.Inputs {
			if inp.Default != "" && ds.Knowledge != nil {
				if dk, ok := ds.Knowledge[name]; ok {
					if dk.Environment == nil {
						dk.Environment = make(map[string]string)
					}
					dk.Environment[inp.Name] = inp.Default
					ds.Knowledge[name] = dk
				}
			}
		}
	}

	// Tool inputs — inject defaults into tool environment directly
	for name, tool := range astroSpec.Integrations {
		for _, inp := range tool.Inputs {
			if inp.Default != "" && ds.Integrations != nil {
				if dt, ok := ds.Integrations[name]; ok {
					if dt.Environment == nil {
						dt.Environment = make(map[string]string)
					}
					dt.Environment[inp.Name] = inp.Default
					ds.Integrations[name] = dt
				}
			}
		}
	}

	// Ingestion inputs → ingestion.<name> target
	for name, ingestion := range astroSpec.Ingestion {
		for _, inp := range ingestion.Inputs {
			addVariable(inp, []string{"ingestion." + name})
			if inp.Default != "" && ds.Ingestion != nil {
				if di, ok := ds.Ingestion[name]; ok {
					if di.Environment == nil {
						di.Environment = make(map[string]string)
					}
					di.Environment[inp.Name] = inp.Default
					ds.Ingestion[name] = di
				}
			}
		}
	}
}
