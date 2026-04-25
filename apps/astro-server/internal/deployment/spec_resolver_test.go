package deployment

import (
	"encoding/json"
	"sort"
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
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"SEARCH_URL": "${integrations.search.http.url}",
			},
		},
		Integrations: map[string]spec.DeploymentIntegration{
			"search": {Image: "search:latest", Endpoints: httpEndpoints(3000)},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	url := result.ConfigMapData["SEARCH_URL"]
	if !strings.Contains(url, "agent-integration-search") {
		t.Errorf("SEARCH_URL: expected agent-integration-search, got %s", url)
	}
	if !strings.Contains(url, ":3000") {
		t.Errorf("SEARCH_URL: expected :3000, got %s", url)
	}
}

func TestResolveDeploymentSpecEnv_VariableReferences(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{},
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

	// Variable ref in env referencing a secret should be in SecretData, not ConfigMapData
	if result.SecretData["API_KEY"] != "sk-secret-123" {
		t.Errorf("SecretData API_KEY: expected sk-secret-123, got %s", result.SecretData["API_KEY"])
	}
	if _, inCM := result.ConfigMapData["API_KEY"]; inCM {
		t.Error("API_KEY should not be in ConfigMapData when it references a secret variable")
	}

	// Secret variable itself should also be in SecretData (from phase 1)
	if result.SecretData["ANTHROPIC_API_KEY"] != "sk-secret-123" {
		t.Errorf("SecretData: expected sk-secret-123, got %s", result.SecretData["ANTHROPIC_API_KEY"])
	}
}

func TestResolveDeploymentSpecEnv_SourceReferences(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "my-agent", Build: "abc123", Account: "acme"},
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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

	// Should have ASTRO_AGENT_URL
	if !strings.HasPrefix(result.ConfigMapData["ASTRO_AGENT_URL"], "http://") {
		t.Error("ASTRO_AGENT_URL: expected http:// prefix")
	}

	// Should have OTEL endpoint
	if !strings.Contains(result.ConfigMapData["OTEL_EXPORTER_OTLP_ENDPOINT"], "4318") {
		t.Error("OTEL_EXPORTER_OTLP_ENDPOINT: expected port 4318")
	}
}

func TestResolveDeploymentSpecEnv_OTELCustomPort(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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
		Target: spec.DeploymentTarget{},
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

	// Interface env variable refs to secrets should be in SecretData, not ConfigMapData
	if result.SecretData["SLACK_BOT_TOKEN"] != "xoxb-test-token" {
		t.Errorf("SecretData SLACK_BOT_TOKEN: expected xoxb-test-token, got %s", result.SecretData["SLACK_BOT_TOKEN"])
	}
	if result.SecretData["SLACK_APP_TOKEN"] != "xapp-test-token" {
		t.Errorf("SecretData SLACK_APP_TOKEN: expected xapp-test-token, got %s", result.SecretData["SLACK_APP_TOKEN"])
	}
	if _, inCM := result.ConfigMapData["SLACK_BOT_TOKEN"]; inCM {
		t.Error("SLACK_BOT_TOKEN should not be in ConfigMapData")
	}
	if _, inCM := result.ConfigMapData["SLACK_APP_TOKEN"]; inCM {
		t.Error("SLACK_APP_TOKEN should not be in ConfigMapData")
	}
}

func TestResolveDeploymentSpecEnv_JiraIntegrationSecrets(t *testing.T) {
	// Verifies that Jira-style secret variables are correctly resolved into
	// SecretData and that ${variables.*} references in the agent environment
	// resolve to the actual values.
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{},
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

	// Variable references to secrets should resolve to SecretData, not ConfigMapData.
	if result.SecretData["JIRA_API_KEY"] != "jira-token-abc" {
		t.Errorf("SecretData JIRA_API_KEY: expected jira-token-abc, got %s", result.SecretData["JIRA_API_KEY"])
	}
	if result.SecretData["JIRA_BASE_URL"] != "https://myorg.atlassian.net" {
		t.Errorf("SecretData JIRA_BASE_URL: expected https://myorg.atlassian.net, got %s", result.SecretData["JIRA_BASE_URL"])
	}
	if result.SecretData["JIRA_EMAIL"] != "bot@myorg.com" {
		t.Errorf("SecretData JIRA_EMAIL: expected bot@myorg.com, got %s", result.SecretData["JIRA_EMAIL"])
	}
	// Should NOT be in ConfigMapData
	for _, key := range []string{"JIRA_API_KEY", "JIRA_BASE_URL", "JIRA_EMAIL"} {
		if _, inCM := result.ConfigMapData[key]; inCM {
			t.Errorf("%s should not be in ConfigMapData", key)
		}
	}
}

