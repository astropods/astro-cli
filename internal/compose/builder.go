package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/postman/astro/packages/astro-spec"
)

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

			// Add healthcheck for model services
			interval := types.Duration(10000000000) // 10 seconds
			timeout := types.Duration(5000000000)   // 5 seconds
			retries := uint64(3)
			service.HealthCheck = &types.HealthCheckConfig{
				Test:     types.HealthCheckTest([]string{"CMD-SHELL", fmt.Sprintf("wget --no-verbose --tries=1 --spider http://localhost:%d/ || exit 1", model.Container.Port)}),
				Interval: &interval,
				Timeout:  &timeout,
				Retries:  &retries,
			}
		}

		project.Services[serviceName] = service
	}

	// Add self-hosted knowledge stores
	for name, knowledge := range s.Knowledge {
		serviceName := fmt.Sprintf("knowledge-%s", name)
		service := types.ServiceConfig{
			Name: serviceName,
			Networks: map[string]*types.ServiceNetworkConfig{
				"astro-dev": nil,
			},
		}

		// Build or image configuration
		if knowledge.Container.Build != nil {
			service.Build = &types.BuildConfig{
				Context:    filepath.Join(workingDir, knowledge.Container.Build.Context),
				Dockerfile: knowledge.Container.Build.Dockerfile,
			}
		} else if knowledge.Container.Image != "" {
			service.Image = knowledge.Container.Image
		}

		// Port mapping
		if knowledge.Container.Port > 0 {
			service.Ports = []types.ServicePortConfig{
				{
					Target:    uint32(knowledge.Container.Port),
					Published: fmt.Sprintf("%d", knowledge.Container.Port),
				},
			}
		} else if knowledge.Provider == "qdrant" {
			// Expose Qdrant dashboard on default port 6333 in dev mode
			service.Ports = []types.ServicePortConfig{
				{
					Target:    6333,
					Published: "6333",
				},
			}
		}

		// Add healthcheck for knowledge services (e.g., Qdrant, Redis)
		interval := types.Duration(10000000000) // 10 seconds
		retries := uint64(3)
		if knowledge.Provider == "qdrant" {
			timeout := types.Duration(5000000000) // 5 seconds
			// Use default Qdrant port if not specified
			port := knowledge.Container.Port
			if port == 0 {
				port = 6333
			}
			service.HealthCheck = &types.HealthCheckConfig{
				Test:     types.HealthCheckTest([]string{"CMD-SHELL", fmt.Sprintf("wget --no-verbose --tries=1 --spider http://localhost:%d/healthz || exit 1", port)}),
				Interval: &interval,
				Timeout:  &timeout,
				Retries:  &retries,
			}
		} else if knowledge.Provider == "redis" {
			timeout := types.Duration(3000000000) // 3 seconds
			service.HealthCheck = &types.HealthCheckConfig{
				Test:     types.HealthCheckTest([]string{"CMD", "redis-cli", "ping"}),
				Interval: &interval,
				Timeout:  &timeout,
				Retries:  &retries,
			}
		}

		// Add persistent volume if needed
		if knowledge.Container.Persistent {
			volumeName := fmt.Sprintf("%s-data", serviceName)
			project.Volumes[volumeName] = types.VolumeConfig{
				Name: volumeName,
			}

			// Use provider-specific mount path
			mountPath := "/data"
			if knowledge.Provider == "qdrant" {
				mountPath = "/qdrant/storage"
			}

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
		// Check for messaging interfaces (with or without messaging/ prefix)
		if iface.Type == "slack" || iface.Type == "discord" || iface.Type == "teams" ||
			iface.Type == "messaging/slack" || iface.Type == "messaging/discord" || iface.Type == "messaging/teams" {
			messagingService := types.ServiceConfig{
				Name:  "astro-messaging",
				Image: fmt.Sprintf("%s/prod-astro-messaging:latest", envVars["REGISTRY_URL"]),
				Networks: map[string]*types.ServiceNetworkConfig{
					"astro-dev": nil,
				},
				Environment: buildMessagingEnvironment(s, envVars),
				Ports: []types.ServicePortConfig{
					{
						Target:    9090,
						Published: "9090",
					},
				},
			}
			project.Services["astro-messaging"] = messagingService
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
					redisService := fmt.Sprintf("knowledge-%s", name)
					redisURL := fmt.Sprintf("redis://%s:6379", redisService)
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
	if s.Container.Build != nil {
		agentService.Build = &types.BuildConfig{
			Context:    filepath.Join(workingDir, s.Container.Build.Context),
			Dockerfile: s.Container.Build.Dockerfile,
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

		// Common patterns for connection strings
		switch knowledge.Provider {
		case "qdrant":
			hostKey := fmt.Sprintf("QDRANT_HOST")
			portKey := fmt.Sprintf("QDRANT_PORT")
			env[hostKey] = &serviceName
			port := "6333"
			env[portKey] = &port
		case "redis":
			hostKey := fmt.Sprintf("REDIS_HOST")
			portKey := fmt.Sprintf("REDIS_PORT")
			env[hostKey] = &serviceName
			port := "6379"
			env[portKey] = &port
		case "postgres":
			hostKey := fmt.Sprintf("POSTGRES_HOST")
			portKey := fmt.Sprintf("POSTGRES_PORT")
			env[hostKey] = &serviceName
			port := "5432"
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

	// Add GRPC_SERVER_ADDR if messaging interface is configured
	for _, iface := range s.Interfaces {
		if iface.Type == "slack" || iface.Type == "discord" || iface.Type == "teams" ||
			iface.Type == "messaging/slack" || iface.Type == "messaging/discord" || iface.Type == "messaging/teams" {
			grpcAddr := "astro-messaging:9090"
			env["GRPC_SERVER_ADDR"] = &grpcAddr
			break
		}
	}

	return env
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

	// Deployment mode - determine which adapters to enable
	deploymentMode := "all"
	env["DEPLOYMENT_MODE"] = &deploymentMode

	// Configure adapters based on interfaces
	for _, iface := range s.Interfaces {
		switch iface.Type {
		case "slack", "messaging/slack":
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

		case "discord", "messaging/discord":
			// Enable Discord adapter (when implemented)
			enabled := "true"
			env["DISCORD_ENABLED"] = &enabled

			if val, ok := envVars["DISCORD_BOT_TOKEN"]; ok {
				env["DISCORD_BOT_TOKEN"] = &val
			}

		case "teams", "messaging/teams":
			// Enable Teams adapter (when implemented)
			enabled := "true"
			env["TEAMS_ENABLED"] = &enabled

			if val, ok := envVars["TEAMS_APP_ID"]; ok {
				env["TEAMS_APP_ID"] = &val
			}
			if val, ok := envVars["TEAMS_APP_PASSWORD"]; ok {
				env["TEAMS_APP_PASSWORD"] = &val
			}
		}
	}

	return env
}

