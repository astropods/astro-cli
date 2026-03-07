package deployment

import (
	"strings"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

func httpEndpoints(port int) map[string]spec.Endpoint {
	return map[string]spec.Endpoint{"http": {Port: port}}
}

func TestResolveDeploymentSpecEnv_ModelReferences(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "my-agent", Build: "abc123"},
		Target: spec.DeploymentTarget{Namespace: "prod"},
		Agent: spec.DeploymentAgent{
			Image:     "agent:latest",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"LLM_URL":  "${models.llm.http.url}",
				"LLM_HOST": "${models.llm.host}",
				"LLM_PORT": "${models.llm.http.port}",
			},
		},
		Models: map[string]spec.DeploymentModel{
			"llm": {Image: "ollama:latest", Endpoints: httpEndpoints(11434)},
		},
	}

	rctx := ResolveContext{Namespace: "prod", AgentName: "my-agent", BuildID: "abc123"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	// LLM_HOST should be the service DNS name
	llmHost := result.ConfigMapData["LLM_HOST"]
	if !strings.Contains(llmHost, "my-agent-model-llm") {
		t.Errorf("LLM_HOST: expected service DNS containing 'my-agent-model-llm', got %s", llmHost)
	}
	if !strings.HasSuffix(llmHost, ".prod.svc.cluster.local") {
		t.Errorf("LLM_HOST: expected .prod.svc.cluster.local suffix, got %s", llmHost)
	}

	// LLM_PORT should be 11434
	if result.ConfigMapData["LLM_PORT"] != "11434" {
		t.Errorf("LLM_PORT: expected 11434, got %s", result.ConfigMapData["LLM_PORT"])
	}

	// LLM_URL should be http://host:port
	llmURL := result.ConfigMapData["LLM_URL"]
	if !strings.HasPrefix(llmURL, "http://") {
		t.Errorf("LLM_URL: expected http:// prefix, got %s", llmURL)
	}
	if !strings.Contains(llmURL, ":11434") {
		t.Errorf("LLM_URL: expected :11434, got %s", llmURL)
	}
}

func TestResolveDeploymentSpecEnv_KnowledgeReferences(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"QDRANT_HOST": "${knowledge.docs.host}",
				"QDRANT_PORT": "${knowledge.docs.http.port}",
			},
		},
		Knowledge: map[string]spec.DeploymentKnowledge{
			"docs": {Image: "qdrant:latest", Endpoints: httpEndpoints(6333)},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	host := result.ConfigMapData["QDRANT_HOST"]
	if !strings.Contains(host, "agent-knowledge-docs") {
		t.Errorf("QDRANT_HOST: expected agent-knowledge-docs, got %s", host)
	}
	if result.ConfigMapData["QDRANT_PORT"] != "6333" {
		t.Errorf("QDRANT_PORT: expected 6333, got %s", result.ConfigMapData["QDRANT_PORT"])
	}
}

func TestResolveDeploymentSpecEnv_ToolReferences(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"SEARCH_URL": "${tools.search.http.url}",
			},
		},
		Tools: map[string]spec.DeploymentTool{
			"search": {Image: "search:latest", Endpoints: httpEndpoints(3000)},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	url := result.ConfigMapData["SEARCH_URL"]
	if !strings.Contains(url, "agent-tool-search") {
		t.Errorf("SEARCH_URL: expected agent-tool-search, got %s", url)
	}
	if !strings.Contains(url, ":3000") {
		t.Errorf("SEARCH_URL: expected :3000, got %s", url)
	}
}

func TestResolveDeploymentSpecEnv_VariableReferences(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"API_KEY": "${variables.ANTHROPIC_API_KEY}",
			},
		},
		Variables: map[string]spec.Variable{
			"ANTHROPIC_API_KEY": {Value: "sk-secret-123", Secret: true},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	// Variable ref in env should resolve to the actual value
	if result.ConfigMapData["API_KEY"] != "sk-secret-123" {
		t.Errorf("API_KEY: expected sk-secret-123, got %s", result.ConfigMapData["API_KEY"])
	}

	// Secret variable should also be in SecretData
	if result.SecretData["ANTHROPIC_API_KEY"] != "sk-secret-123" {
		t.Errorf("SecretData: expected sk-secret-123, got %s", result.SecretData["ANTHROPIC_API_KEY"])
	}
}

func TestResolveDeploymentSpecEnv_SourceReferences(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "my-agent", Build: "abc123", Account: "acme"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"AGENT_NAME": "${source.name}",
				"BUILD_ID":   "${source.build}",
				"ACCOUNT":    "${source.account}",
			},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "my-agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	if result.ConfigMapData["AGENT_NAME"] != "my-agent" {
		t.Errorf("AGENT_NAME: expected my-agent, got %s", result.ConfigMapData["AGENT_NAME"])
	}
	if result.ConfigMapData["BUILD_ID"] != "abc123" {
		t.Errorf("BUILD_ID: expected abc123, got %s", result.ConfigMapData["BUILD_ID"])
	}
	if result.ConfigMapData["ACCOUNT"] != "acme" {
		t.Errorf("ACCOUNT: expected acme, got %s", result.ConfigMapData["ACCOUNT"])
	}
}

