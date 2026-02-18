package deployment

import (
	"strings"
	"testing"

	"github.com/postman/astro/packages/astro-spec"
)

// --- helpers ---

func baseInput() TemplateInput {
	return TemplateInput{
		Spec: &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "registry.example.com/my-agent:abc123"},
		},
		Account:     "acme",
		BuildID:     "abc123",
		RegistryURL: "registry.example.com",
	}
}

func mustGenerate(t *testing.T, input TemplateInput) *spec.AstroDeploymentSpec {
	t.Helper()
	ds, err := GenerateDeploymentTemplate(input)
	if err != nil {
		t.Fatalf("GenerateDeploymentTemplate: %v", err)
	}
	return ds
}

func assertEnvRef(t *testing.T, env map[string]string, key, expectedRef string) {
	t.Helper()
	val, ok := env[key]
	if !ok {
		t.Errorf("expected env var %s, not found. env keys: %v", key, mapKeys(env))
		return
	}
	if val != expectedRef {
		t.Errorf("env %s: expected %q, got %q", key, expectedRef, val)
	}
}

func assertEnvExists(t *testing.T, env map[string]string, key string) {
	t.Helper()
	if _, ok := env[key]; !ok {
		t.Errorf("expected env var %s, not found. env keys: %v", key, mapKeys(env))
	}
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ===== Phase 1: Source / Target / Metadata =====

func TestTemplate_SourceMetadata(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	if ds.Spec != "deployment/v1" {
		t.Errorf("spec: expected deployment/v1, got %s", ds.Spec)
	}
	if ds.Source.Account != "acme" {
		t.Errorf("source.account: expected acme, got %s", ds.Source.Account)
	}
	if ds.Source.Name != "my-agent" {
		t.Errorf("source.name: expected my-agent, got %s", ds.Source.Name)
	}
	if ds.Source.Build != "abc123" {
		t.Errorf("source.build: expected abc123, got %s", ds.Source.Build)
	}
	if ds.Source.Registry != "registry.example.com" {
		t.Errorf("source.registry: expected registry.example.com, got %s", ds.Source.Registry)
	}
}

func TestTemplate_TargetDefaults(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	if ds.Target.Runtime != "kubernetes" {
		t.Errorf("target.runtime: expected kubernetes, got %s", ds.Target.Runtime)
	}
	if ds.Target.Namespace != "" {
		t.Errorf("target.namespace: expected empty placeholder, got %s", ds.Target.Namespace)
	}
}

func TestTemplate_ObservabilityDefaults(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	if !ds.Observability.Enabled {
		t.Error("observability.enabled: expected true")
	}
	if ds.Observability.Provider != "galileo" {
		t.Errorf("observability.provider: expected galileo, got %s", ds.Observability.Provider)
	}
}

func TestTemplate_EditableFieldsPresent(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	if len(ds.Editable) == 0 {
		t.Fatal("editable: expected non-empty list")
	}

	// Spot-check critical editable paths
	expected := []string{
		"target.namespace",
		"agent.replicas",
		"agent.environment",
		"credentials.*.value",
		"interfaces.adapters",
	}
	editSet := make(map[string]bool, len(ds.Editable))
	for _, e := range ds.Editable {
		editSet[e] = true
	}
	for _, e := range expected {
		if !editSet[e] {
			t.Errorf("editable: missing expected field path %q", e)
		}
	}
}

// ===== Phase 2: Agent Block =====

func TestTemplate_AgentBlock(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	if ds.Agent.Image != "registry.example.com/my-agent:abc123" {
		t.Errorf("agent.image: got %s", ds.Agent.Image)
	}
	if ds.Agent.Port != 8080 {
		t.Errorf("agent.port: expected 8080, got %d", ds.Agent.Port)
	}
	if ds.Agent.Replicas != 1 {
		t.Errorf("agent.replicas: expected 1, got %d", ds.Agent.Replicas)
	}
	if ds.Agent.Resources != spec.StandardResources {
		t.Errorf("agent.resources: expected StandardResources, got %+v", ds.Agent.Resources)
	}
	if ds.Agent.Update.Strategy != "rolling" {
		t.Errorf("agent.update.strategy: expected rolling, got %s", ds.Agent.Update.Strategy)
	}
	if ds.Agent.Expose.Enabled {
		t.Error("agent.expose.enabled: expected false")
	}
}

func TestTemplate_AgentImageFallback(t *testing.T) {
	// When astro-spec has no image, template constructs from registry/name:build
	input := baseInput()
	input.Spec.Agent.Image = ""

	ds := mustGenerate(t, input)

	expected := "registry.example.com/my-agent:abc123"
	if ds.Agent.Image != expected {
		t.Errorf("agent.image fallback: expected %s, got %s", expected, ds.Agent.Image)
	}
}

func TestTemplate_AgentHealthcheckPassthrough(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Healthcheck = &spec.Healthcheck{Path: "/health", Interval: "30s"}

	ds := mustGenerate(t, input)

	if ds.Agent.Healthcheck == nil {
		t.Fatal("agent.healthcheck: expected non-nil")
	}
	if ds.Agent.Healthcheck.Path != "/health" {
		t.Errorf("agent.healthcheck.path: expected /health, got %s", ds.Agent.Healthcheck.Path)
	}
}

func TestTemplate_PlatformEnvVars(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	assertEnvRef(t, ds.Agent.Environment, "ASTRO_AGENT_NAME", "${source.name}")
	assertEnvRef(t, ds.Agent.Environment, "ASTRO_AGENT_BUILD", "${source.build}")
}

// ===== Phase 3: Models =====

func TestTemplate_ProviderModel(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"local_llm": {Provider: "ollama"},
	}

	ds := mustGenerate(t, input)

	m, ok := ds.Models["local_llm"]
	if !ok {
		t.Fatal("models.local_llm: not found")
	}
	if m.Image != "ollama/ollama:latest" {
		t.Errorf("models.local_llm.image: expected ollama/ollama:latest, got %s", m.Image)
	}
	if m.Port != 11434 {
		t.Errorf("models.local_llm.port: expected 11434, got %d", m.Port)
	}
	if m.Replicas != 1 {
		t.Errorf("models.local_llm.replicas: expected 1, got %d", m.Replicas)
	}

	// Check agent environment references — provider-specific env vars
	assertEnvRef(t, ds.Agent.Environment, "OLLAMA_HOST", "${models.local_llm.host}")
	assertEnvRef(t, ds.Agent.Environment, "OLLAMA_PORT", "${models.local_llm.port}")
	assertEnvRef(t, ds.Agent.Environment, "OLLAMA_URL", "${models.local_llm.url}")
	assertEnvRef(t, ds.Agent.Environment, "OLLAMA_BASE_URL", "${models.local_llm.url}/api")
}

