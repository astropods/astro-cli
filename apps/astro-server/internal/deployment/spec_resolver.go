package deployment

import (
	"fmt"
	"strings"

	"github.com/postman/astro/packages/astro-spec"
)

// ResolveContext provides the runtime context needed to resolve ${} references
// in a deployment spec's environment maps into concrete values.
type ResolveContext struct {
	Namespace  string
	AgentName  string
	BuildID    string
	SecretName string
}

// ResolvedEnv holds the resolved environment: plain key-value pairs go into the
// ConfigMap, and credential references become secret key refs.
type ResolvedEnv struct {
	// ConfigMap entries (plain key=value, including resolved service URLs)
	ConfigMapData map[string]string
	// SecretData entries (credential key → actual value for the k8s Secret)
	SecretData map[string]string
}

// ResolveDeploymentSpecEnv resolves all ${} references in the deployment spec's
// environment maps into concrete k8s values (service DNS, credential values, etc).
// It also builds the standard connection-string ConfigMap and credential Secret data.
func ResolveDeploymentSpecEnv(ds *spec.AstroDeploymentSpec, rctx ResolveContext) *ResolvedEnv {
	result := &ResolvedEnv{
		ConfigMapData: make(map[string]string),
		SecretData:    make(map[string]string),
	}

	// Collect all credential values into secret data
	for key, cred := range ds.Credentials {
		if cred.Value != "" {
			result.SecretData[strings.ToUpper(key)] = cred.Value
		}
	}

	// Build a lookup of service DNS names and ports for each component
	componentLookup := buildComponentLookup(ds, rctx)

	// Resolve agent environment
	resolveEnvMap(ds.Agent.Environment, componentLookup, ds, rctx, result)

	// Resolve interface environment
	if ds.Interfaces != nil {
		resolveEnvMap(ds.Interfaces.Environment, componentLookup, ds, rctx, result)
	}

	// Resolve ingestion environments
	for _, ing := range ds.Ingestion {
		resolveEnvMap(ing.Environment, componentLookup, ds, rctx, result)
	}

	// Add standard platform vars
	result.ConfigMapData["ASTRO_AGENT_NAME"] = ds.Source.Name
	if ds.Source.Build != "" {
		result.ConfigMapData["ASTRO_AGENT_BUILD"] = ds.Source.Build
	}

	// Add agent's own URL
	agentServiceName := GenerateAgentResourceName(ds.Source.Name, "agent")
	agentHost := GenerateServiceDNS(agentServiceName, rctx.Namespace)
	result.ConfigMapData["AGENT_URL"] = fmt.Sprintf("http://%s:%d", agentHost, ds.Agent.Port)
	result.ConfigMapData["AGENT_HOST"] = agentHost

	// Add OTel collector endpoint
	collectorServiceName := GenerateAgentResourceName(ds.Source.Name, "collector")
	collectorHost := GenerateServiceDNS(collectorServiceName, rctx.Namespace)
	otlpPort := ds.Observability.Port
	if otlpPort == 0 {
		otlpPort = 4318
	}
	result.ConfigMapData["OTEL_EXPORTER_OTLP_ENDPOINT"] = fmt.Sprintf("http://%s:%d", collectorHost, otlpPort)

	// Resolve observability environment
	if len(ds.Observability.Environment) > 0 {
		resolveEnvMap(ds.Observability.Environment, componentLookup, ds, rctx, result)
	}

	// Add messaging gRPC address if interfaces are configured
	if ds.Interfaces != nil && len(ds.Interfaces.Adapters) > 0 {
		grpcPort := ds.Interfaces.Port
		if grpcPort == 0 {
			grpcPort = 9090
		}
		messagingServiceName := GenerateAgentResourceName(ds.Source.Name, "messaging")
		messagingHost := GenerateServiceDNS(messagingServiceName, rctx.Namespace)
		result.ConfigMapData["GRPC_SERVER_ADDR"] = fmt.Sprintf("%s:%d", messagingHost, grpcPort)
	}

	return result
}

