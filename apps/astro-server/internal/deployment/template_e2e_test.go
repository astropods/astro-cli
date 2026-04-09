package deployment

import (
	"encoding/json"
	"strings"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// ===== End-to-end: YAML → JSON round-trip → template generation =====
// Reproduces the server code path: YAML parse → JSON marshal (storage) → JSON unmarshal → generate template.
// Covers all built-in providers and all ingestion trigger types.

func TestTemplate_E2E_YAMLRoundTrip(t *testing.T) {
	const rawYAML = `
spec: package/v1
name: my-agent

meta:
  description: A comprehensive test agent

agent:
  image: proxy.example.com/testuser/my-agent:test-build
  build:
    context: .
    dockerfile: Dockerfile
  inputs:
    - name: EMBEDDING_MODEL
      datatype: string
      description: Embedding model in provider/model format
      default: nomic-embed-text
      optional: true
    - name: EMBEDDING_DIMENSION
      datatype: number
      description: Vector dimension of the embedding model
      default: "768"
      optional: true

models:
  # Self-hosted
  ollama:
    provider: ollama
    model: qwen3.5:2b
  # Cloud
  anthropic:
    provider: anthropic
  openai:
    provider: openai
  google:
    provider: google
  gemini:
    provider: gemini
  cohere:
    provider: cohere

knowledge:
  # Self-hosted
  cache:
    provider: redis
  docs:
    provider: qdrant
    persistent: true
  graph:
    provider: neo4j
  db:
    provider: postgres
    persistent: true
  # Cloud
  vectors:
    provider: pinecone

providers:
  cloudflare:
    scope: [integrations]
    variables:
      - name: AI_API_KEY
        datatype: string
        description: Cloudflare Workers AI API key
        secret: true
      - name: ACCOUNT_ID
        datatype: string
        description: Cloudflare account ID
        secret: true

integrations:
  # Cloud
  github:
    provider: github
  gitlab:
    provider: gitlab
  # Custom provider
  cloudflare:
    provider: cloudflare

ingestion:
  webhook_ingest:
    container:
      build:
        context: .
        dockerfile: ingestion/webhook/Dockerfile
      port: 3001
    trigger:
      type: webhook
  scheduled_sync:
    container:
      build:
        context: .
        dockerfile: ingestion/sync/Dockerfile
    trigger:
      type: schedule
  boot_loader:
    container:
      build:
        context: .
        dockerfile: ingestion/boot/Dockerfile
    trigger:
      type: startup
  manual_reindex:
    container:
      build:
        context: .
        dockerfile: ingestion/reindex/Dockerfile
    trigger:
      type: manual

dev:
  interfaces: [web]
`

	// Step 1: Parse YAML (same as astro-cli push)
	astroSpec, err := spec.Parse([]byte(rawYAML))
	if err != nil {
		t.Fatalf("Parse YAML: %v", err)
	}

	// Step 2: JSON round-trip (simulates server storage & retrieval)
	jsonBytes, err := json.Marshal(astroSpec)
	if err != nil {
		t.Fatalf("JSON marshal: %v", err)
	}

	var restored spec.AstroSpec
	if err := json.Unmarshal(jsonBytes, &restored); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}

	// Step 3: Generate deployment template
	ds, err := GenerateDeploymentTemplate(TemplateInput{
		Spec:              &restored,
		AgentName:         restored.Name,
		Account:           "testuser",
		BuildID:           "test-build",
		RegistryURL:       "registry.example.com",
		ProxyRegistryHost: "proxy.example.com",
		Environment:       "prod",
	})
	if err != nil {
		t.Fatalf("GenerateDeploymentTemplate: %v", err)
	}

	if ds.Spec != "deployment-template/v1" {
		t.Errorf("spec: expected deployment-template/v1, got %s", ds.Spec)
	}
	if ds.Source.Name != "my-agent" {
		t.Errorf("source.name: expected my-agent, got %s", ds.Source.Name)
	}

	// === Models ===
	// Only ollama deploys a container; anthropic/openai/google/gemini/cohere are cloud-only
	if len(ds.Models) != 1 {
		t.Errorf("models: expected 1 (ollama only), got %d", len(ds.Models))
	}
	if _, ok := ds.Models["ollama"]; !ok {
		t.Error("models: expected ollama entry")
	}

	// === Knowledge ===
	// redis/qdrant/neo4j/postgres deploy containers; pinecone is cloud-only
	if len(ds.Knowledge) != 4 {
		t.Errorf("knowledge: expected 4 (redis/qdrant/neo4j/postgres), got %d", len(ds.Knowledge))
	}
	for _, name := range []string{"cache", "docs", "graph", "db"} {
		if _, ok := ds.Knowledge[name]; !ok {
			t.Errorf("knowledge: missing %s entry", name)
		}
	}

	// === Integrations ===
	// github/gitlab are cloud, cloudflare is custom — none deploy containers
	if len(ds.Tools) != 0 {
		t.Errorf("tools: expected 0 (all cloud/custom providers), got %d", len(ds.Tools))
	}

	// === Ingestion: all four trigger types ===
	if len(ds.Ingestion) != 4 {
		t.Fatalf("ingestion: expected 4, got %d", len(ds.Ingestion))
	}

	// === Variables: all cloud provider credentials ===
	expectedVars := []string{
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY",
		"GEMINI_API_KEY", "COHERE_API_KEY",
		"PINECONE_API_KEY",
		"GITHUB_TOKEN", "GITLAB_TOKEN",
		"CLOUDFLARE_AI_API_KEY", "CLOUDFLARE_ACCOUNT_ID",
		"EMBEDDING_MODEL", "EMBEDDING_DIMENSION",
	}
	for _, key := range expectedVars {
		if _, ok := ds.Variables[key]; !ok {
			t.Errorf("missing variable %s", key)
		}
	}

	// All ${} references should be valid
	for key, val := range ds.Agent.Environment {
		if strings.HasPrefix(val, "${") {
			refs := spec.ParseReferences(val)
			errs := spec.ValidateReferences(refs, ds)
			if len(errs) > 0 {
				t.Errorf("env %s: reference %q does not resolve: %v", key, val, errs)
			}
		}
	}

	// Patch build-only ingestion images for round-trip
	for name, ing := range ds.Ingestion {
		ing.Image = "registry.example.com/" + name + ":test-build"
		ds.Ingestion[name] = ing
	}
	yamlBytes, err := spec.SerializeDeploymentSpec(ds)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	if _, err := spec.ParseDeploymentSpec(yamlBytes); err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
}