func TestTemplate_ContainerModel(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"embedder": {
			Container: &spec.ContainerConfig{
				Image: "custom-embedder:v1",
				Port:  9999,
				Environment: map[string]string{
					"MODEL_NAME": "all-MiniLM-L6-v2",
				},
			},
		},
	}

	ds := mustGenerate(t, input)

	m := ds.Models["embedder"]
	if m.Image != "custom-embedder:v1" {
		t.Errorf("image: expected custom-embedder:v1, got %s", m.Image)
	}
	if m.Port != 9999 {
		t.Errorf("port: expected 9999, got %d", m.Port)
	}
	if m.Environment["MODEL_NAME"] != "all-MiniLM-L6-v2" {
		t.Errorf("environment: MODEL_NAME not preserved")
	}
}

func TestTemplate_GPUModel(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"llm": {
			Container: &spec.ContainerConfig{
				Image: "vllm:latest",
				Port:  8000,
				GPU:   &spec.GPUConfig{VRAM: "24Gi"},
			},
		},
	}

	ds := mustGenerate(t, input)

	m := ds.Models["llm"]
	if m.GPU == nil {
		t.Fatal("gpu: expected non-nil")
	}
	if m.GPU.VRAM != "24Gi" {
		t.Errorf("gpu.vram: expected 24Gi, got %s", m.GPU.VRAM)
	}
	if m.GPU.Runtime != "cuda" {
		t.Errorf("gpu.runtime: expected cuda default, got %s", m.GPU.Runtime)
	}
	if m.GPU.Count != 1 {
		t.Errorf("gpu.count: expected 1, got %d", m.GPU.Count)
	}
	if m.Resources != spec.GPUResources {
		t.Errorf("resources: expected GPUResources for GPU model, got %+v", m.Resources)
	}
	if m.Update.Strategy != "recreate" {
		t.Errorf("update.strategy: expected recreate for GPU model, got %s", m.Update.Strategy)
	}
}

func TestTemplate_GPUModelCustomRuntime(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"llm": {
			Container: &spec.ContainerConfig{
				Image: "vllm:latest",
				Port:  8000,
				GPU:   &spec.GPUConfig{VRAM: "16Gi", Runtime: "rocm"},
			},
		},
	}

	ds := mustGenerate(t, input)
	if ds.Models["llm"].GPU.Runtime != "rocm" {
		t.Errorf("gpu.runtime: expected rocm, got %s", ds.Models["llm"].GPU.Runtime)
	}
}

func TestTemplate_ModelDefaultPort(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"noport": {
			Container: &spec.ContainerConfig{Image: "model:latest"},
		},
	}

	ds := mustGenerate(t, input)
	if ds.Models["noport"].Port != 8080 {
		t.Errorf("port: expected 8080 default, got %d", ds.Models["noport"].Port)
	}
}

// ===== Phase 4: Knowledge =====

func TestTemplate_ProviderKnowledge_Qdrant(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"docs": {Provider: "qdrant", Persistent: true},
	}

	ds := mustGenerate(t, input)

	k := ds.Knowledge["docs"]
	if k.Image != "qdrant/qdrant:latest" {
		t.Errorf("image: expected qdrant/qdrant:latest, got %s", k.Image)
	}
	if k.Port != 6333 {
		t.Errorf("port: expected 6333, got %d", k.Port)
	}
	if !k.Persistent {
		t.Error("persistent: expected true")
	}
	if k.Storage == nil {
		t.Fatal("storage: expected non-nil for persistent store")
	}
	if k.Storage.Size != "10Gi" {
		t.Errorf("storage.size: expected 10Gi, got %s", k.Storage.Size)
	}
	if k.Storage.AccessMode != "ReadWriteOnce" {
		t.Errorf("storage.access_mode: expected ReadWriteOnce, got %s", k.Storage.AccessMode)
	}
	if k.Update.Strategy != "recreate" {
		t.Errorf("update.strategy: expected recreate for persistent store, got %s", k.Update.Strategy)
	}

	// Qdrant has HTTP health path
	if k.Healthcheck == nil || k.Healthcheck.Path != "/healthz" {
		t.Errorf("healthcheck: expected path /healthz for qdrant provider, got %+v", k.Healthcheck)
	}

	// Env uses provider prefix QDRANT_*
	assertEnvRef(t, ds.Agent.Environment, "QDRANT_HOST", "${knowledge.docs.host}")
	assertEnvRef(t, ds.Agent.Environment, "QDRANT_PORT", "${knowledge.docs.port}")
	assertEnvRef(t, ds.Agent.Environment, "QDRANT_URL", "${knowledge.docs.url}")
}

