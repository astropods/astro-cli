package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	spec "github.com/postman/astro/packages/astro-spec"
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
		Name:       s.Agent,
		WorkingDir: workingDir,
		Services:   make(types.Services),
		Networks:   make(types.Networks),
		Volumes:    make(types.Volumes),
	}

	// Create default network
	project.Networks["astro-dev"] = types.NetworkConfig{
		Name:   fmt.Sprintf("%s-network", s.Agent),
		Driver: "bridge",
	}

	// Add self-hosted models
	for name, model := range s.Models {
		serviceName := fmt.Sprintf("model-%s", name)
		service := types.ServiceConfig{
			Name: serviceName,
			Networks: map[string]*types.ServiceNetworkConfig{
				"astro-dev": nil,
			},
		}

		// Build or image configuration
		if model.Container.Build != nil {
			service.Build = &types.BuildConfig{
				Context:    filepath.Join(workingDir, model.Container.Build.Context),
				Dockerfile: model.Container.Build.Dockerfile,
				Target:     model.Container.Build.Target,
				Args:       types.MappingWithEquals(convertArgs(model.Container.Build.Args)),
				Secrets:    buildSecretsConfig(model.Container.Build.Secrets, project),
			}
		} else if model.Container.Image != "" {
			service.Image = model.Container.Image
		}

		// Port mapping
		if model.Container.Port > 0 {
			service.Ports = []types.ServicePortConfig{
				{
					Target:    uint32(model.Container.Port),
					Published: fmt.Sprintf("%d", model.Container.Port),
				},
			}

			// Add healthcheck only if defined in spec
			if model.Container.Healthcheck != nil {
				interval := types.Duration(10000000000) // 10 seconds
				timeout := types.Duration(5000000000)   // 5 seconds
				retries := uint64(3)

				test := buildHealthCheckTest(model.Container.Healthcheck, "", model.Container.Port)
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

		project.Services[serviceName] = service
	}

	// Add self-hosted knowledge stores
	for name, knowledge := range s.Knowledge {
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
					Target:    uint32(container.Port),
					Published: fmt.Sprintf("%d", container.Port),
				},
			}
		} else if knowledge.Provider == "qdrant" {
			// Expose Qdrant dashboard on default port in dev mode
			qdrantPort := spec.GetProvider("qdrant").DefaultPort
			service.Ports = []types.ServicePortConfig{
				{
					Target:    uint32(qdrantPort),
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

		project.Services[serviceName] = service
	}

	// Add self-hosted tools (with containers)
	for name, tool := range s.Tools {
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
						Target:    uint32(tool.Container.Port),
						Published: fmt.Sprintf("%d", tool.Container.Port),
					},
				}
			}

			project.Services[serviceName] = service
		}
	}

	// Add interface services (messaging sidecar, grpc services, etc.)
	for _, iface := range s.Interfaces {
		// Check for messaging interfaces
		if iface.Type == "slack" || iface.Type == "web" {
			messagingService := types.ServiceConfig{
				Name:       "astro-messaging",
				Image:      "ghcr.io/saswatds/astro-messaging:latest",
				PullPolicy: types.PullPolicyAlways,
				Networks: map[string]*types.ServiceNetworkConfig{
					"astro-dev": nil,
				},
				Environment: buildMessagingEnvironment(s, envVars),
				Ports:       buildMessagingPorts(s),
			}
			project.Services["astro-messaging"] = messagingService
		}

		// Add playground for web interface
		if iface.Type == "web" {
			// Empty string = use relative URLs (nginx proxies /api to astro-messaging)
			apiURL := ""
			playgroundService := types.ServiceConfig{
				Name:       "playground",
				Image:      "ghcr.io/saswatds/astro-playground:latest",
				PullPolicy: types.PullPolicyAlways,
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

		// Check for custom interfaces with explicit service configuration
		if iface.Type == "custom" && iface.Service != nil {
			service := types.ServiceConfig{
				Name: iface.Service.Name,
				Networks: map[string]*types.ServiceNetworkConfig{
					"astro-dev": nil,
				},
			}

			// Build or image configuration
			if iface.Service.Build != nil {
				service.Build = &types.BuildConfig{
					Context:    filepath.Join(workingDir, iface.Service.Build.Context),
					Dockerfile: iface.Service.Build.Dockerfile,
					Target:     iface.Service.Build.Target,
					Args:       types.MappingWithEquals(convertArgs(iface.Service.Build.Args)),
					Secrets:    buildSecretsConfig(iface.Service.Build.Secrets, project),
				}
			} else if iface.Service.Image != "" {
				service.Image = iface.Service.Image
			}

			// Port mappings
			if len(iface.Service.Ports) > 0 {
				for _, portMapping := range iface.Service.Ports {
					var portConfig types.ServicePortConfig
					fmt.Sscanf(portMapping, "%d:%d", &portConfig.Published, &portConfig.Target)
					if portConfig.Target == 0 {
						// Single port format "8080"
						var port uint32
						fmt.Sscanf(portMapping, "%d", &port)
						portConfig.Target = port
						portConfig.Published = fmt.Sprintf("%d", port)
					}
					service.Ports = append(service.Ports, portConfig)
				}
			}

			// Environment variables from service config
			if service.Environment == nil {
				service.Environment = make(types.MappingWithEquals)
			}

			// Add service-specific environment variables
			if len(iface.Service.Environment) > 0 {
				for key, val := range iface.Service.Environment {
					// Expand environment variables
					expandedVal := os.ExpandEnv(val)
					// If the value references an env var from .env, use that
					if envVal, ok := envVars[key]; ok {
						service.Environment[key] = &envVal
					} else {
						service.Environment[key] = &expandedVal
					}
				}
			}

			// Auto-inject Redis connection info if Redis knowledge store exists
			for name, knowledge := range s.Knowledge {
				if knowledge.Provider == "redis" {
					redisProv := spec.GetProvider("redis")
					redisService := fmt.Sprintf("knowledge-%s", name)
					redisURL := fmt.Sprintf("%s://%s:%d", redisProv.URLScheme, redisService, redisProv.DefaultPort)
					service.Environment["REDIS_URL"] = &redisURL
				}
			}

			project.Services[iface.Service.Name] = service
		}
	}

	// Add ingestion services if defined
	// Each ingestion is a container that runs on a trigger (schedule, manual, startup)
	for name, ingestion := range s.Ingestion {
		serviceName := fmt.Sprintf("ingestion-%s", name)
		service := types.ServiceConfig{
			Name: serviceName,
			Networks: map[string]*types.ServiceNetworkConfig{
				"astro-dev": nil,
			},
			// Don't auto-start - triggered by scheduler or manually
			Profiles: []string{"ingestion"},
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

	// Add observability collector sidecar
	collectorService := types.ServiceConfig{
		Name:       "astro-collector",
		Image:      "ghcr.io/saswatds/astro-collector:latest",
		PullPolicy: types.PullPolicyAlways,
		Networks: map[string]*types.ServiceNetworkConfig{
			"astro-dev": nil,
		},
		Environment: buildCollectorEnvironment(s, envVars),
		Ports: []types.ServicePortConfig{
			{
				Target:    4317,
				Published: "4317",
			},
			{
				Target:    4318,
				Published: "4318",
			},
		},
	}
	project.Services["astro-collector"] = collectorService

	// Add agent service
	agentService := types.ServiceConfig{
		Name: "agent",
		Networks: map[string]*types.ServiceNetworkConfig{
			"astro-dev": nil,
		},
	}

	// Build configuration
	if s.Container.Build != nil {
		agentService.Build = &types.BuildConfig{
			Context:    filepath.Join(workingDir, s.Container.Build.Context),
			Dockerfile: s.Container.Build.Dockerfile,
			Target:     s.Container.Build.Target,
			Args:       types.MappingWithEquals(convertArgs(s.Container.Build.Args)),
			Secrets:    buildSecretsConfig(s.Container.Build.Secrets, project),
		}
	} else if s.Container.Image != "" {
		agentService.Image = s.Container.Image
	}

	// Volume mount for hot reload
	if s.Container.Build != nil {
		agentService.Volumes = []types.ServiceVolumeConfig{
			{
				Type:   types.VolumeTypeBind,
				Source: filepath.Join(workingDir, "agent"),
				Target: "/app/agent",
			},
		}
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

	// Ports for interfaces
	for _, iface := range s.Interfaces {
		if iface.Type == "http" {
			if port, ok := iface.Config["port"].(int); ok {
				agentService.Ports = []types.ServicePortConfig{
					{
						Target:    uint32(port),
						Published: fmt.Sprintf("%d", port),
					},
				}
			}
		}
	}

	// Add agent service last
	project.Services["agent"] = agentService

	return project, nil
}

// buildEnvironment creates environment variables for the agent container
func buildEnvironment(s *spec.AstroSpec, envVars map[string]string) types.MappingWithEquals {
	env := make(types.MappingWithEquals)

	// Auto-inject connection strings for self-hosted components
	for name := range s.Models {
		serviceName := fmt.Sprintf("model-%s", name)
		envKey := strings.ToUpper(fmt.Sprintf("%s_HOST", name))
		env[envKey] = &serviceName
	}

	for name, knowledge := range s.Knowledge {
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

	// Inject integration credentials from .env
	for _, model := range s.Integrations.Models {
		// Determine env var prefix
		prefix := ""
		if model.Env != nil && model.Env.Prefix != "" {
			prefix = model.Env.Prefix
		}

		switch model.Provider {
		case "anthropic":
			key := prefix + "ANTHROPIC_API_KEY"
			if val, ok := envVars[key]; ok {
				env[key] = &val
			}
		case "openai":
			key := prefix + "OPENAI_API_KEY"
			if val, ok := envVars[key]; ok {
				env[key] = &val
			}
		}
	}

	for _, tool := range s.Integrations.Tools {
		// Determine env var prefix
		prefix := ""
		if tool.Env != nil && tool.Env.Prefix != "" {
			prefix = tool.Env.Prefix
		}

		switch tool.Provider {
		case "github":
			key := prefix + "GITHUB_TOKEN"
			if val, ok := envVars[key]; ok {
				env[key] = &val
			}
		case "slack":
			key := prefix + "SLACK_BOT_TOKEN"
			if val, ok := envVars[key]; ok {
				env[key] = &val
			}
		case "tavily":
			key := prefix + "TAVILY_API_KEY"
			if val, ok := envVars[key]; ok {
				env[key] = &val
			}
		default:
			// Generic pattern: PROVIDER_API_KEY
			key := prefix + fmt.Sprintf("%s_API_KEY", strings.ToUpper(tool.Provider))
			if val, ok := envVars[key]; ok {
				env[key] = &val
			}
		}
	}

	// Note: Messaging interface credentials (Slack, Discord, etc.) are NOT passed to the agent
	// They are passed to the astro-messaging sidecar which handles all messaging platform communication

	// Add GRPC_SERVER_ADDR if any interface is configured
	if len(s.Interfaces) > 0 {
		grpcAddr := "astro-messaging:9090"
		env["GRPC_SERVER_ADDR"] = &grpcAddr
	}

	// Inject OTel collector endpoint for automatic agent telemetry export
	otelEndpoint := "http://astro-collector:4318"
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = &otelEndpoint

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
	for _, iface := range s.Interfaces {
		if iface.Type == "web" {
			ports = append(ports, types.ServicePortConfig{
				Target:    8080,
				Published: "8080",
			})
			break
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
	for _, iface := range s.Interfaces {
		switch iface.Type {
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

	return env
}

// buildCollectorEnvironment creates environment variables for the astro-collector sidecar.
// Dev mode disables Galileo forwarding; spans are logged locally via the debug exporter.
func buildCollectorEnvironment(s *spec.AstroSpec, envVars map[string]string) types.MappingWithEquals {
	env := make(types.MappingWithEquals)

	mode := "dev"
	env["ASTRO_COLLECTOR_MODE"] = &mode

	agentName := s.Agent
	env["ASTRO_AGENT_NAME"] = &agentName

	if s.Meta.Version != "" {
		version := s.Meta.Version
		env["ASTRO_AGENT_VERSION"] = &version
	}

	// Optional collector tuning from .env
	if val, ok := envVars["COLLECTOR_LOG_LEVEL"]; ok {
		env["COLLECTOR_LOG_LEVEL"] = &val
	}
	if val, ok := envVars["COLLECTOR_DEBUG_VERBOSITY"]; ok {
		env["COLLECTOR_DEBUG_VERBOSITY"] = &val
	}
	if val, ok := envVars["ASTRO_REDACT_PROMPTS"]; ok {
		env["ASTRO_REDACT_PROMPTS"] = &val
	}

	return env
}