func TestResolveDeploymentSpecEnv_CompositeReferences(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"COMBINED": "http://${models.llm.host}:${models.llm.http.port}/v1",
			},
		},
		Models: map[string]spec.DeploymentModel{
			"llm": {Image: "ollama:latest", Endpoints: httpEndpoints(11434)},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	combined := result.ConfigMapData["COMBINED"]
	if !strings.HasPrefix(combined, "http://") {
		t.Errorf("COMBINED: expected http:// prefix, got %s", combined)
	}
	if !strings.Contains(combined, ":11434/v1") {
		t.Errorf("COMBINED: expected :11434/v1, got %s", combined)
	}
}

func TestResolveDeploymentSpecEnv_PlainValues(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"LOG_LEVEL": "debug",
				"MAX_RETRY": "3",
			},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	if result.ConfigMapData["LOG_LEVEL"] != "debug" {
		t.Errorf("LOG_LEVEL: expected debug, got %s", result.ConfigMapData["LOG_LEVEL"])
	}
	if result.ConfigMapData["MAX_RETRY"] != "3" {
		t.Errorf("MAX_RETRY: expected 3, got %s", result.ConfigMapData["MAX_RETRY"])
	}
}

func TestResolveDeploymentSpecEnv_PlatformVars(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "my-agent", Build: "abc123"},
		Target: spec.DeploymentTarget{Namespace: "prod"},
		Agent:  spec.DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
	}

	rctx := ResolveContext{Namespace: "prod", AgentName: "my-agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	if result.ConfigMapData["ASTRO_AGENT_NAME"] != "my-agent" {
		t.Errorf("ASTRO_AGENT_NAME: expected my-agent, got %s", result.ConfigMapData["ASTRO_AGENT_NAME"])
	}
	if result.ConfigMapData["ASTRO_AGENT_BUILD"] != "abc123" {
		t.Errorf("ASTRO_AGENT_BUILD: expected abc123, got %s", result.ConfigMapData["ASTRO_AGENT_BUILD"])
	}

	// Should have AGENT_URL
	if !strings.HasPrefix(result.ConfigMapData["AGENT_URL"], "http://") {
		t.Error("AGENT_URL: expected http:// prefix")
	}

	// Should have OTEL endpoint
	if !strings.Contains(result.ConfigMapData["OTEL_EXPORTER_OTLP_ENDPOINT"], "4318") {
		t.Error("OTEL_EXPORTER_OTLP_ENDPOINT: expected port 4318")
	}
}

