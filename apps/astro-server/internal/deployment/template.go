package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	spec "github.com/astropods/astro-spec"
	"github.com/robfig/cron/v3"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"

	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// ProviderEnvKeys returns the env-var keys for a provider entry, following
// RFC-1 §8.1 and §8.2. It accepts:
//   - basePrefix:   the provider's env prefix (e.g. "POSTGRES")
//   - entryName:    the user's name for this entry (e.g. "users")
//   - providerName: the canonical provider name (e.g. "postgres")
//   - suffix:       the field suffix (e.g. "HOST", "USER")
//   - isDuplicate:  true when multiple entries share the same provider
//   - isPrimary:    true when this entry is the "primary" — the entry whose
//     name matches the provider, or the first alphabetically when no name
//     matches.
//
// Rules:
//   - Single entry (!isDuplicate)                       → bare only.
//   - Multiple entries, entryName == providerName       → bare only (qualified is redundant).
//   - Multiple entries, !isPrimary                      → qualified only.
//   - Multiple entries, isPrimary, entryName != provider → qualified + bare.
func ProviderEnvKeys(basePrefix, entryName, providerName, suffix string, isDuplicate, isPrimary bool) []string {
	if !isDuplicate {
		return []string{basePrefix + "_" + suffix}
	}
	if entryName == providerName {
		// Skip the redundant qualified form; the bare key is sufficient.
		return []string{basePrefix + "_" + suffix}
	}
	keys := []string{basePrefix + "_" + spec.SanitizeEnvName(entryName) + "_" + suffix}
	if isPrimary {
		keys = append(keys, basePrefix+"_"+suffix)
	}
	return keys
}