// componentInfo holds the resolved DNS and port for a deployed component.
type componentInfo struct {
	Host      string
	Port      int
	URLScheme string // e.g. "http", "redis", "bolt" — defaults to "http"
}

// buildComponentLookup builds a map of component references to their resolved DNS/port.
func buildComponentLookup(ds *spec.AstroDeploymentSpec, rctx ResolveContext) map[string]componentInfo {
	lookup := make(map[string]componentInfo)

	for name, model := range ds.Models {
		resourceName := GenerateResourceName(ds.Source.Name, "model", name)
		host := GenerateServiceDNS(resourceName, rctx.Namespace)
		lookup["models."+name] = componentInfo{Host: host, Port: model.Port}
	}

	for name, knowledge := range ds.Knowledge {
		resourceName := GenerateResourceName(ds.Source.Name, "knowledge", name)
		host := GenerateServiceDNS(resourceName, rctx.Namespace)
		urlScheme := "http"
		if knowledge.Provider != "" {
			if prov := spec.GetProvider(knowledge.Provider); prov.URLScheme != "" {
				urlScheme = prov.URLScheme
			}
		}
		lookup["knowledge."+name] = componentInfo{Host: host, Port: knowledge.Port, URLScheme: urlScheme}
	}

	for name, tool := range ds.Tools {
		resourceName := GenerateResourceName(ds.Source.Name, "tool", name)
		host := GenerateServiceDNS(resourceName, rctx.Namespace)
		lookup["tools."+name] = componentInfo{Host: host, Port: tool.Port}
	}

	return lookup
}

// resolveEnvMap resolves ${} references in an environment map and adds the
// resolved values to the result's ConfigMapData (or SecretData for credentials).
func resolveEnvMap(
	env map[string]string,
	lookup map[string]componentInfo,
	ds *spec.AstroDeploymentSpec,
	rctx ResolveContext,
	result *ResolvedEnv,
) {
	for key, value := range env {
		resolved := resolveValue(value, lookup, ds, rctx)
		// Credential references go into ConfigMap pointing at secret keys,
		// but the actual values are already in SecretData.
		// For simplicity, all resolved env vars go into ConfigMap.
		result.ConfigMapData[key] = resolved
	}
}

// resolveValue resolves a single value that may contain ${} references.
func resolveValue(value string, lookup map[string]componentInfo, ds *spec.AstroDeploymentSpec, rctx ResolveContext) string {
	refs := spec.ParseReferences(value)
	if len(refs) == 0 {
		return value
	}

	resolved := value
	for _, ref := range refs {
		var replacement string

		switch ref.Kind {
		case spec.RefModel, spec.RefKnowledge, spec.RefTool:
			prefix := string(ref.Kind) // "models", "knowledge", "tools"
			key := prefix + "." + ref.Name
			info, ok := lookup[key]
			if !ok {
				continue
			}
			switch ref.Attribute {
			case "host":
				replacement = info.Host
			case "port":
				replacement = fmt.Sprintf("%d", info.Port)
			case "url":
				scheme := info.URLScheme
				if scheme == "" {
					scheme = "http"
				}
				replacement = fmt.Sprintf("%s://%s:%d", scheme, info.Host, info.Port)
			}

		case spec.RefCredential:
			// Credential refs resolve to the credential key name (uppercase).
			// The actual value is injected via envFrom on the k8s Secret.
			// But for inline references like "Bearer ${credentials.API_KEY}",
			// we need the actual value.
			if cred, ok := ds.Credentials[ref.Name]; ok {
				replacement = cred.Value
			}

		case spec.RefSource:
			switch ref.Name {
			case "name":
				replacement = ds.Source.Name
			case "build":
				replacement = ds.Source.Build
			case "account":
				replacement = ds.Source.Account
			case "registry":
				replacement = ds.Source.Registry
			}
		}

		if replacement != "" {
			resolved = strings.Replace(resolved, ref.Raw, replacement, 1)
		}
	}

	return resolved
}
