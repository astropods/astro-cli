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
  namespace: prod
agent:
  image: registry.example.com/my-agent:abc123
  port: 8080
  replicas: 1
  resources:
    cpu: "100m"
    memory: "256Mi"
  update:
    strategy: rolling
  expose:
    enabled: false
observability:
  enabled: true
  provider: galileo
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
	if ds.Agent.Port != 8080 {
		t.Errorf("agent.port: expected 8080, got %d", ds.Agent.Port)
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
  port: 8080
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing agent.image")
	}
}

func TestParseDeploymentSpec_MissingAgentPort(t *testing.T) {
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
		t.Fatal("expected error for missing agent.port")
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
  port: 8080
models:
  llm:
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
  port: 8080
knowledge:
  docs:
    image: qdrant/qdrant:latest
    port: 6333
    persistent: true
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for persistent knowledge without storage")
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
spec: deployment/v1
source:
  account: acme
  name: full-agent
  build: b1
  registry: registry.example.com
target:
  runtime: kubernetes
  namespace: prod
agent:
  image: agent:latest
  port: 8080
  replicas: 2
  resources:
    cpu: "200m"
    memory: "512Mi"
    cpu_limit: "2"
    memory_limit: "2Gi"
  environment:
    LLM_URL: "${models.llm.url}"
    API_KEY: "${credentials.ANTHROPIC_API_KEY}"
  update:
    strategy: rolling
    max_unavailable: "1"
    max_surge: "1"
  expose:
    enabled: true
    domain: agent.example.com
    port: 8080
models:
  llm:
    image: ollama/ollama:latest
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
    port: 6333
    persistent: true
    storage:
      size: "20Gi"
      access_mode: ReadWriteOnce
    update:
      strategy: recreate
tools:
  search:
    image: search:latest
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
  port: 9090
  resources:
    cpu: "100m"
    memory: "128Mi"
  expose:
    enabled: true
    port: 8080
    domain: chat.example.com
credentials:
  ANTHROPIC_API_KEY:
    value: ""
    description: Anthropic API key
    optional: false
  SLACK_BOT_TOKEN:
    value: ""
    description: Slack bot token
observability:
  enabled: true
  provider: galileo
editable:
  - credentials.*.value
  - target.namespace
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ds.Agent.Replicas != 2 {
		t.Errorf("agent.replicas: expected 2, got %d", ds.Agent.Replicas)
	}
	if ds.Agent.Expose.Domain != "agent.example.com" {
		t.Errorf("agent.expose.domain: expected agent.example.com, got %s", ds.Agent.Expose.Domain)
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
	if len(ds.Editable) != 2 {
		t.Errorf("editable: expected 2, got %d", len(ds.Editable))
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
		Target: DeploymentTarget{Runtime: "kubernetes", Namespace: "prod"},
		Agent: DeploymentAgent{
			Image:    "agent:latest",
			Port:     8080,
			Replicas: 1,
			Resources: DeploymentResources{
				CPU: "100m", Memory: "256Mi",
			},
			Environment: map[string]string{
				"KEY": "${credentials.API_KEY}",
			},
			Update: UpdateStrategy{Strategy: "rolling"},
		},
		Models: map[string]DeploymentModel{
			"llm": {Image: "ollama:latest", Port: 11434, Replicas: 1},
		},
		Credentials: map[string]DeploymentCredential{
			"API_KEY": {Description: "test key"},
		},
		Observability: DeploymentObservability{Enabled: true, Provider: "galileo"},
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
	if parsed.Agent.Environment["KEY"] != "${credentials.API_KEY}" {
		t.Error("environment lost in round-trip")
	}
	if parsed.Models["llm"].Port != 11434 {
		t.Errorf("models.llm.port: expected 11434, got %d", parsed.Models["llm"].Port)
	}
}

func TestStripCredentialValues(t *testing.T) {
	ds := &AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: DeploymentSource{
			Name: "test", Build: "b1", Registry: "r",
		},
		Agent: DeploymentAgent{Image: "x", Port: 8080},
		Credentials: map[string]DeploymentCredential{
			"API_KEY":     {Value: "sk-secret-key", Description: "API key", Optional: false},
			"SLACK_TOKEN": {Value: "xoxb-secret", Description: "Slack token", Optional: true},
		},
	}

	stripped := StripCredentialValues(ds)

	// Original should be unchanged
	if ds.Credentials["API_KEY"].Value != "sk-secret-key" {
		t.Error("original mutated")
	}

	// Stripped should have empty values but keep metadata
	if stripped.Credentials["API_KEY"].Value != "" {
		t.Errorf("stripped value should be empty, got %s", stripped.Credentials["API_KEY"].Value)
	}
	if stripped.Credentials["API_KEY"].Description != "API key" {
		t.Error("description lost in strip")
	}
	if stripped.Credentials["SLACK_TOKEN"].Value != "" {
		t.Error("slack token value should be empty")
	}
	if !stripped.Credentials["SLACK_TOKEN"].Optional {
		t.Error("optional flag lost in strip")
	}
}

func TestParseDeploymentSpec_WebhookIngestionRequiresPort(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  port: 8080
ingestion:
  data:
    image: ingest:latest
    trigger:
      type: webhook
`
	_, err := ParseDeploymentSpec([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for webhook ingestion without port")
	}
}

func TestParseDeploymentSpec_WebhookIngestionWithPort(t *testing.T) {
	yaml := `
spec: deployment/v1
source:
  name: x
  build: b1
  registry: r
agent:
  image: x
  port: 8080
ingestion:
  data:
    image: ingest:latest
    port: 3001
    trigger:
      type: webhook
`
	ds, err := ParseDeploymentSpec([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ds.Ingestion["data"].Port != 3001 {
		t.Errorf("expected port 3001, got %d", ds.Ingestion["data"].Port)
	}
}