// PickPrimaryName returns the entry name that should receive the bare key
// among a group of entries sharing one provider. The entry whose name matches
// the provider name wins; otherwise the alphabetically-first entry is primary.
// names is mutated (sorted) — caller must pass a copy if it cares.
func PickPrimaryName(names []string, providerName string) string {
	for _, n := range names {
		if n == providerName {
			return n
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[0]
}

// TemplateInput holds the parameters needed to generate a deployment template.
type TemplateInput struct {
	Spec              *spec.AstroSpec
	AgentName         string // canonical agent name from the registry (used in DeploymentSource and image fallback)
	Account           string // account name (display, used in DeploymentSource)
	BuildID           string
	RegistryURL       string
	ProxyRegistryHost string // Host of the tenant's private image registry (e.g. "registry.astropods.ai")
	Environment       string // Environment prefix for ECR tenant repos (e.g. "prod", "preview")
	MessagingImage    string // Override for the messaging sidecar image; empty uses defaultMessagingImage
}

// defaultMessagingImage is the messaging sidecar image reference used when no
// override is supplied via TemplateInput.MessagingImage (MESSAGING_IMAGE). It is
// a bare Docker Hub reference; resolveImage rewrites it to the ECR pull-through
// path at generation time.
const defaultMessagingImage = "astropods/messaging:latest"

// GenerateDeploymentTemplate creates a deployment spec template from a registered astro-spec.
// The template has placeholder values for user-fillable fields and ${} references
// for component wiring. The spec version is "deployment-template/v1".
func GenerateDeploymentTemplate(input TemplateInput) (*AstroDeploymentSpec, error) {
	astroSpec := input.Spec
	if astroSpec == nil {
		return nil, fmt.Errorf("astro spec is required")
	}

	ds := &AstroDeploymentSpec{
		Spec: "deployment-template/v1",
		Source: DeploymentSource{
			Account:  input.Account,
			Name:     input.AgentName,
			Build:    input.BuildID,
			Registry: input.RegistryURL,
		},
		Target: DeploymentTarget{
			Runtime: "kubernetes",
		},
		Observability: DeploymentObservability{
			Enabled:   true,
			Provider:  "langfuse",
			Image:     resolveImage("astropods/collector:latest", input),
			Port:      4318,
			Resources: CollectorResources,
		},
	}

	// Build agent environment with ${} references
	agentEnv := make(map[string]string)

	// Process models. Only container-mode models deploy a sidecar; provider-mode
	// models are cloud (credentials only) or custom, and inject no connection wiring.
	if len(astroSpec.Models) > 0 {
		modelNames := make([]string, 0, len(astroSpec.Models))
		for name := range astroSpec.Models {
			modelNames = append(modelNames, name)
		}
		sort.Strings(modelNames)

		for _, name := range modelNames {
			model := astroSpec.Models[name]
			if !model.DeploysContainer(astroSpec.Providers) {
				continue
			}

			if ds.Models == nil {
				ds.Models = make(map[string]DeploymentModel)
			}
			dm := buildDeploymentModel(model, name, input)
			ds.Models[name] = dm

			// Wire container-mode connection env vars into the agent environment.
			primaryEp := primaryEndpointName(dm.Endpoints)
			envPrefix := fmt.Sprintf("MODEL_%s", spec.SanitizeEnvName(name))
			agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${models.%s.host}", name)
			agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${models.%s.%s.port}", name, primaryEp)
			agentEnv[envPrefix+"_URL"] = fmt.Sprintf("${models.%s.%s.url}", name, primaryEp)
		}
	}

	// Process knowledge
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

		// For each provider prefix, identify the primary entry (RFC §8.2):
		// the entry whose name matches the provider, or the alphabetically-
		// first when none matches. Primary gets the bare key.
		knowledgePrimaryByPrefix := make(map[string]string)
		knowledgeGroupNames := make(map[string][]string)
		for _, name := range knowledgeNames {
			knowledge := astroSpec.Knowledge[name]
			if !knowledge.IsProviderMode() || !knowledge.DeploysContainer(astroSpec.Providers) {
				continue
			}
			prov := spec.GetProvider(knowledge.Provider)
			if prov.EnvPrefix == "" {
				continue
			}
			knowledgeGroupNames[prov.EnvPrefix] = append(knowledgeGroupNames[prov.EnvPrefix], name)
		}
		for prefix, names := range knowledgeGroupNames {
			provName := astroSpec.Knowledge[names[0]].Provider
			knowledgePrimaryByPrefix[prefix] = PickPrimaryName(append([]string(nil), names...), provName)
		}

		for _, name := range knowledgeNames {
			knowledge := astroSpec.Knowledge[name]
			if !knowledge.DeploysContainer(astroSpec.Providers) {
				continue
			}

			if ds.Knowledge == nil {
				ds.Knowledge = make(map[string]DeploymentKnowledge)
			}
			dk := buildDeploymentKnowledge(knowledge, name, input)
			ds.Knowledge[name] = dk

			primaryEp := primaryEndpointName(dk.Endpoints)

			// Wire references — use provider env prefix when available
			if knowledge.IsProviderMode() {
				prov := spec.GetProvider(knowledge.Provider)
				if prov.EnvPrefix != "" {
					isDup := knowledgeProviderCount[prov.EnvPrefix] > 1
					isPrimary := knowledgePrimaryByPrefix[prov.EnvPrefix] == name

					for _, key := range ProviderEnvKeys(prov.EnvPrefix, name, knowledge.Provider, "HOST", isDup, isPrimary) {
						agentEnv[key] = fmt.Sprintf("${knowledge.%s.host}", name)
					}
					for _, key := range ProviderEnvKeys(prov.EnvPrefix, name, knowledge.Provider, "PORT", isDup, isPrimary) {
						agentEnv[key] = fmt.Sprintf("${knowledge.%s.%s.port}", name, primaryEp)
					}
					if prov.URLScheme != "" {
						for _, key := range ProviderEnvKeys(prov.EnvPrefix, name, knowledge.Provider, "URL", isDup, isPrimary) {
							agentEnv[key] = fmt.Sprintf("${knowledge.%s.%s.url}", name, primaryEp)
						}
					}

					// Knowledge credential env vars (POSTGRES_USER/_PASSWORD/_DB
					// per store, with per-store renaming for multi-store
					// disambiguation) are NOT injected into agent.Environment.
					// They flow directly via secretKeyRef entries the applier
					// emits from knowledgeCredEnvVars — no resolved-value path,
					// no duplicate with the agent's full Secret. The same
					// secretKeyRef list is also passed to ingestion containers
					// so they keep their access to provider creds.
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
				ds.Integrations = make(map[string]DeploymentIntegration)
			}
			dt := buildDeploymentIntegration(tool, name, input)
			ds.Integrations[name] = dt

			primaryEp := primaryEndpointName(dt.Endpoints)
			envPrefix := fmt.Sprintf("INTEGRATION_%s", spec.SanitizeEnvName(name))
			agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${integrations.%s.host}", name)
			agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${integrations.%s.%s.port}", name, primaryEp)
			agentEnv[envPrefix+"_URL"] = fmt.Sprintf("${integrations.%s.%s.url}", name, primaryEp)
		}
	}

	// Build variables: merge provider credentials + user inputs
	variables := make(map[string]Variable)

	// Extract credentials (cloud providers + custom provider secrets) → variables with secret:true
	validator := NewValidator()
	credInfos := validator.GetRequiredCredentials(astroSpec, nil)
	for _, ci := range credInfos {
		variables[ci.Key] = Variable{
			Description: ci.Description,
			Optional:    ci.Optional,
			Secret:      true,
			Targets:     []string{"agent"},
		}
		// Wire credential references into agent environment
		agentEnv[ci.Key] = fmt.Sprintf("${variables.%s}", ci.Key)
	}

	// Build ingestion before collecting inputs so that component-level input
	// defaults can be injected into the ingestion container environment.
	if len(astroSpec.Ingestion) > 0 {
		ds.Ingestion = make(map[string]DeploymentIngestion, len(astroSpec.Ingestion))
		for name, ingestion := range astroSpec.Ingestion {
			ds.Ingestion[name] = buildDeploymentIngestion(ingestion, name, input)
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

	// Frontend agents must serve on :80 (spec §validation rule 15). Inject PORT
	// so frameworks that read it (Express, FastAPI, etc.) bind to the correct
	// port instead of their default (3000 / 8000) and crash-looping behind ingress.
	if astroSpec.Agent.HasFrontend() {
		agentEnv["PORT"] = "80"
	}

	// Build agent block
	agentImage := resolveBuiltImage(spec.ComponentAgent, "", astroSpec.Agent.Image, astroSpec.Agent.Build, input)
	if agentImage == "" {
		return nil, fmt.Errorf("agent image is not set in spec")
	}
	agentEndpoints := map[string]Endpoint{
		"http": {Port: 8080, Protocol: "http"},
	}
	// When the agent declares a frontend, expose its HTTP endpoint for ingress
	if astroSpec.Agent.HasFrontend() {
		agentEndpoints["http"] = Endpoint{
			Port: 80, Protocol: "http",
			Expose: &EndpointExpose{Enabled: true},
		}
	}
	// Every deployment gets a persistent disk by default. Defaulting the volume
	// mount routes the agent through the StatefulSet + PVC path; the messaging
	// sidecar shares this volume (see spec_applier.go). Users may override size,
	// class, or mount path via provisioning (applyVolume).
	agentStorage := DefaultStorageConfig()
	agentStorage.Size = DefaultAgentStorageSize
	ds.Agent = DeploymentAgent{
		Image:           agentImage,
		Endpoints:       agentEndpoints,
		Replicas:        1,
		Resources:       StandardResources,
		Volume:          spec.DefaultAgentVolumeMount,
		Storage:         &agentStorage,
		Environment:     agentEnv,
		Healthcheck:     astroSpec.Agent.Healthcheck,
		Update:          DefaultUpdateStrategy(),
		AIGateway:       astroSpec.UsesGateway(),
		ResponseTimeout: DefaultResponseTimeout,
	}

	// Interfaces block — only emitted when the agent supports messaging
	if astroSpec.Agent.HasMessaging() {
		messagingImage := input.MessagingImage
		if messagingImage == "" {
			messagingImage = defaultMessagingImage
		}
		ds.Interfaces = &DeploymentInterfaces{
			Adapters:  []string{},
			Image:     resolveImage(messagingImage, input),
			Resources: MessagingResources,
			Endpoints: map[string]Endpoint{
				"grpc": {Port: 9090, Protocol: "grpc"},
				"http": {Port: 8080, Protocol: "http", Expose: &EndpointExpose{Enabled: false}},
			},
			Auth: &DeploymentInterfacesAuth{
				Web: &DeploymentWebAuth{Type: "oidc"},
			},
		}

		if ds.Variables == nil {
			ds.Variables = make(map[string]Variable)
		}

		// Slack variables are NOT injected here. They are adapter-specific and
		// only belong in the spec once the user selects the slack adapter.
		// ApplyAdapterShaping handles injection at that point.

		wireInterfaceEnvironment(ds)
	}

	return ds, nil
}

// applyCompute overlays a user-facing ComponentCompute on top of base
// DeploymentResources. Any non-empty field on c becomes both request and
// limit on the result (Guaranteed QoS); empty fields keep the base values.
func applyCompute(base DeploymentResources, c *ComponentCompute) DeploymentResources {
	out := base
	if c == nil {
		return out
	}
	if c.CPU != "" {
		out.CPU = c.CPU
		out.CPULimit = c.CPU
	}
	if c.Memory != "" {
		out.Memory = c.Memory
		out.MemoryLimit = c.Memory
	}
	return out
}

// applyVolume overlays a ComponentVolume override onto the agent's
// existing volume + storage. A non-empty Mount switches the agent to
// the StatefulSet path; supplying Storage when no mount exists is a
// no-op (the user must set a mount first).
func applyVolume(agent *DeploymentAgent, v *ComponentVolume) {
	if v == nil {
		return
	}
	if v.Mount != "" {
		agent.Volume = v.Mount
	}
	if agent.Volume == "" {
		return
	}
	if agent.Storage == nil {
		def := DefaultStorageConfig()
		agent.Storage = &def
	}
	if v.Storage == nil {
		return
	}
	if v.Storage.Size != "" {
		agent.Storage.Size = v.Storage.Size
	}
	if v.Storage.Class != "" {
		agent.Storage.Class = v.Storage.Class
	}
	if v.Storage.AccessMode != "" {
		agent.Storage.AccessMode = v.Storage.AccessMode
	}
}

// ShapeOptions carries optional dependencies for binding resolution in ShapeTemplate.
// nil is safe — binding shaping is simply skipped.
type ShapeOptions struct {
	KnowledgeStore *knowledgestore.Store
	AccountID      string
	// ConfiguredInlineSecrets lists user_var names with stored inline secrets
	// (omitted from the client on configure prefill). Marked on the schema map
	// before validation so required checks treat them as filled.
	ConfiguredInlineSecrets []string
}

func markConfiguredInlineSecrets(
	vars map[string]Variable,
	names []string,
	inputs map[string]VariableInput,
) {
	for _, name := range names {
		v, ok := vars[name]
		if !ok || !v.Secret {
			continue
		}
		if input, supplied := inputs[name]; supplied && (input.Value != "" || input.Ref != "") {
			continue
		}
		v.Configured = true
		v.Value = ""
		v.Ref = ""
		vars[name] = v
	}
}

// ShapeTemplate applies deploy-time inputs (adapters, variables, bindings) to a base template
// and returns a TemplateResponse with the shaped template, variable schema, and validation.
func ShapeTemplate(ctx context.Context, base *AstroDeploymentSpec, req *TemplateRequest, opts *ShapeOptions) *TemplateResponse {
	// Deep-copy via JSON round-trip so mutations don't affect the base.
	shaped := deepCopySpec(base)

	// --- Interface shaping ---
	// A custom-interface-only agent omits a messaging interfaces block, but its
	// access config (interfaces.auth.custom) still rides in via the request, so
	// create the block when the request carries auth. Messaging stays gated on a
	// non-empty adapter list, so an auth-only block spins up no sidecar.
	if req.Interfaces != nil && (shaped.Interfaces != nil || len(req.Interfaces.Adapters) > 0 || req.Interfaces.Auth != nil) {
		if shaped.Interfaces == nil {
			shaped.Interfaces = &DeploymentInterfaces{}
		}
		shaped.Interfaces.Adapters = req.Interfaces.Adapters
		if req.Interfaces.Auth != nil {
			shaped.Interfaces.Auth = req.Interfaces.Auth
		}
		// When web is selected, expose the HTTP endpoint for ingress.
		// (expose is editable, so this doesn't need to live in ApplyAdapterShaping)
		if slices.Contains(req.Interfaces.Adapters, "web") {
			if ep, ok := shaped.Interfaces.Endpoints["http"]; ok {
				if ep.Expose == nil {
					ep.Expose = &EndpointExpose{}
				}
				ep.Expose.Enabled = true
				shaped.Interfaces.Endpoints["http"] = ep
			}
		}
	}

	// Apply all adapter-dependent mutations that touch server-owned fields.
	// Always runs against the shaped template's current adapter list — even
	// when the request didn't supply an interfaces block — so prefill paths
	// (e.g. POST {deployment_id} with no overrides) still get clean output
	// without dead variables/env refs from non-selected adapters.
	if shaped.Interfaces != nil {
		ApplyAdapterShaping(shaped, shaped.Interfaces.Adapters)
	}

	// --- Binding shaping ---
	var errs []ValidationError
	var resolvedBindings *ResolvedBindings
	if opts != nil && opts.KnowledgeStore != nil && req.Bindings != nil && len(req.Bindings.Knowledge) > 0 {
		resolved, bindingErrs := ResolveBindings(
			ctx, opts.KnowledgeStore, opts.AccountID,
			shaped.Knowledge, req.Bindings.Knowledge,
		)
		errs = append(errs, bindingErrs...)

		if len(resolved) > 0 {
			resolvedBindings = &ResolvedBindings{
				Knowledge: make(map[string]KnowledgeBindingInfo, len(resolved)),
			}
			// Build set of bound entry names for variable/editable filtering.
			boundNames := make(map[string]bool, len(resolved))
			for name, rb := range resolved {
				boundNames[name] = true
				// Zero container fields but preserve binding ARN and provider
				// so reference resolution can look up provider endpoints.
				shaped.Knowledge[name] = DeploymentKnowledge{
					Binding:  rb.ARN,
					Provider: shaped.Knowledge[name].Provider,
				}
				resolvedBindings.Knowledge[name] = KnowledgeBindingInfo{
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
		}
	}

	// --- Provisioning shaping ---
	if req.Provisioning != nil && req.Provisioning.Agent != nil {
		p := req.Provisioning.Agent
		shaped.Agent.Resources = applyCompute(shaped.Agent.Resources, p.Compute)
		applyVolume(&shaped.Agent, p.Volume)
		if p.ResponseTimeout != "" {
			shaped.Agent.ResponseTimeout = p.ResponseTimeout
		}
	}
	// Fresh templates set this, but stored specs predating the field arrive
	// empty — default so the echo and deployed Ingress always carry a value.
	if shaped.Agent.ResponseTimeout == "" {
		shaped.Agent.ResponseTimeout = DefaultResponseTimeout
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
	schemaVars := make(map[string]Variable, len(shaped.Variables))
	maps.Copy(schemaVars, shaped.Variables)
	// Configured is server-derived deployment state, never blueprint input.
	for name, variable := range schemaVars {
		variable.Configured = false
		schemaVars[name] = variable
	}
	for name, variable := range shaped.Variables {
		variable.Configured = false
		shaped.Variables[name] = variable
	}
	if opts != nil && len(opts.ConfiguredInlineSecrets) > 0 {
		markConfiguredInlineSecrets(schemaVars, opts.ConfiguredInlineSecrets, req.Variables)
		if req.Finalize {
			// The signed template carries only an opaque preservation sentinel.
			// /deploy resolves it from encrypted deployment storage after
			// authorization; secret plaintext never crosses the browser boundary.
			markConfiguredInlineSecrets(shaped.Variables, opts.ConfiguredInlineSecrets, req.Variables)
		}
	}

	// Template = deployment/v1 ready: strip template-only fields
	shaped.Spec = "deployment/v1"
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
		if !req.Finalize {
			v.Configured = false
		}
		shaped.Variables[key] = v
	}

	// --- Validation ---
	// Required variables (adapter shaping already flipped optionality,
	// so slack tokens are caught here when slack is selected).
	for key, v := range schemaVars {
		if !v.Optional && v.Value == "" && v.Ref == "" && !v.Configured {
			errs = append(errs, ValidationError{
				Field:   "variables." + key,
				Message: "required variable is empty",
			})
		}
	}

	// Ingestion cron validation
	for name, ing := range shaped.Ingestion {
		if ing.Trigger.Type == "schedule" {
			if ing.Trigger.Schedule == "" {
				errs = append(errs, ValidationError{
					Field:   "ingestion." + name + ".trigger.schedule",
					Message: "cron expression required for schedule trigger",
				})
			} else if !isValidCron(ing.Trigger.Schedule) {
				errs = append(errs, ValidationError{
					Field:   "ingestion." + name + ".trigger.schedule",
					Message: "invalid cron expression",
				})
			}
		}
	}

	// Agent provisioning validation
	errs = append(errs, validateAgentProvisioning(&shaped.Agent)...)

	// Sort errors for deterministic output
	sort.Slice(errs, func(i, j int) bool { return errs[i].Field < errs[j].Field })

	// Promote the user-editable interface config to the response root.
	respInterfaces := TemplateInterfaces{Adapters: []string{}}
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

	// Promote resolved agent provisioning to the response root so clients
	// can render sizing controls without diffing nested template fields.
	respProvisioning := TemplateProvisioning{
		Agent: &ComponentProvisioning{
			Compute: &ComponentCompute{
				CPU:    shaped.Agent.Resources.CPU,
				Memory: shaped.Agent.Resources.Memory,
			},
			ResponseTimeout: shaped.Agent.ResponseTimeout,
		},
	}
	if shaped.Agent.Volume != "" {
		respProvisioning.Agent.Volume = &ComponentVolume{
			Mount:   shaped.Agent.Volume,
			Storage: shaped.Agent.Storage,
		}
	}

	return &TemplateResponse{
		Spec:         "deployment-template/v1",
		Template:     *shaped,
		Variables:    schemaVars,
		Interfaces:   respInterfaces,
		Schedules:    respSchedules,
		Bindings:     resolvedBindings,
		Provisioning: respProvisioning,
		Validation: TemplateValidation{
			Valid:  len(errs) == 0,
			Errors: errs,
		},
	}
}

// deepCopySpec creates a deep copy of an AstroDeploymentSpec via JSON round-trip.
func deepCopySpec(s *AstroDeploymentSpec) *AstroDeploymentSpec {
	b, err := json.Marshal(s)
	if err != nil {
		// Should never happen with a well-formed spec.
		panic("deployment: failed to marshal spec for deep copy: " + err.Error())
	}
	var copy AstroDeploymentSpec
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

// validateAgentProvisioning checks that the agent's resolved compute and
// volume settings are well-formed. Field paths match the wire shape so
// clients can map errors back to the controls the user touched.
func validateAgentProvisioning(a *DeploymentAgent) []ValidationError {
	var errs []ValidationError
	checkQuantity := func(field, value string) {
		if value == "" {
			return
		}
		if _, err := k8sresource.ParseQuantity(value); err != nil {
			errs = append(errs, ValidationError{Field: field, Message: "invalid quantity: " + err.Error()})
		}
	}
	checkQuantity("agent.compute.cpu", a.Resources.CPU)
	checkQuantity("agent.compute.memory", a.Resources.Memory)
	if a.Volume != "" && !path.IsAbs(a.Volume) {
		errs = append(errs, ValidationError{
			Field:   "agent.volume.mount",
			Message: "mount path must be absolute",
		})
	}
	if a.Storage != nil {
		checkQuantity("agent.volume.storage.size", a.Storage.Size)
	}
	if a.ResponseTimeout != "" {
		if d, err := time.ParseDuration(a.ResponseTimeout); err != nil {
			errs = append(errs, ValidationError{
				Field:   "agent.responseTimeout",
				Message: "invalid duration: use a Go duration like 15s or 2m",
			})
		} else if d <= 0 {
			errs = append(errs, ValidationError{
				Field:   "agent.responseTimeout",
				Message: "must be greater than zero",
			})
		} else if d > MaxResponseTimeout {
			errs = append(errs, ValidationError{
				Field:   "agent.responseTimeout",
				Message: "must not exceed " + MaxResponseTimeout.String(),
			})
		}
	}
	return errs
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
func primaryEndpointName(endpoints map[string]Endpoint) string {
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
//
// injectSlackVariables adds SLACK_BOT_TOKEN, SLACK_APP_TOKEN, and SLACK_CONFIG
// to ds.Variables. It merges into any values already present (preserving
// injectSlackVariables adds SLACK_BOT_TOKEN, SLACK_APP_TOKEN, and SLACK_CONFIG
// to ds.Variables. Called by ApplyAdapterShaping when the slack adapter is
// selected. Merges into any values already present from user inputs, preserving
// Value, Ref, and Default while overwriting platform-owned metadata.
func injectSlackVariables(ds *AstroDeploymentSpec) {
	if ds.Variables == nil {
		ds.Variables = make(map[string]Variable)
	}
	merge := func(key string, v Variable) {
		if existing, ok := ds.Variables[key]; ok {
			// Preserve user-supplied content; overwrite platform-owned metadata.
			v.Value = existing.Value
			v.Ref = existing.Ref
			v.Default = existing.Default
			ds.Variables[key] = v
		} else {
			ds.Variables[key] = v
		}
	}
	merge("SLACK_BOT_TOKEN", Variable{
		Description: "Slack bot token for API access and messaging",
		Label:       "Slack Bot Token",
		Placeholder: "xoxb-...",
		HelpURL:     "https://docs.slack.dev/authentication/tokens/",
		Optional:    true,
		Secret:      true,
		Targets:     []string{"interface.slack"},
	})
	merge("SLACK_APP_TOKEN", Variable{
		Description: "Slack app-level token for socket mode connections",
		Label:       "Slack App Token",
		Placeholder: "xapp-...",
		HelpURL:     "https://docs.slack.dev/authentication/tokens/",
		Optional:    true,
		Secret:      true,
		Targets:     []string{"interface.slack"},
	})
	if _, ok := ds.Variables["SLACK_CONFIG"]; !ok {
		ds.Variables["SLACK_CONFIG"] = Variable{
			Description: "Slack adapter configuration",
			Label:       "Slack Configuration",
			Datatype:    "object",
			Optional:    true,
			Secret:      false,
			Targets:     []string{"interface.slack"},
			Fields: map[string]VariableField{
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
					Deprecated:  "User-ID gating is no longer enforced. Restrict access via allowed_channel_ids instead.",
				},
				"observe_channel_ids": {
					Label:       "Observe Channel IDs",
					Description: "Channels where non-mention messages are forwarded to the agent instead of dropped",
					Placeholder: "C12345, C67890",
					Datatype:    "csv",
					Optional:    true,
				},
			},
		}
	}
}

func ApplyAdapterShaping(ds *AstroDeploymentSpec, selectedAdapters []string) {
	if ds.Interfaces == nil {
		return
	}
	selectedSet := make(map[string]bool, len(selectedAdapters))
	for _, a := range selectedAdapters {
		selectedSet[a] = true
	}

	// 1. When slack is selected, ensure slack variables are present.
	if selectedSet["slack"] {
		injectSlackVariables(ds)
		wireInterfaceEnvironment(ds)
	}

	// 2. Strip variables belonging exclusively to non-selected adapters.
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

	// 3. Slack token optionality: required when slack is selected, optional otherwise.
	slackSelected := selectedSet["slack"]
	for _, key := range []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"} {
		if v, ok := ds.Variables[key]; ok {
			v.Optional = !slackSelected
			ds.Variables[key] = v
		}
	}
}

// ApplyStoredBindingsToRequest seeds req.Bindings from a stored deployment
// spec when the client did not send explicit binding intent. A non-nil
// req.Bindings — even an empty Knowledge map, or one whose ARNs are all
// empty strings — is treated as explicit intent and left untouched, so the
// client can clear bindings on a deployment that already has them.
func ApplyStoredBindingsToRequest(log *logger.Logger, req *TemplateRequest, storedSpecJSON string) {
	if req.Bindings != nil {
		return
	}
	if restored := RestoreBindingsFromSpec(log, storedSpecJSON); restored != nil {
		req.Bindings = restored
	}
}

// RestoreBindingsFromSpec extracts knowledge binding ARNs from a stored
// deployment spec JSON. Returns nil if no bound entries are found (or on
// parse error). Used by the template handler to seed the TemplateRequest
// when the client opens the configure panel for an existing deployment.
func RestoreBindingsFromSpec(log *logger.Logger, specJSON string) *TemplateBindings {
	if specJSON == "" {
		return nil
	}
	var stored AstroDeploymentSpec
	if err := json.Unmarshal([]byte(specJSON), &stored); err != nil {
		// The JSON came from our own DB — a decode failure means corruption
		// or a schema break we should surface, not silently skip.
		if log != nil {
			log.Warn("Failed to unmarshal stored deployment spec for binding restore", "error", err)
		}
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
	return &TemplateBindings{Knowledge: restored}
}

// ApplyBindingShaping adjusts a template so that knowledge entries whose
// submitted counterparts carry a binding ARN are zeroed to match the shape
// the client originally received from ShapeTemplate.
func ApplyBindingShaping(template *AstroDeploymentSpec, submitted *AstroDeploymentSpec) {
	boundNames := make(map[string]bool)
	for name, k := range submitted.Knowledge {
		if k.IsBound() {
			boundNames[name] = true
			// Zero container fields but preserve binding + provider.
			template.Knowledge[name] = DeploymentKnowledge{
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
}

func buildDeploymentModel(model spec.Model, name string, input TemplateInput) DeploymentModel {
	container := model.ResolvedContainer()
	port := container.Port
	if port == 0 {
		port = 8080
	}

	dm := DeploymentModel{
		Image:       resolveBuiltImage(spec.ComponentModel, name, container.Image, container.Build, input),
		Endpoints:   SingleEndpoint("http", port, "http"),
		Replicas:    1,
		Resources:   StandardResources,
		Healthcheck: container.Healthcheck,
		Update:      DefaultUpdateStrategy(),
	}

	// Container-mode GPU (explicit gpu block in the spec)
	if container.HasGPU() {
		dm.Resources = GPUResources
		dm.GPU = &DeploymentGPU{
			VRAM:    container.GPU.VRAM,
			Runtime: container.GPU.Runtime,
			Count:   1,
		}
		if dm.GPU.Runtime == "" {
			dm.GPU.Runtime = "cuda"
		}
		dm.Update = UpdateStrategy{Strategy: "recreate"}
	}

	if len(container.Environment) > 0 {
		dm.Environment = container.Environment
	}
	return dm
}

func buildDeploymentKnowledge(knowledge spec.Knowledge, name string, input TemplateInput) DeploymentKnowledge {
	container := knowledge.ResolvedContainer()
	port := container.Port
	if port == 0 {
		port = 8080
	}

	dk := DeploymentKnowledge{
		Image:       resolveBuiltImage(spec.ComponentKnowledge, name, container.Image, container.Build, input),
		Endpoints:   SingleEndpoint("http", port, "http"),
		Replicas:    1,
		Resources:   StandardResources,
		Persistent:  container.Persistent,
		Healthcheck: container.Healthcheck,
		Update:      DefaultUpdateStrategy(),
		Provider:    knowledge.Provider,
	}

	// Provider-specific port and multi-port
	if knowledge.IsProviderMode() {
		prov := spec.GetProvider(knowledge.Provider)
		if prov.DefaultPort != 0 {
			dk.Endpoints = SingleEndpoint("http", prov.DefaultPort, "http")
		}
		if len(prov.ExtraPorts) > 0 {
			for _, ep := range prov.ExtraPorts {
				dk.Endpoints[ep.Name] = Endpoint{Port: ep.Port, Protocol: portNameToProtocol(ep.Name)}
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
		dk.Storage = &StorageConfig{
			Size:       "10Gi",
			AccessMode: "ReadWriteOnce",
		}
		// Persistent stores default to recreate strategy
		dk.Update = UpdateStrategy{Strategy: "recreate"}
	}

	dk.Volume = container.Volume
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

func buildDeploymentIntegration(tool spec.Integration, name string, input TemplateInput) DeploymentIntegration {
	port := 8080
	dt := DeploymentIntegration{
		Replicas:  1,
		Resources: StandardResources,
		Update:    DefaultUpdateStrategy(),
	}
	if tool.Container != nil {
		dt.Image = resolveBuiltImage(spec.ComponentIntegration, name, tool.Container.Image, tool.Container.Build, input)
		if tool.Container.Port != 0 {
			port = tool.Container.Port
		}
		dt.Healthcheck = tool.Container.Healthcheck
		if len(tool.Container.Environment) > 0 {
			dt.Environment = tool.Container.Environment
		}
	}
	dt.Endpoints = SingleEndpoint("http", port, "http")
	return dt
}

func buildDeploymentIngestion(ingestion spec.Ingestion, name string, input TemplateInput) DeploymentIngestion {
	di := DeploymentIngestion{
		Image:     resolveBuiltImage(spec.ComponentIngestion, name, ingestion.Container.Image, ingestion.Container.Build, input),
		Resources: StandardResources,
		Trigger: DeploymentTrigger{
			Type: ingestion.Trigger.Type,
		},
		Healthcheck: ingestion.Container.Healthcheck,
	}
	if len(ingestion.Container.Environment) > 0 {
		di.Environment = ingestion.Container.Environment
	}
	// Webhook triggers expose a port via endpoints
	if ingestion.Container.Port > 0 {
		di.Endpoints = SingleEndpoint("http", ingestion.Container.Port, "http")
	}
	// Schedule triggers get an empty placeholder
	if ingestion.Trigger.Type == "schedule" {
		di.Trigger.Schedule = ""
	}
	return di
}

// resolveBuiltImage is resolveImage with a fallback for components whose source
// spec uses container.build without an explicit image. In that case the image
// name is synthesized using the canonical {agent}-{kind}-{name} convention so
// the deployment spec passes validation. This mirrors what the build pipeline
// does via TransformSpecForRegistry, ensuring the deployment generator works
// against either a raw or a registry-rewritten spec.
func resolveBuiltImage(kind spec.ComponentKind, name, image string, build *spec.BuildConfig, input TemplateInput) string {
	if image == "" && build != nil {
		image = spec.ComponentImageName(kind, input.AgentName, name)
	}
	return resolveImage(image, input)
}

// resolveImage maps an image reference to its final pull path:
//   - Tenant images (hosted on ProxyRegistryHost) → unchanged. The pushed
//     reference is already the pull URL; astro-registry maps the account
//     namespace to its ECR repo at pull time. See docs/01-spec/registry-pull-through-spec.md.
//   - Public images (bare Docker Hub reference, no registry host) → ECR pull-through cache: {ecrHost}/dockerhub/{image}
//     Official library images (no org prefix) are placed under "library/".
//   - Third-party images (explicit registry host such as gcr.io, ghcr.io) → unchanged.
func resolveImage(image string, input TemplateInput) string {
	if image == "" {
		return image
	}

	// 1. Tenant image (hosted on the proxy registry) → passed through unchanged.
	// The pushed reference (registry.<domain>/{account}/{image}) is already the
	// pull URL; astro-registry maps the account namespace to its ECR repo at pull
	// time (see registry-pull-through spec). No rewriting in the control plane.
	if input.ProxyRegistryHost != "" && strings.HasPrefix(image, input.ProxyRegistryHost+"/") {
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
// references for every variable (secret or not) that targets an interface
// adapter. The resolver routes each entry to ConfigMapData or SecretData based
// on whether its value references a secret variable; the messaging-side
// applier then mounts secrets via a scoped messaging-only Secret. So secrets
// MUST appear here — otherwise they'd never reach the messaging container.
func wireInterfaceEnvironment(ds *AstroDeploymentSpec) {
	if ds.Interfaces == nil || len(ds.Variables) == 0 {
		return
	}
	for key, v := range ds.Variables {
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

// collectVariablesFromInputs gathers all Input declarations from the astro spec into
// the variables map and injects default values into the relevant container environments.
func collectVariablesFromInputs(astroSpec *spec.AstroSpec, ds *AstroDeploymentSpec, agentEnv map[string]string, variables map[string]Variable) {
	addVariable := func(input spec.Input, targets []string) {
		v := Variable{
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

	// Gateway model entries → a deploy-time model selector. The declared models
	// are selectable options; the deployer picks one and it is injected as
	// MODEL_<name>. Endpoint/auth come from the shared ASTRO_GATEWAY_* pair
	// (injected by the applier when AIGateway is enabled). An empty options list
	// is enable-only (no selector; agent hard-codes the model).
	gatewayNames := make([]string, 0, len(astroSpec.Models))
	for name := range astroSpec.Models {
		gatewayNames = append(gatewayNames, name)
	}
	sort.Strings(gatewayNames)
	for _, name := range gatewayNames {
		model := astroSpec.Models[name]
		if !model.IsGateway() {
			continue
		}
		options := model.ResolvedModels()
		if len(options) == 0 {
			continue
		}
		envName := "MODEL_" + spec.SanitizeEnvName(name)
		addVariable(spec.Input{
			Name:        envName,
			Datatype:    "string",
			Description: fmt.Sprintf("Model for %q (Astro AI Gateway) — chosen at deploy, injected as %s", name, envName),
			DisplayAs:   "select",
			Options:     options,
			Default:     options[0],
			Optional:    true,
		}, []string{"agent"})
		agentEnv[envName] = fmt.Sprintf("${variables.%s}", envName)
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
