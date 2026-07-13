package compose

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	"github.com/astropods/astro/apps/astro-cli/internal/utils"
	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
)

const (
	// MessagingWebHostPort is the host port the messaging sidecar's HTTP API is
	// published on. The astro CLI serves the chat UI on its own port and proxies
	// chat/messaging requests here (see internal/chatui). It is deliberately not
	// 3100 — that port now belongs to the CLI-served chat UI.
	MessagingWebHostPort = "3110"

	// chatDataMountPath is where the messaging sidecar's SQLite chat store lives.
	// Mirrors astro-server's deployed sidecar (CHAT_DB_PATH=/data/chat.db on a
	// persistent volume); locally it's a named volume so history survives
	// container restarts and across `ast dev` sessions.
	chatDataMountPath = "/data"
	chatDBPath        = "/data/chat.db"

	// messagingUserUIDGID is the uid:gid of the non-root "astro" user the
	// published messaging image runs as. A one-shot init container (see the web
	// adapter block below) chowns the chat-data volume to this owner so the
	// sidecar can create its SQLite DB on a fresh, root-owned named volume.
	messagingUserUIDGID = "1000:1000"
)

// buildSecretsConfig converts spec secrets to compose secrets configuration
func buildSecretsConfig(secrets []spec.BuildSecret, project *types.Project) []types.ServiceSecretConfig {
	if len(secrets) == 0 {
		return nil
	}
	if project.Secrets == nil {
		project.Secrets = make(types.Secrets)
	}
	result := make([]types.ServiceSecretConfig, 0, len(secrets))
	for _, s := range secrets {
		project.Secrets[s.ID] = types.SecretConfig{
			Name:        s.ID,
			Environment: s.Env,
		}
		result = append(result, types.ServiceSecretConfig{
			Source: s.ID,
		})
	}
	return result
}

// convertArgs converts map[string]string to map[string]*string for compose
func convertArgs(args map[string]string) map[string]*string {
	if len(args) == 0 {
		return nil
	}
	result := make(map[string]*string, len(args))
	for k, v := range args {
		val := v
		result[k] = &val
	}
	return result
}

// buildModelHealthCheckTest generates the health check command for model providers.
func buildModelHealthCheckTest(healthcheck *spec.Healthcheck, provider string, port int) types.HealthCheckTest {
	if len(healthcheck.Test) > 0 {
		return types.HealthCheckTest(healthcheck.Test)
	}

	prov := spec.GetModelProvider(provider)

	if len(prov.HealthCheck) > 0 {
		return types.HealthCheckTest(append([]string{"CMD"}, prov.HealthCheck...))
	}

	if prov.HealthPath != "" {
		if port == 0 {
			port = prov.DefaultPort
		}
		return types.HealthCheckTest([]string{"CMD-SHELL", fmt.Sprintf("curl -f http://localhost:%d%s || exit 1", port, prov.HealthPath)})
	}

	if healthcheck.Path != "" {
		if port == 0 {
			port = 8080
		}
		return types.HealthCheckTest([]string{"CMD-SHELL", fmt.Sprintf("curl -f http://localhost:%d%s || exit 1", port, healthcheck.Path)})
	}

	return nil
}

// buildHealthCheckTest generates the appropriate health check command
func buildHealthCheckTest(healthcheck *spec.Healthcheck, provider string, port int) types.HealthCheckTest {
	// If custom test command is provided, use it
	if len(healthcheck.Test) > 0 {
		return types.HealthCheckTest(healthcheck.Test)
	}

	prov := spec.GetProvider(provider)

	// Exec-based health check from provider registry
	if len(prov.HealthCheck) > 0 {
		return types.HealthCheckTest(append([]string{"CMD"}, prov.HealthCheck...))
	}

	// HTTP health check from provider registry
	if prov.HealthPath != "" {
		if port == 0 {
			port = prov.DefaultPort
		}
		return types.HealthCheckTest([]string{"CMD-SHELL", fmt.Sprintf("curl -f http://localhost:%d%s || exit 1", port, prov.HealthPath)})
	}

	// Fallback: if a path is provided, use HTTP health check with curl
	if healthcheck.Path != "" {
		if port == 0 {
			port = 8080
		}
		return types.HealthCheckTest([]string{"CMD-SHELL", fmt.Sprintf("curl -f http://localhost:%d%s || exit 1", port, healthcheck.Path)})
	}

	// No health check
	return nil
}

// BuildOptions controls optional behavior when generating the Compose project.
type BuildOptions struct {
	// NativeOllama skips the Ollama container and points env vars at the
	// host's native Ollama instance (via host.docker.internal).
	NativeOllama bool
}