func TestTemplate_ProviderKnowledge_Redis(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"cache": {Provider: "redis"},
	}

	ds := mustGenerate(t, input)

	k := ds.Knowledge["cache"]
	if k.Image != "redis:7-alpine" {
		t.Errorf("image: expected redis:7-alpine, got %s", k.Image)
	}
	if k.Port != 6379 {
		t.Errorf("port: expected 6379, got %d", k.Port)
	}
	if k.Persistent {
		t.Error("persistent: expected false (not set in spec)")
	}
	if k.Storage != nil {
		t.Error("storage: expected nil for non-persistent store")
	}

	// Redis has exec health check
	if k.Healthcheck == nil || len(k.Healthcheck.Test) == 0 {
		t.Errorf("healthcheck: expected exec test for redis provider, got %+v", k.Healthcheck)
	}

	assertEnvRef(t, ds.Agent.Environment, "REDIS_HOST", "${knowledge.cache.host}")
	assertEnvRef(t, ds.Agent.Environment, "REDIS_PORT", "${knowledge.cache.port}")
	assertEnvRef(t, ds.Agent.Environment, "REDIS_URL", "${knowledge.cache.url}")
}

func TestTemplate_ProviderKnowledge_Neo4j(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"graph": {Provider: "neo4j"},
	}

	ds := mustGenerate(t, input)

	k := ds.Knowledge["graph"]
	if k.Image != "neo4j:5-community" {
		t.Errorf("image: expected neo4j:5-community, got %s", k.Image)
	}
	if k.Port != 7474 {
		t.Errorf("port: expected 7474, got %d", k.Port)
	}

	// Neo4j has default env vars
	if k.Environment == nil || k.Environment["NEO4J_AUTH"] != "none" {
		t.Errorf("environment: expected NEO4J_AUTH=none, got %v", k.Environment)
	}
}

func TestTemplate_ContainerKnowledge(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"custom_db": {
			Container: &spec.ContainerConfig{
				Image: "my-db:latest",
				Port:  5000,
			},
		},
	}

	ds := mustGenerate(t, input)

	k := ds.Knowledge["custom_db"]
	if k.Image != "my-db:latest" {
		t.Errorf("image: expected my-db:latest, got %s", k.Image)
	}
	if k.Port != 5000 {
		t.Errorf("port: expected 5000, got %d", k.Port)
	}

	// Container mode uses KNOWLEDGE_* prefix
	assertEnvRef(t, ds.Agent.Environment, "KNOWLEDGE_CUSTOM-DB_HOST", "${knowledge.custom_db.host}")
	assertEnvRef(t, ds.Agent.Environment, "KNOWLEDGE_CUSTOM-DB_PORT", "${knowledge.custom_db.port}")
}

func TestTemplate_KnowledgeNonPersistent_NoStorage(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"cache": {Provider: "qdrant", Persistent: false},
	}

	ds := mustGenerate(t, input)

	k := ds.Knowledge["cache"]
	if k.Persistent {
		t.Error("persistent: expected false")
	}
	if k.Storage != nil {
		t.Error("storage: expected nil when not persistent")
	}
	if k.Update.Strategy != "rolling" {
		t.Errorf("update.strategy: expected rolling for non-persistent, got %s", k.Update.Strategy)
	}
}

// ===== Phase 5: Tools =====

func TestTemplate_Tool(t *testing.T) {
	input := baseInput()
	input.Spec.Tools = map[string]spec.Tool{
		"websearch": {
			Container: &spec.ContainerConfig{
				Image: "search:v2",
				Port:  3000,
			},
		},
	}

	ds := mustGenerate(t, input)

	tool := ds.Tools["websearch"]
	if tool.Image != "search:v2" {
		t.Errorf("image: expected search:v2, got %s", tool.Image)
	}
	if tool.Port != 3000 {
		t.Errorf("port: expected 3000, got %d", tool.Port)
	}
	if tool.Replicas != 1 {
		t.Errorf("replicas: expected 1, got %d", tool.Replicas)
	}
	if tool.Resources != spec.StandardResources {
		t.Errorf("resources: expected StandardResources, got %+v", tool.Resources)
	}

	assertEnvRef(t, ds.Agent.Environment, "TOOL_WEBSEARCH_HOST", "${tools.websearch.host}")
	assertEnvRef(t, ds.Agent.Environment, "TOOL_WEBSEARCH_PORT", "${tools.websearch.port}")
	assertEnvRef(t, ds.Agent.Environment, "TOOL_WEBSEARCH_URL", "${tools.websearch.url}")
}

func TestTemplate_ToolDefaultPort(t *testing.T) {
	input := baseInput()
	input.Spec.Tools = map[string]spec.Tool{
		"noport": {Container: &spec.ContainerConfig{Image: "tool:latest"}},
	}

	ds := mustGenerate(t, input)
	if ds.Tools["noport"].Port != 8080 {
		t.Errorf("port: expected 8080 default, got %d", ds.Tools["noport"].Port)
	}
}

func TestTemplate_ToolEnvironmentPassthrough(t *testing.T) {
	input := baseInput()
	input.Spec.Tools = map[string]spec.Tool{
		"mcp": {
			Container: &spec.ContainerConfig{
				Image:       "mcp:latest",
				Environment: map[string]string{"WORKERS": "4"},
			},
		},
	}

	ds := mustGenerate(t, input)
	if ds.Tools["mcp"].Environment["WORKERS"] != "4" {
		t.Error("tool environment not preserved")
	}
}

// ===== Phase 6: Ingestion =====

