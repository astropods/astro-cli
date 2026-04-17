package deployment

import (
	"encoding/json"
	"strings"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// --- helpers ---

func baseInput() TemplateInput {
	return TemplateInput{
		Spec: &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "registry.example.com/my-agent:abc123"},
		},
		AgentName:   "my-agent",
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

	if ds.Spec != "deployment-template/v1" {
		t.Errorf("spec: expected deployment-template/v1, got %s", ds.Spec)
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
}

func TestTemplate_ObservabilityDefaults(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	if !ds.Observability.Enabled {
		t.Error("observability.enabled: expected true")
	}
	if ds.Observability.Provider != "langfuse" {
		t.Errorf("observability.provider: expected langfuse, got %s", ds.Observability.Provider)
	}
}

func TestTemplate_EditableFieldsPresent(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	if len(ds.Editable) == 0 {
		t.Fatal("editable: expected non-empty list")
	}

	// Spot-check critical editable paths
	expected := []string{
		"agent.replicas",
		"agent.environment",
		"variables.*.value",
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
	if spec.PrimaryPort(ds.Agent.Endpoints) != 8080 {
		t.Errorf("agent.endpoints http port: expected 8080, got %d", spec.PrimaryPort(ds.Agent.Endpoints))
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
	// Agent expose should be false by default (no exposed endpoint)
	if ep := spec.ExposedEndpoint(ds.Agent.Endpoints); ep != nil && ep.Expose != nil && ep.Expose.Enabled {
		t.Error("agent: expected no exposed endpoint by default")
	}
}

func TestTemplate_AgentImageMissing(t *testing.T) {
	// When astro-spec has no image, template generation must fail.
	input := baseInput()
	input.Spec.Agent.Image = ""

	_, err := GenerateDeploymentTemplate(input)
	if err == nil {
		t.Fatal("expected error when agent image is empty, got nil")
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
	if m.Image != "registry.example.com/dockerhub/ollama/ollama:latest" {
		t.Errorf("models.local_llm.image: expected registry.example.com/dockerhub/ollama/ollama:latest, got %s", m.Image)
	}
	if spec.PrimaryPort(m.Endpoints) != 11434 {
		t.Errorf("models.local_llm endpoints primary port: expected 11434, got %d", spec.PrimaryPort(m.Endpoints))
	}
	if m.Replicas != 1 {
		t.Errorf("models.local_llm.replicas: expected 1, got %d", m.Replicas)
	}

	// Check agent environment references — provider-specific env vars
	assertEnvRef(t, ds.Agent.Environment, "OLLAMA_HOST", "${models.local_llm.host}")
	assertEnvRef(t, ds.Agent.Environment, "OLLAMA_PORT", "${models.local_llm.http.port}")
	assertEnvRef(t, ds.Agent.Environment, "OLLAMA_URL", "${models.local_llm.http.url}")
	assertEnvRef(t, ds.Agent.Environment, "OLLAMA_BASE_URL", "${models.local_llm.http.url}/api")
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
	if m.Image != "registry.example.com/dockerhub/library/custom-embedder:v1" {
		t.Errorf("image: expected registry.example.com/dockerhub/library/custom-embedder:v1, got %s", m.Image)
	}
	if spec.PrimaryPort(m.Endpoints) != 9999 {
		t.Errorf("endpoints primary port: expected 9999, got %d", spec.PrimaryPort(m.Endpoints))
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
	if spec.PrimaryPort(ds.Models["noport"].Endpoints) != 8080 {
		t.Errorf("endpoints: expected 8080 default, got %d", spec.PrimaryPort(ds.Models["noport"].Endpoints))
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
	if k.Image != "registry.example.com/dockerhub/qdrant/qdrant:latest" {
		t.Errorf("image: expected registry.example.com/dockerhub/qdrant/qdrant:latest, got %s", k.Image)
	}
	if spec.PrimaryPort(k.Endpoints) != 6333 {
		t.Errorf("endpoints primary port: expected 6333, got %d", spec.PrimaryPort(k.Endpoints))
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
	assertEnvRef(t, ds.Agent.Environment, "QDRANT_PORT", "${knowledge.docs.http.port}")
	assertEnvRef(t, ds.Agent.Environment, "QDRANT_URL", "${knowledge.docs.http.url}")
}

func TestTemplate_ProviderKnowledge_Redis(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"cache": {Provider: "redis"},
	}

	ds := mustGenerate(t, input)

	k := ds.Knowledge["cache"]
	if k.Image != "registry.example.com/dockerhub/library/redis:7-alpine" {
		t.Errorf("image: expected registry.example.com/dockerhub/library/redis:7-alpine, got %s", k.Image)
	}
	if spec.PrimaryPort(k.Endpoints) != 6379 {
		t.Errorf("endpoints primary port: expected 6379, got %d", spec.PrimaryPort(k.Endpoints))
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
	assertEnvRef(t, ds.Agent.Environment, "REDIS_PORT", "${knowledge.cache.http.port}")
	assertEnvRef(t, ds.Agent.Environment, "REDIS_URL", "${knowledge.cache.http.url}")
}

func TestTemplate_ProviderKnowledge_Neo4j(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"graph": {Provider: "neo4j"},
	}

	ds := mustGenerate(t, input)

	k := ds.Knowledge["graph"]
	if k.Image != "registry.example.com/dockerhub/library/neo4j:5-community" {
		t.Errorf("image: expected registry.example.com/dockerhub/library/neo4j:5-community, got %s", k.Image)
	}
	if spec.PrimaryPort(k.Endpoints) != 7474 {
		t.Errorf("endpoints primary port: expected 7474, got %d", spec.PrimaryPort(k.Endpoints))
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
	if k.Image != "registry.example.com/dockerhub/library/my-db:latest" {
		t.Errorf("image: expected registry.example.com/dockerhub/library/my-db:latest, got %s", k.Image)
	}
	if spec.PrimaryPort(k.Endpoints) != 5000 {
		t.Errorf("endpoints primary port: expected 5000, got %d", spec.PrimaryPort(k.Endpoints))
	}

	// Container mode uses KNOWLEDGE_* prefix
	assertEnvRef(t, ds.Agent.Environment, "KNOWLEDGE_CUSTOM_DB_HOST", "${knowledge.custom_db.host}")
	assertEnvRef(t, ds.Agent.Environment, "KNOWLEDGE_CUSTOM_DB_PORT", "${knowledge.custom_db.http.port}")
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
	input.Spec.Integrations = map[string]spec.Integration{
		"websearch": {
			Container: &spec.ContainerConfig{
				Image: "search:v2",
				Port:  3000,
			},
		},
	}

	ds := mustGenerate(t, input)

	tool := ds.Integrations["websearch"]
	if tool.Image != "registry.example.com/dockerhub/library/search:v2" {
		t.Errorf("image: expected registry.example.com/dockerhub/library/search:v2, got %s", tool.Image)
	}
	if spec.PrimaryPort(tool.Endpoints) != 3000 {
		t.Errorf("endpoints primary port: expected 3000, got %d", spec.PrimaryPort(tool.Endpoints))
	}
	if tool.Replicas != 1 {
		t.Errorf("replicas: expected 1, got %d", tool.Replicas)
	}
	if tool.Resources != spec.StandardResources {
		t.Errorf("resources: expected StandardResources, got %+v", tool.Resources)
	}

	assertEnvRef(t, ds.Agent.Environment, "INTEGRATION_WEBSEARCH_HOST", "${integrations.websearch.host}")
	assertEnvRef(t, ds.Agent.Environment, "INTEGRATION_WEBSEARCH_PORT", "${integrations.websearch.http.port}")
	assertEnvRef(t, ds.Agent.Environment, "INTEGRATION_WEBSEARCH_URL", "${integrations.websearch.http.url}")
}

func TestTemplate_IntegrationDefaultPort(t *testing.T) {
	input := baseInput()
	input.Spec.Integrations = map[string]spec.Integration{
		"noport": {Container: &spec.ContainerConfig{Image: "tool:latest"}},
	}

	ds := mustGenerate(t, input)
	if spec.PrimaryPort(ds.Integrations["noport"].Endpoints) != 8080 {
		t.Errorf("endpoints: expected 8080 default, got %d", spec.PrimaryPort(ds.Integrations["noport"].Endpoints))
	}
}

func TestTemplate_IntegrationEnvironmentPassthrough(t *testing.T) {
	input := baseInput()
	input.Spec.Integrations = map[string]spec.Integration{
		"mcp": {
			Container: &spec.ContainerConfig{
				Image:       "mcp:latest",
				Environment: map[string]string{"WORKERS": "4"},
			},
		},
	}

	ds := mustGenerate(t, input)
	if ds.Integrations["mcp"].Environment["WORKERS"] != "4" {
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
	if ing.Image != "registry.example.com/dockerhub/library/sync:latest" {
		t.Errorf("image: expected registry.example.com/dockerhub/library/sync:latest, got %s", ing.Image)
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
	if spec.PrimaryPort(ing.Endpoints) != 3001 {
		t.Errorf("endpoints primary port: expected 3001, got %d", spec.PrimaryPort(ing.Endpoints))
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
				// No port — template should have empty endpoints; validator rejects at deploy time
			},
			Trigger: spec.IngestionTrigger{Type: "webhook"},
		},
	}

	ds := mustGenerate(t, input)

	if spec.PrimaryPort(ds.Ingestion["data"].Endpoints) != 0 {
		t.Errorf("endpoints: expected 0 (unset), got %d", spec.PrimaryPort(ds.Ingestion["data"].Endpoints))
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
	if spec.PrimaryPort(ds.Ingestion["webhook"].Endpoints) != 9000 {
		t.Errorf("webhook endpoints port: expected 9000, got %d", spec.PrimaryPort(ds.Ingestion["webhook"].Endpoints))
	}
}

// ===== Phase 7: Variables (credentials + inputs) =====

func TestTemplate_VariablesFromCloudProviders(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"anthropic": {Provider: "anthropic"},
	}
	input.Spec.Integrations = map[string]spec.Integration{
		"github": {Provider: "github"},
	}

	ds := mustGenerate(t, input)

	if len(ds.Variables) < 2 {
		t.Fatalf("expected at least 2 variables, got %d", len(ds.Variables))
	}

	// Anthropic
	v, ok := ds.Variables["ANTHROPIC_API_KEY"]
	if !ok {
		t.Fatal("variables: ANTHROPIC_API_KEY not found")
	}
	if v.Value != "" {
		t.Errorf("variable value: expected empty placeholder, got %s", v.Value)
	}
	if v.Description == "" {
		t.Error("variable description: expected non-empty")
	}
	if !v.Secret {
		t.Error("provider credential should have secret=true")
	}

	// GitHub
	if _, ok := ds.Variables["GITHUB_TOKEN"]; !ok {
		t.Fatal("variables: GITHUB_TOKEN not found")
	}

	// Cloud models should NOT appear in ds.Models (no container)
	if len(ds.Models) != 0 {
		t.Errorf("cloud models should not be in deployment spec, got %d", len(ds.Models))
	}
	// Cloud integrations should NOT appear in ds.Integrations
	if len(ds.Integrations) != 0 {
		t.Errorf("cloud integrations should not be in deployment spec, got %d", len(ds.Integrations))
	}

	// Check agent env references wired for variables
	assertEnvRef(t, ds.Agent.Environment, "ANTHROPIC_API_KEY", "${variables.ANTHROPIC_API_KEY}")
	assertEnvRef(t, ds.Agent.Environment, "GITHUB_TOKEN", "${variables.GITHUB_TOKEN}")
}

func TestTemplate_ManagedProviderNoVariable(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"claude": {Provider: "anthropic-managed"},
	}

	ds := mustGenerate(t, input)

	// Managed providers must NOT create any variables — the server injects at deploy time
	if _, ok := ds.Variables["ANTHROPIC_MANAGED_API_KEY"]; ok {
		t.Error("managed provider should not create ANTHROPIC_MANAGED_API_KEY variable")
	}
	if _, ok := ds.Variables["ANTHROPIC_API_KEY"]; ok {
		t.Error("managed provider should not create ANTHROPIC_API_KEY variable")
	}

	// Agent env should NOT reference a managed credential variable
	if ref, ok := ds.Agent.Environment["ANTHROPIC_MANAGED_API_KEY"]; ok {
		t.Errorf("managed credential should not be wired to agent env, got %q", ref)
	}
}

func TestTemplate_ManagedAndRegularProvidersTogether(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"managed-claude": {Provider: "anthropic-managed"},
		"user-openai":    {Provider: "openai"},
	}

	ds := mustGenerate(t, input)

	// Only openai should produce a variable
	if _, ok := ds.Variables["OPENAI_API_KEY"]; !ok {
		t.Error("openai should produce OPENAI_API_KEY variable")
	}
	if _, ok := ds.Variables["ANTHROPIC_MANAGED_API_KEY"]; ok {
		t.Error("managed provider should not create ANTHROPIC_MANAGED_API_KEY variable")
	}

	assertEnvRef(t, ds.Agent.Environment, "OPENAI_API_KEY", "${variables.OPENAI_API_KEY}")
}

func TestTemplate_VariablesCustomProvider(t *testing.T) {
	// Variable names are suffixes per §5; full key is {UPPER(provider)}_{varName}.
	input := baseInput()
	input.Spec.Providers = map[string]spec.CustomProvider{
		"myapi": {
			Scope: []string{"integrations"},
			Variables: []spec.Input{
				{Name: "API_KEY", Datatype: "string", Secret: true, Description: "main key"},
				{Name: "SECRET", Datatype: "string", Secret: true, Description: "optional secret", Optional: true},
			},
		},
	}
	input.Spec.Integrations = map[string]spec.Integration{
		"myapi": {Provider: "myapi"},
	}

	ds := mustGenerate(t, input)

	if _, ok := ds.Variables["MYAPI_API_KEY"]; !ok {
		t.Error("variables: MYAPI_API_KEY not found")
	}
	v, ok := ds.Variables["MYAPI_SECRET"]
	if !ok {
		t.Fatal("variables: MYAPI_SECRET not found")
	}
	if !v.Optional {
		t.Error("expected MYAPI_SECRET to be optional")
	}
}

func TestTemplate_JiraIntegrationInputs(t *testing.T) {
	// End-to-end: a Jira custom provider with scope: [integrations] and three
	// secret variables must produce the correct deployment template variables
	// and wire ${variables.*} references into the agent environment.
	input := baseInput()
	input.Spec.Providers = map[string]spec.CustomProvider{
		"jira": {
			Scope: []string{"integrations"},
			Variables: []spec.Input{
				{Name: "API_KEY", Datatype: "string", Secret: true, Description: "Jira API token"},
				{Name: "BASE_URL", Datatype: "string", Secret: true, Description: "Jira instance base URL (e.g. https://your-org.atlassian.net)"},
				{Name: "EMAIL", Datatype: "string", Secret: true, Description: "Atlassian account email for Jira API authentication"},
			},
		},
	}
	input.Spec.Integrations = map[string]spec.Integration{
		"jira": {Provider: "jira"},
	}

	ds := mustGenerate(t, input)

	// All three variables must exist in the deployment template.
	for _, key := range []string{"JIRA_API_KEY", "JIRA_BASE_URL", "JIRA_EMAIL"} {
		v, ok := ds.Variables[key]
		if !ok {
			t.Errorf("variables: %s not found", key)
			continue
		}
		if !v.Secret {
			t.Errorf("%s: expected secret=true", key)
		}

		// Agent environment must reference the variable.
		assertEnvRef(t, ds.Agent.Environment, key, "${variables."+key+"}")
	}

	// Descriptions should be preserved.
	if ds.Variables["JIRA_API_KEY"].Description != "Jira API token" {
		t.Errorf("JIRA_API_KEY description = %q", ds.Variables["JIRA_API_KEY"].Description)
	}
	if ds.Variables["JIRA_EMAIL"].Description != "Atlassian account email for Jira API authentication" {
		t.Errorf("JIRA_EMAIL description = %q", ds.Variables["JIRA_EMAIL"].Description)
	}
}

func TestTemplate_TopLevelInputs_WiredAsVariableRefs(t *testing.T) {
	// Non-secret top-level inputs must always produce ${variables.NAME} references
	// in the agent environment, regardless of whether they have a default value.
	input := baseInput()
	input.Spec.Inputs = map[string]spec.Input{
		"CUSTOM_PROMPT": {Name: "CUSTOM_PROMPT", Datatype: "string", Description: "Custom prompt", Optional: true},
		"SERVICE_URL":   {Name: "SERVICE_URL", Datatype: "string", Description: "Service URL", Default: "http://localhost:8080"},
	}

	ds := mustGenerate(t, input)

	// Both inputs must appear in Variables
	if _, ok := ds.Variables["CUSTOM_PROMPT"]; !ok {
		t.Error("variables: CUSTOM_PROMPT not found")
	}
	if _, ok := ds.Variables["SERVICE_URL"]; !ok {
		t.Error("variables: SERVICE_URL not found")
	}

	// Both must be wired as ${variables.*} references in agent environment
	assertEnvRef(t, ds.Agent.Environment, "CUSTOM_PROMPT", "${variables.CUSTOM_PROMPT}")
	assertEnvRef(t, ds.Agent.Environment, "SERVICE_URL", "${variables.SERVICE_URL}")

	// Default value must be stored on the variable
	if ds.Variables["SERVICE_URL"].Value != "http://localhost:8080" {
		t.Errorf("SERVICE_URL variable value: expected %q, got %q", "http://localhost:8080", ds.Variables["SERVICE_URL"].Value)
	}
}

func TestTemplate_TopLevelInputs_SecretNotInAgentEnv(t *testing.T) {
	// Secret inputs must NOT be wired into agent environment (they go via SecretData).
	input := baseInput()
	input.Spec.Inputs = map[string]spec.Input{
		"CUSTOM_SECRET": {Name: "CUSTOM_SECRET", Datatype: "string", Secret: true, Description: "A secret"},
	}

	ds := mustGenerate(t, input)

	if _, ok := ds.Variables["CUSTOM_SECRET"]; !ok {
		t.Error("variables: CUSTOM_SECRET not found")
	}
	if _, ok := ds.Agent.Environment["CUSTOM_SECRET"]; ok {
		t.Error("secret input CUSTOM_SECRET should not appear in agent environment (uses SecretData path)")
	}
}

func TestTemplate_NoIntegrations_AdapterVariablesPresent(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	// Template always includes adapter credential placeholders so users know what to fill in.
	if _, ok := ds.Variables["SLACK_BOT_TOKEN"]; !ok {
		t.Error("expected SLACK_BOT_TOKEN adapter variable in template")
	}
	if _, ok := ds.Variables["SLACK_APP_TOKEN"]; !ok {
		t.Error("expected SLACK_APP_TOKEN adapter variable in template")
	}
	if !ds.Variables["SLACK_BOT_TOKEN"].Optional {
		t.Error("SLACK_BOT_TOKEN should be optional in template (adapter is disabled by default)")
	}
	if !ds.Variables["SLACK_APP_TOKEN"].Optional {
		t.Error("SLACK_APP_TOKEN should be optional in template (adapter is disabled by default)")
	}
}

func TestTemplate_SlackConfigVariable_WithSpecConfig(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }

	input := baseInput()
	input.Spec.Dev = &spec.Dev{
		Interfaces: &spec.DevInterfaces{
			Messaging: &spec.DevMessaging{
				Adapters: []string{"slack"},
				Slack: &spec.SlackAdapterConfig{
					ActionableReactions: []string{"ticket", "bug"},
					AllowedChannelIDs:   []string{"C123", "C999"},
					AllowedUserIDs:      []string{"U123", "U999"},
					SocketMode:          boolPtr(false),
					AutoThread:          boolPtr(true),
				},
			},
		},
	}

	ds := mustGenerate(t, input)

	v, ok := ds.Variables["SLACK_CONFIG"]
	if !ok {
		t.Fatal("SLACK_CONFIG variable not found in template")
	}
	if v.Secret {
		t.Error("SLACK_CONFIG should not be secret")
	}
	if !v.Optional {
		t.Error("SLACK_CONFIG should be optional")
	}
	if len(v.Targets) != 1 || v.Targets[0] != "interface.slack" {
		t.Errorf("SLACK_CONFIG targets = %v, want [interface.slack]", v.Targets)
	}
	if v.Default != v.Value {
		t.Errorf("SLACK_CONFIG default should equal value, got default=%q value=%q", v.Default, v.Value)
	}

	// Value must be valid JSON containing the expected fields
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.Value), &parsed); err != nil {
		t.Fatalf("SLACK_CONFIG value is not valid JSON: %v (value=%q)", err, v.Value)
	}
	reactions, _ := parsed["actionable_reactions"].([]any)
	if len(reactions) != 2 || reactions[0] != "ticket" || reactions[1] != "bug" {
		t.Errorf("SLACK_CONFIG actionable_reactions = %v, want [ticket bug]", reactions)
	}
	channels, _ := parsed["allowed_channel_ids"].([]any)
	if len(channels) != 2 || channels[0] != "C123" || channels[1] != "C999" {
		t.Errorf("SLACK_CONFIG allowed_channel_ids = %v, want [C123 C999]", channels)
	}
	users, _ := parsed["allowed_user_ids"].([]any)
	if len(users) != 2 || users[0] != "U123" || users[1] != "U999" {
		t.Errorf("SLACK_CONFIG allowed_user_ids = %v, want [U123 U999]", users)
	}
	if parsed["socket_mode"] != false {
		t.Errorf("SLACK_CONFIG socket_mode = %v, want false", parsed["socket_mode"])
	}
	if parsed["auto_thread"] != true {
		t.Errorf("SLACK_CONFIG auto_thread = %v, want true", parsed["auto_thread"])
	}

	envRef, ok := ds.Interfaces.Environment["SLACK_CONFIG"]
	if !ok {
		t.Fatal("interfaces.environment should contain SLACK_CONFIG")
	}
	if envRef != "${variables.SLACK_CONFIG}" {
		t.Errorf("interfaces.environment ref = %q, want ${variables.SLACK_CONFIG}", envRef)
	}

	if _, ok := ds.Interfaces.Environment["SLACK_BOT_TOKEN"]; ok {
		t.Error("secret variables should not appear in interfaces.environment")
	}
}

func TestTemplate_SlackConfigVariable_NoSpecConfig(t *testing.T) {
	ds := mustGenerate(t, baseInput())

	v, ok := ds.Variables["SLACK_CONFIG"]
	if !ok {
		t.Fatal("SLACK_CONFIG variable should always be present when messaging is enabled")
	}
	if v.Value != "" {
		t.Errorf("SLACK_CONFIG value should be empty when no slack config in spec, got %q", v.Value)
	}
	if v.Default != "" {
		t.Errorf("SLACK_CONFIG default should be empty when no slack config in spec, got %q", v.Default)
	}
}

func TestTemplate_SlackConfigVariable_MessagingDisabled(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: false}
	ds := mustGenerate(t, input)

	if _, ok := ds.Variables["SLACK_CONFIG"]; ok {
		t.Error("SLACK_CONFIG should not be present when messaging is disabled")
	}
}

func TestTemplate_NameDerivedVariableKeys(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"fallback": {Provider: "anthropic"},
	}

	ds := mustGenerate(t, input)

	// Single entry uses provider-prefixed key: ANTHROPIC_API_KEY
	if _, ok := ds.Variables["ANTHROPIC_API_KEY"]; !ok {
		t.Error("expected ANTHROPIC_API_KEY from provider-prefixed key")
	}
	assertEnvRef(t, ds.Agent.Environment, "ANTHROPIC_API_KEY", "${variables.ANTHROPIC_API_KEY}")
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
	grpcEp := spec.EndpointByName(ds.Interfaces.Endpoints, "grpc")
	if grpcEp == nil || grpcEp.Port != 9090 {
		t.Errorf("interfaces.endpoints.grpc.port: expected 9090, got %v", grpcEp)
	}
	if ds.Interfaces.Resources != spec.MessagingResources {
		t.Errorf("interfaces.resources: expected MessagingResources, got %+v", ds.Interfaces.Resources)
	}
	if ds.Interfaces.Image != "registry.example.com/dockerhub/astropods/messaging:latest" {
		t.Errorf("interfaces.image: expected registry.example.com/dockerhub/astropods/messaging:latest, got %s", ds.Interfaces.Image)
	}
	// http endpoint should have expose.enabled=false
	httpEp := spec.EndpointByName(ds.Interfaces.Endpoints, "http")
	if httpEp != nil && httpEp.Expose != nil && httpEp.Expose.Enabled {
		t.Error("interfaces.endpoints.http.expose.enabled: expected false")
	}
	// auth should default to oidc
	if ds.Interfaces.Auth == nil || ds.Interfaces.Auth.Web == nil || ds.Interfaces.Auth.Web.Type != "oidc" {
		t.Errorf("interfaces.auth.web.type: expected oidc, got %v", ds.Interfaces.Auth)
	}
}