// ProjectName returns the Docker Compose project name used for an agent spec.
// Scoped spec names like "@org/my-agent" become "my-agent"; unscoped names pass through.
// This is the single source of truth for the project-name string used by Up,
// Down, Logs, health checks, and the .running state file.
func ProjectName(s *spec.AstroSpec) string {
	return ProjectNameFromSpecName(s.Name)
}

// postgresDevCredentials resolves the POSTGRES_USER/PASSWORD/DB triple for the
// dev compose project. Defaults mirror prod (generateKnowledgeCredentials in
// apps/astro-server/internal/k8s/spec_applier.go) so agent code that reads
// these env vars works identically locally and after deploy. envVars (.env /
// ast configure) wins so users can pin a known value when needed.
//
// The password default is intentionally stable — random-per-run would force a
// volume wipe on every restart.
func postgresDevCredentials(s *spec.AstroSpec, envVars map[string]string) (user, password, db string) {
	user = "astro"
	if v, ok := envVars["POSTGRES_USER"]; ok && v != "" {
		user = v
	}
	password = "localdev"
	if v, ok := envVars["POSTGRES_PASSWORD"]; ok && v != "" {
		password = v
	}
	db = spec.SanitizeDBName(s.Name)
	if v, ok := envVars["POSTGRES_DB"]; ok && v != "" {
		db = v
	}
	return
}

// ProjectNameFromSpecName returns the compose project name for a raw spec name.
// Exposed separately so callers that only have the raw string (e.g. a legacy
// `.running` state file) can normalize it without constructing a full spec.
func ProjectNameFromSpecName(raw string) string {
	_, agentName := utils.ParseAgentName(raw)
	return agentName
}