func TestTemplate_IngestionSchedule(t *testing.T) {
	input := baseInput()
	input.Spec.Ingestion = map[string]spec.Ingestion{
		"sync": {
			Container: spec.ContainerConfig{
				Image:       "sync:latest",
				Environment: map[string]string{"TARGET": "docs"},
			},
			Trigger: spec.IngestionTrigger{Type: "schedule"},
		},
	}

	ds := mustGenerate(t, input)

	ing := ds.Ingestion["sync"]
	if ing.Image != "sync:latest" {
		t.Errorf("image: expected sync:latest, got %s", ing.Image)
	}
	if ing.Trigger.Type != "schedule" {
		t.Errorf("trigger.type: expected schedule, got %s", ing.Trigger.Type)
	}
	if ing.Trigger.Schedule != "" {
		t.Errorf("trigger.schedule: expected empty placeholder, got %s", ing.Trigger.Schedule)
	}
	if ing.Environment["TARGET"] != "docs" {
		t.Error("environment not preserved")
	}
	if ing.Resources != spec.StandardResources {
		t.Errorf("resources: expected StandardResources, got %+v", ing.Resources)
	}
}

func TestTemplate_IngestionWebhookPort(t *testing.T) {
	input := baseInput()
	input.Spec.Ingestion = map[string]spec.Ingestion{
		"data": {
			Container: spec.ContainerConfig{
				Image: "registry.example.com/ingestion:latest",
				Port:  3001,
			},
			Trigger: spec.IngestionTrigger{Type: "webhook"},
		},
	}

	ds := mustGenerate(t, input)

	ing := ds.Ingestion["data"]
	if ing.Port != 3001 {
		t.Errorf("port: expected 3001, got %d", ing.Port)
	}
	if ing.Trigger.Type != "webhook" {
		t.Errorf("trigger.type: expected webhook, got %s", ing.Trigger.Type)
	}
}

func TestTemplate_IngestionWebhookNoPort(t *testing.T) {
	input := baseInput()
	input.Spec.Ingestion = map[string]spec.Ingestion{
		"data": {
			Container: spec.ContainerConfig{
				Image: "registry.example.com/ingestion:latest",
				// No port — template should carry 0; validator rejects at deploy time
			},
			Trigger: spec.IngestionTrigger{Type: "webhook"},
		},
	}

	ds := mustGenerate(t, input)

	if ds.Ingestion["data"].Port != 0 {
		t.Errorf("port: expected 0 (unset), got %d", ds.Ingestion["data"].Port)
	}
}

func TestTemplate_IngestionAllTypes(t *testing.T) {
	input := baseInput()
	input.Spec.Ingestion = map[string]spec.Ingestion{
		"sched":   {Container: spec.ContainerConfig{Image: "i:1"}, Trigger: spec.IngestionTrigger{Type: "schedule"}},
		"startup": {Container: spec.ContainerConfig{Image: "i:2"}, Trigger: spec.IngestionTrigger{Type: "startup"}},
		"webhook": {Container: spec.ContainerConfig{Image: "i:3", Port: 9000}, Trigger: spec.IngestionTrigger{Type: "webhook"}},
		"manual":  {Container: spec.ContainerConfig{Image: "i:4"}, Trigger: spec.IngestionTrigger{Type: "manual"}},
	}

	ds := mustGenerate(t, input)

	if len(ds.Ingestion) != 4 {
		t.Fatalf("expected 4 ingestion entries, got %d", len(ds.Ingestion))
	}
	if ds.Ingestion["sched"].Trigger.Type != "schedule" {
		t.Error("schedule type not preserved")
	}
	if ds.Ingestion["webhook"].Port != 9000 {
		t.Errorf("webhook port: expected 9000, got %d", ds.Ingestion["webhook"].Port)
	}
}

// ===== Phase 7: Credentials =====

func TestTemplate_CredentialsFromCloudProviders(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"anthropic": {Provider: "anthropic"},
	}
	input.Spec.Tools = map[string]spec.Tool{
		"github": {Provider: "github"},
	}

	ds := mustGenerate(t, input)

	if len(ds.Credentials) < 2 {
		t.Fatalf("expected at least 2 credentials, got %d", len(ds.Credentials))
	}

	// Anthropic
	cred, ok := ds.Credentials["ANTHROPIC_API_KEY"]
	if !ok {
		t.Fatal("credentials: ANTHROPIC_API_KEY not found")
	}
	if cred.Value != "" {
		t.Errorf("credential value: expected empty placeholder, got %s", cred.Value)
	}
	if cred.Description == "" {
		t.Error("credential description: expected non-empty")
	}

	// GitHub
	if _, ok := ds.Credentials["GITHUB_TOKEN"]; !ok {
		t.Fatal("credentials: GITHUB_TOKEN not found")
	}

	// Cloud models should NOT appear in ds.Models (no container)
	if len(ds.Models) != 0 {
		t.Errorf("cloud models should not be in deployment spec, got %d", len(ds.Models))
	}
	// Cloud tools should NOT appear in ds.Tools
	if len(ds.Tools) != 0 {
		t.Errorf("cloud tools should not be in deployment spec, got %d", len(ds.Tools))
	}

	// Check agent env references wired for credentials
	assertEnvRef(t, ds.Agent.Environment, "ANTHROPIC_API_KEY", "${credentials.ANTHROPIC_API_KEY}")
	assertEnvRef(t, ds.Agent.Environment, "GITHUB_TOKEN", "${credentials.GITHUB_TOKEN}")
}

func TestTemplate_CredentialsIntegration(t *testing.T) {
	input := baseInput()
	input.Spec.Integrations = map[string]spec.Integration{
		"myapi": {
			Credentials: []spec.CustomCredential{
				{Suffix: "API_KEY", Description: "main key"},
				{Suffix: "SECRET", Description: "optional secret", Optional: true},
			},
		},
	}

	ds := mustGenerate(t, input)

	if _, ok := ds.Credentials["MYAPI_API_KEY"]; !ok {
		t.Error("credentials: MYAPI_API_KEY not found")
	}
	sec, ok := ds.Credentials["MYAPI_SECRET"]
	if !ok {
		t.Fatal("credentials: MYAPI_SECRET not found")
	}
	if !sec.Optional {
		t.Error("expected MYAPI_SECRET to be optional")
	}
}

