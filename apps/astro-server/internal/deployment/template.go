package deployment

import (
	"fmt"
	"sort"
	"strings"

	spec "github.com/postman/astro/packages/astro-spec"
)

// providerEnvKey returns the env-var key for a provider entry.
// When isDuplicate is false, it returns basePrefix+"_"+suffix (e.g. "QDRANT_HOST").
// When isDuplicate is true, it returns basePrefix+"_"+NAME+"_"+suffix for all entries,
// and additionally basePrefix+"_"+suffix for the first entry (alphabetically).
func providerEnvKeys(basePrefix, name, suffix string, isDuplicate, isFirst bool) []string {
	if !isDuplicate {
		return []string{basePrefix + "_" + suffix}
	}
	keys := []string{basePrefix + "_" + strings.ToUpper(SanitizeName(name)) + "_" + suffix}
	if isFirst {
		keys = append(keys, basePrefix+"_"+suffix)
	}
	return keys
}

// TemplateInput holds the parameters needed to generate a deployment template.
type TemplateInput struct {
	Spec              *spec.AstroSpec
	Account           string
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
			Name:     astroSpec.Name,
			Build:    input.BuildID,
			Registry: input.RegistryURL,
		},
		Target: spec.DeploymentTarget{
			Runtime:   "kubernetes",
			Namespace: "",
		},
		Observability: spec.DeploymentObservability{
			Enabled:   true,
			Provider:  "galileo",
			Image:     fmt.Sprintf("%s/prod-astro-collector:latest", input.RegistryURL),
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
					if model.Model != "" {
						for _, key := range providerEnvKeys(prov.EnvPrefix, name, "MODEL", isDup, isFirst) {
							agentEnv[key] = model.Model
						}
					}
				}
			} else {
				// Generic env vars for container-mode models
				envPrefix := fmt.Sprintf("MODEL_%s", strings.ToUpper(SanitizeName(name)))
				agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${models.%s.host}", name)
				agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${models.%s.%s.port}", name, primaryEp)
				agentEnv[envPrefix+"_URL"] = fmt.Sprintf("${models.%s.%s.url}", name, primaryEp)
			}
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
					if prov.URLScheme != "" {
						for _, key := range providerEnvKeys(prov.EnvPrefix, name, "URL", isDup, isFirst) {
							agentEnv[key] = fmt.Sprintf("${knowledge.%s.%s.url}", name, primaryEp)
						}
					}
				} else {
					envPrefix := fmt.Sprintf("KNOWLEDGE_%s", strings.ToUpper(SanitizeName(name)))
					agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${knowledge.%s.host}", name)
					agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${knowledge.%s.%s.port}", name, primaryEp)
				}
			} else {
				envPrefix := fmt.Sprintf("KNOWLEDGE_%s", strings.ToUpper(SanitizeName(name)))
				agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${knowledge.%s.host}", name)
				agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${knowledge.%s.%s.port}", name, primaryEp)
			}
		}
	}

	// Process tools
	if len(astroSpec.Tools) > 0 {
		for name, tool := range astroSpec.Tools {
			if !tool.DeploysContainer(astroSpec.Providers) {
				continue
			}

			if ds.Tools == nil {
				ds.Tools = make(map[string]spec.DeploymentTool)
			}
			dt := buildDeploymentTool(tool, input)
			ds.Tools[name] = dt

			primaryEp := primaryEndpointName(dt.Endpoints)
			envPrefix := fmt.Sprintf("TOOL_%s", strings.ToUpper(SanitizeName(name)))
			agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${tools.%s.host}", name)
			agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${tools.%s.%s.port}", name, primaryEp)
			agentEnv[envPrefix+"_URL"] = fmt.Sprintf("${tools.%s.%s.url}", name, primaryEp)
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
	if agentImage == "" && input.RegistryURL != "" {
		agentImage = fmt.Sprintf("%s/%s:%s", input.RegistryURL, astroSpec.Name, input.BuildID)
	}
	ds.Agent = spec.DeploymentAgent{
		Image: agentImage,
		Endpoints: map[string]spec.Endpoint{
			"http": {Port: 8080, Protocol: "http"},
		},
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

	// Interfaces block — empty adapters, user fills in
	ds.Interfaces = &spec.DeploymentInterfaces{
		Adapters:  []string{},
		Image:     resolveImage("astropods/messaging:latest", input),
		Resources: spec.MessagingResources,
		Endpoints: map[string]spec.Endpoint{
			"grpc": {Port: 9090, Protocol: "grpc"},
			"http": {Port: 8080, Protocol: "http", Expose: &spec.EndpointExpose{Enabled: false}},
		},
	}

	// Add adapter credential variables as optional entries so users know what to fill in.
	// These are optional in the template since adapters are disabled by default; the
	// resolver enforces values when a specific adapter is enabled.
	if ds.Variables == nil {
		ds.Variables = make(map[string]spec.Variable)
	}
	if _, exists := ds.Variables["SLACK_BOT_TOKEN"]; !exists {
		ds.Variables["SLACK_BOT_TOKEN"] = spec.Variable{
			Description: "Slack bot token for API access and messaging (required when slack adapter is enabled)",
			Optional:    true,
			Secret:      true,
			Targets:     []string{"interface.slack"},
		}
	}
	if _, exists := ds.Variables["SLACK_APP_TOKEN"]; !exists {
		ds.Variables["SLACK_APP_TOKEN"] = spec.Variable{
			Description: "Slack app-level token for socket mode connections (required when slack adapter is enabled)",
			Optional:    true,
			Secret:      true,
			Targets:     []string{"interface.slack"},
		}
	}

	// Editable fields
	ds.Editable = defaultEditableFields()

	return ds, nil
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

		// Set model name for pull
		if model.Model != "" {
			dm.Model = model.Model
			dm.Persistent = true
		}

		// Model-aware readiness: verify model is pulled, not just server up
		if dm.Healthcheck == nil {
			if model.Model != "" {
				dm.Healthcheck = &spec.Healthcheck{
					Test: []string{"sh", "-c",
						fmt.Sprintf("ollama list | grep -q '%s'", model.Model),
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

	return dk
}

func buildDeploymentTool(tool spec.Tool, input TemplateInput) spec.DeploymentTool {
	port := 8080
	dt := spec.DeploymentTool{
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
			return fmt.Sprintf("%s/%s-tenant-%s/%s", stripScheme(input.RegistryURL), input.Environment, parts[0], parts[1])
		}
		return image
	}

	// 2. Public image (no registry host in first segment) → ECR pull-through cache
	if input.RegistryURL != "" {
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

func defaultEditableFields() []string {
	return []string{
		"target.namespace",
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
		"tools.*.replicas",
		"tools.*.resources",
		"tools.*.environment",
		"tools.*.healthcheck",
		"tools.*.update",
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
		// Use first-write-wins to avoid overwriting a more specific target
		if _, exists := variables[input.Name]; !exists {
			variables[input.Name] = v
		}
	}

	// Top-level inputs → agent + ingestion
	for _, inp := range astroSpec.Inputs {
		addVariable(inp, []string{"agent", "ingestion"})
		if inp.Default != "" {
			agentEnv[inp.Name] = inp.Default
		}
	}

	// Agent inputs → agent only
	for _, inp := range astroSpec.Agent.Inputs {
		addVariable(inp, []string{"agent"})
		if inp.Default != "" {
			agentEnv[inp.Name] = inp.Default
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
	for name, tool := range astroSpec.Tools {
		for _, inp := range tool.Inputs {
			if inp.Default != "" && ds.Tools != nil {
				if dt, ok := ds.Tools[name]; ok {
					if dt.Environment == nil {
						dt.Environment = make(map[string]string)
					}
					dt.Environment[inp.Name] = inp.Default
					ds.Tools[name] = dt
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
