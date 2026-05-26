package spec

import (
	"testing"
)

func TestParseDeploymentSpec_Valid(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  account: acme
  name: my-agent
  build: abc123
  registry: registry.example.com
target:
  runtime: kubernetes
agent:
  image: registry.example.com/my-agent:abc123
  endpoints:
    http:
      port: 8080
  replicas: 1
  resources:
    cpu: "100m"
    memory: "256Mi"
  update:
    strategy: rolling
observability:
  enabled: true
  provider: langfuse
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ds.Spec != "deployment/v1" {
		t.Errorf("spec: expected deployment/v1, got %s", ds.Spec)
	}
	if ds.Source.Name != "my-agent" {
		t.Errorf("source.name: expected my-agent, got %s", ds.Source.Name)
	}
	if PrimaryPort(ds.Agent.Endpoints) != 8080 {
		t.Errorf("agent.endpoints.http.port: expected 8080, got %d", PrimaryPort(ds.Agent.Endpoints))
	}
}

func TestParseDeploymentSpec_TemplateVersion(t *testing.T) {
	yaml := `
spec: deployment-template/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
editable:
  - variables.*.value
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error for deployment-template/v1: %v", err)
	}
	if ds.Spec != "deployment-template/v1" {
		t.Errorf("spec: expected deployment-template/v1, got %s", ds.Spec)
	}
}

func TestParseDeploymentSpec_InvalidVersion(t *testing.T) {
	yaml := `
spec: invalid/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid spec version")
	}
}

func TestParseDeploymentSpec_MissingSourceName(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing source.name")
	}
}

func TestParseDeploymentSpec_MissingAgentImage(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  endpoints:
    http:
      port: 8080
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing agent.image")
	}
}

func TestParseDeploymentSpec_MissingAgentEndpoints(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing agent.endpoints")
	}
}

func TestParseDeploymentSpec_MissingModelImage(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
models:
  llm:
    endpoints:
      http:
        port: 11434
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing model image")
	}
}

func TestParseDeploymentSpec_PersistentKnowledgeWithoutStorage(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
knowledge:
  docs:
    image: qdrant/qdrant:latest
    endpoints:
      http:
        port: 6333
    persistent: true
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for persistent knowledge without storage")
	}
}

func TestParseDeploymentSpec_BoundKnowledgeSkipsContainerValidation(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
knowledge:
  docs:
    binding: "arn:knowledge-store:acct123:my-pg-store"
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("bound knowledge should not require image/endpoints: %v", err)
	}
	if !ds.Knowledge["docs"].IsBound() {
		t.Fatal("expected knowledge entry to be bound")
	}
}

func TestParseDeploymentSpec_BoundAndInlineKnowledgeMixed(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
knowledge:
  managed_db:
    binding: "arn:knowledge-store:acct123:pg-store"
  local_cache:
    image: redis:7
    endpoints:
      tcp:
        port: 6379
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("mixed bound/inline knowledge should parse: %v", err)
	}
	if !ds.Knowledge["managed_db"].IsBound() {
		t.Error("expected managed_db to be bound")
	}
	if ds.Knowledge["local_cache"].IsBound() {
		t.Error("expected local_cache to not be bound")
	}
	if ds.Knowledge["local_cache"].Image != "redis:7" {
		t.Errorf("expected local_cache image redis:7, got %s", ds.Knowledge["local_cache"].Image)
	}
}

func TestParseDeploymentSpec_InlineKnowledgeStillRequiresImage(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
knowledge:
  docs:
    endpoints:
      http:
        port: 6333
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for inline knowledge without image")
	}
}