// BuildProject converts an AstroSpec to a Docker Compose project.
// An optional BuildOptions can be passed to customize generation.
func BuildProject(s *spec.AstroSpec, workingDir string, envVars map[string]string, opts ...BuildOptions) (*types.Project, error) {
	var opt BuildOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	agentName := ProjectName(s)
	project := &types.Project{
		Name:       agentName,
		WorkingDir: workingDir,
		Services:   make(types.Services),
		Networks:   make(types.Networks),
		Volumes:    make(types.Volumes),
	}

	// Create default network
	project.Networks["astro-dev"] = types.NetworkConfig{
		Name:   fmt.Sprintf("%s-network", agentName),
		Driver: "bridge",
	}

	// Add models that deploy a container (skip cloud and custom providers)
	for name, model := range s.Models {
		if !model.DeploysContainer(s.Providers) {
			continue
		}
		// Skip Ollama containers when using native Ollama on the host
		if opt.NativeOllama && model.Provider == "ollama" {
			continue
		}
		resolved := model.ResolvedContainer()
		serviceName := fmt.Sprintf("model-%s", name)
		service := types.ServiceConfig{
			Name: serviceName,
			Networks: map[string]*types.ServiceNetworkConfig{
				"astro-dev": nil,
			},
		}

		// Build or image configuration
		if model.Container != nil && model.Container.Build != nil {
			service.Build = &types.BuildConfig{
				Context:    filepath.Join(workingDir, model.Container.Build.Context),
				Dockerfile: model.Container.Build.Dockerfile,
				Target:     model.Container.Build.Target,
				Args:       types.MappingWithEquals(convertArgs(model.Container.Build.Args)),
				Secrets:    buildSecretsConfig(model.Container.Build.Secrets, project),
			}
		} else if resolved.Image != "" {
			service.Image = resolved.Image
		}

		// Inject environment variables from resolved container
		if len(resolved.Environment) > 0 {
			service.Environment = make(types.MappingWithEquals)
			for k, v := range resolved.Environment {
				val := v
				service.Environment[k] = &val
			}
		}

		// Port mapping
		if resolved.Port > 0 {
			service.Ports = []types.ServicePortConfig{
				{
					Target:    uint32(resolved.Port), //nolint:gosec
					Published: fmt.Sprintf("%d", resolved.Port),
				},
			}

			// Add healthcheck: model-aware when model name is set
			if len(model.ResolvedModels()) > 0 {
				interval := types.Duration(15000000000) // 15 seconds
				timeout := types.Duration(5000000000)   // 5 seconds
				retries := uint64(40)                   // ~10 min for large model pulls
				var grepParts []string
				for _, m := range model.ResolvedModels() {
					grepParts = append(grepParts, fmt.Sprintf("ollama list | grep -q '%s'", m))
				}
				test := types.HealthCheckTest([]string{
					"CMD-SHELL",
					strings.Join(grepParts, " && "),
				})
				service.HealthCheck = &types.HealthCheckConfig{
					Test:     test,
					Interval: &interval,
					Timeout:  &timeout,
					Retries:  &retries,
				}
			} else {
				healthcheck := resolved.Healthcheck
				if healthcheck == nil && model.IsProviderMode() {
					prov := spec.GetModelProvider(model.Provider)
					if prov.HealthPath != "" {
						healthcheck = &spec.Healthcheck{Path: prov.HealthPath}
					} else if len(prov.HealthCheck) > 0 {
						healthcheck = &spec.Healthcheck{Test: prov.HealthCheck}
					}
				}
				if healthcheck != nil {
					interval := types.Duration(10000000000) // 10 seconds
					timeout := types.Duration(5000000000)   // 5 seconds
					retries := uint64(3)
					test := buildModelHealthCheckTest(healthcheck, model.Provider, resolved.Port)
					if test != nil {
						service.HealthCheck = &types.HealthCheckConfig{
							Test:     test,
							Interval: &interval,
							Timeout:  &timeout,
							Retries:  &retries,
						}
					}
				}
			}
		}

		// Provider-specific dev enhancements
		if model.IsProviderMode() {
			prov := spec.GetModelProvider(model.Provider)

			// Persistent volume for model storage
			if prov.MountPath != "" {
				volumeName := fmt.Sprintf("%s-data", serviceName)
				project.Volumes[volumeName] = types.VolumeConfig{
					Name: volumeName,
				}
				service.Volumes = []types.ServiceVolumeConfig{
					{
						Type:   types.VolumeTypeVolume,
						Source: volumeName,
						Target: prov.MountPath,
					},
				}
			}

			// GPU passthrough for providers that require it (skip on macOS — no nvidia support)
			if prov.GPU && runtime.GOOS != "darwin" {
				service.Deploy = &types.DeployConfig{
					Resources: types.Resources{
						Reservations: &types.Resource{
							Devices: []types.DeviceRequest{
								{
									Driver:       "nvidia",
									Count:        types.DeviceCount(1),
									Capabilities: []string{"gpu"},
								},
							},
						},
					},
				}
			}

			// Auto-pull model after server starts (ollama-specific)
			if len(model.ResolvedModels()) > 0 && model.Provider == "ollama" {
				var pullParts []string
				for _, m := range model.ResolvedModels() {
					pullParts = append(pullParts, fmt.Sprintf("ollama pull %s", m))
				}
				service.Entrypoint = types.ShellCommand{
					"/bin/sh", "-c",
					fmt.Sprintf("ollama serve & until ollama list >/dev/null 2>&1; do sleep 1; done; %s; wait", strings.Join(pullParts, " && ")),
				}
			}
		}

		project.Services[serviceName] = service
	}

	// Add knowledge stores that deploy a container (skip cloud and custom providers)
	for name, knowledge := range s.Knowledge {
		if !knowledge.DeploysContainer(s.Providers) {
			continue
		}
		container := knowledge.ResolvedContainer()
		serviceName := fmt.Sprintf("knowledge-%s", name)
		service := types.ServiceConfig{
			Name: serviceName,
			Networks: map[string]*types.ServiceNetworkConfig{
				"astro-dev": nil,
			},
		}

		// Build or image configuration
		if container.Build != nil {
			service.Build = &types.BuildConfig{
				Context:    filepath.Join(workingDir, container.Build.Context),
				Dockerfile: container.Build.Dockerfile,
				Target:     container.Build.Target,
				Args:       types.MappingWithEquals(convertArgs(container.Build.Args)),
				Secrets:    buildSecretsConfig(container.Build.Secrets, project),
			}
		} else if container.Image != "" {
			service.Image = container.Image
		}

		// Port mapping
		if container.Port > 0 {
			service.Ports = []types.ServicePortConfig{
				{
					Target:    uint32(container.Port), //nolint:gosec
					Published: fmt.Sprintf("%d", container.Port),
				},
			}
		} else if knowledge.Provider == "qdrant" {
			// Expose Qdrant dashboard on default port in dev mode
			qdrantPort := spec.GetProvider("qdrant").DefaultPort
			service.Ports = []types.ServicePortConfig{
				{
					Target:    uint32(qdrantPort), //nolint:gosec
					Published: fmt.Sprintf("%d", qdrantPort),
				},
			}
		}

		// Publish extra ports defined by the provider (e.g. Neo4j bolt on 7687)
		if prov := spec.GetProvider(knowledge.Provider); len(prov.ExtraPorts) > 0 {
			for _, ep := range prov.ExtraPorts {
				service.Ports = append(service.Ports, types.ServicePortConfig{
					Target:    uint32(ep.Port), //nolint:gosec
					Published: fmt.Sprintf("%d", ep.Port),
				})
			}
		}

		// Add healthcheck only if defined in spec
		if container.Healthcheck != nil {
			interval := types.Duration(10000000000) // 10 seconds
			timeout := types.Duration(5000000000)   // 5 seconds
			retries := uint64(3)
			port := container.Port
			if port == 0 {
				port = spec.GetProvider(knowledge.Provider).DefaultPort
			}

			test := buildHealthCheckTest(container.Healthcheck, knowledge.Provider, port)
			if test != nil {
				service.HealthCheck = &types.HealthCheckConfig{
					Test:     test,
					Interval: &interval,
					Timeout:  &timeout,
					Retries:  &retries,
				}
			}
		}

		if container.Persistent {
			volumeName := fmt.Sprintf("%s-data", serviceName)
			project.Volumes[volumeName] = types.VolumeConfig{
				Name: volumeName,
			}

			service.Volumes = []types.ServiceVolumeConfig{
				{
					Type:   types.VolumeTypeVolume,
					Source: volumeName,
					Target: container.Volume,
				},
			}
		}

		// Apply provider default environment variables
		if prov := spec.GetProvider(knowledge.Provider); len(prov.DefaultEnv) > 0 {
			if service.Environment == nil {
				service.Environment = make(types.MappingWithEquals)
			}
			for k, v := range prov.DefaultEnv {
				val := v
				service.Environment[k] = &val
			}
		}

		// For postgres, inject USER/PASSWORD/DB so the sidecar boots with the
		// same credentials the agent will connect with. Mirrors prod, where
		// the deployer generates these and injects them as secrets into both
		// the postgres pod and the agent (spec_applier.go:generateKnowledgeCredentials).
		// envVars (.env / ast configure) wins so users can pin a known password.
		if knowledge.Provider == "postgres" {
			if service.Environment == nil {
				service.Environment = make(types.MappingWithEquals)
			}
			user, password, dbName := postgresDevCredentials(s, envVars)
			service.Environment["POSTGRES_USER"] = &user
			service.Environment["POSTGRES_PASSWORD"] = &password
			service.Environment["POSTGRES_DB"] = &dbName
		}

		// Inject knowledge inputs (from ast configure / .env, with default fallback)
		for _, inp := range knowledge.Inputs {
			val := inp.Default
			if v, ok := envVars[inp.Name]; ok {
				val = v
			}
			if val != "" {
				if service.Environment == nil {
					service.Environment = make(types.MappingWithEquals)
				}
				v := val
				service.Environment[inp.Name] = &v
			}
		}

		project.Services[serviceName] = service
	}

	// Add tools that deploy a container (skip cloud and custom providers)
	for name, tool := range s.Integrations {
		if !tool.DeploysContainer(s.Providers) {
			continue
		}
		if tool.Container != nil {
			serviceName := fmt.Sprintf("tool-%s", name)
			service := types.ServiceConfig{
				Name:  serviceName,
				Image: tool.Container.Image,
				Networks: map[string]*types.ServiceNetworkConfig{
					"astro-dev": nil,
				},
			}

			if tool.Container.Port > 0 {
				service.Ports = []types.ServicePortConfig{
					{
						Target:    uint32(tool.Container.Port), //nolint:gosec
						Published: fmt.Sprintf("%d", tool.Container.Port),
					},
				}
			}

			// Inject integration inputs (from ast configure / .env, with default fallback)
			for _, inp := range tool.Inputs {
				val := inp.Default
				if v, ok := envVars[inp.Name]; ok {
					val = v
				}
				if val != "" {
					if service.Environment == nil {
						service.Environment = make(types.MappingWithEquals)
					}
					v := val
					service.Environment[inp.Name] = &v
				}
			}

			project.Services[serviceName] = service
		}
	}

	// Add interface services (messaging sidecar, grpc services, etc.)
	if s.Dev.HasMessagingAdapters() {
		adapters := s.Dev.MessagingAdapters()
		hasMessagingAdapter := false
		for _, name := range adapters {
			if name == "slack" || name == "web" {
				hasMessagingAdapter = true
			}
		}

		if hasMessagingAdapter {
			messagingImage := "astropods/messaging:latest"
			messagingPull := types.PullPolicyAlways
			if s.Dev.Overrides != nil && s.Dev.Overrides.MessagingImage != "" {
				messagingImage = s.Dev.Overrides.MessagingImage
				messagingPull = ""
			}
			messagingService := types.ServiceConfig{
				Name:       "astro-messaging",
				Image:      messagingImage,
				PullPolicy: messagingPull,
				Networks: map[string]*types.ServiceNetworkConfig{
					"astro-dev": nil,
				},
				Environment: buildMessagingEnvironment(s, envVars),
				Ports:       buildMessagingPorts(s),
			}
			// Persist the sidecar's SQLite chat store on a named volume so chat
			// history survives container restarts and across `ast dev` sessions
			// (CHAT_DB_PATH is set in buildMessagingEnvironment). Only needed for
			// the web adapter, which is the chat path.
			if slices.Contains(adapters, "web") {
				chatVolume := fmt.Sprintf("%s-chat-data", agentName)
				project.Volumes[chatVolume] = types.VolumeConfig{Name: chatVolume}
				chatVolumeMount := types.ServiceVolumeConfig{
					Type:   types.VolumeTypeVolume,
					Source: chatVolume,
					Target: chatDataMountPath,
				}
				messagingService.Volumes = append(messagingService.Volumes, chatVolumeMount)

				// The published messaging image runs as the non-root "astro" user
				// and does not pre-create /data. Docker initializes a fresh named
				// volume's mountpoint as root:root, so the sidecar cannot create its
				// SQLite chat DB there (SQLITE_CANTOPEN) and crashes on startup.
				// Mirror the deployed sidecar's fsGroup/init behavior with a one-shot
				// init container that chowns the volume to the astro uid before the
				// sidecar starts. Reuses the messaging image so no extra pull.
				initName := "astro-messaging-init"
				project.Services[initName] = types.ServiceConfig{
					Name:       initName,
					Image:      messagingImage,
					PullPolicy: messagingPull,
					User:       "0:0",
					Entrypoint: types.ShellCommand{
						"/bin/sh", "-c",
						fmt.Sprintf("mkdir -p %s && chown -R %s %s", chatDataMountPath, messagingUserUIDGID, chatDataMountPath),
					},
					Volumes: []types.ServiceVolumeConfig{chatVolumeMount},
					Networks: map[string]*types.ServiceNetworkConfig{
						"astro-dev": nil,
					},
				}
				if messagingService.DependsOn == nil {
					messagingService.DependsOn = make(types.DependsOnConfig)
				}
				messagingService.DependsOn[initName] = types.ServiceDependency{
					Condition: types.ServiceConditionCompletedSuccessfully,
					Required:  true,
				}
			}
			project.Services["astro-messaging"] = messagingService
		}

	}

	// Collector is not included in dev mode — it runs as a K8s sidecar in
	// local-k8 and production deployments, not in Docker Compose.

	// Add ingestion services if defined
	// Each ingestion is a container that runs on a trigger (schedule, manual, startup, webhook)
	for name, ingestion := range s.Ingestion {
		serviceName := fmt.Sprintf("ingestion-%s", name)
		service := types.ServiceConfig{
			Name: serviceName,
			Networks: map[string]*types.ServiceNetworkConfig{
				"astro-dev": nil,
			},
		}

		// webhook ingestions run as persistent servers — start with compose up and expose their port.
		// All other types are triggered on-demand (scheduler or manual) and use the ingestion profile.
		if ingestion.Trigger.Type == "webhook" {
			port := ingestion.Container.Port
			if port == 0 {
				port = 3001
			}
			service.Ports = []types.ServicePortConfig{
				{Target: uint32(port), Published: fmt.Sprintf("%d", port)},
			}
		} else {
			service.Profiles = []string{"ingestion"}
		}

		// Build or image configuration
		if ingestion.Container.Build != nil {
			service.Build = &types.BuildConfig{
				Context:    filepath.Join(workingDir, ingestion.Container.Build.Context),
				Dockerfile: ingestion.Container.Build.Dockerfile,
				Target:     ingestion.Container.Build.Target,
				Args:       types.MappingWithEquals(convertArgs(ingestion.Container.Build.Args)),
				Secrets:    buildSecretsConfig(ingestion.Container.Build.Secrets, project),
			}
		} else if ingestion.Container.Image != "" {
			service.Image = ingestion.Container.Image
		}

		// Environment variables - inherit from agent
		service.Environment = BuildEnvironment(s, envVars, opt)

		// Inject ingestion-specific inputs (from ast configure / .env, with default fallback)
		for _, inp := range ingestion.Inputs {
			val := inp.Default
			if v, ok := envVars[inp.Name]; ok {
				val = v
			}
			if val != "" {
				if service.Environment == nil {
					service.Environment = make(types.MappingWithEquals)
				}
				v := val
				service.Environment[inp.Name] = &v
			}
		}

		project.Services[serviceName] = service
	}

	// Add agent service
	agentService := types.ServiceConfig{
		Name: "agent",
		Networks: map[string]*types.ServiceNetworkConfig{
			"astro-dev": nil,
		},
	}

	// Build configuration
	if s.Agent.Build != nil {
		agentService.Build = &types.BuildConfig{
			Context:    filepath.Join(workingDir, s.Agent.Build.Context),
			Dockerfile: s.Agent.Build.Dockerfile,
			Target:     s.Agent.Build.Target,
			Args:       types.MappingWithEquals(convertArgs(s.Agent.Build.Args)),
			Secrets:    buildSecretsConfig(s.Agent.Build.Secrets, project),
		}
	} else if s.Agent.Image != "" {
		agentService.Image = s.Agent.Image
	}

	// Persistent disk: every agent gets a /data volume, matching the default
	// persistent disk provisioned in production. Keeps local state (e.g. a
	// SQLite database under /data) across `ast dev` restarts.
	const agentDataVolume = "agent-data"
	project.Volumes[agentDataVolume] = types.VolumeConfig{Name: agentDataVolume}
	agentService.Volumes = []types.ServiceVolumeConfig{
		{
			Type:   types.VolumeTypeVolume,
			Source: agentDataVolume,
			Target: spec.DefaultAgentVolumeMount,
		},
	}

	// Volume mount for hot reload
	if s.Agent.Build != nil {
		agentService.Volumes = append(agentService.Volumes, types.ServiceVolumeConfig{
			Type:   types.VolumeTypeBind,
			Source: filepath.Join(workingDir, "agent"),
			Target: "/app/agent",
		})
	}

	// Override container command from dev.command
	if s.Dev != nil && s.Dev.Command != "" {
		agentService.Command = types.ShellCommand{"sh", "-c", s.Dev.Command}
	}

	// Environment variables
	agentService.Environment = BuildEnvironment(s, envVars, opt)

	// Dependencies - depend on all other services
	dependsOn := make(types.DependsOnConfig)
	for _, service := range project.Services {
		dependsOn[service.Name] = types.ServiceDependency{
			Condition: types.ServiceConditionStarted,
		}
	}
	agentService.DependsOn = dependsOn

	// If the agent serves its own frontend, publish the configured dev port (default 80).
	if s.Agent.HasFrontend() {
		targetPort := 80
		if s.Dev != nil && s.Dev.Interfaces != nil && s.Dev.Interfaces.Frontend != nil && s.Dev.Interfaces.Frontend.Port != 0 {
			targetPort = s.Dev.Interfaces.Frontend.Port
		}
		agentService.Ports = append(agentService.Ports, types.ServicePortConfig{
			Target:    uint32(targetPort),
			Published: "3200",
		})
	}

	// When using native Ollama, the agent container must resolve host.docker.internal
	// to reach the host's Ollama server.
	if opt.NativeOllama {
		agentService.ExtraHosts = types.HostsList{
			"host.docker.internal": {"host-gateway"},
		}
	}

	// Add agent service last
	project.Services["agent"] = agentService

	// Set required compose labels on every service so containers are correctly
	// tagged and discoverable by the SDK. This mirrors what the compose-go YAML
	// loader does in postProcessProject.
	for name, svc := range project.Services {
		svc.CustomLabels = types.Labels{
			api.ProjectLabel:    project.Name,
			api.ServiceLabel:    name,
			api.VersionLabel:    api.ComposeVersion,
			api.WorkingDirLabel: project.WorkingDir,
			api.OneoffLabel:     "False",
		}
		project.Services[name] = svc
	}

	return project, nil
}