func TestTemplate_MessagingDisabled(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: false}
	ds := mustGenerate(t, input)

	if ds.Interfaces != nil {
		t.Error("interfaces: expected nil when messaging is disabled")
	}
	// Slack variables should not be present
	if _, ok := ds.Variables["SLACK_BOT_TOKEN"]; ok {
		t.Error("SLACK_BOT_TOKEN should not be present when messaging is disabled")
	}
}

func TestTemplate_FrontendEnabled(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Frontend: true, Messaging: false}
	ds := mustGenerate(t, input)

	// Agent endpoint should be port 80 with expose enabled
	httpEp := spec.EndpointByName(ds.Agent.Endpoints, "http")
	if httpEp == nil {
		t.Fatal("agent.endpoints.http: expected non-nil")
	}
	if httpEp.Port != 80 {
		t.Errorf("agent.endpoints.http.port: expected 80, got %d", httpEp.Port)
	}
	if httpEp.Expose == nil || !httpEp.Expose.Enabled {
		t.Error("agent.endpoints.http.expose.enabled: expected true for frontend")
	}
	// No messaging sidecar
	if ds.Interfaces != nil {
		t.Error("interfaces: expected nil when messaging is disabled")
	}
}

func TestTemplate_FrontendAndMessaging(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Frontend: true, Messaging: true}
	ds := mustGenerate(t, input)

	// Agent endpoint exposed for frontend
	httpEp := spec.EndpointByName(ds.Agent.Endpoints, "http")
	if httpEp == nil || httpEp.Expose == nil || !httpEp.Expose.Enabled {
		t.Error("agent.endpoints.http.expose.enabled: expected true for frontend")
	}
	// Messaging sidecar present
	if ds.Interfaces == nil {
		t.Fatal("interfaces: expected non-nil when messaging is enabled")
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
			Integrations: map[string]spec.Integration{
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
		AgentName:   "engineering-assistant",
		Account:     "acme",
		BuildID:     "build42",
		RegistryURL: "registry.example.com",
	}

	ds := mustGenerate(t, input)

	// Models — only self-hosted (ollama), not cloud (anthropic)
	if len(ds.Models) != 1 {
		t.Errorf("models: expected 1 (ollama only), got %d", len(ds.Models))
	}
	if ds.Models["local_llm"].Image != "registry.example.com/dockerhub/ollama/ollama:latest" {
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

	// Integrations — only self-hosted (websearch), not cloud (github)
	if len(ds.Integrations) != 1 {
		t.Errorf("tools: expected 1 (websearch only), got %d", len(ds.Integrations))
	}

	// Ingestion
	if len(ds.Ingestion) != 1 {
		t.Errorf("ingestion: expected 1, got %d", len(ds.Ingestion))
	}
	if ds.Ingestion["docs_sync"].Environment["SOURCE_REPO"] != "company/docs" {
		t.Error("ingestion environment not preserved")
	}

	// Variables from cloud providers
	if _, ok := ds.Variables["ANTHROPIC_API_KEY"]; !ok {
		t.Error("missing ANTHROPIC_API_KEY variable")
	}
	if _, ok := ds.Variables["GITHUB_TOKEN"]; !ok {
		t.Error("missing GITHUB_TOKEN variable")
	}

	// Agent environment — check all component refs exist
	env := ds.Agent.Environment
	assertEnvExists(t, env, "OLLAMA_HOST")
	assertEnvExists(t, env, "QDRANT_HOST")
	assertEnvExists(t, env, "REDIS_HOST")
	assertEnvExists(t, env, "INTEGRATION_WEBSEARCH_HOST")
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
	if len(ds.Integrations) != 0 {
		t.Errorf("integrations: expected 0 for empty spec, got %d", len(ds.Integrations))
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
	if ds.Models["ollama"].Image != "registry.example.com/dockerhub/ollama/ollama:latest" {
		t.Errorf("ollama image: got %s", ds.Models["ollama"].Image)
	}
	if ds.Models["custom"].Image != "registry.example.com/dockerhub/library/custom:latest" {
		t.Errorf("custom image: got %s", ds.Models["custom"].Image)
	}
	if spec.PrimaryPort(ds.Models["custom"].Endpoints) != 5000 {
		t.Errorf("custom endpoints port: expected 5000, got %d", spec.PrimaryPort(ds.Models["custom"].Endpoints))
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
	if parsed.Spec != "deployment-template/v1" {
		t.Errorf("spec: expected deployment-template/v1, got %s", parsed.Spec)
	}
	if parsed.Source.Name != "my-agent" {
		t.Errorf("source.name: expected my-agent, got %s", parsed.Source.Name)
	}
	if parsed.Models["llm"].Image != "registry.example.com/dockerhub/ollama/ollama:latest" {
		t.Errorf("models.llm.image lost in round-trip: got %s", parsed.Models["llm"].Image)
	}
	if !parsed.Knowledge["docs"].Persistent {
		t.Error("knowledge.docs.persistent lost in round-trip")
	}
	if parsed.Knowledge["docs"].Storage == nil {
		t.Error("knowledge.docs.storage lost in round-trip")
	}
	if _, ok := parsed.Variables["ANTHROPIC_API_KEY"]; !ok {
		t.Error("variables.ANTHROPIC_API_KEY lost in round-trip")
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
		AgentName:         "my-agent",
		Account:           "acme",
		BuildID:           "abc123",
		RegistryURL:       "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
		ProxyRegistryHost: "proxy.registry.io",
		Environment:       "prod",
	}
}

func TestResolveImage_TenantImage(t *testing.T) {
	input := proxyInput()
	got := resolveImage("proxy.registry.io/acme/my-app:v1", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/my-app:v1"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveImage_TenantImageMissingImageSegment(t *testing.T) {
	input := proxyInput()
	// Only account, no image name — malformed, returned unchanged
	got := resolveImage("proxy.registry.io/acme", input)
	if got != "proxy.registry.io/acme" {
		t.Errorf("malformed tenant path should be unchanged, got %s", got)
	}
}

func TestResolveImage_TenantImageRegistryURLWithoutScheme(t *testing.T) {
	input := proxyInput()
	input.RegistryURL = "123456789.dkr.ecr.us-east-1.amazonaws.com"
	got := resolveImage("proxy.registry.io/acme/my-app:v1", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/my-app:v1"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveImage_PublicOrgImage(t *testing.T) {
	input := proxyInput()
	got := resolveImage("astropods/messaging:latest", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/astropods/messaging:latest"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveImage_PublicLibraryImage(t *testing.T) {
	input := proxyInput()
	got := resolveImage("nginx:1.27", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/nginx:1.27"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveImage_ThirdPartyImageUnchanged(t *testing.T) {
	input := proxyInput()
	cases := []string{
		"gcr.io/my-project/my-app:v1",
		"123456789.dkr.ecr.us-east-1.amazonaws.com/my-repo:latest",
		"registry.example.com/org/image:tag",
		"localhost:5000/my-image:latest",
		"docker.io/library/python:3.11",
	}
	for _, img := range cases {
		got := resolveImage(img, input)
		if got != img {
			t.Errorf("third-party image %q should be unchanged, got %q", img, got)
		}
	}
}

func TestResolveImage_EmptyImage(t *testing.T) {
	got := resolveImage("", proxyInput())
	if got != "" {
		t.Errorf("empty image should stay empty, got %s", got)
	}
}

func TestResolveImage_NoRegistryConfigured(t *testing.T) {
	input := proxyInput()
	input.RegistryURL = ""
	input.ProxyRegistryHost = ""
	got := resolveImage("astropods/messaging:latest", input)
	if got != "astropods/messaging:latest" {
		t.Errorf("without registry config, image should be unchanged, got %s", got)
	}
}

func TestResolveImage_TenantImageWithoutRegistryURL(t *testing.T) {
	// ProxyRegistryHost configured but RegistryURL missing — must not produce a malformed path
	input := proxyInput()
	input.RegistryURL = ""
	got := resolveImage("proxy.registry.io/acme/my-app:v1", input)
	if got != "proxy.registry.io/acme/my-app:v1" {
		t.Errorf("tenant image without RegistryURL should be unchanged, got %s", got)
	}
}

func TestResolveImage_RegistryURLWithTrailingSlash(t *testing.T) {
	input := proxyInput()
	input.RegistryURL = "https://123456789.dkr.ecr.us-east-1.amazonaws.com/"
	got := resolveImage("nginx:1.27", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/nginx:1.27"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveImage_PublicImageWithNoTag(t *testing.T) {
	input := proxyInput()
	got := resolveImage("nginx", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/nginx"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveImage_DockerIOPrefixIsThirdParty(t *testing.T) {
	// docker.io is an explicit registry host — treated as third-party, not routed to cache
	input := proxyInput()
	image := "docker.io/library/python:3.11"
	got := resolveImage(image, input)
	if got != image {
		t.Errorf("docker.io-prefixed image should be unchanged, got %s", got)
	}
}

// TestResolveImage_LocalEnvironmentSkipsDockerHubRewrite verifies that public
// Docker Hub images are returned unchanged when Environment is "local", since
// local Kubernetes pulls directly from Docker Hub (or the local daemon).
func TestResolveImage_LocalEnvironmentSkipsDockerHubRewrite(t *testing.T) {
	input := proxyInput()
	input.Environment = "local"
	input.RegistryURL = "docker.io/library"

	cases := []struct {
		name  string
		image string
	}{
		{"org image", "astropods/messaging:latest"},
		{"library image", "redis:7-alpine"},
		{"library image with org", "qdrant/qdrant:latest"},
		{"library image no tag", "neo4j"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveImage(tc.image, input)
			if got != tc.image {
				t.Errorf("in local env, %q should be unchanged, got %q", tc.image, got)
			}
		})
	}
}

// TestResolveImage_LocalEnvironmentStillResolvesTenantImages verifies that
// proxy-registry-hosted tenant images are still rewritten to ECR even in local
// mode, since tenant images are stored in the private registry.
func TestResolveImage_LocalEnvironmentStillResolvesTenantImages(t *testing.T) {
	input := proxyInput()
	input.Environment = "local"

	got := resolveImage("proxy.registry.io/acme/my-app:v1", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/local-tenant-acme/my-app:v1"
	if got != expected {
		t.Errorf("tenant images should still resolve in local env, expected %s, got %s", expected, got)
	}
}

// TestResolveImage_ProdEnvironmentStillRewritesDockerHub verifies that prod
// environments continue to rewrite public images through the ECR pull-through
// cache as before.
func TestResolveImage_ProdEnvironmentStillRewritesDockerHub(t *testing.T) {
	input := proxyInput()
	input.Environment = "prod"

	got := resolveImage("qdrant/qdrant:latest", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/qdrant/qdrant:latest"
	if got != expected {
		t.Errorf("prod env should rewrite to pull-through cache, expected %s, got %s", expected, got)
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

func TestTemplate_ProviderModelPublicImageViaCache(t *testing.T) {
	input := proxyInput()
	input.Spec.Models = map[string]spec.Model{
		"llm": {Provider: "ollama"},
	}

	ds := mustGenerate(t, input)

	// Public provider images are served through the ECR pull-through cache
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/ollama/ollama:latest"
	if ds.Models["llm"].Image != expected {
		t.Errorf("expected %s, got %s", expected, ds.Models["llm"].Image)
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

func TestTemplate_IntegrationImageResolved(t *testing.T) {
	input := proxyInput()
	input.Spec.Integrations = map[string]spec.Integration{
		"search": {
			Container: &spec.ContainerConfig{
				Image: "proxy.registry.io/acme/search-tool:latest",
				Port:  3000,
			},
		},
	}

	ds := mustGenerate(t, input)

	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/search-tool:latest"
	if ds.Integrations["search"].Image != expected {
		t.Errorf("tool image: expected %s, got %s", expected, ds.Integrations["search"].Image)
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
	input.Spec.Integrations = map[string]spec.Integration{
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
		"agent":       ds.Agent.Image,
		"model":       ds.Models["m"].Image,
		"knowledge":   ds.Knowledge["k"].Image,
		"integration": ds.Integrations["t"].Image,
		"ingestion":   ds.Ingestion["i"].Image,
	}
	for component, image := range checks {
		if !strings.HasPrefix(image, prefix) {
			t.Errorf("%s image not resolved: expected prefix %s, got %s", component, prefix, image)
		}
	}
}

func TestTemplate_MixedTenantAndPublicImages(t *testing.T) {
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
	// Public image is served through the ECR pull-through cache
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/ollama/ollama:latest"
	if ds.Models["public"].Image != expected {
		t.Errorf("expected %s, got %s", expected, ds.Models["public"].Image)
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
			Integrations: map[string]spec.Integration{
				"search": {Container: &spec.ContainerConfig{Image: "search:latest", Port: 3000}},
				"github": {Provider: "github"},
			},
		},
		AgentName:   "ref-test",
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
		AgentName:   "sasbot",
		Account:     "acme",
		BuildID:     "abc123",
		RegistryURL: "registry.example.com",
	}

	ds := mustGenerate(t, input)

	// Agent
	if ds.Agent.Image != "registry.example.com/sasbot:abc123" {
		t.Errorf("agent.image: got %s", ds.Agent.Image)
	}
	if spec.PrimaryPort(ds.Agent.Endpoints) != 8080 {
		t.Errorf("agent endpoints primary port: expected 8080, got %d", spec.PrimaryPort(ds.Agent.Endpoints))
	}

	// Knowledge
	if len(ds.Knowledge) != 2 {
		t.Fatalf("knowledge: expected 2, got %d", len(ds.Knowledge))
	}
	if spec.PrimaryPort(ds.Knowledge["cache"].Endpoints) != 6379 {
		t.Errorf("knowledge.cache endpoints port: expected 6379, got %d", spec.PrimaryPort(ds.Knowledge["cache"].Endpoints))
	}
	if spec.PrimaryPort(ds.Knowledge["graph"].Endpoints) != 7474 {
		t.Errorf("knowledge.graph endpoints port: expected 7474, got %d", spec.PrimaryPort(ds.Knowledge["graph"].Endpoints))
	}

	// Ingestion — webhook with explicit port
	ing := ds.Ingestion["data"]
	if ing.Image != "registry.example.com/sasbot-ingestion:abc123" {
		t.Errorf("ingestion.data.image: got %s", ing.Image)
	}
	if spec.PrimaryPort(ing.Endpoints) != 3001 {
		t.Errorf("ingestion.data endpoints port: expected 3001, got %d", spec.PrimaryPort(ing.Endpoints))
	}
	if ing.Trigger.Type != "webhook" {
		t.Errorf("ingestion.data.trigger.type: expected webhook, got %s", ing.Trigger.Type)
	}

	// Variables
	if _, ok := ds.Variables["ANTHROPIC_API_KEY"]; !ok {
		t.Error("missing ANTHROPIC_API_KEY variable")
	}

	// Interfaces present
	if ds.Interfaces == nil {
		t.Fatal("interfaces: expected non-nil")
	}

	// Deployment spec should pass validation when variables are filled
	ds.Variables["ANTHROPIC_API_KEY"] = spec.Variable{Value: "sk-test", Secret: true}
	// Fill in the webhook ingestion endpoint to make it parseable
	ds.Ingestion["data"] = spec.DeploymentIngestion{
		Image:     ing.Image,
		Endpoints: ing.Endpoints,
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
	if spec.PrimaryPort(parsed.Ingestion["data"].Endpoints) != 3001 {
		t.Errorf("round-trip port: expected 3001, got %d", spec.PrimaryPort(parsed.Ingestion["data"].Endpoints))
	}
}

// ===== Phase 14: ECR Namespace Backward Compatibility =====

// Tests that the ECRNamespace field in TemplateInput is used for ECR path
// construction, and that omitting it (empty string) falls back to parsing
// the account name from the image path — preserving pre-migration behavior.

func TestResolveImage_ECRNamespace_UsedWhenSet(t *testing.T) {
	input := proxyInput()
	input.ECRNamespace = "target-account"

	// Image path says "acme" but ECRNamespace says "target-account"
	got := resolveImage("proxy.registry.io/acme/my-app:v1", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-target-account/my-app:v1"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestResolveImage_ECRNamespace_FallbackWhenEmpty(t *testing.T) {
	input := proxyInput()
	input.ECRNamespace = "" // not set — pre-migration behavior

	got := resolveImage("proxy.registry.io/acme/my-app:v1", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-acme/my-app:v1"
	if got != expected {
		t.Errorf("expected fallback to image path account, got %s", got)
	}
}

func TestResolveImage_ECRNamespace_TransferScenario(t *testing.T) {
	// Simulates: agent was pushed under "alice", transferred to "bob".
	// The stored image path still says "alice" but ECRNamespace is "alice"
	// (frozen at push time). Images resolve to alice's ECR repos.
	input := proxyInput()
	input.Account = "bob"        // current owner after transfer
	input.ECRNamespace = "alice" // where images physically are

	got := resolveImage("proxy.registry.io/alice/my-agent:abc123", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-alice/my-agent:abc123"
	if got != expected {
		t.Errorf("transferred agent should resolve to original ECR namespace, got %s", got)
	}
}

func TestResolveImage_ECRNamespace_NewPushAfterTransfer(t *testing.T) {
	// After transfer to "bob", bob pushes a new build. The new version's
	// ECRNamespace is "bob" and images are in bob's ECR repos.
	input := proxyInput()
	input.Account = "bob"
	input.ECRNamespace = "bob"

	got := resolveImage("proxy.registry.io/bob/my-agent:newbuild", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-bob/my-agent:newbuild"
	if got != expected {
		t.Errorf("new push should resolve to new owner's ECR namespace, got %s", got)
	}
}

func TestResolveImage_ECRNamespace_DoesNotAffectPublicImages(t *testing.T) {
	input := proxyInput()
	input.ECRNamespace = "some-namespace"

	// Public images go through the pull-through cache regardless of ECRNamespace
	got := resolveImage("ollama/ollama:latest", input)
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/ollama/ollama:latest"
	if got != expected {
		t.Errorf("public images should be unaffected by ECRNamespace, got %s", got)
	}
}

func TestResolveImage_ECRNamespace_DoesNotAffectThirdParty(t *testing.T) {
	input := proxyInput()
	input.ECRNamespace = "some-namespace"

	image := "gcr.io/my-project/my-app:v1"
	got := resolveImage(image, input)
	if got != image {
		t.Errorf("third-party images should be unaffected by ECRNamespace, got %s", got)
	}
}

func TestTemplate_ECRNamespace_AllComponentsUseIt(t *testing.T) {
	// Verify that ECRNamespace flows through to all component types
	input := proxyInput()
	input.ECRNamespace = "original-owner"
	input.Spec.Agent.Image = "proxy.registry.io/acme/agent:v1"
	input.Spec.Models = map[string]spec.Model{
		"m": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/acme/model:v1", Port: 8000}},
	}
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"k": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/acme/knowledge:v1", Port: 5000}},
	}
	input.Spec.Integrations = map[string]spec.Integration{
		"t": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/acme/tool:v1", Port: 3000}},
	}
	input.Spec.Ingestion = map[string]spec.Ingestion{
		"i": {
			Container: spec.ContainerConfig{Image: "proxy.registry.io/acme/ingest:v1"},
			Trigger:   spec.IngestionTrigger{Type: "startup"},
		},
	}

	ds := mustGenerate(t, input)

	// All tenant images should resolve using "original-owner", not "acme"
	prefix := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-original-owner/"
	checks := map[string]string{
		"agent":       ds.Agent.Image,
		"model":       ds.Models["m"].Image,
		"knowledge":   ds.Knowledge["k"].Image,
		"integration": ds.Integrations["t"].Image,
		"ingestion":   ds.Ingestion["i"].Image,
	}
	for component, image := range checks {
		if !strings.HasPrefix(image, prefix) {
			t.Errorf("%s image should use ECRNamespace 'original-owner': expected prefix %s, got %s", component, prefix, image)
		}
	}
}

func TestTemplate_ECRNamespace_MixedTenantAndPublic(t *testing.T) {
	// Tenant images use ECRNamespace; public images are unaffected
	input := proxyInput()
	input.ECRNamespace = "transferred-ns"
	input.Spec.Models = map[string]spec.Model{
		"custom": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/acme/custom:v1", Port: 8000}},
		"public": {Container: &spec.ContainerConfig{Image: "ollama/ollama:latest", Port: 11434}},
	}

	ds := mustGenerate(t, input)

	// Custom model uses ECRNamespace
	if !strings.Contains(ds.Models["custom"].Image, "prod-tenant-transferred-ns") {
		t.Errorf("tenant model should use ECRNamespace, got %s", ds.Models["custom"].Image)
	}
	// Public model uses pull-through cache
	if !strings.Contains(ds.Models["public"].Image, "dockerhub/ollama") {
		t.Errorf("public model should use pull-through cache, got %s", ds.Models["public"].Image)
	}
}

// ===== Regression: Slack input defaults preserved through interfaces merge =====

func TestGenerateDeploymentTemplate_SlackInputDefaultsPreserved(t *testing.T) {
	input := TemplateInput{
		Spec: &spec.AstroSpec{
			Name: "test-agent",
			Agent: spec.Container{
				Image:      "test:latest",
				Interfaces: &spec.Interfaces{Messaging: true},
			},
			Inputs: map[string]spec.Input{
				"slack_bot_token": {Name: "SLACK_BOT_TOKEN", Secret: true, Default: "xoxb-default", Optional: true},
				"slack_app_token": {Name: "SLACK_APP_TOKEN", Secret: true, Default: "xapp-default", Optional: true},
			},
		},
		AgentName:   "test-agent",
		Account:     "acme",
		BuildID:     "abc",
		RegistryURL: "registry.example.com",
	}
	ds := mustGenerate(t, input)

	botVar := ds.Variables["SLACK_BOT_TOKEN"]
	if botVar.Default != "xoxb-default" {
		t.Errorf("SLACK_BOT_TOKEN.Default: expected xoxb-default, got %q", botVar.Default)
	}
	if botVar.Value != "xoxb-default" {
		t.Errorf("SLACK_BOT_TOKEN.Value: expected xoxb-default, got %q", botVar.Value)
	}
	if len(botVar.Targets) != 1 || botVar.Targets[0] != "interface.slack" {
		t.Errorf("SLACK_BOT_TOKEN.Targets: expected [interface.slack], got %v", botVar.Targets)
	}

	appVar := ds.Variables["SLACK_APP_TOKEN"]
	if appVar.Default != "xapp-default" {
		t.Errorf("SLACK_APP_TOKEN.Default: expected xapp-default, got %q", appVar.Default)
	}
	if appVar.Value != "xapp-default" {
		t.Errorf("SLACK_APP_TOKEN.Value: expected xapp-default, got %q", appVar.Value)
	}
	if len(appVar.Targets) != 1 || appVar.Targets[0] != "interface.slack" {
		t.Errorf("SLACK_APP_TOKEN.Targets: expected [interface.slack], got %v", appVar.Targets)
	}
}

// ===== Regression: Credential+input collision preserves input default =====

func TestGenerateDeploymentTemplate_CredentialInputDefaultMerge(t *testing.T) {
	input := TemplateInput{
		Spec: &spec.AstroSpec{
			Name:  "test-agent",
			Agent: spec.Container{Image: "test:latest"},
			Models: map[string]spec.Model{
				"anthropic": {Provider: "anthropic"},
			},
			Inputs: map[string]spec.Input{
				"anthropic_api_key": {Name: "ANTHROPIC_API_KEY", Secret: true, Default: "sk-test", Optional: true},
			},
		},
		AgentName:   "test-agent",
		Account:     "acme",
		BuildID:     "abc",
		RegistryURL: "registry.example.com",
	}
	ds := mustGenerate(t, input)

	v := ds.Variables["ANTHROPIC_API_KEY"]
	if v.Default != "sk-test" {
		t.Errorf("ANTHROPIC_API_KEY.Default: expected sk-test, got %q", v.Default)
	}
	if v.Value != "sk-test" {
		t.Errorf("ANTHROPIC_API_KEY.Value: expected sk-test, got %q", v.Value)
	}
	if !v.Secret {
		t.Error("ANTHROPIC_API_KEY.Secret: expected true")
	}
}

// ===== ECR namespace migration: old builds (account name) vs new builds (UUID) =====
//
// After the ecr-tenant-correction change, ECRNamespace stores the account UUID.
// Existing agent_version rows still store the account name. Both must continue
// to resolve to valid ECR image URIs without any data migration.

const (
	testAccountName = "saswatds"
	testAccountUUID = "01kggdgfrw46qcsnxeqbr1hr1z"
)

// migrationProxyInput returns a TemplateInput wired up with proxy/ECR config,
// leaving ECRNamespace unset so each test can set it explicitly.
func migrationProxyInput() TemplateInput {
	return TemplateInput{
		Spec: &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "proxy.registry.io/" + testAccountName + "/my-agent:abc"},
		},
		AgentName:         "my-agent",
		Account:           testAccountName,
		BuildID:           "abc",
		RegistryURL:       "https://123456789.dkr.ecr.us-east-1.amazonaws.com",
		ProxyRegistryHost: "proxy.registry.io",
		Environment:       "prod",
	}
}

// TestResolveImage_OldBuild_AccountNameNamespace verifies that a pre-migration
// agent_version row (ECRNamespace = account name) still resolves to the correct
// ECR path. The ECR repo under the account name still exists, so this must work.
func TestResolveImage_OldBuild_AccountNameNamespace(t *testing.T) {
	input := migrationProxyInput()
	input.ECRNamespace = testAccountName // old format: stored as "saswatds"

	got := resolveImage("proxy.registry.io/"+testAccountName+"/my-agent:abc", input)
	want := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-saswatds/my-agent:abc"
	if got != want {
		t.Errorf("old build: expected %s, got %s", want, got)
	}
}

// TestResolveImage_NewBuild_UUIDNamespace verifies that a post-migration
// agent_version row (ECRNamespace = UUID) resolves to the UUID-namespaced ECR path.
func TestResolveImage_NewBuild_UUIDNamespace(t *testing.T) {
	input := migrationProxyInput()
	input.ECRNamespace = testAccountUUID // new format: stored as UUID

	got := resolveImage("proxy.registry.io/"+testAccountName+"/my-agent:newbuild", input)
	want := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-01kggdgfrw46qcsnxeqbr1hr1z/my-agent:newbuild"
	if got != want {
		t.Errorf("new build: expected %s, got %s", want, got)
	}
}

// TestResolveImage_OldAndNewBuilds_IndependentECRPaths verifies that the same
// agent with an old build and a new build resolve to different ECR paths — the
// old path under the account name, the new path under the UUID. Both ECR repos
// coexist and each build resolves to its own repo.
func TestResolveImage_OldAndNewBuilds_IndependentECRPaths(t *testing.T) {
	base := migrationProxyInput()
	image := "proxy.registry.io/" + testAccountName + "/my-agent:tag"

	oldBuildInput := base
	oldBuildInput.ECRNamespace = testAccountName
	oldPath := resolveImage(image, oldBuildInput)

	newBuildInput := base
	newBuildInput.ECRNamespace = testAccountUUID
	newPath := resolveImage(image, newBuildInput)

	wantOld := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-saswatds/my-agent:tag"
	wantNew := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-01kggdgfrw46qcsnxeqbr1hr1z/my-agent:tag"

	if oldPath != wantOld {
		t.Errorf("old build path: expected %s, got %s", wantOld, oldPath)
	}
	if newPath != wantNew {
		t.Errorf("new build path: expected %s, got %s", wantNew, newPath)
	}
	if oldPath == newPath {
		t.Error("old and new builds must resolve to different ECR paths")
	}
}

// TestTemplate_OldBuild_AllComponentsResolveWithAccountName verifies that a
// full template generated from an old agent_version (ECRNamespace = account name)
// produces valid ECR URIs for every component type.
func TestTemplate_OldBuild_AllComponentsResolveWithAccountName(t *testing.T) {
	input := migrationProxyInput()
	input.ECRNamespace = testAccountName
	input.Spec.Models = map[string]spec.Model{
		"m": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/" + testAccountName + "/model:abc", Port: 8000}},
	}
	input.Spec.Integrations = map[string]spec.Integration{
		"t": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/" + testAccountName + "/tool:abc", Port: 3000}},
	}

	ds := mustGenerate(t, input)

	wantPrefix := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-saswatds/"
	checks := map[string]string{
		"agent":       ds.Agent.Image,
		"model":       ds.Models["m"].Image,
		"integration": ds.Integrations["t"].Image,
	}
	for component, image := range checks {
		if !strings.HasPrefix(image, wantPrefix) {
			t.Errorf("old build %s: expected prefix %s, got %s", component, wantPrefix, image)
		}
	}
}

// TestTemplate_NewBuild_AllComponentsResolveWithUUID verifies that a full
// template generated from a new agent_version (ECRNamespace = UUID) produces
// UUID-namespaced ECR URIs for every component type.
func TestTemplate_NewBuild_AllComponentsResolveWithUUID(t *testing.T) {
	input := migrationProxyInput()
	input.ECRNamespace = testAccountUUID
	input.Spec.Models = map[string]spec.Model{
		"m": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/" + testAccountName + "/model:new", Port: 8000}},
	}
	input.Spec.Integrations = map[string]spec.Integration{
		"t": {Container: &spec.ContainerConfig{Image: "proxy.registry.io/" + testAccountName + "/tool:new", Port: 3000}},
	}

	ds := mustGenerate(t, input)

	wantPrefix := "123456789.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-01kggdgfrw46qcsnxeqbr1hr1z/"
	checks := map[string]string{
		"agent":       ds.Agent.Image,
		"model":       ds.Models["m"].Image,
		"integration": ds.Integrations["t"].Image,
	}
	for component, image := range checks {
		if !strings.HasPrefix(image, wantPrefix) {
			t.Errorf("new build %s: expected prefix %s, got %s", component, wantPrefix, image)
		}
	}
}