func TestParseDeploymentSpec_BoundKnowledgeInTemplate(t *testing.T) {
	yaml := `
spec: deployment-template/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
knowledge:
  docs:
    binding: "arn:knowledge-store:acct123:pg-store"
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("bound knowledge in template should parse: %v", err)
	}
	if !ds.Knowledge["docs"].IsBound() {
		t.Fatal("expected knowledge entry to be bound")
	}
}

func TestParseDeploymentSpec_EmptyBindingStillRequiresImage(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
knowledge:
  docs:
    binding: ""
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for knowledge with empty binding and no image")
	}
}

func TestParseDeploymentSpec_MissingIngestionTriggerType(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
ingestion:
  sync:
    image: ingest:latest
    trigger:
      schedule: "0 * * * *"
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing ingestion trigger type")
	}
}

func TestParseDeploymentSpec_WithAllComponents(t *testing.T) {
	yaml := `
spec: deployment-template/v1
source:
  account: acme
  name: full-agent
  build: b1
  registry: registry.example.com
target:
  runtime: kubernetes
agent:
  image: agent:latest
  endpoints:
    http:
      port: 8080
      expose:
        enabled: true
        domain: agent.example.com
  replicas: 2
  resources:
    cpu: "200m"
    memory: "512Mi"
    cpu_limit: "2"
    memory_limit: "2Gi"
  environment:
    LLM_URL: "${models.llm.http.url}"
    API_KEY: "${variables.ANTHROPIC_API_KEY}"
  update:
    strategy: rolling
    max_unavailable: "1"
    max_surge: "1"
  distributed: true
models:
  llm:
    image: ollama/ollama:latest
    endpoints:
      http:
        port: 11434
    replicas: 1
    resources:
      cpu: "2"
      memory: "8Gi"
    gpu:
      vram: "24Gi"
      runtime: cuda
      count: 1
    update:
      strategy: recreate
knowledge:
  docs:
    image: qdrant/qdrant:latest
    endpoints:
      http:
        port: 6333
    persistent: true
    storage:
      size: "20Gi"
      access_mode: ReadWriteOnce
    update:
      strategy: recreate
integrations:
  search:
    image: search:latest
    endpoints:
      http:
        port: 3000
    replicas: 1
    resources:
      cpu: "100m"
      memory: "256Mi"
    update:
      strategy: rolling
ingestion:
  sync:
    image: ingest:latest
    trigger:
      type: schedule
      schedule: "0 */6 * * *"
    environment:
      TARGET: docs
interfaces:
  adapters: [slack, web]
  image: messaging:latest
  endpoints:
    grpc:
      port: 9090
      protocol: grpc
    http:
      port: 8080
      expose:
        enabled: true
        domain: chat.example.com
  resources:
    cpu: "100m"
    memory: "128Mi"
variables:
  ANTHROPIC_API_KEY:
    description: Anthropic API key
    secret: true
    optional: false
    targets:
      - agent
  SLACK_BOT_TOKEN:
    description: Slack bot token
    secret: true
    targets:
      - interface.slack
observability:
  enabled: true
  provider: langfuse
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ds.Agent.Replicas != 2 {
		t.Errorf("agent.replicas: expected 2, got %d", ds.Agent.Replicas)
	}
	if !ds.Agent.Distributed {
		t.Error("agent.distributed: expected true")
	}
	// Check agent expose endpoint
	httpEp, ok := ds.Agent.Endpoints["http"]
	if !ok || httpEp.Expose == nil || httpEp.Expose.Domain != "agent.example.com" {
		t.Errorf("agent.endpoints.http.expose.domain: expected agent.example.com")
	}
	if ds.Models["llm"].GPU.VRAM != "24Gi" {
		t.Errorf("models.llm.gpu.vram: expected 24Gi, got %s", ds.Models["llm"].GPU.VRAM)
	}
	if ds.Knowledge["docs"].Storage.Size != "20Gi" {
		t.Errorf("knowledge.docs.storage.size: expected 20Gi, got %s", ds.Knowledge["docs"].Storage.Size)
	}
	if len(ds.Interfaces.Adapters) != 2 {
		t.Errorf("interfaces.adapters: expected 2, got %d", len(ds.Interfaces.Adapters))
	}
	if ds.Ingestion["sync"].Trigger.Schedule != "0 */6 * * *" {
		t.Errorf("ingestion trigger.schedule: got %s", ds.Ingestion["sync"].Trigger.Schedule)
	}
}

