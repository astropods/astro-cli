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
// ConfigMap, and secret variable references become secret key refs.
type ResolvedEnv struct {
	// ConfigMap entries (plain key=value, including resolved service URLs)
	ConfigMapData map[string]string
	// SecretData entries (variable key → actual value for the k8s Secret)
	SecretData map[string]string
}

// ResolveDeploymentSpecEnv resolves all ${} references in the deployment spec's
// environment maps into concrete k8s values (service DNS, variable values, etc).
// It also builds the standard connection-string ConfigMap and secret data.
func ResolveDeploymentSpecEnv(ds *spec.AstroDeploymentSpec, rctx ResolveContext) *ResolvedEnv {
	result := &ResolvedEnv{
		ConfigMapData: make(map[string]string),
		SecretData:    make(map[string]string),
	}

	// Collect all secret variable values into secret data
	for key, v := range ds.Variables {
		if v.Secret && v.Value != "" {
			result.SecretData[strings.ToUpper(key)] = v.Value
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
	agentPort := spec.PrimaryPort(ds.Agent.Endpoints)
	if agentPort == 0 {
		agentPort = 8080
	}
	result.ConfigMapData["AGENT_URL"] = fmt.Sprintf("http://%s:%d", agentHost, agentPort)
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
		// Prefer "grpc" endpoint; fall back to primary port; default 9090
		grpcPort := 0
		if ep := spec.EndpointByName(ds.Interfaces.Endpoints, "grpc"); ep != nil {
			grpcPort = ep.Port
		}
		if grpcPort == 0 {
			grpcPort = spec.PrimaryPort(ds.Interfaces.Endpoints)
		}
		if grpcPort == 0 {
			grpcPort = 9090
		}
		messagingServiceName := GenerateAgentResourceName(ds.Source.Name, "messaging")
		messagingHost := GenerateServiceDNS(messagingServiceName, rctx.Namespace)
		result.ConfigMapData["GRPC_SERVER_ADDR"] = fmt.Sprintf("%s:%d", messagingHost, grpcPort)
	}

	return result
}

// componentEndpointInfo holds the port and protocol for a single named endpoint.
type componentEndpointInfo struct {
	Port      int
	Protocol  string
}

// componentInfo holds the resolved DNS and per-endpoint info for a deployed component.
type componentInfo struct {
	Host      string
	Endpoints map[string]componentEndpointInfo
	URLScheme string // provider-specific URL scheme (e.g. "redis", "bolt")
}

// buildComponentLookup builds a map of component references to their resolved DNS/ports.
func buildComponentLookup(ds *spec.AstroDeploymentSpec, rctx ResolveContext) map[string]componentInfo {
	lookup := make(map[string]componentInfo)

	for name, model := range ds.Models {
		resourceName := GenerateResourceName(ds.Source.Name, "model", name)
		host := GenerateServiceDNS(resourceName, rctx.Namespace)
		eps := endpointInfoMap(model.Endpoints)
		lookup["models."+name] = componentInfo{Host: host, Endpoints: eps}
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
		eps := endpointInfoMap(knowledge.Endpoints)
		lookup["knowledge."+name] = componentInfo{Host: host, Endpoints: eps, URLScheme: urlScheme}
	}

	for name, tool := range ds.Tools {
		resourceName := GenerateResourceName(ds.Source.Name, "tool", name)
		host := GenerateServiceDNS(resourceName, rctx.Namespace)
		eps := endpointInfoMap(tool.Endpoints)
		lookup["tools."+name] = componentInfo{Host: host, Endpoints: eps}
	}

	return lookup
}

func endpointInfoMap(endpoints map[string]spec.Endpoint) map[string]componentEndpointInfo {
	m := make(map[string]componentEndpointInfo, len(endpoints))
	for name, ep := range endpoints {
		m[name] = componentEndpointInfo{Port: ep.Port, Protocol: ep.Protocol}
	}
	return m
}

// resolveEnvMap resolves ${} references in an environment map and adds the
// resolved values to the result's ConfigMapData (or SecretData for secrets).
func resolveEnvMap(
	env map[string]string,
	lookup map[string]componentInfo,
	ds *spec.AstroDeploymentSpec,
	rctx ResolveContext,
	result *ResolvedEnv,
) {
	for key, value := range env {
		resolved := resolveValue(value, lookup, ds, rctx)
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
			if ref.Attribute == "host" {
				// 3-part host ref
				replacement = info.Host
			} else if ref.Endpoint != "" {
				// 4-part endpoint ref: section.name.endpoint.attr
				ep, epOK := info.Endpoints[ref.Endpoint]
				if !epOK {
					continue
				}
				switch ref.Attribute {
				case "port":
					replacement = fmt.Sprintf("%d", ep.Port)
				case "url":
					scheme := ep.Protocol
					if scheme == "" || scheme == "tcp" || scheme == "grpc" {
						// Use provider URL scheme for non-HTTP protocols
						if info.URLScheme != "" && info.URLScheme != "http" {
							scheme = info.URLScheme
						} else {
							scheme = "http"
						}
					}
					if info.URLScheme != "" && info.URLScheme != "http" {
						scheme = info.URLScheme
					}
					replacement = fmt.Sprintf("%s://%s:%d", scheme, info.Host, ep.Port)
				}
			}

		case spec.RefVariable:
			if v, ok := ds.Variables[ref.Name]; ok {
				replacement = v.Value
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