// TestTemplate_E2E_StoredJSON reproduces the exact server code path using
// stored JSON from the agent index. Covers all built-in providers, all ingestion
// trigger types, and the DevInterfaces legacy format JSON round-trip.
func TestTemplate_E2E_StoredJSON(t *testing.T) {
	const storedJSON = `{
		"spec": "package/v1",
		"name": "my-agent",
		"meta": {"description": "A comprehensive test agent"},
		"agent": {
			"image": "registry.astropods.ai/testuser/my-agent:abc123",
			"inputs": [
				{"name": "EMBEDDING_MODEL", "datatype": "string", "default": "nomic-embed-text", "description": "Embedding model", "optional": true},
				{"name": "EMBEDDING_DIMENSION", "datatype": "number", "default": "768", "description": "Vector dimension", "optional": true}
			]
		},
		"models": {
			"ollama": {"provider": "ollama", "model": "qwen3.5:2b"},
			"anthropic": {"provider": "anthropic"},
			"openai": {"provider": "openai"},
			"google": {"provider": "google"},
			"gemini": {"provider": "gemini"},
			"cohere": {"provider": "cohere"}
		},
		"knowledge": {
			"cache": {"provider": "redis"},
			"docs": {"provider": "qdrant", "persistent": true},
			"graph": {"provider": "neo4j"},
			"db": {"provider": "postgres", "persistent": true},
			"vectors": {"provider": "pinecone"}
		},
		"providers": {
			"cloudflare": {
				"scope": ["integrations"],
				"variables": [
					{"name": "AI_API_KEY", "datatype": "string", "description": "Cloudflare Workers AI API key", "secret": true},
					{"name": "ACCOUNT_ID", "datatype": "string", "description": "Cloudflare account ID", "secret": true}
				]
			}
		},
		"integrations": {
			"github": {"provider": "github"},
			"gitlab": {"provider": "gitlab"},
			"cloudflare": {"provider": "cloudflare"}
		},
		"ingestion": {
			"webhook_ingest": {"container": {"image": "registry.astropods.ai/testuser/my-agent-webhook:abc123", "port": 3001}, "trigger": {"type": "webhook"}},
			"scheduled_sync": {"container": {"image": "registry.astropods.ai/testuser/my-agent-sync:abc123"}, "trigger": {"type": "schedule"}},
			"boot_loader": {"container": {"image": "registry.astropods.ai/testuser/my-agent-boot:abc123"}, "trigger": {"type": "startup"}},
			"manual_reindex": {"container": {"image": "registry.astropods.ai/testuser/my-agent-reindex:abc123"}, "trigger": {"type": "manual"}}
		},
		"dev": {"interfaces": ["web"]}
	}`

	// === JSON unmarshal (the code path that was failing) ===
	var astroSpec spec.AstroSpec
	if err := json.Unmarshal([]byte(storedJSON), &astroSpec); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}

	// DevInterfaces legacy format regression
	if astroSpec.Dev == nil {
		t.Fatal("dev: expected non-nil")
	}
	if astroSpec.Dev.Interfaces == nil {
		t.Fatal("dev.interfaces: expected non-nil")
	}
	if astroSpec.Dev.Interfaces.Messaging == nil || len(astroSpec.Dev.Interfaces.Messaging.Adapters) != 1 {
		t.Fatalf("dev.interfaces.messaging.adapters: expected [web], got %+v", astroSpec.Dev.Interfaces)
	}

	// === Generate deployment template ===
	ds, err := GenerateDeploymentTemplate(TemplateInput{
		Spec:              &astroSpec,
		AgentName:         astroSpec.Name,
		Account:           "testuser",
		BuildID:           "abc123",
		RegistryURL:       "registry.astropods.ai",
		ProxyRegistryHost: "registry.astropods.ai",
		Environment:       "prod",
	})
	if err != nil {
		t.Fatalf("GenerateDeploymentTemplate: %v", err)
	}

	// === Source & Target ===
	if ds.Spec != "deployment-template/v1" {
		t.Errorf("spec: expected deployment-template/v1, got %s", ds.Spec)
	}
	if ds.Source.Account != "testuser" {
		t.Errorf("source.account: expected testuser, got %s", ds.Source.Account)
	}
	if ds.Source.Name != "my-agent" {
		t.Errorf("source.name: expected my-agent, got %s", ds.Source.Name)
	}
	if ds.Source.Build != "abc123" {
		t.Errorf("source.build: expected abc123, got %s", ds.Source.Build)
	}
	if ds.Target.Runtime != "kubernetes" {
		t.Errorf("target.runtime: expected kubernetes, got %s", ds.Target.Runtime)
	}

	// === Agent ===
	if ds.Agent.Image != "registry.astropods.ai/prod-tenant-testuser/my-agent:abc123" {
		t.Errorf("agent.image: got %s", ds.Agent.Image)
	}
	if ds.Agent.Replicas != 1 {
		t.Errorf("agent.replicas: expected 1, got %d", ds.Agent.Replicas)
	}
	if spec.PrimaryPort(ds.Agent.Endpoints) != 8080 {
		t.Errorf("agent port: expected 8080, got %d", spec.PrimaryPort(ds.Agent.Endpoints))
	}

	// === Models ===
	// Self-hosted: ollama only
	if len(ds.Models) != 1 {
		t.Fatalf("models: expected 1 (ollama only), got %d", len(ds.Models))
	}
	ollama := ds.Models["ollama"]
	if ollama.Provider != "ollama" {
		t.Errorf("models.ollama.provider: expected ollama, got %s", ollama.Provider)
	}
	if ollama.Model != "qwen3.5:2b" {
		t.Errorf("models.ollama.model: expected qwen3.5:2b, got %s", ollama.Model)
	}
	if !strings.Contains(ollama.Image, "ollama/ollama") {
		t.Errorf("models.ollama.image: expected ollama image, got %s", ollama.Image)
	}
	if ollama.GPU == nil {
		t.Error("models.ollama.gpu: expected GPU config")
	}
	if ollama.Healthcheck == nil || len(ollama.Healthcheck.Test) == 0 {
		t.Error("models.ollama.healthcheck: expected model-aware healthcheck")
	}
	// Cloud models must NOT appear in deployment
	for _, cloudModel := range []string{"anthropic", "openai", "google", "gemini", "cohere"} {
		if _, ok := ds.Models[cloudModel]; ok {
			t.Errorf("models.%s: cloud model should not deploy a container", cloudModel)
		}
	}

	// === Knowledge ===
	// Self-hosted: redis, qdrant, neo4j, postgres. Cloud: pinecone excluded.
	if len(ds.Knowledge) != 4 {
		t.Fatalf("knowledge: expected 4, got %d", len(ds.Knowledge))
	}

	// Redis
	cache := ds.Knowledge["cache"]
	if cache.Provider != "redis" {
		t.Errorf("knowledge.cache.provider: expected redis, got %s", cache.Provider)
	}
	if spec.PrimaryPort(cache.Endpoints) != 6379 {
		t.Errorf("knowledge.cache port: expected 6379, got %d", spec.PrimaryPort(cache.Endpoints))
	}
	if cache.Persistent {
		t.Error("knowledge.cache.persistent: expected false")
	}
	if cache.Healthcheck == nil || len(cache.Healthcheck.Test) == 0 {
		t.Error("knowledge.cache.healthcheck: expected redis-cli ping")
	}

	// Qdrant
	docs := ds.Knowledge["docs"]
	if docs.Provider != "qdrant" {
		t.Errorf("knowledge.docs.provider: expected qdrant, got %s", docs.Provider)
	}
	if spec.PrimaryPort(docs.Endpoints) != 6333 {
		t.Errorf("knowledge.docs port: expected 6333, got %d", spec.PrimaryPort(docs.Endpoints))
	}
	if _, ok := docs.Endpoints["grpc"]; !ok {
		t.Error("knowledge.docs: expected grpc endpoint for qdrant")
	}
	if !docs.Persistent {
		t.Error("knowledge.docs.persistent: expected true")
	}
	if docs.Storage == nil {
		t.Error("knowledge.docs.storage: expected non-nil for persistent store")
	}
	if docs.Update.Strategy != "recreate" {
		t.Errorf("knowledge.docs.update.strategy: expected recreate for persistent, got %s", docs.Update.Strategy)
	}
	if docs.Healthcheck == nil || docs.Healthcheck.Path != "/healthz" {
		t.Error("knowledge.docs.healthcheck: expected /healthz path")
	}

	// Neo4j
	graph := ds.Knowledge["graph"]
	if graph.Provider != "neo4j" {
		t.Errorf("knowledge.graph.provider: expected neo4j, got %s", graph.Provider)
	}
	if spec.PrimaryPort(graph.Endpoints) != 7474 {
		t.Errorf("knowledge.graph port: expected 7474, got %d", spec.PrimaryPort(graph.Endpoints))
	}
	if _, ok := graph.Endpoints["bolt"]; !ok {
		t.Error("knowledge.graph: expected bolt endpoint for neo4j")
	}
	if graph.Healthcheck == nil || graph.Healthcheck.Path != "/" {
		t.Error("knowledge.graph.healthcheck: expected / path")
	}
	if graph.Environment["NEO4J_AUTH"] != "none" {
		t.Errorf("knowledge.graph.environment.NEO4J_AUTH: expected none, got %s", graph.Environment["NEO4J_AUTH"])
	}

	// Postgres
	db := ds.Knowledge["db"]
	if db.Provider != "postgres" {
		t.Errorf("knowledge.db.provider: expected postgres, got %s", db.Provider)
	}
	if spec.PrimaryPort(db.Endpoints) != 5432 {
		t.Errorf("knowledge.db port: expected 5432, got %d", spec.PrimaryPort(db.Endpoints))
	}
	if !db.Persistent {
		t.Error("knowledge.db.persistent: expected true")
	}
	if db.Storage == nil {
		t.Error("knowledge.db.storage: expected non-nil for persistent store")
	}
	if db.Healthcheck == nil || len(db.Healthcheck.Test) == 0 {
		t.Error("knowledge.db.healthcheck: expected pg_isready check")
	}

	// Pinecone (cloud) must NOT appear
	if _, ok := ds.Knowledge["vectors"]; ok {
		t.Error("knowledge.vectors: cloud provider (pinecone) should not deploy a container")
	}

	// === Integrations: all cloud/custom, no containers ===
	if len(ds.Tools) != 0 {
		keys := make([]string, 0, len(ds.Tools))
		for k := range ds.Tools {
			keys = append(keys, k)
		}
		t.Errorf("tools: expected 0 (all cloud/custom providers), got %d — keys: %v", len(ds.Tools), keys)
	}

	// === Ingestion: all four trigger types ===
	if len(ds.Ingestion) != 4 {
		t.Fatalf("ingestion: expected 4, got %d", len(ds.Ingestion))
	}

	// Webhook — has endpoints with port
	webhookIng := ds.Ingestion["webhook_ingest"]
	if webhookIng.Trigger.Type != "webhook" {
		t.Errorf("ingestion.webhook_ingest.trigger.type: expected webhook, got %s", webhookIng.Trigger.Type)
	}
	if spec.PrimaryPort(webhookIng.Endpoints) != 3001 {
		t.Errorf("ingestion.webhook_ingest port: expected 3001, got %d", spec.PrimaryPort(webhookIng.Endpoints))
	}
	if webhookIng.Image != "registry.astropods.ai/prod-tenant-testuser/my-agent-webhook:abc123" {
		t.Errorf("ingestion.webhook_ingest.image: got %s", webhookIng.Image)
	}

	// Schedule — empty schedule placeholder, no endpoints
	schedIng := ds.Ingestion["scheduled_sync"]
	if schedIng.Trigger.Type != "schedule" {
		t.Errorf("ingestion.scheduled_sync.trigger.type: expected schedule, got %s", schedIng.Trigger.Type)
	}
	if schedIng.Trigger.Schedule != "" {
		t.Errorf("ingestion.scheduled_sync.trigger.schedule: expected empty placeholder, got %s", schedIng.Trigger.Schedule)
	}
	if len(schedIng.Endpoints) != 0 {
		t.Errorf("ingestion.scheduled_sync.endpoints: expected none, got %d", len(schedIng.Endpoints))
	}

	// Startup — no schedule, no endpoints
	startupIng := ds.Ingestion["boot_loader"]
	if startupIng.Trigger.Type != "startup" {
		t.Errorf("ingestion.boot_loader.trigger.type: expected startup, got %s", startupIng.Trigger.Type)
	}
	if len(startupIng.Endpoints) != 0 {
		t.Errorf("ingestion.boot_loader.endpoints: expected none, got %d", len(startupIng.Endpoints))
	}

	// Manual — no schedule, no endpoints
	manualIng := ds.Ingestion["manual_reindex"]
	if manualIng.Trigger.Type != "manual" {
		t.Errorf("ingestion.manual_reindex.trigger.type: expected manual, got %s", manualIng.Trigger.Type)
	}
	if len(manualIng.Endpoints) != 0 {
		t.Errorf("ingestion.manual_reindex.endpoints: expected none, got %d", len(manualIng.Endpoints))
	}

	// All ingestion entries should have standard resources
	for name, ing := range ds.Ingestion {
		if ing.Resources.CPU == "" {
			t.Errorf("ingestion.%s.resources: expected standard resources", name)
		}
	}

	// === Interfaces: no explicit interfaces → defaults to messaging ===
	if ds.Interfaces == nil {
		t.Fatal("interfaces: expected non-nil (messaging enabled by default)")
	}
	if len(ds.Interfaces.Adapters) != 0 {
		t.Errorf("interfaces.adapters: expected empty (user fills in), got %v", ds.Interfaces.Adapters)
	}

	// === Variables: all provider credentials ===
	// Cloud model credentials
	cloudModelVars := map[string]string{
		"ANTHROPIC_API_KEY": "Anthropic",
		"OPENAI_API_KEY":    "OpenAI",
		"GOOGLE_API_KEY":    "Google",
		"GEMINI_API_KEY":    "Gemini",
		"COHERE_API_KEY":    "Cohere",
	}
	for key, provider := range cloudModelVars {
		v, ok := ds.Variables[key]
		if !ok {
			t.Errorf("missing %s variable (%s)", key, provider)
			continue
		}
		if !v.Secret {
			t.Errorf("%s should be secret", key)
		}
	}

	// Cloud knowledge credentials
	if v, ok := ds.Variables["PINECONE_API_KEY"]; !ok {
		t.Error("missing PINECONE_API_KEY variable")
	} else if !v.Secret {
		t.Error("PINECONE_API_KEY should be secret")
	}

	// Cloud tool credentials
	if _, ok := ds.Variables["GITHUB_TOKEN"]; !ok {
		t.Error("missing GITHUB_TOKEN variable")
	}
	if _, ok := ds.Variables["GITLAB_TOKEN"]; !ok {
		t.Error("missing GITLAB_TOKEN variable")
	}

	// Custom provider (cloudflare) credentials
	if v, ok := ds.Variables["CLOUDFLARE_AI_API_KEY"]; !ok {
		t.Error("missing CLOUDFLARE_AI_API_KEY variable")
	} else if !v.Secret {
		t.Error("CLOUDFLARE_AI_API_KEY should be secret")
	}
	if v, ok := ds.Variables["CLOUDFLARE_ACCOUNT_ID"]; !ok {
		t.Error("missing CLOUDFLARE_ACCOUNT_ID variable")
	} else if !v.Secret {
		t.Error("CLOUDFLARE_ACCOUNT_ID should be secret")
	}

	// Agent inputs as variables
	if v, ok := ds.Variables["EMBEDDING_MODEL"]; !ok {
		t.Error("missing EMBEDDING_MODEL variable")
	} else {
		if v.Datatype != "string" {
			t.Errorf("EMBEDDING_MODEL.datatype: expected string, got %s", v.Datatype)
		}
		if v.Default != "nomic-embed-text" {
			t.Errorf("EMBEDDING_MODEL.default: expected nomic-embed-text, got %s", v.Default)
		}
		if v.Value != "nomic-embed-text" {
			t.Errorf("EMBEDDING_MODEL.value: expected default pre-filled, got %s", v.Value)
		}
		if !v.Optional {
			t.Error("EMBEDDING_MODEL.optional: expected true")
		}
	}
	if v, ok := ds.Variables["EMBEDDING_DIMENSION"]; !ok {
		t.Error("missing EMBEDDING_DIMENSION variable")
	} else {
		if v.Datatype != "number" {
			t.Errorf("EMBEDDING_DIMENSION.datatype: expected number, got %s", v.Datatype)
		}
		if v.Default != "768" {
			t.Errorf("EMBEDDING_DIMENSION.default: expected 768, got %s", v.Default)
		}
	}

	// Slack adapter tokens (from messaging interfaces)
	if v, ok := ds.Variables["SLACK_BOT_TOKEN"]; !ok {
		t.Error("missing SLACK_BOT_TOKEN variable")
	} else if !v.Optional {
		t.Error("SLACK_BOT_TOKEN should be optional")
	}
	if _, ok := ds.Variables["SLACK_APP_TOKEN"]; !ok {
		t.Error("missing SLACK_APP_TOKEN variable")
	}

	// === Agent environment wiring ===
	env := ds.Agent.Environment

	// Self-hosted model refs
	assertEnvRef(t, env, "OLLAMA_HOST", "${models.ollama.host}")
	assertEnvExists(t, env, "OLLAMA_PORT")
	assertEnvExists(t, env, "OLLAMA_URL")
	assertEnvExists(t, env, "OLLAMA_BASE_URL")
	assertEnvRef(t, env, "OLLAMA_MODEL", "qwen3.5:2b")

	// Self-hosted knowledge refs
	assertEnvRef(t, env, "REDIS_HOST", "${knowledge.cache.host}")
	assertEnvExists(t, env, "REDIS_PORT")
	assertEnvRef(t, env, "QDRANT_HOST", "${knowledge.docs.host}")
	assertEnvExists(t, env, "QDRANT_PORT")
	assertEnvRef(t, env, "NEO4J_HOST", "${knowledge.graph.host}")
	assertEnvExists(t, env, "NEO4J_PORT")
	assertEnvRef(t, env, "POSTGRES_HOST", "${knowledge.db.host}")
	assertEnvExists(t, env, "POSTGRES_PORT")

	// Cloud credential refs wired into agent env
	assertEnvRef(t, env, "ANTHROPIC_API_KEY", "${variables.ANTHROPIC_API_KEY}")
	assertEnvRef(t, env, "OPENAI_API_KEY", "${variables.OPENAI_API_KEY}")
	assertEnvRef(t, env, "GOOGLE_API_KEY", "${variables.GOOGLE_API_KEY}")
	assertEnvRef(t, env, "GEMINI_API_KEY", "${variables.GEMINI_API_KEY}")
	assertEnvRef(t, env, "COHERE_API_KEY", "${variables.COHERE_API_KEY}")
	assertEnvRef(t, env, "PINECONE_API_KEY", "${variables.PINECONE_API_KEY}")
	assertEnvRef(t, env, "GITHUB_TOKEN", "${variables.GITHUB_TOKEN}")
	assertEnvRef(t, env, "GITLAB_TOKEN", "${variables.GITLAB_TOKEN}")
	assertEnvRef(t, env, "CLOUDFLARE_AI_API_KEY", "${variables.CLOUDFLARE_AI_API_KEY}")
	assertEnvRef(t, env, "CLOUDFLARE_ACCOUNT_ID", "${variables.CLOUDFLARE_ACCOUNT_ID}")

	// Agent inputs wired as ${variables.*} references (default value stored on the variable, not hardcoded)
	assertEnvRef(t, env, "EMBEDDING_MODEL", "${variables.EMBEDDING_MODEL}")
	assertEnvRef(t, env, "EMBEDDING_DIMENSION", "${variables.EMBEDDING_DIMENSION}")

	// Platform metadata
	assertEnvRef(t, env, "ASTRO_AGENT_NAME", "${source.name}")
	assertEnvRef(t, env, "ASTRO_AGENT_BUILD", "${source.build}")

	// === All ${} references should resolve ===
	for key, val := range env {
		if strings.HasPrefix(val, "${") {
			refs := spec.ParseReferences(val)
			errs := spec.ValidateReferences(refs, ds)
			if len(errs) > 0 {
				t.Errorf("env %s: reference %q does not resolve: %v", key, val, errs)
			}
		}
	}

	// === Observability ===
	if !ds.Observability.Enabled {
		t.Error("observability.enabled: expected true")
	}
	if ds.Observability.Provider != "langfuse" {
		t.Errorf("observability.provider: expected langfuse, got %s", ds.Observability.Provider)
	}

	// === Editable fields ===
	if len(ds.Editable) == 0 {
		t.Error("editable: expected non-empty")
	}

	// === YAML round-trip ===
	yamlBytes, err := spec.SerializeDeploymentSpec(ds)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	parsed, err := spec.ParseDeploymentSpec(yamlBytes)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if parsed.Source.Name != "my-agent" {
		t.Errorf("round-trip source.name: expected my-agent, got %s", parsed.Source.Name)
	}
	if len(parsed.Models) != 1 {
		t.Errorf("round-trip models: expected 1, got %d", len(parsed.Models))
	}
	if len(parsed.Knowledge) != 4 {
		t.Errorf("round-trip knowledge: expected 4, got %d", len(parsed.Knowledge))
	}
	if len(parsed.Ingestion) != 4 {
		t.Errorf("round-trip ingestion: expected 4, got %d", len(parsed.Ingestion))
	}
}
