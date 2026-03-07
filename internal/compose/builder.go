package compose

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/compose-spec/compose-go/v2/types"
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

// BuildProject converts an AstroSpec to a Docker Compose project
func BuildProject(s *spec.AstroSpec, workingDir string, envVars map[string]string) (*types.Project, error) {
	project := &types.Project{
		Name:       s.Name,
		WorkingDir: workingDir,
		Services:   make(types.Services),
		Networks:   make(types.Networks),
		Volumes:    make(types.Volumes),
	}

	// Create default network
	project.Networks["astro-dev"] = types.NetworkConfig{
		Name:   fmt.Sprintf("%s-network", s.Name),
		Driver: "bridge",
	}

	// Add models that deploy a container (skip cloud and custom providers)
	for name, model := range s.Models {
		if !model.DeploysContainer(s.Providers) {
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

		// Add persistent volume if needed
		if container.Persistent {
			volumeName := fmt.Sprintf("%s-data", serviceName)
			project.Volumes[volumeName] = types.VolumeConfig{
				Name: volumeName,
			}

			mountPath := spec.GetProvider(knowledge.Provider).MountPath

			service.Volumes = []types.ServiceVolumeConfig{
				{
					Type:   types.VolumeTypeVolume,
					Source: volumeName,
					Target: mountPath,
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

		project.Services[serviceName] = service
	}

	// Add tools that deploy a container (skip cloud and custom providers)
	for name, tool := range s.Tools {
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

			project.Services[serviceName] = service
		}
	}

	// Add interface services (messaging sidecar, grpc services, etc.)
	if s.Dev != nil {
		for _, name := range s.Dev.Interfaces {
			// Check for messaging interfaces
			if name == "slack" || name == "web" {
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
				project.Services["astro-messaging"] = messagingService
			}

			// Add playground for web interface
			if name == "web" {
				// Empty string = use relative URLs (nginx proxies /api to astro-messaging)
				apiURL := ""
				playgroundImage := "astropods/playground:latest"
				playgroundPull := types.PullPolicyAlways
				if s.Dev.Overrides != nil && s.Dev.Overrides.PlaygroundImage != "" {
					playgroundImage = s.Dev.Overrides.PlaygroundImage
					playgroundPull = ""
				}
				playgroundService := types.ServiceConfig{
					Name:       "playground",
					Image:      playgroundImage,
					PullPolicy: playgroundPull,
					Networks: map[string]*types.ServiceNetworkConfig{
						"astro-dev": nil,
					},
					Environment: types.MappingWithEquals{
						"API_URL": &apiURL,
					},
					Ports: []types.ServicePortConfig{
						{
							Target:    80,
							Published: "3000",
						},
					},
					DependsOn: types.DependsOnConfig{
						"astro-messaging": types.ServiceDependency{
							Condition: types.ServiceConditionStarted,
							Required:  true,
						},
					},
				}
				project.Services["playground"] = playgroundService
			}
		}
	}

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
		service.Environment = buildEnvironment(s, envVars)

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

	// Volume mount for hot reload
	if s.Agent.Build != nil {
		agentService.Volumes = []types.ServiceVolumeConfig{
			{
				Type:   types.VolumeTypeBind,
				Source: filepath.Join(workingDir, "agent"),
				Target: "/app/agent",
			},
		}
	}

	// Override container command from dev.command
	if s.Dev != nil && s.Dev.Command != "" {
		agentService.Command = types.ShellCommand{"sh", "-c", s.Dev.Command}
	}

	// Environment variables
	agentService.Environment = buildEnvironment(s, envVars)

	// Dependencies - depend on all other services
	dependsOn := make(types.DependsOnConfig)
	for _, service := range project.Services {
		dependsOn[service.Name] = types.ServiceDependency{
			Condition: types.ServiceConditionStarted,
		}
	}
	agentService.DependsOn = dependsOn

	// Ports for interfaces — no longer configured via spec; agent exposes 8080 by default

	// Add agent service last
	project.Services["agent"] = agentService

	return project, nil
}

// buildEnvironment creates environment variables for the agent container
func buildEnvironment(s *spec.AstroSpec, envVars map[string]string) types.MappingWithEquals {
	env := make(types.MappingWithEquals)

	// Auto-inject connection strings for container-deploying components;
	// inject credentials for cloud providers.
	for name, model := range s.Models {
		if !model.DeploysContainer(s.Providers) {
			// Cloud provider — inject credentials from .env
			if suffixes, ok := spec.GetCloudModelCredentials(model.Provider); ok {
				for _, cs := range suffixes {
					key := strings.ToUpper(name) + "_" + cs.Suffix
					if val, ok := envVars[key]; ok {
						env[key] = &val
					}
				}
			}
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
			if suffixes, ok := spec.GetCloudKnowledgeCredentials(knowledge.Provider); ok {
				for _, cs := range suffixes {
					key := strings.ToUpper(name) + "_" + cs.Suffix
					if val, ok := envVars[key]; ok {
						env[key] = &val
					}
				}
			}
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
		}
	}

	// Inject cloud tool provider credentials from .env
	for name, tool := range s.Tools {
		if !tool.DeploysContainer(s.Providers) {
			if suffixes, ok := spec.GetCloudToolCredentials(tool.Provider); ok {
				for _, cs := range suffixes {
					key := strings.ToUpper(name) + "_" + cs.Suffix
					if val, ok := envVars[key]; ok {
						env[key] = &val
					}
				}
			}
		}
	}

	// Inject custom provider variables from .env (read using variable name directly)
	for _, model := range s.Models {
		if model.IsProviderMode() {
			if cp, ok := s.Providers[model.Provider]; ok {
				for _, v := range cp.Variables {
					if val, exists := envVars[v.Name]; exists {
						val := val
						env[v.Name] = &val
					}
				}
			}
		}
	}
	for _, knowledge := range s.Knowledge {
		if knowledge.IsProviderMode() {
			if cp, ok := s.Providers[knowledge.Provider]; ok {
				for _, v := range cp.Variables {
					if val, exists := envVars[v.Name]; exists {
						val := val
						env[v.Name] = &val
					}
				}
			}
		}
	}
	for _, tool := range s.Tools {
		if tool.IsProviderMode() {
			if cp, ok := s.Providers[tool.Provider]; ok {
				for _, v := range cp.Variables {
					if val, exists := envVars[v.Name]; exists {
						val := val
						env[v.Name] = &val
					}
				}
			}
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

	// Add GRPC_SERVER_ADDR if any interface is configured
	if s.Dev != nil && len(s.Dev.Interfaces) > 0 {
		grpcAddr := "astro-messaging:9090"
		env["GRPC_SERVER_ADDR"] = &grpcAddr
	}

	return env
}

// buildMessagingPorts creates port mappings for the astro-messaging sidecar
func buildMessagingPorts(s *spec.AstroSpec) []types.ServicePortConfig {
	ports := []types.ServicePortConfig{
		{
			Target:    9090,
			Published: "9090",
		},
	}

	// Add HTTP port if web adapter is enabled
	if s.Dev != nil {
		for _, name := range s.Dev.Interfaces {
			if name == "web" {
				ports = append(ports, types.ServicePortConfig{
					Target:    8080,
					Published: "3100",
				})
				break
			}
		}
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

	// Log level
	logLevel := "info"
	env["LOG_LEVEL"] = &logLevel

	// Configure adapters based on interfaces
	if s.Dev != nil {
		for _, name := range s.Dev.Interfaces {
			switch name {
			case "slack":
				// Enable Slack adapter
				enabled := "true"
				env["SLACK_ENABLED"] = &enabled
				env["SLACK_SOCKET_MODE"] = &enabled

				// Slack credentials from .env
				if val, ok := envVars["SLACK_BOT_TOKEN"]; ok {
					env["SLACK_BOT_TOKEN"] = &val
				}
				if val, ok := envVars["SLACK_APP_TOKEN"]; ok {
					env["SLACK_APP_TOKEN"] = &val
				}

			case "web":
				// Enable Web adapter for HTTP/SSE access
				enabled := "true"
				env["WEB_ENABLED"] = &enabled
				listenAddr := ":8080"
				env["WEB_LISTEN_ADDR"] = &listenAddr
			}
		}
	}

	return env
}
