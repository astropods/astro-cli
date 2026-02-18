package deployment

import (
	"fmt"
	"sort"
	"strings"

	"github.com/postman/astro/packages/astro-spec"
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
	ProxyRegistryHost string // Proxy registry host for tenant image resolution (e.g. "registry.astropod.ai")
	Environment       string // Environment prefix for ECR tenant repos (e.g. "prod", "preview")
}

// GenerateDeploymentTemplate creates a deployment spec template from a registered astro-spec.
// The template has placeholder values for user-fillable fields and ${} references
// for component wiring.
func GenerateDeploymentTemplate(input TemplateInput) (*spec.AstroDeploymentSpec, error) {
	astroSpec := input.Spec
	if astroSpec == nil {
		return nil, fmt.Errorf("astro spec is required")
	}

	ds := &spec.AstroDeploymentSpec{
		Spec: "deployment/v1",
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
			if model.IsProviderMode() && !spec.IsCloudModelProvider(model.Provider) {
				prov := spec.GetModelProvider(model.Provider)
				if prov.EnvPrefix != "" {
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
			// Skip cloud providers — they produce credentials, not containers
			if model.IsProviderMode() && spec.IsCloudModelProvider(model.Provider) {
				continue
			}

			if ds.Models == nil {
				ds.Models = make(map[string]spec.DeploymentModel)
			}
			dm := buildDeploymentModel(model, input)
			ds.Models[name] = dm

			// Wire references into agent environment
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
						agentEnv[key] = fmt.Sprintf("${models.%s.port}", name)
					}
					for _, key := range providerEnvKeys(prov.EnvPrefix, name, "URL", isDup, isFirst) {
						agentEnv[key] = fmt.Sprintf("${models.%s.url}", name)
					}
					for _, key := range providerEnvKeys(prov.EnvPrefix, name, "BASE_URL", isDup, isFirst) {
						agentEnv[key] = fmt.Sprintf("${models.%s.url}", name) + "/api"
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
				agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${models.%s.port}", name)
				agentEnv[envPrefix+"_URL"] = fmt.Sprintf("${models.%s.url}", name)
			}
		}
	}

	// Process knowledge
	if len(astroSpec.Knowledge) > 0 {
		// Count provider occurrences among self-hosted knowledge stores
		knowledgeProviderCount := make(map[string]int)
		for _, knowledge := range astroSpec.Knowledge {
			if knowledge.IsProviderMode() && !spec.IsCloudKnowledgeProvider(knowledge.Provider) {
				prov := spec.GetProvider(knowledge.Provider)
				if prov.EnvPrefix != "" {
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
			// Skip cloud providers — they produce credentials, not containers
			if knowledge.IsProviderMode() && spec.IsCloudKnowledgeProvider(knowledge.Provider) {
				continue
			}

			if ds.Knowledge == nil {
				ds.Knowledge = make(map[string]spec.DeploymentKnowledge)
			}
			dk := buildDeploymentKnowledge(knowledge, input)
			ds.Knowledge[name] = dk

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
						agentEnv[key] = fmt.Sprintf("${knowledge.%s.port}", name)
					}
					if prov.URLScheme != "" {
						for _, key := range providerEnvKeys(prov.EnvPrefix, name, "URL", isDup, isFirst) {
							agentEnv[key] = fmt.Sprintf("${knowledge.%s.url}", name)
						}
					}
				} else {
					envPrefix := fmt.Sprintf("KNOWLEDGE_%s", strings.ToUpper(SanitizeName(name)))
					agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${knowledge.%s.host}", name)
					agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${knowledge.%s.port}", name)
				}
			} else {
				envPrefix := fmt.Sprintf("KNOWLEDGE_%s", strings.ToUpper(SanitizeName(name)))
				agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${knowledge.%s.host}", name)
				agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${knowledge.%s.port}", name)
			}
		}
	}

	// Process tools
	if len(astroSpec.Tools) > 0 {
		for name, tool := range astroSpec.Tools {
			// Skip cloud providers — they produce credentials, not containers
			if tool.IsProviderMode() && spec.IsCloudToolProvider(tool.Provider) {
				continue
			}

			if ds.Tools == nil {
				ds.Tools = make(map[string]spec.DeploymentTool)
			}
			dt := buildDeploymentTool(tool, input)
			ds.Tools[name] = dt

			envPrefix := fmt.Sprintf("TOOL_%s", strings.ToUpper(SanitizeName(name)))
			agentEnv[envPrefix+"_HOST"] = fmt.Sprintf("${tools.%s.host}", name)
			agentEnv[envPrefix+"_PORT"] = fmt.Sprintf("${tools.%s.port}", name)
			agentEnv[envPrefix+"_URL"] = fmt.Sprintf("${tools.%s.url}", name)
		}
	}

	// Extract credentials from integrations
	validator := NewValidator()
	credInfos := validator.GetRequiredCredentials(astroSpec, nil)
	if len(credInfos) > 0 {
		ds.Credentials = make(map[string]spec.DeploymentCredential, len(credInfos))
		for _, ci := range credInfos {
			ds.Credentials[ci.Key] = spec.DeploymentCredential{
				Description: ci.Description,
				Optional:    ci.Optional,
			}
			// Wire credential references into agent environment
			agentEnv[ci.Key] = fmt.Sprintf("${credentials.%s}", ci.Key)
		}
	}

	// Platform metadata
	agentEnv["ASTRO_AGENT_NAME"] = "${source.name}"
	agentEnv["ASTRO_AGENT_BUILD"] = "${source.build}"

	// Build agent block
	agentImage := resolveTenantImage(astroSpec.Agent.Image, input)
	if agentImage == "" && input.RegistryURL != "" {
		agentImage = fmt.Sprintf("%s/%s:%s", input.RegistryURL, astroSpec.Name, input.BuildID)
	}
	ds.Agent = spec.DeploymentAgent{
		Image:       agentImage,
		Port:        8080,
		Replicas:    1,
		Resources:   spec.StandardResources,
		Environment: agentEnv,
		Healthcheck: astroSpec.Agent.Healthcheck,
		Update:      spec.DefaultUpdateStrategy(),
		Expose:      spec.ExposeConfig{Enabled: false},
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
		Image:     fmt.Sprintf("%s/prod-astro-messaging:latest", input.RegistryURL),
		Port:      9090,
		Resources: spec.MessagingResources,
		Expose:    spec.ExposeConfig{Enabled: false},
	}

	// Editable fields
	ds.Editable = defaultEditableFields()

	return ds, nil
}