func TestResolveDeploymentSpecEnv_OTELCustomPort(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent:  spec.DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Observability: spec.DeploymentObservability{
			Enabled: true,
			Port:    5318,
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	otelEndpoint := result.ConfigMapData["OTEL_EXPORTER_OTLP_ENDPOINT"]
	if !strings.Contains(otelEndpoint, ":5318") {
		t.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT: expected :5318, got %s", otelEndpoint)
	}
}

func TestResolveDeploymentSpecEnv_OTELDefaultPort(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent:  spec.DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Observability: spec.DeploymentObservability{
			Enabled: true,
			// Port: 0 — should default to 4318
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	otelEndpoint := result.ConfigMapData["OTEL_EXPORTER_OTLP_ENDPOINT"]
	if !strings.Contains(otelEndpoint, ":4318") {
		t.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT: expected default :4318, got %s", otelEndpoint)
	}
}

func TestResolveDeploymentSpecEnv_ObservabilityEnv(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent:  spec.DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Observability: spec.DeploymentObservability{
			Enabled: true,
			Environment: map[string]string{
				"CUSTOM_COLLECTOR_VAR": "some-value",
			},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	if result.ConfigMapData["CUSTOM_COLLECTOR_VAR"] != "some-value" {
		t.Errorf("CUSTOM_COLLECTOR_VAR: expected some-value, got %s", result.ConfigMapData["CUSTOM_COLLECTOR_VAR"])
	}
}

func TestResolveDeploymentSpecEnv_InterfacesGRPCAddr(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent:  spec.DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Interfaces: &spec.DeploymentInterfaces{
			Adapters: []string{"slack", "web"},
			Image:    "messaging:latest",
			Endpoints: map[string]spec.Endpoint{
				"grpc": {Port: 9090, Protocol: "grpc"},
				"http": {Port: 8080, Expose: &spec.EndpointExpose{Enabled: true}},
			},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	grpcAddr := result.ConfigMapData["GRPC_SERVER_ADDR"]
	if !strings.Contains(grpcAddr, ":9090") {
		t.Errorf("GRPC_SERVER_ADDR: expected :9090, got %s", grpcAddr)
	}
}

func TestResolveDeploymentSpecEnv_InterfacesCustomPort(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent:  spec.DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Interfaces: &spec.DeploymentInterfaces{
			Adapters: []string{"slack"},
			Image:    "messaging:latest",
			Endpoints: map[string]spec.Endpoint{
				"grpc": {Port: 7070, Protocol: "grpc"},
			},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	grpcAddr := result.ConfigMapData["GRPC_SERVER_ADDR"]
	if !strings.Contains(grpcAddr, ":7070") {
		t.Errorf("GRPC_SERVER_ADDR: expected :7070, got %s", grpcAddr)
	}
}

func TestResolveDeploymentSpecEnv_InterfacesDefaultPort(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent:  spec.DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Interfaces: &spec.DeploymentInterfaces{
			Adapters: []string{"slack"},
			Image:    "messaging:latest",
			// No endpoints — should default to 9090
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	grpcAddr := result.ConfigMapData["GRPC_SERVER_ADDR"]
	if !strings.Contains(grpcAddr, ":9090") {
		t.Errorf("GRPC_SERVER_ADDR: expected default :9090, got %s", grpcAddr)
	}
}

func TestResolveDeploymentSpecEnv_InterfaceEnvVariableRefs(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent:  spec.DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Interfaces: &spec.DeploymentInterfaces{
			Adapters: []string{"slack"},
			Image:    "messaging:latest",
			Endpoints: map[string]spec.Endpoint{
				"grpc": {Port: 9090, Protocol: "grpc"},
			},
			Environment: map[string]string{
				"SLACK_BOT_TOKEN": "${variables.SLACK_BOT_TOKEN}",
				"SLACK_APP_TOKEN": "${variables.SLACK_APP_TOKEN}",
			},
		},
		Variables: map[string]spec.Variable{
			"SLACK_BOT_TOKEN": {Value: "xoxb-test-token", Secret: true},
			"SLACK_APP_TOKEN": {Value: "xapp-test-token", Secret: true},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	// Interface env variable refs should be resolved in ConfigMapData
	if result.ConfigMapData["SLACK_BOT_TOKEN"] != "xoxb-test-token" {
		t.Errorf("SLACK_BOT_TOKEN: expected xoxb-test-token, got %s", result.ConfigMapData["SLACK_BOT_TOKEN"])
	}
	if result.ConfigMapData["SLACK_APP_TOKEN"] != "xapp-test-token" {
		t.Errorf("SLACK_APP_TOKEN: expected xapp-test-token, got %s", result.ConfigMapData["SLACK_APP_TOKEN"])
	}

	// Secret variable values should also be in SecretData
	if result.SecretData["SLACK_BOT_TOKEN"] != "xoxb-test-token" {
		t.Errorf("SecretData SLACK_BOT_TOKEN: expected xoxb-test-token, got %s", result.SecretData["SLACK_BOT_TOKEN"])
	}
}

func TestResolveDeploymentSpecEnv_JiraIntegrationSecrets(t *testing.T) {
	// Verifies that Jira-style secret variables are correctly resolved into
	// SecretData and that ${variables.*} references in the agent environment
	// resolve to the actual values.
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"JIRA_API_KEY":  "${variables.JIRA_API_KEY}",
				"JIRA_BASE_URL": "${variables.JIRA_BASE_URL}",
				"JIRA_EMAIL":    "${variables.JIRA_EMAIL}",
			},
		},
		Variables: map[string]spec.Variable{
			"JIRA_API_KEY":  {Value: "jira-token-abc", Secret: true},
			"JIRA_BASE_URL": {Value: "https://myorg.atlassian.net", Secret: true},
			"JIRA_EMAIL":    {Value: "bot@myorg.com", Secret: true},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	// Variable references in agent env should resolve to concrete values.
	if result.ConfigMapData["JIRA_API_KEY"] != "jira-token-abc" {
		t.Errorf("JIRA_API_KEY: expected jira-token-abc, got %s", result.ConfigMapData["JIRA_API_KEY"])
	}
	if result.ConfigMapData["JIRA_BASE_URL"] != "https://myorg.atlassian.net" {
		t.Errorf("JIRA_BASE_URL: expected https://myorg.atlassian.net, got %s", result.ConfigMapData["JIRA_BASE_URL"])
	}
	if result.ConfigMapData["JIRA_EMAIL"] != "bot@myorg.com" {
		t.Errorf("JIRA_EMAIL: expected bot@myorg.com, got %s", result.ConfigMapData["JIRA_EMAIL"])
	}

	// All three must appear in SecretData (since secret=true).
	for _, key := range []string{"JIRA_API_KEY", "JIRA_BASE_URL", "JIRA_EMAIL"} {
		if _, ok := result.SecretData[key]; !ok {
			t.Errorf("SecretData: expected %s to be present", key)
		}
	}
	if result.SecretData["JIRA_API_KEY"] != "jira-token-abc" {
		t.Errorf("SecretData JIRA_API_KEY: expected jira-token-abc, got %s", result.SecretData["JIRA_API_KEY"])
	}
}

func TestResolveDeploymentSpecEnv_EmptyVariables(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{Namespace: "ns"},
		Agent:  spec.DeploymentAgent{Image: "x", Endpoints: httpEndpoints(8080)},
		Variables: map[string]spec.Variable{
			"EMPTY_KEY": {Value: "", Secret: true},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	// Empty secret variables should NOT be in secret data
	if _, ok := result.SecretData["EMPTY_KEY"]; ok {
		t.Error("empty secret variable should not be in SecretData")
	}
}