func TestSerializeDeploymentSpec_RoundTrip(t *testing.T) {
	original := &AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: DeploymentSource{
			Account:  "acme",
			Name:     "test",
			Build:    "b1",
			Registry: "reg.io",
		},
		Target: DeploymentTarget{Runtime: "kubernetes"},
		Agent: DeploymentAgent{
			Image: "agent:latest",
			Endpoints: map[string]Endpoint{
				"http": {Port: 8080},
			},
			Replicas: 1,
			Resources: DeploymentResources{
				CPU: "100m", Memory: "256Mi",
			},
			Environment: map[string]string{
				"KEY": "${variables.API_KEY}",
			},
			Update: UpdateStrategy{Strategy: "rolling"},
		},
		Models: map[string]DeploymentModel{
			"llm": {
				Image: "ollama:latest",
				Endpoints: map[string]Endpoint{
					"http": {Port: 11434},
				},
				Replicas: 1,
			},
		},
		Variables: map[string]Variable{
			"API_KEY": {Value: "sk-test", Secret: true, Targets: []string{"agent"}},
		},
		Observability: DeploymentObservability{Enabled: true, Provider: "langfuse"},
	}

	data, err := SerializeDeploymentSpec(original)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	parsed, err := ParseDeploymentSpec(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.Source.Name != "test" {
		t.Errorf("source.name: expected test, got %s", parsed.Source.Name)
	}
	if parsed.Agent.Environment["KEY"] != "${variables.API_KEY}" {
		t.Error("environment lost in round-trip")
	}
	if PrimaryPort(parsed.Models["llm"].Endpoints) != 11434 {
		t.Errorf("models.llm.endpoints http port: expected 11434, got %d", PrimaryPort(parsed.Models["llm"].Endpoints))
	}
}

func TestStripSecretVariableValues(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: DeploymentSource{
			Name: "test", Build: "b1", Registry: "r",
		},
		Agent: DeploymentAgent{
			Image:     "x",
			Endpoints: map[string]Endpoint{"http": {Port: 8080}},
		},
		Variables: map[string]Variable{
			"API_KEY":     {Value: "sk-secret-key", Description: "API key", Optional: false, Secret: true},
			"SLACK_TOKEN": {Value: "xoxb-secret", Description: "Slack token", Optional: true, Secret: true},
			"LOG_LEVEL":   {Value: "debug", Description: "Log level", Secret: false},
		},
	}

	stripped := StripSecretVariableValues(ds)

	// Original should be unchanged
	if ds.Variables["API_KEY"].Value != "sk-secret-key" {
		t.Error("original mutated")
	}

	// Stripped should have empty values for secret vars but keep non-secret values
	if stripped.Variables["API_KEY"].Value != "" {
		t.Errorf("stripped value should be empty, got %s", stripped.Variables["API_KEY"].Value)
	}
	if stripped.Variables["API_KEY"].Description != "API key" {
		t.Error("description lost in strip")
	}
	if stripped.Variables["SLACK_TOKEN"].Value != "" {
		t.Error("slack token value should be empty")
	}
	if !stripped.Variables["SLACK_TOKEN"].Optional {
		t.Error("optional flag lost in strip")
	}
	// Non-secret values should be preserved
	if stripped.Variables["LOG_LEVEL"].Value != "debug" {
		t.Errorf("non-secret value should be preserved, got %s", stripped.Variables["LOG_LEVEL"].Value)
	}
}

func TestParseDeploymentSpec_WebAdapterRequiresExposeEndpoint(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
interfaces:
  adapters: [slack, web]
  image: messaging:latest
  endpoints:
    grpc:
      port: 9090
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for web adapter without exposed endpoint")
	}
}

func TestParseDeploymentSpec_WebAdapterWithExposeEndpoint(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
interfaces:
  adapters: [slack, web]
  image: messaging:latest
  endpoints:
    grpc:
      port: 9090
    http:
      port: 8080
      expose:
        enabled: true
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	httpEp, ok := ds.Interfaces.Endpoints["http"]
	if !ok || httpEp.Expose == nil || !httpEp.Expose.Enabled {
		t.Error("expected http endpoint with expose.enabled=true")
	}
}

func TestParseDeploymentSpec_SlackOnlyAdapterNoExpose(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
interfaces:
  adapters: [slack]
  image: messaging:latest
  endpoints:
    grpc:
      port: 9090
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: slack-only adapter should not require exposed endpoint: %v", err)
	}
}

func TestParseDeploymentSpec_WebhookIngestionRequiresEndpoints(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
ingestion:
  data:
    image: ingest:latest
    trigger:
      type: webhook
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for webhook ingestion without endpoints")
	}
}