func TestResolveDeploymentSpecEnv_CompositeSecretRef(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"AUTH_HEADER": "Bearer ${variables.API_KEY}",
			},
		},
		Variables: map[string]spec.Variable{
			"API_KEY": {Value: "sk-secret-123", Secret: true},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	// Composite value containing a secret ref should go to SecretData
	if result.SecretData["AUTH_HEADER"] != "Bearer sk-secret-123" {
		t.Errorf("SecretData AUTH_HEADER: expected 'Bearer sk-secret-123', got %s", result.SecretData["AUTH_HEADER"])
	}
	if _, inCM := result.ConfigMapData["AUTH_HEADER"]; inCM {
		t.Error("AUTH_HEADER should not be in ConfigMapData")
	}
}

func TestResolveDeploymentSpecEnv_HardcodedValueMatchingSecretKey(t *testing.T) {
	// When a hardcoded env key matches a secret variable key (collected in phase 1),
	// the hardcoded value should stay in SecretData (not duplicate into ConfigMap).
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"API_KEY": "hardcoded-override",
			},
		},
		Variables: map[string]spec.Variable{
			"API_KEY": {Value: "sk-secret-123", Secret: true},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	// The env key "API_KEY" already exists in SecretData from phase 1,
	// so the hardcoded override should also go to SecretData.
	if result.SecretData["API_KEY"] != "hardcoded-override" {
		t.Errorf("SecretData API_KEY: expected hardcoded-override, got %s", result.SecretData["API_KEY"])
	}
	if _, inCM := result.ConfigMapData["API_KEY"]; inCM {
		t.Error("API_KEY should not be in ConfigMapData when key exists in SecretData")
	}
}

func TestResolveDeploymentSpecEnv_EmptyVariables(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{},
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

func TestResolveDeploymentSpecEnv_EmptyNonSecretVariableResolvesToEmptyString(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"SLACK_CONFIG": "${variables.SLACK_CONFIG}",
			},
		},
		Variables: map[string]spec.Variable{
			"SLACK_CONFIG": {Value: "", Secret: false},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	if got := result.ConfigMapData["SLACK_CONFIG"]; got != "" {
		t.Errorf("SLACK_CONFIG: expected empty string, got %q", got)
	}
}

func TestResolveDeploymentSpecEnv_StrippedSecretRouting(t *testing.T) {
	// Stripped spec (empty secret values): env keys referencing secret variables
	// should still route to SecretData so backfill/repair key sets are correct.
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Target: spec.DeploymentTarget{},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"API_KEY":     "${variables.API_KEY}",
				"AUTH_HEADER": "Bearer ${variables.API_KEY}",
				"LOG_LEVEL":   "debug",
				"APP_NAME":    "${variables.APP_NAME}",
			},
		},
		Variables: map[string]spec.Variable{
			"API_KEY":  {Value: "", Secret: true}, // stripped
			"APP_NAME": {Value: "my-app", Secret: false},
		},
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	// Secret-referencing keys go to SecretData even with empty values
	if _, ok := result.SecretData["API_KEY"]; !ok {
		t.Error("expected API_KEY in SecretData")
	}
	if _, ok := result.ConfigMapData["API_KEY"]; ok {
		t.Error("API_KEY should not be in ConfigMapData")
	}
	if _, ok := result.SecretData["AUTH_HEADER"]; !ok {
		t.Error("expected AUTH_HEADER in SecretData")
	}

	// Non-secret keys stay in ConfigMapData
	if result.ConfigMapData["LOG_LEVEL"] != "debug" {
		t.Errorf("LOG_LEVEL: expected debug, got %s", result.ConfigMapData["LOG_LEVEL"])
	}
	if result.ConfigMapData["APP_NAME"] != "my-app" {
		t.Errorf("APP_NAME: expected my-app, got %s", result.ConfigMapData["APP_NAME"])
	}

	// HasSecretValues should be false (all secret values are empty/unresolved)
	if result.HasSecretValues() {
		t.Error("HasSecretValues should be false for stripped spec")
	}
}