// BuildEnvironment creates environment variables for the agent container.
// Exported so buildLocalAgentEnv can reuse it for --local mode.
func BuildEnvironment(s *spec.AstroSpec, envVars map[string]string, opts ...BuildOptions) types.MappingWithEquals {
	var opt BuildOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	env := make(types.MappingWithEquals)

	// Cloud-provider credentials (models / knowledge / integrations). The
	// resolver is the single source of truth for env-var names across both
	// local dev and prod deploy paths — historically composeBuilder used a
	// different `<NAME>_<SUFFIX>` convention which diverged from what the
	// deployer injects in prod (`<PROVIDER>_[<NAME>_]<SUFFIX>` per §8.1).
	// Calling CloudCredentialKeys here keeps dev/prod names identical, so
	// agent code that works locally also works after a deploy.
	for credKey := range spec.CloudCredentialKeys(s) {
		if val, ok := envVars[credKey]; ok {
			env[credKey] = &val
		}
	}

	// AI Gateway opt-in (agent.astro_ai_gateway: true) — the CLI fetches a dev key
	// before BuildProject runs and stuffs ASTRO_GATEWAY_URL + ASTRO_GATEWAY_API_KEY
	// into envVars; copy them through so the compose-managed agent container
	// sees the same names the deployer injects in prod.
	if s.Agent.AIGateway {
		for _, k := range []string{"ASTRO_GATEWAY_URL", "ASTRO_GATEWAY_API_KEY"} {
			if val, ok := envVars[k]; ok {
				env[k] = &val
			}
		}
	}

	// Auto-inject connection strings for container-deploying components.
	for name, model := range s.Models {
		// Native Ollama: inject env vars pointing at host, skip container logic.
		if opt.NativeOllama && model.Provider == "ollama" {
			prov := spec.GetModelProvider("ollama")
			host := "host.docker.internal"
			port := fmt.Sprintf("%d", prov.DefaultPort)
			if prov.EnvPrefix != "" {
				env[prov.EnvPrefix+"_HOST"] = &host
				env[prov.EnvPrefix+"_PORT"] = &port
				modelURL := fmt.Sprintf("http://%s:%s", host, port)
				env[prov.EnvPrefix+"_URL"] = &modelURL
				baseURL := fmt.Sprintf("http://%s:%s/api", host, port)
				env[prov.EnvPrefix+"_BASE_URL"] = &baseURL
			}
			if len(model.ResolvedModels()) > 0 && prov.EnvPrefix != "" {
				m := strings.Join(model.ResolvedModels(), ",")
				env[prov.EnvPrefix+"_MODEL"] = &m
			}
			continue
		}

		if !model.DeploysContainer(s.Providers) {
			// Cloud provider credentials are injected via CloudCredentialKeys above.
			continue
		}

		serviceName := fmt.Sprintf("model-%s", name)
		resolved := model.ResolvedContainer()
		port := fmt.Sprintf("%d", resolved.Port)
		if resolved.Port == 0 {
			port = "8080"
		}

		if model.IsProviderMode() {
			// Provider-specific env vars (e.g., OLLAMA_HOST, OLLAMA_BASE_URL, OLLAMA_MODEL)
			prov := spec.GetModelProvider(model.Provider)
			if prov.EnvPrefix != "" {
				env[prov.EnvPrefix+"_HOST"] = &serviceName
				env[prov.EnvPrefix+"_PORT"] = &port
				modelURL := fmt.Sprintf("http://%s:%s", serviceName, port)
				env[prov.EnvPrefix+"_URL"] = &modelURL
				baseURL := fmt.Sprintf("http://%s:%s/api", serviceName, port)
				env[prov.EnvPrefix+"_BASE_URL"] = &baseURL
			}
			if len(model.ResolvedModels()) > 0 && prov.EnvPrefix != "" {
				m := strings.Join(model.ResolvedModels(), ",")
				env[prov.EnvPrefix+"_MODEL"] = &m
			}
		} else {
			// Generic env vars for container-mode models
			envPrefix := "MODEL_" + strings.ToUpper(name)
			env[envPrefix+"_HOST"] = &serviceName
			env[envPrefix+"_PORT"] = &port
			modelURL := fmt.Sprintf("http://%s:%s", serviceName, port)
			env[envPrefix+"_URL"] = &modelURL
		}
	}

	for name, knowledge := range s.Knowledge {
		if !knowledge.DeploysContainer(s.Providers) {
			// Cloud provider credentials are injected via CloudCredentialKeys above.
			continue
		}

		serviceName := fmt.Sprintf("knowledge-%s", name)

		prov := spec.GetProvider(knowledge.Provider)
		if prov.EnvPrefix != "" {
			hostKey := prov.EnvPrefix + "_HOST"
			portKey := prov.EnvPrefix + "_PORT"
			env[hostKey] = &serviceName
			port := fmt.Sprintf("%d", prov.DefaultPort)
			env[portKey] = &port

			// For postgres, mirror prod and inject the full USER/PASSWORD/DB
			// triple so the agent can connect without the user having to
			// declare them as inputs. Sidecar gets the same values via
			// BuildProject's postgres block — keep these in sync.
			if knowledge.Provider == "postgres" {
				user, password, dbName := postgresDevCredentials(s, envVars)
				env["POSTGRES_USER"] = &user
				env["POSTGRES_PASSWORD"] = &password
				env["POSTGRES_DB"] = &dbName
			}
		}
	}

	// Cloud-integration credentials are injected via CloudCredentialKeys above.

	// Custom-provider variables — names come from spec.CustomProviderCredentialKeys,
	// the same source of truth the deployer uses in prod. Resolver-correct names
	// only: agent code (and .env entries) must use the prefixed name.
	for credKey := range spec.CustomProviderCredentialKeys(s) {
		if val, ok := envVars[credKey]; ok {
			env[credKey] = &val
		}
	}

	// Inject top-level inputs (default or from .env)
	for _, inp := range s.Inputs {
		val := inp.Default
		if v, ok := envVars[inp.Name]; ok {
			val = v
		}
		if val != "" {
			v := val
			env[inp.Name] = &v
		}
	}

	// Inject agent-level inputs (default or from .env)
	for _, inp := range s.Agent.Inputs {
		val := inp.Default
		if v, ok := envVars[inp.Name]; ok {
			val = v
		}
		if val != "" {
			v := val
			env[inp.Name] = &v
		}
	}

	// Note: Messaging interface credentials (Slack, Discord, etc.) are NOT passed to the agent
	// They are passed to the astro-messaging sidecar which handles all messaging platform communication

	// Add GRPC_SERVER_ADDR if messaging is configured
	if s.Dev.HasMessagingAdapters() {
		grpcAddr := "astro-messaging:9090"
		env["GRPC_SERVER_ADDR"] = &grpcAddr
	}

	// Frontend agents: inject PORT so frameworks that read process.env.PORT
	// (Express, FastAPI, etc.) bind to the port the compose builder publishes
	// — the dev override when set, otherwise 80 (matching the prod contract).
	if s.Agent.HasFrontend() {
		port := "80"
		if s.Dev != nil && s.Dev.Interfaces != nil && s.Dev.Interfaces.Frontend != nil && s.Dev.Interfaces.Frontend.Port != 0 {
			port = fmt.Sprintf("%d", s.Dev.Interfaces.Frontend.Port)
		}
		env["PORT"] = &port
	}

	return env
}