func TestParseDeploymentSpec_WebhookIngestionWithEndpoints(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
ingestion:
  data:
    image: ingest:latest
    endpoints:
      http:
        port: 3001
    trigger:
      type: webhook
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if PrimaryPort(ds.Ingestion["data"].Endpoints) != 3001 {
		t.Errorf("expected port 3001, got %d", PrimaryPort(ds.Ingestion["data"].Endpoints))
	}
}

func TestParseDeploymentSpec_DeploymentV1RejectsTemplateOnlyFields(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
variables:
  KEY:
    default: "foo"
    secret: true
    optional: true
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error: deployment/v1 must not have default on variables")
	}
}

func TestParseDeploymentSpec_DistributedFalseRejectsMultipleReplicas(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
  replicas: 3
  distributed: false
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error: replicas must be 1 when distributed is false")
	}
}

// ===== Rule 3: target.runtime =====

func TestParseDeploymentSpec_InvalidRuntime(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
target:
  runtime: ecs
agent:
  image: x
  endpoints:
    http:
      port: 8080
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid target.runtime")
	}
}

func TestParseDeploymentSpec_ValidRuntime(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
target:
  runtime: kubernetes
agent:
  image: x
  endpoints:
    http:
      port: 8080
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error for valid runtime: %v", err)
	}
}

// ===== Rule 6a: endpoint protocol enum =====

func TestParseDeploymentSpec_InvalidEndpointProtocol(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
      protocol: websocket
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid endpoint protocol")
	}
}