// Regression test: backfill resolves a stripped spec and must produce the same
// key sets (which keys are in ConfigMap vs Secret) as a fresh deploy with full
// values. Before the fix, stripped specs routed secret-referencing env keys to
// ConfigMapData because referencesSecret required v.Value != "".
func TestResolveDeploymentSpecEnv_BackfillKeySetMatchesFreshDeploy(t *testing.T) {
	makeSpec := func(secretValue string) *spec.AstroDeploymentSpec {
		return &spec.AstroDeploymentSpec{
			Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
			Target: spec.DeploymentTarget{},
			Agent: spec.DeploymentAgent{
				Image:     "x",
				Endpoints: httpEndpoints(8080),
				Environment: map[string]string{
					"API_KEY":     "${variables.API_KEY}",
					"AUTH_HEADER": "Bearer ${variables.API_KEY}",
					"LOG_LEVEL":   "debug",
					"APP_NAME":    "${variables.APP_NAME}",
				},
			},
			Variables: map[string]spec.Variable{
				"API_KEY":  {Value: secretValue, Secret: true},
				"APP_NAME": {Value: "my-app", Secret: false},
			},
		}
	}

	rctx := ResolveContext{Namespace: "ns", AgentName: "agent"}

	// Fresh deploy: secret has a real value
	fresh := ResolveDeploymentSpecEnv(makeSpec("sk-secret-123"), rctx)

	// Backfill: secret value stripped (empty)
	stripped := ResolveDeploymentSpecEnv(makeSpec(""), rctx)

	// Key sets must match: same keys in ConfigMapData, same keys in SecretData
	freshCMKeys := sortedKeys(fresh.ConfigMapData)
	strippedCMKeys := sortedKeys(stripped.ConfigMapData)
	if !equalSlices(freshCMKeys, strippedCMKeys) {
		t.Errorf("ConfigMap key mismatch:\n  fresh:    %v\n  stripped: %v", freshCMKeys, strippedCMKeys)
	}

	freshSecKeys := sortedKeys(fresh.SecretData)
	strippedSecKeys := sortedKeys(stripped.SecretData)
	if !equalSlices(freshSecKeys, strippedSecKeys) {
		t.Errorf("Secret key mismatch:\n  fresh:    %v\n  stripped: %v", freshSecKeys, strippedSecKeys)
	}

	// Fresh deploy should have real values → HasSecretValues true
	if !fresh.HasSecretValues() {
		t.Error("fresh deploy should have secret values")
	}
	// Stripped spec should not → HasSecretValues false
	if stripped.HasSecretValues() {
		t.Error("stripped spec should not have secret values")
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// supportSignalSpecJSON is the exact package/v1 spec from the support-signal-agent
// deployment that triggered false drift (secret keys appearing in configmap expected keys).
const supportSignalSpecJSON = `{"agent":{"image":"registry.astropods.ai/zain/support-signal-agent:8f406458"},"dev":{"interfaces":["web","slack"]},"ingestion":{"sync":{"container":{"image":"registry.astropods.ai/zain/support-signal-agent-ingestion-sync:8f406458"},"trigger":{"type":"manual"}}},"inputs":{"anthropic_api_key":{"datatype":"string","default":"sk-ant-api03-test","name":"ANTHROPIC_API_KEY","optional":true,"secret":true},"cue_api_token":{"datatype":"string","default":"ATATT3xFfGF0-test","name":"CUE_API_TOKEN","optional":true,"secret":true},"cue_base_url":{"datatype":"string","default":"https://postmanlabs.atlassian.net","name":"CUE_BASE_URL","optional":true},"cue_email":{"datatype":"string","default":"zain.jandali@postman.com","name":"CUE_EMAIL","optional":true},"gong_access_key":{"datatype":"string","default":"HI3ORHRGYZRY4J7V-test","name":"GONG_ACCESS_KEY","optional":true,"secret":true},"gong_access_key_secret":{"datatype":"string","default":"eyJhbGci-test","name":"GONG_ACCESS_KEY_SECRET","optional":true,"secret":true},"gong_base_url":{"datatype":"string","default":"https://us-14496.api.gong.io","name":"GONG_BASE_URL","optional":true},"gong_lookback_days":{"datatype":"string","default":"365","name":"GONG_LOOKBACK_DAYS","optional":true},"kepler_client_id":{"datatype":"string","default":"13568d48-test","name":"KEPLER_CLIENT_ID","optional":true,"secret":true},"kepler_client_secret":{"datatype":"string","default":"ucIOdfid-test","name":"KEPLER_CLIENT_SECRET","optional":true,"secret":true},"local_mode":{"datatype":"string","default":"false","name":"LOCAL_MODE","optional":true},"openai_api_key":{"datatype":"string","default":"sk-proj-test","name":"OPENAI_API_KEY","optional":true,"secret":true},"otel_exporter_otlp_endpoint":{"datatype":"string","default":"http://localhost:4318","name":"OTEL_EXPORTER_OTLP_ENDPOINT","optional":true},"qdrant_api_key":{"datatype":"string","default":"eyJhbGci-qdrant-test","name":"QDRANT_API_KEY","optional":true,"secret":true},"qdrant_url":{"datatype":"string","default":"https://qdrant.example.com","name":"QDRANT_URL","optional":true},"redash_api_key":{"datatype":"string","default":"LItUCCzab-test","name":"REDASH_API_KEY","optional":true,"secret":true},"salesforce_consumer_key":{"datatype":"string","default":"3MVG9szVa-test","name":"SALESFORCE_CONSUMER_KEY","optional":true,"secret":true},"salesforce_consumer_secret":{"datatype":"string","default":"516C80A7CA-test","name":"SALESFORCE_CONSUMER_SECRET","optional":true,"secret":true},"salesforce_instance_url":{"datatype":"string","default":"https://postman.my.salesforce.com","name":"SALESFORCE_INSTANCE_URL","optional":true},"salesforce_login_url":{"datatype":"string","default":"https://login.salesforce.com","name":"SALESFORCE_LOGIN_URL","optional":true},"salesforce_refresh_token":{"datatype":"string","default":"5Aep861HDR-test","name":"SALESFORCE_REFRESH_TOKEN","optional":true,"secret":true},"slack_allowed_channel_ids":{"datatype":"string","default":"C0AJ478DB1Q","name":"SLACK_ALLOWED_CHANNEL_IDS","optional":true},"slack_allowed_team_ids":{"datatype":"string","default":"","name":"SLACK_ALLOWED_TEAM_IDS","optional":true},"slack_allowed_user_ids":{"datatype":"string","default":"","name":"SLACK_ALLOWED_USER_IDS","optional":true},"slack_app_token":{"datatype":"string","default":"xapp-test","name":"SLACK_APP_TOKEN","optional":true,"secret":true},"slack_bot_token":{"datatype":"string","default":"xoxb-test","name":"SLACK_BOT_TOKEN","optional":true,"secret":true},"slack_direct":{"datatype":"string","default":"false","name":"SLACK_DIRECT","optional":true},"web_enabled":{"datatype":"string","default":"true","name":"WEB_ENABLED","optional":true},"zendesk_access_token":{"datatype":"string","default":"47597e57-test","name":"ZENDESK_ACCESS_TOKEN","optional":true,"secret":true},"zendesk_client_id":{"datatype":"string","default":"fde_integration","name":"ZENDESK_CLIENT_ID","optional":true,"secret":true},"zendesk_client_secret":{"datatype":"string","default":"c57596e9-test","name":"ZENDESK_CLIENT_SECRET","optional":true,"secret":true},"zendesk_lookback_days":{"datatype":"string","default":"90","name":"ZENDESK_LOOKBACK_DAYS","optional":true},"zendesk_max_description_chars":{"datatype":"string","default":"1000","name":"ZENDESK_MAX_DESCRIPTION_CHARS","optional":true},"zendesk_refresh_token":{"datatype":"string","default":"9562510682-test","name":"ZENDESK_REFRESH_TOKEN","optional":true,"secret":true},"zendesk_retention_days":{"datatype":"string","default":"180","name":"ZENDESK_RETENTION_DAYS","optional":true},"zendesk_subdomain":{"datatype":"string","default":"postman","name":"ZENDESK_SUBDOMAIN","optional":true}},"meta":{"description":"Evaluates inbound support tickets during the deal process, implementation, and ongoing work"},"models":{"anthropic":{"provider":"anthropic"},"openai":{"provider":"openai"}},"name":"support-signal-agent","providers":{"anthropic":{"scope":["models"],"variables":[{"datatype":"string","default":"sk-ant-api03-test","name":"ANTHROPIC_API_KEY","optional":true,"secret":true}]},"openai":{"scope":["models"],"variables":[{"datatype":"string","default":"sk-proj-test","name":"OPENAI_API_KEY","optional":true,"secret":true}]}},"spec":"package/v1"}`

// TestResolveDeploymentSpecEnv_SupportSignalAgent exercises the full
// package/v1 → deployment template → filled deployment/v1 → resolve pipeline
// using the exact support-signal-agent spec that triggered false drift.
func TestResolveDeploymentSpecEnv_SupportSignalAgent(t *testing.T) {
	// Parse the package/v1 spec
	var astroSpec spec.AstroSpec
	if err := json.Unmarshal([]byte(supportSignalSpecJSON), &astroSpec); err != nil {
		t.Fatalf("parse package/v1 spec: %v", err)
	}

	// Generate deployment template (same as the server does)
	template, err := GenerateDeploymentTemplate(TemplateInput{
		Spec:         &astroSpec,
		AgentName:    astroSpec.Name,
		Account:      "zain",
		BuildID:      "8f406458",
		RegistryURL:  "registry.astropods.ai",
		ECRNamespace: "zain",
	})
	if err != nil {
		t.Fatalf("GenerateDeploymentTemplate: %v", err)
	}

	// Fill the template: set variable values from inputs (simulates CLI fill)
	for key, v := range template.Variables {
		if v.Value == "" && v.Default != "" {
			v.Value = v.Default
			template.Variables[key] = v
		}
	}
	// Enable adapters
	if template.Interfaces != nil {
		template.Interfaces.Adapters = []string{"web", "slack"}
	}
	template.Spec = "deployment/v1"
	template.Target.Account = "zain"

	// Resolve (same as deploy handler)
	rctx := ResolveContext{
		Namespace:  "ns-support-signal",
		AgentName:  "support-signal-agent",
		BuildID:    "8f406458",
		SecretName: GenerateSecretName("support-signal-agent", "8f406458"),
	}
	result := ResolveDeploymentSpecEnv(template, rctx)

	// All secret inputs must be in SecretData, NOT ConfigMapData
	secretInputs := []string{
		"ANTHROPIC_API_KEY", "CUE_API_TOKEN", "GONG_ACCESS_KEY",
		"GONG_ACCESS_KEY_SECRET", "KEPLER_CLIENT_ID", "KEPLER_CLIENT_SECRET",
		"OPENAI_API_KEY", "QDRANT_API_KEY", "REDASH_API_KEY",
		"SALESFORCE_CONSUMER_KEY", "SALESFORCE_CONSUMER_SECRET",
		"SALESFORCE_REFRESH_TOKEN", "ZENDESK_ACCESS_TOKEN",
		"ZENDESK_CLIENT_ID", "ZENDESK_CLIENT_SECRET", "ZENDESK_REFRESH_TOKEN",
	}
	for _, key := range secretInputs {
		if _, inSecret := result.SecretData[key]; !inSecret {
			t.Errorf("%s: expected in SecretData, not found", key)
		}
		if _, inCM := result.ConfigMapData[key]; inCM {
			t.Errorf("%s: should NOT be in ConfigMapData (would cause false drift)", key)
		}
	}

	// Non-secret inputs must be in ConfigMapData, NOT SecretData
	nonSecretInputs := []string{
		"CUE_BASE_URL", "CUE_EMAIL", "GONG_BASE_URL", "GONG_LOOKBACK_DAYS",
		"LOCAL_MODE", "QDRANT_URL", "SALESFORCE_INSTANCE_URL",
		"SALESFORCE_LOGIN_URL", "SLACK_ALLOWED_CHANNEL_IDS", "SLACK_DIRECT",
		"WEB_ENABLED", "ZENDESK_LOOKBACK_DAYS", "ZENDESK_MAX_DESCRIPTION_CHARS",
		"ZENDESK_RETENTION_DAYS", "ZENDESK_SUBDOMAIN",
	}
	for _, key := range nonSecretInputs {
		if _, inCM := result.ConfigMapData[key]; !inCM {
			t.Errorf("%s: expected in ConfigMapData, not found", key)
		}
		if _, inSecret := result.SecretData[key]; inSecret {
			t.Errorf("%s: should NOT be in SecretData", key)
		}
	}

	// Platform vars should also be in ConfigMapData
	for _, key := range []string{"ASTRO_AGENT_NAME", "ASTRO_AGENT_BUILD", "ASTRO_AGENT_URL", "ASTRO_AGENT_HOST", "OTEL_EXPORTER_OTLP_ENDPOINT", "GRPC_SERVER_ADDR"} {
		if _, inCM := result.ConfigMapData[key]; !inCM {
			t.Errorf("platform var %s: expected in ConfigMapData", key)
		}
	}

	t.Logf("ConfigMapData keys (%d): %v", len(result.ConfigMapData), sortedKeys(result.ConfigMapData))
	t.Logf("SecretData keys (%d): %v", len(result.SecretData), sortedKeys(result.SecretData))
}

// TestRepairRetemplate_FixesBuggyStoredSpec simulates the repair flow:
// a deployment spec generated by the old buggy template code has duplicate
// credential keys and missing defaults. Re-generating the template from
// the package spec and merging variable values should produce a correct spec.
func TestRepairRetemplate_FixesBuggyStoredSpec(t *testing.T) {
	// Parse the package/v1 spec (same one used in the support-signal-agent test)
	var astroSpec spec.AstroSpec
	if err := json.Unmarshal([]byte(supportSignalSpecJSON), &astroSpec); err != nil {
		t.Fatalf("parse package spec: %v", err)
	}

	// Simulate a BUGGY stored deployment spec: manually inject the problems
	// that the old template code would have produced.
	buggyTemplate, err := GenerateDeploymentTemplate(TemplateInput{
		Spec:         &astroSpec,
		AgentName:    astroSpec.Name,
		Account:      "zain",
		BuildID:      "8f406458",
		RegistryURL:  "registry.astropods.ai",
		ECRNamespace: "zain",
	})
	if err != nil {
		t.Fatalf("generate template: %v", err)
	}

	// Inject the bogus duplicate keys that the old code would have produced.
	// The old CustomProviderCredentialKeys generated ANTHROPIC_ANTHROPIC_API_KEY
	// and OPENAI_OPENAI_API_KEY from the custom provider path.
	buggyTemplate.Variables["ANTHROPIC_ANTHROPIC_API_KEY"] = spec.Variable{
		Secret: true, Optional: true, Targets: []string{"agent"},
	}
	buggyTemplate.Variables["OPENAI_OPENAI_API_KEY"] = spec.Variable{
		Secret: true, Optional: true, Targets: []string{"agent"},
	}
	buggyTemplate.Agent.Environment["ANTHROPIC_ANTHROPIC_API_KEY"] = "${variables.ANTHROPIC_ANTHROPIC_API_KEY}"
	buggyTemplate.Agent.Environment["OPENAI_OPENAI_API_KEY"] = "${variables.OPENAI_OPENAI_API_KEY}"

	// Wipe defaults on credential variables (simulates Bug 3: first-write-wins lost them)
	for _, key := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		if v, ok := buggyTemplate.Variables[key]; ok {
			v.Default = ""
			v.Value = ""
			buggyTemplate.Variables[key] = v
		}
	}

	// Fill the buggy template (simulates user filling at deploy time)
	for key, v := range buggyTemplate.Variables {
		if v.Value == "" && v.Default != "" {
			v.Value = v.Default
			buggyTemplate.Variables[key] = v
		}
	}
	// Manually fill the ones with wiped defaults (user would have typed them)
	fillValue := func(key, val string) {
		if v, ok := buggyTemplate.Variables[key]; ok {
			v.Value = val
			buggyTemplate.Variables[key] = v
		}
	}
	fillValue("ANTHROPIC_API_KEY", "sk-ant-real")
	fillValue("OPENAI_API_KEY", "sk-proj-real")
	fillValue("ANTHROPIC_ANTHROPIC_API_KEY", "sk-ant-dup")
	fillValue("OPENAI_OPENAI_API_KEY", "sk-proj-dup")

	if buggyTemplate.Interfaces != nil {
		buggyTemplate.Interfaces.Adapters = []string{"web", "slack"}
	}
	buggyTemplate.Spec = "deployment/v1"

	// Strip secrets (as the server does before storing)
	stripped := spec.StripSecretVariableValues(buggyTemplate)

	// Verify the buggy spec has the problems
	if _, ok := stripped.Variables["ANTHROPIC_ANTHROPIC_API_KEY"]; !ok {
		t.Fatal("setup: buggy spec should have ANTHROPIC_ANTHROPIC_API_KEY")
	}
	if _, ok := stripped.Agent.Environment["ANTHROPIC_ANTHROPIC_API_KEY"]; !ok {
		t.Fatal("setup: buggy spec should have ANTHROPIC_ANTHROPIC_API_KEY in agent env")
	}

	// --- Simulate repair re-template ---
	// Re-generate template from the package spec (with fixed code)
	newTemplate, err := GenerateDeploymentTemplate(TemplateInput{
		Spec:         &astroSpec,
		AgentName:    astroSpec.Name,
		Account:      "zain",
		BuildID:      "8f406458",
		RegistryURL:  "registry.astropods.ai",
		ECRNamespace: "zain",
	})
	if err != nil {
		t.Fatalf("re-generate template: %v", err)
	}

	// Collect existing variable values (simulates reading from DB)
	existingValues := make(map[string]string)
	for key, v := range buggyTemplate.Variables {
		if v.Value != "" {
			existingValues[key] = v.Value
		}
	}

	// Preserve user-selected adapters
	var userAdapters []string
	if stripped.Interfaces != nil {
		userAdapters = stripped.Interfaces.Adapters
	}

	// Replace variables and agent env with the re-generated template
	stripped.Variables = newTemplate.Variables
	stripped.Agent.Environment = newTemplate.Agent.Environment
	stripped.Interfaces = newTemplate.Interfaces

	// Restore user-selected adapters
	if stripped.Interfaces != nil && userAdapters != nil {
		stripped.Interfaces.Adapters = userAdapters
	}

	// Restore variable values
	for key, v := range stripped.Variables {
		if val, ok := existingValues[key]; ok {
			v.Value = val
			stripped.Variables[key] = v
		}
	}

	// --- Verify the fixed spec ---
	// User-selected adapters must be preserved
	if stripped.Interfaces == nil {
		t.Fatal("Interfaces should not be nil after re-template")
	}
	if len(stripped.Interfaces.Adapters) != 2 || stripped.Interfaces.Adapters[0] != "web" || stripped.Interfaces.Adapters[1] != "slack" {
		t.Errorf("Adapters should be [web slack], got %v", stripped.Interfaces.Adapters)
	}

	// Bogus duplicate keys should be gone
	if _, ok := stripped.Variables["ANTHROPIC_ANTHROPIC_API_KEY"]; ok {
		t.Error("ANTHROPIC_ANTHROPIC_API_KEY should not exist after re-template")
	}
	if _, ok := stripped.Variables["OPENAI_OPENAI_API_KEY"]; ok {
		t.Error("OPENAI_OPENAI_API_KEY should not exist after re-template")
	}
	if _, ok := stripped.Agent.Environment["ANTHROPIC_ANTHROPIC_API_KEY"]; ok {
		t.Error("ANTHROPIC_ANTHROPIC_API_KEY should not be in agent env after re-template")
	}
	if _, ok := stripped.Agent.Environment["OPENAI_OPENAI_API_KEY"]; ok {
		t.Error("OPENAI_OPENAI_API_KEY should not be in agent env after re-template")
	}

	// Correct keys should exist with restored values
	if v := stripped.Variables["ANTHROPIC_API_KEY"]; v.Value != "sk-ant-real" {
		t.Errorf("ANTHROPIC_API_KEY.Value: expected sk-ant-real, got %q", v.Value)
	}
	if v := stripped.Variables["OPENAI_API_KEY"]; v.Value != "sk-proj-real" {
		t.Errorf("OPENAI_API_KEY.Value: expected sk-proj-real, got %q", v.Value)
	}

	// Defaults should be populated from the input (Bug 3 fix)
	if v := stripped.Variables["ANTHROPIC_API_KEY"]; v.Default == "" {
		t.Error("ANTHROPIC_API_KEY.Default should be populated after re-template")
	}

	// Resolve the fixed spec and verify routing
	rctx := ResolveContext{
		Namespace: "ns-test",
		AgentName: "support-signal-agent",
		BuildID:   "8f406458",
	}
	result := ResolveDeploymentSpecEnv(stripped, rctx)

	// Secret inputs must be in SecretData
	for _, key := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"} {
		if _, inSecret := result.SecretData[key]; !inSecret {
			t.Errorf("%s: expected in SecretData after repair", key)
		}
		if _, inCM := result.ConfigMapData[key]; inCM {
			t.Errorf("%s: should NOT be in ConfigMapData after repair", key)
		}
	}

	// Bogus keys must not appear anywhere
	for _, key := range []string{"ANTHROPIC_ANTHROPIC_API_KEY", "OPENAI_OPENAI_API_KEY"} {
		if _, ok := result.SecretData[key]; ok {
			t.Errorf("%s: should not exist in SecretData after repair", key)
		}
		if _, ok := result.ConfigMapData[key]; ok {
			t.Errorf("%s: should not exist in ConfigMapData after repair", key)
		}
	}
}