func buildDeploymentModel(model spec.Model, input TemplateInput) spec.DeploymentModel {
	container := model.ResolvedContainer()
	dm := spec.DeploymentModel{
		Image:       resolveTenantImage(container.Image, input),
		Port:        container.Port,
		Replicas:    1,
		Resources:   spec.StandardResources,
		Healthcheck: container.Healthcheck,
		Update:      spec.DefaultUpdateStrategy(),
	}
	if dm.Port == 0 {
		dm.Port = 8080
	}

	// Provider-mode auto-configuration
	if model.IsProviderMode() {
		prov := spec.GetModelProvider(model.Provider)
		dm.Provider = model.Provider

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
			dm.ModelName = model.Model
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

	// Container-mode GPU (explicit gpu block in astroai.yml)
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
	dk := spec.DeploymentKnowledge{
		Image:       resolveTenantImage(container.Image, input),
		Port:        container.Port,
		Replicas:    1,
		Resources:   spec.StandardResources,
		Persistent:  container.Persistent,
		Healthcheck: container.Healthcheck,
		Update:      spec.DefaultUpdateStrategy(),
		Provider:    knowledge.Provider,
	}
	if dk.Port == 0 {
		dk.Port = 8080
	}

	// Provider-specific healthcheck
	if knowledge.IsProviderMode() && dk.Healthcheck == nil {
		prov := spec.GetProvider(knowledge.Provider)
		if prov.HealthCheck != nil {
			dk.Healthcheck = &spec.Healthcheck{Test: prov.HealthCheck}
		} else if prov.HealthPath != "" {
			dk.Healthcheck = &spec.Healthcheck{Path: prov.HealthPath}
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
	dt := spec.DeploymentTool{
		Replicas:  1,
		Resources: spec.StandardResources,
		Update:    spec.DefaultUpdateStrategy(),
	}
	if tool.Container != nil {
		dt.Image = resolveTenantImage(tool.Container.Image, input)
		dt.Port = tool.Container.Port
		dt.Healthcheck = tool.Container.Healthcheck
		if len(tool.Container.Environment) > 0 {
			dt.Environment = tool.Container.Environment
		}
	}
	if dt.Port == 0 {
		dt.Port = 8080
	}
	return dt
}

func buildDeploymentIngestion(ingestion spec.Ingestion, input TemplateInput) spec.DeploymentIngestion {
	image := resolveTenantImage(ingestion.Container.Image, input)
	di := spec.DeploymentIngestion{
		Image:     image,
		Port:      ingestion.Container.Port,
		Resources: spec.StandardResources,
		Trigger: spec.DeploymentTrigger{
			Type: ingestion.Trigger.Type,
		},
		Healthcheck: ingestion.Container.Healthcheck,
	}
	if len(ingestion.Container.Environment) > 0 {
		di.Environment = ingestion.Container.Environment
	}
	// Schedule triggers get an empty placeholder
	if ingestion.Trigger.Type == "schedule" {
		di.Trigger.Schedule = ""
	}
	return di
}

// resolveTenantImage translates a proxy-registry image reference to an ECR path.
// e.g. "registry.astropod.ai/account/image:tag" → "{ecrHost}/{env}-tenant-account/image:tag"
// Images that don't match the proxy registry host are returned as-is.
func resolveTenantImage(image string, input TemplateInput) string {
	if image == "" || input.ProxyRegistryHost == "" {
		return image
	}
	if !strings.HasPrefix(image, input.ProxyRegistryHost+"/") {
		return image
	}

	// Remove proxy host prefix → "account/image:tag"
	pathWithTag := strings.TrimPrefix(image, input.ProxyRegistryHost+"/")
	parts := strings.SplitN(pathWithTag, "/", 2)
	if len(parts) < 2 {
		return image
	}

	namespace := parts[0]
	imageAndTag := parts[1]

	// Strip scheme from registry URL if present
	registryHost := input.RegistryURL
	if idx := strings.Index(registryHost, "://"); idx >= 0 {
		registryHost = registryHost[idx+3:]
	}

	// Build ECR path: {ecrHost}/{env}-tenant-{namespace}/{imageAndTag}
	return fmt.Sprintf("%s/%s-tenant-%s/%s", registryHost, input.Environment, namespace, imageAndTag)
}

func defaultEditableFields() []string {
	return []string{
		"target.namespace",
		"agent.replicas",
		"agent.resources",
		"agent.environment",
		"agent.healthcheck",
		"agent.update",
		"agent.expose",
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
		"interfaces.expose",
		"credentials.*.value",
		"observability.enabled",
		"observability.resources",
		"observability.environment",
	}
}