func TestTemplate_NoIntegrations_NoCredentials(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	if len(ds.Credentials) != 0 {
		t.Errorf("expected 0 credentials for spec without integrations, got %d", len(ds.Credentials))
	}
}

func TestTemplate_NameDerivedCredentialKeys(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"fallback": {Provider: "anthropic"},
	}

	ds := mustGenerate(t, input)

	// Single entry uses provider-prefixed key: ANTHROPIC_API_KEY
	if _, ok := ds.Credentials["ANTHROPIC_API_KEY"]; !ok {
		t.Error("expected ANTHROPIC_API_KEY from provider-prefixed key")
	}
	assertEnvRef(t, ds.Agent.Environment, "ANTHROPIC_API_KEY", "${credentials.ANTHROPIC_API_KEY}")
}

// ===== Phase 8: Interfaces =====

func TestTemplate_InterfacesDefaults(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	if ds.Interfaces == nil {
		t.Fatal("interfaces: expected non-nil")
	}
	if len(ds.Interfaces.Adapters) != 0 {
		t.Errorf("interfaces.adapters: expected empty, got %v", ds.Interfaces.Adapters)
	}
	if ds.Interfaces.Port != 9090 {
		t.Errorf("interfaces.port: expected 9090, got %d", ds.Interfaces.Port)
	}
	if ds.Interfaces.Resources != spec.MessagingResources {
		t.Errorf("interfaces.resources: expected MessagingResources, got %+v", ds.Interfaces.Resources)
	}
	if !strings.HasSuffix(ds.Interfaces.Image, "/prod-astro-messaging:latest") {
		t.Errorf("interfaces.image: expected messaging sidecar image, got %s", ds.Interfaces.Image)
	}
	if ds.Interfaces.Expose.Enabled {
		t.Error("interfaces.expose.enabled: expected false")
	}
}

// ===== Phase 9: Full Combination =====

func TestTemplate_FullSpec(t *testing.T) {
	input := TemplateInput{
		Spec: &spec.AstroSpec{
			Name:  "engineering-assistant",
			Agent: spec.Container{Image: "registry.example.com/acme/engineering-assistant:build42"},
			Models: map[string]spec.Model{
				"local_llm": {Provider: "ollama"},
				"anthropic": {Provider: "anthropic"},
			},
			Knowledge: map[string]spec.Knowledge{
				"docs":  {Provider: "qdrant", Persistent: true},
				"cache": {Provider: "redis"},
			},
			Tools: map[string]spec.Tool{
				"websearch": {Container: &spec.ContainerConfig{Image: "search:latest", Port: 3000}},
				"github":    {Provider: "github"},
			},
			Ingestion: map[string]spec.Ingestion{
				"docs_sync": {
					Container: spec.ContainerConfig{
						Image:       "ingest:latest",
						Environment: map[string]string{"SOURCE_REPO": "company/docs"},
					},
					Trigger: spec.IngestionTrigger{Type: "schedule"},
				},
			},
		},
		Account:     "acme",
		BuildID:     "build42",
		RegistryURL: "registry.example.com/acme",
	}

	ds := mustGenerate(t, input)

	// Models — only self-hosted (ollama), not cloud (anthropic)
	if len(ds.Models) != 1 {
		t.Errorf("models: expected 1 (ollama only), got %d", len(ds.Models))
	}
	if ds.Models["local_llm"].Image != "ollama/ollama:latest" {
		t.Errorf("models.local_llm.image: got %s", ds.Models["local_llm"].Image)
	}

	// Knowledge
	if len(ds.Knowledge) != 2 {
		t.Errorf("knowledge: expected 2, got %d", len(ds.Knowledge))
	}
	if !ds.Knowledge["docs"].Persistent {
		t.Error("knowledge.docs.persistent: expected true")
	}
	if ds.Knowledge["cache"].Persistent {
		t.Error("knowledge.cache.persistent: expected false")
	}

	// Tools — only self-hosted (websearch), not cloud (github)
	if len(ds.Tools) != 1 {
		t.Errorf("tools: expected 1 (websearch only), got %d", len(ds.Tools))
	}

	// Ingestion
	if len(ds.Ingestion) != 1 {
		t.Errorf("ingestion: expected 1, got %d", len(ds.Ingestion))
	}
	if ds.Ingestion["docs_sync"].Environment["SOURCE_REPO"] != "company/docs" {
		t.Error("ingestion environment not preserved")
	}

	// Credentials from cloud providers
	if _, ok := ds.Credentials["ANTHROPIC_API_KEY"]; !ok {
		t.Error("missing ANTHROPIC_API_KEY credential")
	}
	if _, ok := ds.Credentials["GITHUB_TOKEN"]; !ok {
		t.Error("missing GITHUB_TOKEN credential")
	}

	// Agent environment — check all component refs exist
	env := ds.Agent.Environment
	assertEnvExists(t, env, "OLLAMA_HOST")
	assertEnvExists(t, env, "QDRANT_HOST")
	assertEnvExists(t, env, "REDIS_HOST")
	assertEnvExists(t, env, "TOOL_WEBSEARCH_HOST")
	assertEnvExists(t, env, "ANTHROPIC_API_KEY")
	assertEnvExists(t, env, "GITHUB_TOKEN")
	assertEnvExists(t, env, "ASTRO_AGENT_NAME")
	assertEnvExists(t, env, "ASTRO_AGENT_BUILD")

	// All env values that start with ${ should be valid references
	for key, val := range env {
		if strings.HasPrefix(val, "${") {
			refs := spec.ParseReferences(val)
			if len(refs) == 0 {
				t.Errorf("env %s: value %q looks like a reference but failed to parse", key, val)
			}
			// Validate the reference resolves against the deployment spec
			errs := spec.ValidateReferences(refs, ds)
			if len(errs) > 0 {
				t.Errorf("env %s: reference %q does not resolve: %v", key, val, errs)
			}
		}
	}
}