func TestHasSecretValues(t *testing.T) {
	empty := &ResolvedEnv{
		SecretData: map[string]string{"KEY": ""},
	}
	if empty.HasSecretValues() {
		t.Error("expected false for empty secret values")
	}

	populated := &ResolvedEnv{
		SecretData: map[string]string{"KEY": "real-value"},
	}
	if !populated.HasSecretValues() {
		t.Error("expected true for populated secret values")
	}
}

func TestResolveDeploymentSpecEnv_KnowledgeCredentialRefs(t *testing.T) {
	// Credential refs (${knowledge.*.credentials.*}) should resolve from
	// BoundCredentials for both bound and self-hosted entries.
	ds := &spec.AstroDeploymentSpec{
		Source: spec.DeploymentSource{Name: "agent", Build: "b1"},
		Agent: spec.DeploymentAgent{
			Image:     "x",
			Endpoints: httpEndpoints(8080),
			Environment: map[string]string{
				"POSTGRES_HOST":     "${knowledge.postgres.host}",
				"POSTGRES_PORT":     "${knowledge.postgres.http.port}",
				"POSTGRES_USER":     "${knowledge.postgres.credentials.user}",
				"POSTGRES_PASSWORD": "${knowledge.postgres.credentials.password}",
			},
		},
		Knowledge: map[string]spec.DeploymentKnowledge{
			"postgres": {
				Image:     "pgvector:latest",
				Endpoints: httpEndpoints(5432),
				Provider:  "postgres",
			},
		},
	}

	rctx := ResolveContext{
		Namespace: "ns",
		AgentName: "agent",
		BoundCredentials: map[string]string{
			"postgres.user":     "astro",
			"postgres.password": "s3cret",
		},
	}
	result := ResolveDeploymentSpecEnv(ds, rctx)

	// Credential refs resolve to secret data (not configmap).
	if result.SecretData["POSTGRES_USER"] != "astro" {
		t.Errorf("POSTGRES_USER: expected astro, got %q", result.SecretData["POSTGRES_USER"])
	}
	if result.SecretData["POSTGRES_PASSWORD"] != "s3cret" {
		t.Errorf("POSTGRES_PASSWORD: expected s3cret, got %q", result.SecretData["POSTGRES_PASSWORD"])
	}

	// HOST/PORT resolve to configmap.
	if !strings.Contains(result.ConfigMapData["POSTGRES_HOST"], "agent-knowledge-postgres") {
		t.Errorf("POSTGRES_HOST: expected DNS name, got %q", result.ConfigMapData["POSTGRES_HOST"])
	}
	if result.ConfigMapData["POSTGRES_PORT"] != "5432" {
		t.Errorf("POSTGRES_PORT: expected 5432, got %q", result.ConfigMapData["POSTGRES_PORT"])
	}
}