func TestParseDeploymentSpec_ValidEndpointProtocols(t *testing.T) {
	for _, proto := range []string{"http", "grpc", "tcp"} {
		t.Run(proto, func(t *testing.T) {
			y := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    ep:
      port: 8080
      protocol: ` + proto + `
`
			if _, err := ParseDeploymentSpec([]byte(y)); err != nil {
				t.Fatalf("unexpected error for protocol %q: %v", proto, err)
			}
		})
	}
}

// ===== Rule 10: schedule field forbidden for non-schedule triggers =====

func TestParseDeploymentSpec_NonScheduleTriggerWithSchedule(t *testing.T) {
	cases := []struct{ triggerType, specVer string }{
		{"startup", "deployment/v1"},
		{"webhook", "deployment/v1"},
		{"manual", "deployment/v1"},
	}
	for _, tc := range cases {
		t.Run(tc.triggerType, func(t *testing.T) {
			y := `
spec: ` + tc.specVer + `
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
ingestion:
  data:
    image: ingest:latest
    trigger:
      type: ` + tc.triggerType + `
      schedule: "0 * * * *"
`
			if tc.triggerType == "webhook" {
				y = `
spec: ` + tc.specVer + `
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
ingestion:
  data:
    image: ingest:latest
    endpoints:
      http:
        port: 9000
    trigger:
      type: webhook
      schedule: "0 * * * *"
`
			}
			_, err := ParseDeploymentSpec([]byte(y))
			if err == nil {
				t.Fatalf("expected error: trigger.schedule must not be set for type %q", tc.triggerType)
			}
		})
	}
}

// ===== Rule 11: interfaces validation in deployment/v1 =====

func TestParseDeploymentSpec_InterfacesEmptyAdapters(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
interfaces:
  adapters: []
  image: messaging:latest
  endpoints:
    grpc:
      port: 9090
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for empty interfaces.adapters in deployment/v1")
	}
}

func TestParseDeploymentSpec_InterfacesMissingImage(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
interfaces:
  adapters: [slack]
  endpoints:
    grpc:
      port: 9090
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing interfaces.image in deployment/v1")
	}
}

func TestParseDeploymentSpec_InterfacesEmptyAdaptersAllowedInTemplate(t *testing.T) {
	yaml := `
spec: deployment-template/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
interfaces:
  adapters: []
  image: messaging:latest
  endpoints:
    grpc:
      port: 9090
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("template should allow empty adapters, got: %v", err)
	}
}

// ===== Rule 12 / Rule 12e: variable value/ref/optional combinations =====
//
// Truth table (optional × value × ref):
//   optional=false, value="",  ref=""  → error (no value, no ref)
//   optional=false, value="v", ref=""  → valid
//   optional=false, value="",  ref="R" → valid
//   optional=false, value="v", ref="R" → error (both set)
//   optional=true,  value="",  ref=""  → valid (optional, nothing needed)
//   optional=true,  value="v", ref=""  → valid
//   optional=true,  value="",  ref="R" → valid
//   optional=true,  value="v", ref="R" → error (both set)

func varFixture(optional bool, value, ref string) string {
	optStr := ""
	if optional {
		optStr = "\n    optional: true"
	}
	valueStr := ""
	if value != "" {
		valueStr = "\n    value: " + value
	}
	refStr := ""
	if ref != "" {
		refStr = "\n    ref: " + ref
	}
	return `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
variables:
  KEY:
    secret: true
    targets:
      - agent` + optStr + valueStr + refStr + "\n"
}

func TestParseDeploymentSpec_Variable_RequiredNoValueNoRef(t *testing.T) {
	_, err := ParseDeploymentSpec([]byte(varFixture(false, "", "")))
	if err == nil {
		t.Fatal("expected error: required variable with no value and no ref")
	}
}

func TestParseDeploymentSpec_Variable_RequiredValueOnly(t *testing.T) {
	_, err := ParseDeploymentSpec([]byte(varFixture(false, "v", "")))
	if err != nil {
		t.Fatalf("required variable with value only should be valid, got: %v", err)
	}
}

func TestParseDeploymentSpec_Variable_RequiredRefOnly(t *testing.T) {
	_, err := ParseDeploymentSpec([]byte(varFixture(false, "", "R")))
	if err != nil {
		t.Fatalf("required variable with ref only should be valid, got: %v", err)
	}
}

func TestParseDeploymentSpec_Variable_RequiredValueAndRef(t *testing.T) {
	_, err := ParseDeploymentSpec([]byte(varFixture(false, "v", "R")))
	if err == nil {
		t.Fatal("expected error: cannot set both value and ref")
	}
}

func TestParseDeploymentSpec_Variable_OptionalNoValueNoRef(t *testing.T) {
	_, err := ParseDeploymentSpec([]byte(varFixture(true, "", "")))
	if err != nil {
		t.Fatalf("optional variable with no value and no ref should be valid, got: %v", err)
	}
}

func TestParseDeploymentSpec_Variable_OptionalValueOnly(t *testing.T) {
	_, err := ParseDeploymentSpec([]byte(varFixture(true, "v", "")))
	if err != nil {
		t.Fatalf("optional variable with value only should be valid, got: %v", err)
	}
}

func TestParseDeploymentSpec_Variable_OptionalRefOnly(t *testing.T) {
	_, err := ParseDeploymentSpec([]byte(varFixture(true, "", "R")))
	if err != nil {
		t.Fatalf("optional variable with ref only should be valid, got: %v", err)
	}
}

func TestParseDeploymentSpec_Variable_OptionalValueAndRef(t *testing.T) {
	_, err := ParseDeploymentSpec([]byte(varFixture(true, "v", "R")))
	if err == nil {
		t.Fatal("expected error: cannot set both value and ref")
	}
}

// ===== Rule 12a: variables.*.targets validation in deployment/v1 =====

func TestParseDeploymentSpec_VariablesMissingTargets(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
variables:
  KEY:
    value: "val"
    secret: true
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing variables.*.targets in deployment/v1")
	}
}

func TestParseDeploymentSpec_VariablesInvalidTarget(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
variables:
  KEY:
    value: "val"
    secret: true
    targets:
      - model:foo
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid target value")
	}
}

func TestParseDeploymentSpec_VariablesValidTargets(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
ingestion:
  sync:
    image: ingest:latest
    trigger:
      type: schedule
      schedule: "0 * * * *"
interfaces:
  adapters: [slack]
  image: messaging:latest
  endpoints:
    grpc:
      port: 9090
variables:
  KEY1:
    value: "v"
    secret: true
    targets: [agent]
  KEY2:
    value: "v"
    secret: false
    optional: true
    targets: [ingestion]
  KEY3:
    value: "v"
    secret: true
    targets: [ingestion.sync]
  KEY4:
    value: "v"
    secret: true
    targets: [interface.slack]
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error for valid targets: %v", err)
	}
}