// ===== Phase 10: Edge Cases =====

func TestTemplate_NilSpec(t *testing.T) {
	_, err := GenerateDeploymentTemplate(TemplateInput{Spec: nil})
	if err == nil {
		t.Error("expected error for nil spec")
	}
}

func TestTemplate_EmptySpec(t *testing.T) {
	// Minimal spec with just name and image — should produce valid template
	input := baseInput()
	ds := mustGenerate(t, input)

	if len(ds.Models) != 0 {
		t.Errorf("models: expected 0 for empty spec, got %d", len(ds.Models))
	}
	if len(ds.Knowledge) != 0 {
		t.Errorf("knowledge: expected 0 for empty spec, got %d", len(ds.Knowledge))
	}
	if len(ds.Tools) != 0 {
		t.Errorf("tools: expected 0 for empty spec, got %d", len(ds.Tools))
	}
	if len(ds.Ingestion) != 0 {
		t.Errorf("ingestion: expected 0 for empty spec, got %d", len(ds.Ingestion))
	}
}

func TestTemplate_MultipleModels(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"ollama": {Provider: "ollama"},
		"custom": {Container: &spec.ContainerConfig{Image: "custom:latest", Port: 5000}},
	}

	ds := mustGenerate(t, input)

	if len(ds.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(ds.Models))
	}
	if ds.Models["ollama"].Image != "ollama/ollama:latest" {
		t.Errorf("ollama image: got %s", ds.Models["ollama"].Image)
	}
	if ds.Models["custom"].Image != "custom:latest" {
		t.Errorf("custom image: got %s", ds.Models["custom"].Image)
	}
	if ds.Models["custom"].Port != 5000 {
		t.Errorf("custom port: expected 5000, got %d", ds.Models["custom"].Port)
	}

	// Provider model gets provider-specific env, container model gets generic
	assertEnvExists(t, ds.Agent.Environment, "OLLAMA_HOST")
	assertEnvExists(t, ds.Agent.Environment, "MODEL_CUSTOM_HOST")
}

func TestTemplate_MultipleKnowledgeProviders(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"vectors": {Provider: "qdrant", Persistent: true},
		"cache":   {Provider: "redis"},
		"graph":   {Provider: "neo4j"},
	}

	ds := mustGenerate(t, input)

	if len(ds.Knowledge) != 3 {
		t.Fatalf("expected 3 knowledge entries, got %d", len(ds.Knowledge))
	}

	// Each provider should use its own env prefix
	assertEnvExists(t, ds.Agent.Environment, "QDRANT_HOST")
	assertEnvExists(t, ds.Agent.Environment, "REDIS_HOST")
	assertEnvExists(t, ds.Agent.Environment, "NEO4J_HOST")
}

// ===== Phase 11: YAML Serialization Round-trip =====

func TestTemplate_YAMLRoundTrip(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"llm":       {Provider: "ollama"},
		"anthropic": {Provider: "anthropic"},
	}
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"docs": {Provider: "qdrant", Persistent: true},
	}

	ds := mustGenerate(t, input)

	// Serialize to YAML
	yamlBytes, err := spec.SerializeDeploymentSpec(ds)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}

	// Parse back
	parsed, err := spec.ParseDeploymentSpec(yamlBytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Verify key fields survived round-trip
	if parsed.Spec != "deployment/v1" {
		t.Errorf("spec: expected deployment/v1, got %s", parsed.Spec)
	}
	if parsed.Source.Name != "my-agent" {
		t.Errorf("source.name: expected my-agent, got %s", parsed.Source.Name)
	}
	if parsed.Models["llm"].Image != "ollama/ollama:latest" {
		t.Errorf("models.llm.image lost in round-trip: got %s", parsed.Models["llm"].Image)
	}
	if !parsed.Knowledge["docs"].Persistent {
		t.Error("knowledge.docs.persistent lost in round-trip")
	}
	if parsed.Knowledge["docs"].Storage == nil {
		t.Error("knowledge.docs.storage lost in round-trip")
	}
	if _, ok := parsed.Credentials["ANTHROPIC_API_KEY"]; !ok {
		t.Error("credentials.ANTHROPIC_API_KEY lost in round-trip")
	}
	if parsed.Agent.Environment["ASTRO_AGENT_NAME"] != "${source.name}" {
		t.Error("agent.environment reference lost in round-trip")
	}
}

// ===== Phase 12: Tenant Image Resolution =====

func proxyInput() TemplateInput {
	return TemplateInput{
		Spec: &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "proxy.registry.io/acme/my-agent:abc123"},
		},
		Account:           "acme",
		BuildID:           "abc123",
		RegistryURL:       "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
		ProxyRegistryHost: "proxy.registry.io",
		Environment:       "prod",
	}
}