// buildMessagingPorts creates port mappings for the astro-messaging sidecar
func buildMessagingPorts(s *spec.AstroSpec) []types.ServicePortConfig {
	ports := []types.ServicePortConfig{
		{
			Target: 9090,
			// Publish on a non-default host port to avoid common local conflicts
			// (for example, kubectl port-forwards frequently bind localhost:9090).
			Published: "19090",
		},
	}

	// Add HTTP port if web adapter is enabled. Published on MessagingWebHostPort
	// (not 3100) because the astro CLI now serves the chat UI on 3100 itself and
	// proxies API calls to this sidecar port (see internal/chatui).
	if slices.Contains(s.Dev.MessagingAdapters(), "web") {
		ports = append(ports, types.ServicePortConfig{
			Target:    8080,
			Published: MessagingWebHostPort,
		})
	}

	return ports
}

// buildMessagingEnvironment creates environment variables for the astro-messaging sidecar
func buildMessagingEnvironment(s *spec.AstroSpec, envVars map[string]string) types.MappingWithEquals {
	env := make(types.MappingWithEquals)

	// gRPC configuration
	grpcEnabled := "true"
	env["GRPC_ENABLED"] = &grpcEnabled
	grpcListenAddr := ":9090"
	env["GRPC_LISTEN_ADDR"] = &grpcListenAddr

	// Storage configuration (use memory for dev, redis in production)
	storageType := "memory"
	env["STORAGE_TYPE"] = &storageType

	// Log level — spec override wins, otherwise default to debug since this
	// path runs in local dev where verbose logs aid troubleshooting.
	logLevel := "debug"
	if override := s.Dev.MessagingLogLevel(); override != "" {
		logLevel = override
	}
	env["LOG_LEVEL"] = &logLevel

	// Dev mode — lets the messaging service tag outgoing messages
	devMode := "true"
	env["DEV"] = &devMode

	// Configure adapters based on interfaces
	for _, name := range s.Dev.MessagingAdapters() {
		switch name {
		case "slack":
			// Enable Slack adapter only when credentials are present;
			// the messaging service hard-fails if SLACK_ENABLED=true without a token.
			botToken, hasBotToken := envVars["SLACK_BOT_TOKEN"]
			appToken, hasAppToken := envVars["SLACK_APP_TOKEN"]
			if !hasBotToken {
				fmt.Printf("⚠ Slack adapter listed but SLACK_BOT_TOKEN not set — skipping (run '%s configure' to add it)\n", buildinfo.BinaryName)
				continue
			}
			enabled := "true"
			env["SLACK_ENABLED"] = &enabled
			env["SLACK_BOT_TOKEN"] = &botToken
			if hasAppToken {
				env["SLACK_APP_TOKEN"] = &appToken
			}

			if cfg := s.Dev.SlackConfig(); cfg != nil {
				if data, err := json.Marshal(cfg); err == nil {
					jsonStr := string(data)
					env["SLACK_CONFIG"] = &jsonStr
				}
			}

		case "web":
			// Enable the Web adapter for HTTP/SSE access. The chat UI is now
			// served by the astro CLI (internal/chatui), so the sidecar's bundled
			// playground is disabled — the sidecar is the API/persistence backend
			// only.
			enabled := "true"
			env["WEB_ENABLED"] = &enabled
			listenAddr := ":8080"
			env["WEB_LISTEN_ADDR"] = &listenAddr
			servePlayground := "false"
			env["WEB_SERVE_PLAYGROUND"] = &servePlayground
			// Persist chat history in the sidecar's SQLite store on the mounted
			// volume. Without CHAT_DB_PATH the sidecar disables persistence.
			dbPath := chatDBPath
			env["CHAT_DB_PATH"] = &dbPath
		}
	}

	return env
}
