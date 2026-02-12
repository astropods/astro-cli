package deployment

import (
	"fmt"
	"strings"

	"github.com/postman/astro/packages/astro-spec"
)

// EnvBuilder builds environment variables for deployments
type EnvBuilder struct {
	k8sNamespace string
}

// NewEnvBuilder creates a new environment builder
func NewEnvBuilder(k8sNamespace string) *EnvBuilder {
	return &EnvBuilder{
		k8sNamespace: k8sNamespace,
	}
}

// BuildConnectionStrings builds connection string environment variables
func (b *EnvBuilder) BuildConnectionStrings(astroSpec *spec.AstroSpec) map[string]string {
	env := make(map[string]string)

	// Build model connection strings
	for name, model := range astroSpec.Models {
		if model.Container.Image != "" || model.Container.Build != nil {
			// Self-hosted model
			serviceName := GenerateResourceName(astroSpec.Agent, "model", name)
			host := GenerateServiceDNS(serviceName, b.k8sNamespace)
			port := model.Container.Port
			if port == 0 {
				port = 8080 // default port
			}

			envKey := fmt.Sprintf("MODEL_%s_HOST", strings.ToUpper(SanitizeName(name)))
			env[envKey] = host

			envKey = fmt.Sprintf("MODEL_%s_PORT", strings.ToUpper(SanitizeName(name)))
			env[envKey] = fmt.Sprintf("%d", port)

			envKey = fmt.Sprintf("MODEL_%s_URL", strings.ToUpper(SanitizeName(name)))
			env[envKey] = fmt.Sprintf("http://%s:%d", host, port)
		}
	}

	// Build knowledge store connection strings
	for name, knowledge := range astroSpec.Knowledge {
		container := knowledge.ResolvedContainer()
		if container.Image != "" || container.Build != nil {
			serviceName := GenerateResourceName(astroSpec.Agent, "knowledge", name)
			host := GenerateServiceDNS(serviceName, b.k8sNamespace)
			// Determine provider
			provider := knowledge.Provider

			prov := spec.GetProvider(provider)

			// Set default port based on provider
			port := container.Port
			if port == 0 {
				port = prov.DefaultPort
			}

			// Provider-specific environment variables
			if prov.EnvPrefix != "" {
				env[prov.EnvPrefix+"_HOST"] = host
				env[prov.EnvPrefix+"_PORT"] = fmt.Sprintf("%d", port)
				if prov.URLScheme != "" {
					env[prov.EnvPrefix+"_URL"] = fmt.Sprintf("%s://%s:%d", prov.URLScheme, host, port)
				}
			} else {
				envKey := fmt.Sprintf("KNOWLEDGE_%s_HOST", strings.ToUpper(SanitizeName(name)))
				env[envKey] = host
				envKey = fmt.Sprintf("KNOWLEDGE_%s_PORT", strings.ToUpper(SanitizeName(name)))
				env[envKey] = fmt.Sprintf("%d", port)
			}
		}
	}

	// Build tool connection strings
	for name, tool := range astroSpec.Tools {
		if tool.Container != nil && (tool.Container.Image != "" || tool.Container.Build != nil) {
			serviceName := GenerateResourceName(astroSpec.Agent, "tool", name)
			host := GenerateServiceDNS(serviceName, b.k8sNamespace)
			port := tool.Container.Port
			if port == 0 {
				port = 8080
			}

			envKey := fmt.Sprintf("TOOL_%s_HOST", strings.ToUpper(SanitizeName(name)))
			env[envKey] = host

			envKey = fmt.Sprintf("TOOL_%s_PORT", strings.ToUpper(SanitizeName(name)))
			env[envKey] = fmt.Sprintf("%d", port)

			envKey = fmt.Sprintf("TOOL_%s_URL", strings.ToUpper(SanitizeName(name)))
			env[envKey] = fmt.Sprintf("http://%s:%d", host, port)
		}
	}

	// Add agent's own URL
	agentServiceName := GenerateAgentResourceName(astroSpec.Agent, "agent")
	agentHost := GenerateServiceDNS(agentServiceName, b.k8sNamespace)
	env["AGENT_URL"] = fmt.Sprintf("http://%s:8080", agentHost)
	env["AGENT_HOST"] = agentHost

	// Add messaging service gRPC address for all interface types
	for name := range astroSpec.Interfaces {
		messagingServiceName := GenerateResourceName(astroSpec.Agent, "messaging", name)
		messagingHost := GenerateServiceDNS(messagingServiceName, b.k8sNamespace)
		env["GRPC_SERVER_ADDR"] = fmt.Sprintf("%s:9090", messagingHost)
		break // Only need one messaging service
	}

	// Add OTel collector endpoint for agent telemetry auto-export
	collectorServiceName := GenerateAgentResourceName(astroSpec.Agent, "collector")
	collectorHost := GenerateServiceDNS(collectorServiceName, b.k8sNamespace)
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = fmt.Sprintf("http://%s:4318", collectorHost)

	// Inject agent metadata for the collector sidecar.
	// Collector prov to prod mode (Galileo enabled); only ast dev overrides to dev.
	env["ASTRO_AGENT_NAME"] = astroSpec.Agent
	if astroSpec.Meta.Version != "" {
		env["ASTRO_AGENT_VERSION"] = astroSpec.Meta.Version
	}

	return env
}

// BuildCredentialEnvRefs builds environment variable references to secrets
func (b *EnvBuilder) BuildCredentialEnvRefs(userCredentials map[string]string, secretName string) []EnvVar {
	var envVars []EnvVar

	for key := range userCredentials {
		envVars = append(envVars, EnvVar{
			Name: strings.ToUpper(key),
			ValueFrom: &EnvVarSource{
				SecretKeyRef: &SecretKeyRef{
					Name: secretName,
					Key:  key,
				},
			},
		})
	}

	return envVars
}

// BuildConfigMapEnvRefs builds environment variable references to ConfigMap
func (b *EnvBuilder) BuildConfigMapEnvRefs(configMapName string, keys []string) []EnvVar {
	var envVars []EnvVar

	for _, key := range keys {
		envVars = append(envVars, EnvVar{
			Name: key,
			ValueFrom: &EnvVarSource{
				ConfigMapKeyRef: &ConfigMapKeyRef{
					Name: configMapName,
					Key:  key,
				},
			},
		})
	}

	return envVars
}

// EnvVar represents an environment variable
type EnvVar struct {
	Name      string
	Value     string
	ValueFrom *EnvVarSource
}

// EnvVarSource represents the source of an environment variable value
type EnvVarSource struct {
	SecretKeyRef    *SecretKeyRef
	ConfigMapKeyRef *ConfigMapKeyRef
}

// SecretKeyRef references a key in a Secret
type SecretKeyRef struct {
	Name string
	Key  string
}

// ConfigMapKeyRef references a key in a ConfigMap
type ConfigMapKeyRef struct {
	Name string
	Key  string
}