func TestResolveTenantImage_ProxyImage(t *testing.T) {
	input := proxyInput()
	got := resolveTenantImage("proxy.registry.io/acme/my-app:v1", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/my-app:v1"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveTenantImage_NonProxyImage(t *testing.T) {
	input := proxyInput()
	got := resolveTenantImage("docker.io/library/python:3.11", input)
	if got != "docker.io/library/python:3.11" {
		t.Errorf("non-proxy image should be unchanged, got %s", got)
	}
}

func TestResolveTenantImage_EmptyImage(t *testing.T) {
	input := proxyInput()
	got := resolveTenantImage("", input)
	if got != "" {
		t.Errorf("empty image should stay empty, got %s", got)
	}
}

func TestResolveTenantImage_NoProxyHost(t *testing.T) {
	input := proxyInput()
	input.ProxyRegistryHost = ""
	got := resolveTenantImage("proxy.registry.io/acme/my-app:v1", input)
	if got != "proxy.registry.io/acme/my-app:v1" {
		t.Errorf("without proxy host config, image should be unchanged, got %s", got)
	}
}

func TestResolveTenantImage_RegistryURLWithoutScheme(t *testing.T) {
	input := proxyInput()
	input.RegistryURL = "123456789.dkr.ecr.us-east-1.amazonaws.com"
	got := resolveTenantImage("proxy.registry.io/acme/my-app:v1", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/my-app:v1"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveTenantImage_SingleSegmentPath(t *testing.T) {
	input := proxyInput()
	// Only namespace, no image name — should return as-is
	got := resolveTenantImage("proxy.registry.io/acme", input)
	if got != "proxy.registry.io/acme" {
		t.Errorf("single-segment path should be unchanged, got %s", got)
	}
}

func TestTemplate_AgentImageResolved(t *testing.T) {
	input := proxyInput()
	ds := mustGenerate(t, input)

	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/my-agent:abc123"
	if ds.Agent.Image != expected {
		t.Errorf("agent.image: expected %s, got %s", expected, ds.Agent.Image)
	}
}

func TestTemplate_ModelImageResolved(t *testing.T) {
	input := proxyInput()
	input.Spec.Models = map[string]spec.Model{
		"embedder": {
			Container: &spec.ContainerConfig{
				Image: "proxy.registry.io/acme/custom-embedder:v2",
				Port:  9000,
			},
		},
	}

	ds := mustGenerate(t, input)

	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/custom-embedder:v2"
	if ds.Models["embedder"].Image != expected {
		t.Errorf("model image: expected %s, got %s", expected, ds.Models["embedder"].Image)
	}
}

func TestTemplate_ModelProviderImageUnchanged(t *testing.T) {
	input := proxyInput()
	input.Spec.Models = map[string]spec.Model{
		"llm": {Provider: "ollama"},
	}

	ds := mustGenerate(t, input)

	// Provider images (ollama/ollama:latest) should not be rewritten
	if ds.Models["llm"].Image != "ollama/ollama:latest" {
		t.Errorf("provider model image should be unchanged, got %s", ds.Models["llm"].Image)
	}
}

func TestTemplate_KnowledgeImageResolved(t *testing.T) {
	input := proxyInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"store": {
			Container: &spec.ContainerConfig{
				Image: "proxy.registry.io/acme/custom-store:v3",
				Port:  5432,
			},
		},
	}

	ds := mustGenerate(t, input)

	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/custom-store:v3"
	if ds.Knowledge["store"].Image != expected {
		t.Errorf("knowledge image: expected %s, got %s", expected, ds.Knowledge["store"].Image)
	}
}

func TestTemplate_ToolImageResolved(t *testing.T) {
	input := proxyInput()
	input.Spec.Tools = map[string]spec.Tool{
		"search": {
			Container: &spec.ContainerConfig{
				Image: "proxy.registry.io/acme/search-tool:latest",
				Port:  3000,
			},
		},
	}

	ds := mustGenerate(t, input)

	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/search-tool:latest"
	if ds.Tools["search"].Image != expected {
		t.Errorf("tool image: expected %s, got %s", expected, ds.Tools["search"].Image)
	}
}

func TestTemplate_IngestionImageResolved(t *testing.T) {
	input := proxyInput()
	input.Spec.Ingestion = map[string]spec.Ingestion{
		"data": {
			Container: spec.ContainerConfig{
				Image: "proxy.registry.io/acme/ingestion-data:dfdddc61",
			},
			Trigger: spec.IngestionTrigger{Type: "startup"},
		},
	}

	ds := mustGenerate(t, input)

	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/ingestion-data:dfdddc61"
	if ds.Ingestion["data"].Image != expected {
		t.Errorf("ingestion image: expected %s, got %s", expected, ds.Ingestion["data"].Image)
	}
}

func TestTemplate_AllComponentImagesResolved(t *testing.T) {
	input := proxyInput()
	input.Spec.Agent.Image = "proxy.registry.io/acme/agent:v1"
	input.Spec.Models = map[string]spec.Model{
		"m": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/acme/model:v1", Port: 8000}},
	}
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"k": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/acme/knowledge:v1", Port: 5000}},
	}
	input.Spec.Tools = map[string]spec.Tool{
		"t": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/acme/tool:v1", Port: 3000}},
	}
	input.Spec.Ingestion = map[string]spec.Ingestion{
		"i": {
			Container: spec.ContainerConfig{Image: "proxy.registry.io/acme/ingest:v1"},
			Trigger:   spec.IngestionTrigger{Type: "startup"},
		},
	}

	ds := mustGenerate(t, input)

	prefix := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/"
	checks := map[string]string{
		"agent":     ds.Agent.Image,
		"model":     ds.Models["m"].Image,
		"knowledge": ds.Knowledge["k"].Image,
		"tool":      ds.Tools["t"].Image,
		"ingestion": ds.Ingestion["i"].Image,
	}
	for component, image := range checks {
		if !strings.HasPrefix(image, prefix) {
			t.Errorf("%s image not resolved: expected prefix %s, got %s", component, prefix, image)
		}
	}
}