// ===== Rule 12b: display-as select requires options =====

func TestParseDeploymentSpec_SelectDisplayAsWithoutOptions(t *testing.T) {
	yaml := `
spec: deployment-template/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
variables:
  MODE:
    default: ""
    secret: false
    targets: [agent]
    display-as: select
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error: display-as=select requires options")
	}
}

func TestParseDeploymentSpec_SelectDisplayAsWithOptions(t *testing.T) {
	yaml := `
spec: deployment-template/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
variables:
  MODE:
    default: "fast"
    secret: false
    targets: [agent]
    display-as: select
    options: [fast, slow]
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error for valid select with options: %v", err)
	}
}

// ===== Rule 12c: datatype enum =====

func TestParseDeploymentSpec_InvalidDatatype(t *testing.T) {
	yaml := `
spec: deployment-template/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
variables:
  KEY:
    default: ""
    targets: [agent]
    datatype: integer
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid datatype")
	}
}

// ===== Rule 12d: display-as enum =====

func TestParseDeploymentSpec_InvalidDisplayAs(t *testing.T) {
	yaml := `
spec: deployment-template/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
variables:
  KEY:
    default: ""
    targets: [agent]
    display-as: dropdown
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid display-as value")
	}
}

// ===== Rule 14: duplicate ports within a component =====

func TestParseDeploymentSpec_DuplicateEndpointPorts(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
    grpc:
      port: 8080
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate endpoint ports")
	}
}

func TestParseDeploymentSpec_DuplicateModelEndpointPorts(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
models:
  llm:
    image: ollama:latest
    endpoints:
      http:
        port: 11434
      extra:
        port: 11434
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for duplicate model endpoint ports")
	}
}

// ===== Rule 15: gpu.runtime enum =====

func TestParseDeploymentSpec_InvalidGPURuntime(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
models:
  llm:
    image: vllm:latest
    endpoints:
      http:
        port: 8000
    gpu:
      vram: "24Gi"
      runtime: metal
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid gpu.runtime")
	}
}

func TestParseDeploymentSpec_ValidGPURuntimes(t *testing.T) {
	for _, runtime := range []string{"cuda", "rocm"} {
		t.Run(runtime, func(t *testing.T) {
			y := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
models:
  llm:
    image: vllm:latest
    endpoints:
      http:
        port: 8000
    gpu:
      runtime: ` + runtime + `
`
			if _, err := ParseDeploymentSpec([]byte(y)); err != nil {
				t.Fatalf("unexpected error for gpu.runtime %q: %v", runtime, err)
			}
		})
	}
}

// ===== Rule 16: storage.access_mode enum =====

func TestParseDeploymentSpec_InvalidAccessMode(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
knowledge:
  docs:
    image: qdrant:latest
    endpoints:
      http:
        port: 6333
    persistent: true
    storage:
      size: 10Gi
      access_mode: ReadWriteExecute
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid storage.access_mode")
	}
}

func TestParseDeploymentSpec_ValidAccessModes(t *testing.T) {
	for _, mode := range []string{"ReadWriteOnce", "ReadWriteMany"} {
		t.Run(mode, func(t *testing.T) {
			y := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
knowledge:
  docs:
    image: qdrant:latest
    endpoints:
      http:
        port: 6333
    persistent: true
    storage:
      size: 10Gi
      access_mode: ` + mode + `
`
			if _, err := ParseDeploymentSpec([]byte(y)); err != nil {
				t.Fatalf("unexpected error for access_mode %q: %v", mode, err)
			}
		})
	}
}

// ===== Rule 17: update.strategy enum =====

func TestParseDeploymentSpec_InvalidUpdateStrategy(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
  update:
    strategy: canary
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid update.strategy")
	}
}

func TestParseDeploymentSpec_ValidUpdateStrategies(t *testing.T) {
	for _, strategy := range []string{"rolling", "recreate"} {
		t.Run(strategy, func(t *testing.T) {
			y := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  endpoints:
    http:
      port: 8080
  update:
    strategy: ` + strategy + `
`
			if _, err := ParseDeploymentSpec([]byte(y)); err != nil {
				t.Fatalf("unexpected error for update.strategy %q: %v", strategy, err)
			}
		})
	}
}