func TestTemplate_MixedProxyAndPublicImages(t *testing.T) {
	input := proxyInput()
	input.Spec.Models = map[string]spec.Model{
		"proxy":  {Container: &spec.ContainerConfig{Image: "proxy.registry.io/acme/custom:v1", Port: 8000}},
		"public": {Container: &spec.ContainerConfig{Image: "ollama/ollama:latest", Port: 11434}},
	}

	ds := mustGenerate(t, input)

	// Proxy image should be resolved
	if !strings.Contains(ds.Models["proxy"].Image, "prod-tenant-acme") {
		t.Errorf("proxy image should be resolved, got %s", ds.Models["proxy"].Image)
	}
	// Public image should be unchanged
	if ds.Models["public"].Image != "ollama/ollama:latest" {
		t.Errorf("public image should be unchanged, got %s", ds.Models["public"].Image)
	}
}

// ===== Phase 13: Reference Validation Integration =====

func TestTemplate_AllReferencesValid(t *testing.T) {
	// For every possible combination, all generated ${} references
	// should pass validation against the generated deployment spec.
	input := TemplateInput{
		Spec: &spec.AstroSpec{
			Name:  "ref-test",
			Agent: spec.Container{Image: "agent:latest"},
			Models: map[string]spec.Model{
				"llm":       {Provider: "ollama"},
				"embedder":  {Container: &spec.ContainerConfig{Image: "embed:latest", Port: 8000}},
				"anthropic": {Provider: "anthropic"},
			},
			Knowledge: map[string]spec.Knowledge{
				"vectors": {Provider: "qdrant", Persistent: true},
				"cache":   {Provider: "redis"},
				"custom":  {Container: &spec.ContainerConfig{Image: "mydb:latest", Port: 5432}},
			},
			Tools: map[string]spec.Tool{
				"search": {Container: &spec.ContainerConfig{Image: "search:latest", Port: 3000}},
				"github": {Provider: "github"},
			},
			Integrations: map[string]spec.Integration{
				"myapi": {
					Credentials: []spec.CustomCredential{{Suffix: "TOKEN", Description: "token"}},
				},
			},
		},
		Account:     "testco",
		BuildID:     "b1",
		RegistryURL: "reg.io",
	}

	ds := mustGenerate(t, input)

	// Extract and validate all references from agent environment
	refs := spec.ExtractAllReferences(ds.Agent.Environment)
	if len(refs) == 0 {
		t.Fatal("expected references in agent.environment")
	}

	errs := spec.ValidateReferences(refs, ds)
	if len(errs) > 0 {
		t.Errorf("generated template has invalid references:\n%s", strings.Join(errs, "\n"))
	}
}

// ===== End-to-end: astro-spec → deployment spec → validation =====

func TestTemplate_EndToEnd_WebhookIngestionWithKnowledge(t *testing.T) {
	// Mirrors a real spec: agent + webhook ingestion + knowledge + integrations
	input := TemplateInput{
		Spec: &spec.AstroSpec{
			Name:  "sasbot",
			Agent: spec.Container{Image: "registry.example.com/sasbot:abc123"},
			Models: map[string]spec.Model{
				"anthropic": {Provider: "anthropic"},
			},
			Knowledge: map[string]spec.Knowledge{
				"cache": {Provider: "redis"},
				"graph": {Provider: "neo4j"},
			},
			Ingestion: map[string]spec.Ingestion{
				"data": {
					Container: spec.ContainerConfig{
						Image: "registry.example.com/sasbot-ingestion:abc123",
						Port:  3001,
					},
					Trigger: spec.IngestionTrigger{Type: "webhook"},
				},
			},
		},
		Account:     "acme",
		BuildID:     "abc123",
		RegistryURL: "registry.example.com",
	}

	ds := mustGenerate(t, input)

	// Agent
	if ds.Agent.Image != "registry.example.com/sasbot:abc123" {
		t.Errorf("agent.image: got %s", ds.Agent.Image)
	}
	if ds.Agent.Port != 8080 {
		t.Errorf("agent.port: expected 8080, got %d", ds.Agent.Port)
	}

	// Knowledge
	if len(ds.Knowledge) != 2 {
		t.Fatalf("knowledge: expected 2, got %d", len(ds.Knowledge))
	}
	if ds.Knowledge["cache"].Port != 6379 {
		t.Errorf("knowledge.cache.port: expected 6379, got %d", ds.Knowledge["cache"].Port)
	}
	if ds.Knowledge["graph"].Port != 7474 {
		t.Errorf("knowledge.graph.port: expected 7474, got %d", ds.Knowledge["graph"].Port)
	}

	// Ingestion — webhook with explicit port
	ing := ds.Ingestion["data"]
	if ing.Image != "registry.example.com/sasbot-ingestion:abc123" {
		t.Errorf("ingestion.data.image: got %s", ing.Image)
	}
	if ing.Port != 3001 {
		t.Errorf("ingestion.data.port: expected 3001, got %d", ing.Port)
	}
	if ing.Trigger.Type != "webhook" {
		t.Errorf("ingestion.data.trigger.type: expected webhook, got %s", ing.Trigger.Type)
	}

	// Credentials
	if _, ok := ds.Credentials["ANTHROPIC_API_KEY"]; !ok {
		t.Error("missing ANTHROPIC_API_KEY credential")
	}

	// Interfaces present
	if ds.Interfaces == nil {
		t.Fatal("interfaces: expected non-nil")
	}

	// Deployment spec should pass validation when credentials are filled
	ds.Ingestion["data"] = spec.DeploymentIngestion{
		Image:     ing.Image,
		Port:      ing.Port,
		Resources: ing.Resources,
		Trigger:   ing.Trigger,
	}
	yamlBytes, err := spec.SerializeDeploymentSpec(ds)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	parsed, err := spec.ParseDeploymentSpec(yamlBytes)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if parsed.Ingestion["data"].Port != 3001 {
		t.Errorf("round-trip port: expected 3001, got %d", parsed.Ingestion["data"].Port)
	}
}
