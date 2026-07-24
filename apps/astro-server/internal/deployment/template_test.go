package deployment

import (
	"context"
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

func TestTemplate_AgentBuildWithoutImage(t *testing.T) {
	// agent.container.build without image must synthesize the canonical name
	// so deployment generation succeeds against a raw (not registry-rewritten) spec.
	input := baseInput()
	input.Spec.Agent.Image = ""
	input.Spec.Agent.Build = &spec.BuildConfig{Context: ".", Dockerfile: "Dockerfile"}

	ds := mustGenerate(t, input)

	expected := "registry.example.com/dockerhub/library/my-agent"
	if ds.Agent.Image != expected {
		t.Errorf("agent.image: expected %s, got %s", expected, ds.Agent.Image)
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

func TestTemplate_ContainerModel_BuildWithoutImage(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"embedder": {
			Container: &spec.ContainerConfig{
				Build: &spec.BuildConfig{Context: ".", Dockerfile: "models/embedder/Dockerfile"},
				Port:  9999,
			},
		},
	}

	ds := mustGenerate(t, input)

	m := ds.Models["embedder"]
	expected := "registry.example.com/dockerhub/library/my-agent-model-embedder"
	if m.Image != expected {
		t.Errorf("image: expected %s, got %s", expected, m.Image)
	}
	if spec.PrimaryPort(m.Endpoints) != 9999 {
		t.Errorf("endpoints primary port: expected 9999, got %d", spec.PrimaryPort(m.Endpoints))
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

// D3: openai cloud model — no container, OPENAI_API_KEY variable wired to agent.
func TestTemplate_ProviderModel_OpenAI(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"gpt": {Provider: "openai"},
	}

	ds := mustGenerate(t, input)

	if _, ok := ds.Models["gpt"]; ok {
		t.Error("models.gpt: cloud provider must not produce a container entry")
	}
	v, ok := ds.Variables["OPENAI_API_KEY"]
	if !ok {
		t.Fatal("variables: OPENAI_API_KEY not found")
	}
	if !v.Secret {
		t.Error("variables.OPENAI_API_KEY: expected secret=true")
	}
	assertEnvRef(t, ds.Agent.Environment, "OPENAI_API_KEY", "${variables.OPENAI_API_KEY}")
}

// D4: google cloud model — no container, GOOGLE_API_KEY variable wired to agent.
func TestTemplate_ProviderModel_Google(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"gemini": {Provider: "google"},
	}

	ds := mustGenerate(t, input)

	if _, ok := ds.Models["gemini"]; ok {
		t.Error("models.gemini: cloud provider must not produce a container entry")
	}
	if _, ok := ds.Variables["GOOGLE_API_KEY"]; !ok {
		t.Fatal("variables: GOOGLE_API_KEY not found")
	}
	assertEnvRef(t, ds.Agent.Environment, "GOOGLE_API_KEY", "${variables.GOOGLE_API_KEY}")
}

// D5: cohere cloud model — no container, COHERE_API_KEY variable wired to agent.
func TestTemplate_ProviderModel_Cohere(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"cmd": {Provider: "cohere"},
	}

	ds := mustGenerate(t, input)

	if _, ok := ds.Models["cmd"]; ok {
		t.Error("models.cmd: cloud provider must not produce a container entry")
	}
	if _, ok := ds.Variables["COHERE_API_KEY"]; !ok {
		t.Fatal("variables: COHERE_API_KEY not found")
	}
	assertEnvRef(t, ds.Agent.Environment, "COHERE_API_KEY", "${variables.COHERE_API_KEY}")
}

// D7: two cloud providers together — both credential variables present, both wired.
func TestTemplate_ProviderModel_MultipleCloudProviders(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"claude": {Provider: "anthropic"},
		"gpt":    {Provider: "openai"},
	}

	ds := mustGenerate(t, input)

	if len(ds.Models) != 0 {
		t.Errorf("models: expected 0 container entries for cloud-only providers, got %d", len(ds.Models))
	}
	if _, ok := ds.Variables["ANTHROPIC_API_KEY"]; !ok {
		t.Fatal("variables: ANTHROPIC_API_KEY not found")
	}
	if _, ok := ds.Variables["OPENAI_API_KEY"]; !ok {
		t.Fatal("variables: OPENAI_API_KEY not found")
	}
	assertEnvExists(t, ds.Agent.Environment, "ANTHROPIC_API_KEY")
	assertEnvExists(t, ds.Agent.Environment, "OPENAI_API_KEY")
}

// D8: container and cloud model together — container-mode model deploys a
// sidecar, anthropic produces only a credential variable.
func TestTemplate_ProviderModel_ContainerAndCloud(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"local":  {Container: &spec.ContainerConfig{Image: "my-model:latest", Port: 8000}},
		"claude": {Provider: "anthropic"},
	}

	ds := mustGenerate(t, input)

	if _, ok := ds.Models["local"]; !ok {
		t.Error("models.local: expected container entry for container-mode model")
	}
	if _, ok := ds.Models["claude"]; ok {
		t.Error("models.claude: cloud provider must not produce a container entry")
	}
	assertEnvExists(t, ds.Agent.Environment, "MODEL_LOCAL_HOST")
	assertEnvExists(t, ds.Agent.Environment, "ANTHROPIC_API_KEY")
}

// ===== Phase 4: Knowledge =====

func TestTemplate_ProviderKnowledge_Qdrant(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"docs": {Provider: "qdrant"},
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
	// Redis provider has MountPath /data → persistent by derivation
	if !k.Persistent {
		t.Error("persistent: expected true (redis provider has MountPath)")
	}
	if k.Storage == nil {
		t.Error("storage: expected non-nil for persistent store")
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

func TestTemplate_ContainerKnowledge_BuildWithoutImage(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"chat-sandbox": {
			Container: &spec.ContainerConfig{
				Build: &spec.BuildConfig{
					Context:    ".",
					Dockerfile: "sandbox/Dockerfile",
				},
				Port:   3000,
				Volume: "/data",
			},
		},
	}

	ds := mustGenerate(t, input)

	k := ds.Knowledge["chat-sandbox"]
	// Image should be synthesized from agent name + knowledge entry name
	expectedImage := "registry.example.com/dockerhub/library/my-agent-knowledge-chat-sandbox"
	if k.Image != expectedImage {
		t.Errorf("image: expected %s, got %s", expectedImage, k.Image)
	}
	if spec.PrimaryPort(k.Endpoints) != 3000 {
		t.Errorf("endpoints primary port: expected 3000, got %d", spec.PrimaryPort(k.Endpoints))
	}
	if !k.Persistent {
		t.Error("persistent: expected true (volume is set)")
	}
}

func TestTemplate_ProviderKnowledge_Postgres(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"db": {Provider: "postgres"},
	}

	ds := mustGenerate(t, input)

	k := ds.Knowledge["db"]
	if !strings.Contains(k.Image, "pgvector") {
		t.Errorf("image: expected pgvector image, got %s", k.Image)
	}
	if spec.PrimaryPort(k.Endpoints) != 5432 {
		t.Errorf("endpoints primary port: expected 5432, got %d", spec.PrimaryPort(k.Endpoints))
	}
	if !k.Persistent {
		t.Error("persistent: expected true (postgres provider has MountPath)")
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
	if k.Healthcheck == nil || len(k.Healthcheck.Test) == 0 {
		t.Errorf("healthcheck: expected exec test for postgres provider, got %+v", k.Healthcheck)
	}

	assertEnvRef(t, ds.Agent.Environment, "POSTGRES_HOST", "${knowledge.db.host}")
	assertEnvRef(t, ds.Agent.Environment, "POSTGRES_PORT", "${knowledge.db.http.port}")

	// Credentials must not be in the agent env — they flow via secretKeyRef at apply time.
	for _, cred := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		if _, ok := ds.Agent.Environment[cred]; ok {
			t.Errorf("agent.environment.%s: must not be present — flows via secretKeyRef", cred)
		}
	}
}

func TestTemplate_ProviderKnowledge_Pinecone(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"index": {Provider: "pinecone"},
	}

	ds := mustGenerate(t, input)

	// Cloud knowledge provider — no sidecar container in the spec.
	if _, ok := ds.Knowledge["index"]; ok {
		t.Error("knowledge.index: cloud provider should not produce a container entry")
	}

	v, ok := ds.Variables["PINECONE_API_KEY"]
	if !ok {
		t.Fatal("variables: PINECONE_API_KEY not found")
	}
	if !v.Secret {
		t.Error("variables.PINECONE_API_KEY: expected secret=true")
	}
	assertEnvRef(t, ds.Agent.Environment, "PINECONE_API_KEY", "${variables.PINECONE_API_KEY}")
}

func TestTemplate_KnowledgeNonPersistent_NoStorage(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"cache": {
			Container: &spec.ContainerConfig{
				Image: "my-cache:latest",
				Port:  6000,
			},
		},
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

func TestTemplate_AIGatewayMarkerPropagates(t *testing.T) {
	// agent.astro_ai_gateway: true on the source spec flows through to the
	// deployment spec's DeploymentAgent.AIGateway field. No variables
	// surface — the deployer mints the key at apply time, not via
	// user-supplied variables.
	input := baseInput()
	input.Spec.Agent = spec.Container{Image: "agent:latest", AIGateway: true}

	ds := mustGenerate(t, input)

	if !ds.Agent.AIGateway {
		t.Error("DeploymentAgent.AIGateway must be true when source spec sets agent.astro_ai_gateway")
	}
	if _, ok := ds.Variables["ASTRO_GATEWAY_API_KEY"]; ok {
		t.Error("astro_ai_gateway opt-in must not surface as a user-facing variable")
	}
	if _, ok := ds.Variables["ASTRO_GATEWAY_URL"]; ok {
		t.Error("astro_ai_gateway opt-in must not surface as a user-facing variable")
	}
}

func TestTemplate_AIGatewayAndRegularProvidersTogether(t *testing.T) {
	// Mixing the gateway with a BYOK provider: only the BYOK provider
	// produces a user-facing variable. The gateway's URL/API key are
	// injected by the deployer at apply time.
	input := baseInput()
	input.Spec.Agent = spec.Container{Image: "agent:latest", AIGateway: true}
	input.Spec.Models = map[string]spec.Model{
		"user-openai": {Provider: "openai"},
	}

	ds := mustGenerate(t, input)

	if !ds.Agent.AIGateway {
		t.Error("DeploymentAgent.AIGateway must be true")
	}
	if _, ok := ds.Variables["OPENAI_API_KEY"]; !ok {
		t.Error("openai should produce OPENAI_API_KEY variable")
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

// F4: agent component inputs appear as variables with target="agent" and are
// wired into agent.environment for non-secret inputs.
func TestTemplate_AgentComponentInputs(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Inputs = []spec.Input{
		{Name: "FEATURE_FLAG", Datatype: "boolean", Default: "false", Optional: true, Description: "Toggle feature"},
		{Name: "API_SECRET", Datatype: "string", Secret: true, Description: "A secret API key"},
	}

	ds := mustGenerate(t, input)

	flag, ok := ds.Variables["FEATURE_FLAG"]
	if !ok {
		t.Fatal("variables: FEATURE_FLAG not found")
	}
	if flag.Default != "false" {
		t.Errorf("FEATURE_FLAG.Default: expected false, got %q", flag.Default)
	}
	if !flag.Optional {
		t.Error("FEATURE_FLAG.Optional: expected true")
	}
	if len(flag.Targets) != 1 || flag.Targets[0] != "agent" {
		t.Errorf("FEATURE_FLAG.Targets: expected [agent], got %v", flag.Targets)
	}

	// Non-secret agent input wired to agent env.
	assertEnvRef(t, ds.Agent.Environment, "FEATURE_FLAG", "${variables.FEATURE_FLAG}")

	// Secret agent input must not appear in agent env.
	if _, ok := ds.Agent.Environment["API_SECRET"]; ok {
		t.Error("API_SECRET: secret input must not be in agent.environment")
	}
}

// F5: model component inputs with a default value are injected directly into
// the model container's environment — they do NOT produce a variable entry.
func TestTemplate_ModelComponentInputs(t *testing.T) {
	input := baseInput()
	input.Spec.Models = map[string]spec.Model{
		"llm": {
			Container: &spec.ContainerConfig{Image: "llm:latest", Port: 8000},
			Inputs:    []spec.Input{{Name: "CONTEXT_LENGTH", Datatype: "number", Default: "4096"}},
		},
	}

	ds := mustGenerate(t, input)

	if ds.Models["llm"].Environment["CONTEXT_LENGTH"] != "4096" {
		t.Errorf("models.llm.environment.CONTEXT_LENGTH: expected 4096, got %q", ds.Models["llm"].Environment["CONTEXT_LENGTH"])
	}
	// Model inputs are NOT promoted to the variables map.
	if _, ok := ds.Variables["CONTEXT_LENGTH"]; ok {
		t.Error("variables: CONTEXT_LENGTH must not be in variables — model inputs go to the container env directly")
	}
}

// F6: knowledge component inputs with a default value are injected directly
// into the knowledge container's environment — they do NOT produce a variable entry.
func TestTemplate_KnowledgeComponentInputs(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"cache": {
			Provider: "redis",
			Inputs:   []spec.Input{{Name: "MAXMEMORY", Datatype: "string", Default: "512mb"}},
		},
	}

	ds := mustGenerate(t, input)

	if ds.Knowledge["cache"].Environment["MAXMEMORY"] != "512mb" {
		t.Errorf("knowledge.cache.environment.MAXMEMORY: expected 512mb, got %q", ds.Knowledge["cache"].Environment["MAXMEMORY"])
	}
	if _, ok := ds.Variables["MAXMEMORY"]; ok {
		t.Error("variables: MAXMEMORY must not be in variables — knowledge inputs go to the container env directly")
	}
}

// F7: ingestion component inputs produce a variable with target="ingestion.<name>"
// AND inject the default into the ingestion container's environment.
func TestTemplate_IngestionComponentInputs(t *testing.T) {
	input := baseInput()
	input.Spec.Ingestion = map[string]spec.Ingestion{
		"sync": {
			Container: spec.ContainerConfig{Image: "sync:latest"},
			Trigger:   spec.IngestionTrigger{Type: "startup"},
			Inputs:    []spec.Input{{Name: "BATCH_SIZE", Datatype: "number", Default: "100", Optional: true}},
		},
	}

	ds := mustGenerate(t, input)

	v, ok := ds.Variables["BATCH_SIZE"]
	if !ok {
		t.Fatal("variables: BATCH_SIZE not found")
	}
	if len(v.Targets) != 1 || v.Targets[0] != "ingestion.sync" {
		t.Errorf("BATCH_SIZE.Targets: expected [ingestion.sync], got %v", v.Targets)
	}
	if v.Default != "100" {
		t.Errorf("BATCH_SIZE.Default: expected 100, got %q", v.Default)
	}
	// Default also wired directly into ingestion container env.
	if ds.Ingestion["sync"].Environment["BATCH_SIZE"] != "100" {
		t.Errorf("ingestion.sync.environment.BATCH_SIZE: expected 100, got %q", ds.Ingestion["sync"].Environment["BATCH_SIZE"])
	}
}

// When slack is selected via ApplyAdapterShaping, all three slack vars appear
// with correct metadata and are wired into interfaces.environment.
func TestTemplate_SlackConfigVariable_WithSpecConfig(t *testing.T) {
	ds := mustGenerate(t, baseInput())
	ApplyAdapterShaping(ds, []string{"slack"})

	v, ok := ds.Variables["SLACK_CONFIG"]
	if !ok {
		t.Fatal("SLACK_CONFIG variable not found after slack shaping")
	}
	if v.Secret {
		t.Error("SLACK_CONFIG should not be secret")
	}
	if !v.Optional {
		t.Error("SLACK_CONFIG should remain optional (valid empty defaults exist)")
	}
	if len(v.Targets) != 1 || v.Targets[0] != "interface.slack" {
		t.Errorf("SLACK_CONFIG targets = %v, want [interface.slack]", v.Targets)
	}
	if v.Value != "" {
		t.Errorf("SLACK_CONFIG value should be empty (no dev config), got %q", v.Value)
	}

	envRef, ok := ds.Interfaces.Environment["SLACK_CONFIG"]
	if !ok {
		t.Fatal("interfaces.environment should contain SLACK_CONFIG")
	}
	if envRef != "${variables.SLACK_CONFIG}" {
		t.Errorf("interfaces.environment ref = %q, want ${variables.SLACK_CONFIG}", envRef)
	}

	for _, key := range []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"} {
		if ref, ok := ds.Interfaces.Environment[key]; !ok {
			t.Errorf("interfaces.environment should contain %s ref", key)
		} else if ref != "${variables."+key+"}" {
			t.Errorf("%s ref = %q, want ${variables.%s}", key, ref, key)
		}
	}

	wantFields := []string{
		"actionable_reactions",
		"allowed_channel_ids",
		"allowed_user_ids",
		"observe_channel_ids",
	}
	for _, key := range wantFields {
		f, ok := v.Fields[key]
		if !ok {
			t.Errorf("SLACK_CONFIG.Fields missing %q", key)
			continue
		}
		if f.Datatype != "csv" {
			t.Errorf("SLACK_CONFIG.Fields[%q].Datatype = %q, want csv", key, f.Datatype)
		}
		if !f.Optional {
			t.Errorf("SLACK_CONFIG.Fields[%q] should be optional", key)
		}
		if f.Label == "" {
			t.Errorf("SLACK_CONFIG.Fields[%q].Label is empty", key)
		}
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

// A7: messaging enabled → all interfaces block fields have correct defaults.
func TestYAML_Interfaces_A7_AllDefaults(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
`)

	if ds.Interfaces == nil {
		t.Fatal("interfaces: expected non-nil")
	}

	// No adapter selected by default
	if len(ds.Interfaces.Adapters) != 0 {
		t.Errorf("interfaces.adapters: expected empty, got %v", ds.Interfaces.Adapters)
	}

	// Messaging image resolved through registry
	if ds.Interfaces.Image != "registry.example.com/dockerhub/astropods/messaging:latest" {
		t.Errorf("interfaces.image: got %s", ds.Interfaces.Image)
	}

	// gRPC endpoint on 9090
	grpcEp := spec.EndpointByName(ds.Interfaces.Endpoints, "grpc")
	if grpcEp == nil {
		t.Fatal("interfaces.endpoints.grpc: expected non-nil")
	}
	if grpcEp.Port != 9090 {
		t.Errorf("interfaces.endpoints.grpc.port: expected 9090, got %d", grpcEp.Port)
	}

	// HTTP endpoint present but not publicly exposed
	httpEp := spec.EndpointByName(ds.Interfaces.Endpoints, "http")
	if httpEp == nil {
		t.Fatal("interfaces.endpoints.http: expected non-nil")
	}
	if httpEp.Port != 8080 {
		t.Errorf("interfaces.endpoints.http.port: expected 8080, got %d", httpEp.Port)
	}
	if httpEp.Expose != nil && httpEp.Expose.Enabled {
		t.Error("interfaces.endpoints.http.expose.enabled: expected false by default")
	}

	// Resources match MessagingResources
	if ds.Interfaces.Resources != spec.MessagingResources {
		t.Errorf("interfaces.resources: expected MessagingResources %+v, got %+v", spec.MessagingResources, ds.Interfaces.Resources)
	}

	// Auth defaults to web OIDC; no slack auth configured
	if ds.Interfaces.Auth == nil {
		t.Fatal("interfaces.auth: expected non-nil")
	}
	if ds.Interfaces.Auth.Web == nil {
		t.Fatal("interfaces.auth.web: expected non-nil")
	}
	if ds.Interfaces.Auth.Web.Type != "oidc" {
		t.Errorf("interfaces.auth.web.type: expected oidc, got %q", ds.Interfaces.Auth.Web.Type)
	}
	if ds.Interfaces.Auth.Slack != nil {
		t.Errorf("interfaces.auth.slack: expected nil by default, got %+v", ds.Interfaces.Auth.Slack)
	}
}

// ===== Phase 9: Full Combination =====

// TestTemplate_FullSpec exercises every spec feature together: all self-hosted
// and cloud model providers, all knowledge providers, all ingestion trigger
// types, a custom provider, inputs at every level, interfaces, and a managed
// store binding.
// The primary assertion is that every ${...} reference in the generated
// template resolves correctly against the spec — this catches wiring bugs
// that individual provider tests would miss.
func TestTemplate_FullSpec(t *testing.T) {
	input := TemplateInput{
		Spec: &spec.AstroSpec{
			Name: "research-assistant",
			Agent: spec.Container{
				Image:      "registry.example.com/acme/research-assistant:build1",
				Interfaces: &spec.Interfaces{Messaging: true},
				Inputs: []spec.Input{
					{Name: "LOG_LEVEL", Datatype: "string", Default: "info", Optional: true},
				},
			},
			Models: map[string]spec.Model{
				"local":   {Container: &spec.ContainerConfig{Image: "my-model:latest", Port: 8000}},
				"claude":  {Provider: "anthropic"},
				"gpt":     {Provider: "openai"},
				"gemini":  {Provider: "google"},
				"command": {Provider: "cohere"},
			},
			Knowledge: map[string]spec.Knowledge{
				"db":      {Provider: "postgres"},
				"cache":   {Provider: "redis"},
				"vectors": {Provider: "qdrant"},
				"graph":   {Provider: "neo4j"},
				"index":   {Provider: "pinecone"},
			},
			Providers: map[string]spec.CustomProvider{
				"acme": {
					Scope: []string{"integrations"},
					Variables: []spec.Input{
						{Name: "API_KEY", Datatype: "string", Secret: true, Description: "ACME API key"},
					},
				},
			},
			Integrations: map[string]spec.Integration{
				"search": {Container: &spec.ContainerConfig{Image: "search:latest", Port: 3000}},
				"acme":   {Provider: "acme"},
			},
			Ingestion: map[string]spec.Ingestion{
				"nightly": {
					Container: spec.ContainerConfig{Image: "sync:latest"},
					Trigger:   spec.IngestionTrigger{Type: "schedule"},
				},
				"hook": {
					Container: spec.ContainerConfig{Image: "hook:latest", Port: 8090},
					Trigger:   spec.IngestionTrigger{Type: "webhook"},
				},
				"boot": {
					Container: spec.ContainerConfig{Image: "seed:latest"},
					Trigger:   spec.IngestionTrigger{Type: "startup"},
				},
			},
			Inputs: map[string]spec.Input{
				"debug": {Name: "DEBUG", Datatype: "boolean", Default: "false", Optional: true},
			},
		},
		AgentName:   "research-assistant",
		Account:     "acme",
		BuildID:     "build1",
		RegistryURL: "registry.example.com",
	}

	ds := mustGenerate(t, input)

	// --- Models ---
	// Container-mode model only; all cloud providers produce no container.
	if len(ds.Models) != 1 {
		t.Errorf("models: expected 1 container (local), got %d", len(ds.Models))
	}
	if _, ok := ds.Models["local"]; !ok {
		t.Error("models.local: container-mode model missing")
	}
	for _, cloud := range []string{"claude", "gpt", "gemini", "command"} {
		if _, ok := ds.Models[cloud]; ok {
			t.Errorf("models.%s: cloud provider must not produce a container", cloud)
		}
	}

	// Cloud credentials for all four cloud providers.
	for _, key := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "COHERE_API_KEY"} {
		if _, ok := ds.Variables[key]; !ok {
			t.Errorf("variables: %s not found", key)
		}
	}

	// --- Knowledge ---
	// Self-hosted stores present; cloud (pinecone) absent.
	for _, name := range []string{"db", "cache", "vectors", "graph"} {
		if _, ok := ds.Knowledge[name]; !ok {
			t.Errorf("knowledge.%s: expected container entry", name)
		}
	}
	if _, ok := ds.Knowledge["index"]; ok {
		t.Error("knowledge.index: cloud provider (pinecone) must not produce a container")
	}
	if _, ok := ds.Variables["PINECONE_API_KEY"]; !ok {
		t.Error("variables: PINECONE_API_KEY not found for cloud knowledge provider")
	}
	if !ds.Knowledge["db"].Persistent {
		t.Error("knowledge.db: expected persistent=true")
	}
	if !ds.Knowledge["vectors"].Persistent {
		t.Error("knowledge.vectors: expected persistent=true")
	}

	// --- Integrations ---
	// Self-hosted: search only; cloud (acme) absent.
	if _, ok := ds.Integrations["search"]; !ok {
		t.Error("integrations.search: expected container entry")
	}
	if _, ok := ds.Integrations["acme"]; ok {
		t.Error("integrations.acme: custom cloud provider must not produce a container")
	}
	if _, ok := ds.Variables["ACME_API_KEY"]; !ok {
		t.Error("variables: ACME_API_KEY not found for custom provider")
	}

	// --- Ingestion ---
	if len(ds.Ingestion) != 3 {
		t.Errorf("ingestion: expected 3, got %d", len(ds.Ingestion))
	}
	if ds.Ingestion["nightly"].Trigger.Type != "schedule" {
		t.Error("ingestion.nightly: expected schedule trigger")
	}
	if ds.Ingestion["hook"].Trigger.Type != "webhook" {
		t.Error("ingestion.hook: expected webhook trigger")
	}
	if ds.Ingestion["boot"].Trigger.Type != "startup" {
		t.Error("ingestion.boot: expected startup trigger")
	}

	// --- Inputs ---
	// Top-level DEBUG and agent-level LOG_LEVEL both in variables.
	if _, ok := ds.Variables["DEBUG"]; !ok {
		t.Error("variables: DEBUG (top-level input) not found")
	}
	if _, ok := ds.Variables["LOG_LEVEL"]; !ok {
		t.Error("variables: LOG_LEVEL (agent input) not found")
	}

	// --- Agent environment ---
	env := ds.Agent.Environment
	for _, key := range []string{
		"MODEL_LOCAL_HOST",
		"POSTGRES_HOST", "REDIS_HOST", "QDRANT_HOST", "NEO4J_HOST",
		"INTEGRATION_SEARCH_HOST",
		"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "COHERE_API_KEY",
		"PINECONE_API_KEY", "ACME_API_KEY",
		"DEBUG", "LOG_LEVEL",
		"ASTRO_AGENT_NAME", "ASTRO_AGENT_BUILD",
	} {
		assertEnvExists(t, env, key)
	}

	// --- Reference integrity ---
	// Every ${...} value in the agent environment must parse and resolve.
	for key, val := range env {
		if !strings.HasPrefix(val, "${") {
			continue
		}
		refs := spec.ParseReferences(val)
		if len(refs) == 0 {
			t.Errorf("env %s: %q looks like a reference but failed to parse", key, val)
			continue
		}
		if errs := spec.ValidateReferences(refs, ds); len(errs) > 0 {
			t.Errorf("env %s: reference %q does not resolve: %v", key, val, errs)
		}
	}

	// --- Bindings ---
	// Bind the postgres store to a managed store. The container fields should be
	// zeroed, credential variables removed, and editable fields for that entry gone.
	bindingARN := "arn:knowledge-store:acme:pg-managed"
	submitted := &spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"db": {Binding: bindingARN, Provider: "postgres"},
		},
	}
	ApplyBindingShaping(ds, submitted)

	bound := ds.Knowledge["db"]
	if bound.Binding != bindingARN {
		t.Errorf("knowledge.db.binding: expected %q, got %q", bindingARN, bound.Binding)
	}
	if bound.Image != "" {
		t.Errorf("knowledge.db.image: expected empty after binding, got %q", bound.Image)
	}
	if len(bound.Endpoints) != 0 {
		t.Errorf("knowledge.db.endpoints: expected empty after binding, got %v", bound.Endpoints)
	}
	// Credential variables scoped to the bound entry should be removed.
	for _, cred := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		if v, ok := ds.Variables[cred]; ok {
			for _, target := range v.Targets {
				if target == "knowledge.db" {
					t.Errorf("variables.%s: should have been removed after binding knowledge.db", cred)
				}
			}
		}
	}
	// Unbound entries must be untouched.
	if ds.Knowledge["cache"].Image == "" {
		t.Error("knowledge.cache: unbound entry should still have its image")
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
		"llm":    {Container: &spec.ContainerConfig{Image: "my-model:latest", Port: 8000}},
		"custom": {Container: &spec.ContainerConfig{Image: "custom:latest", Port: 5000}},
	}

	ds := mustGenerate(t, input)

	if len(ds.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(ds.Models))
	}
	if ds.Models["llm"].Image != "registry.example.com/dockerhub/library/my-model:latest" {
		t.Errorf("llm image: got %s", ds.Models["llm"].Image)
	}
	if ds.Models["custom"].Image != "registry.example.com/dockerhub/library/custom:latest" {
		t.Errorf("custom image: got %s", ds.Models["custom"].Image)
	}
	if spec.PrimaryPort(ds.Models["custom"].Endpoints) != 5000 {
		t.Errorf("custom endpoints port: expected 5000, got %d", spec.PrimaryPort(ds.Models["custom"].Endpoints))
	}

	// Each container-mode model gets generic MODEL_<NAME>_* env vars
	assertEnvExists(t, ds.Agent.Environment, "MODEL_LLM_HOST")
	assertEnvExists(t, ds.Agent.Environment, "MODEL_CUSTOM_HOST")
}

func TestTemplate_MultipleKnowledgeProviders(t *testing.T) {
	input := baseInput()
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"vectors": {Provider: "qdrant"},
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
		"llm":       {Container: &spec.ContainerConfig{Image: "my-model:latest", Port: 8000}},
		"anthropic": {Provider: "anthropic"},
	}
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"docs": {Provider: "qdrant"},
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
	if parsed.Models["llm"].Image != "registry.example.com/dockerhub/library/my-model:latest" {
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
	// Pull-through: the pushed reference passes through unchanged; astro-registry
	// maps the "acme" namespace to its ECR repo at pull time.
	expected := "proxy.registry.io/acme/my-app:v1"
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
	// RegistryURL no longer affects tenant resolution under pull-through; the
	// image stays on the proxy registry host regardless of the ECR URL form.
	input := proxyInput()
	input.RegistryURL = "123456789.dkr.ecr.us-east-1.amazonaws.com"
	got := resolveImage("proxy.registry.io/acme/my-app:v1", input)
	expected := "proxy.registry.io/acme/my-app:v1"
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
	expected := "proxy.registry.io/acme/my-app:v1"
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

	expected := "proxy.registry.io/acme/my-agent:abc123"
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

	expected := "proxy.registry.io/acme/custom-embedder:v2"
	if ds.Models["embedder"].Image != expected {
		t.Errorf("model image: expected %s, got %s", expected, ds.Models["embedder"].Image)
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

	expected := "proxy.registry.io/acme/custom-store:v3"
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

	expected := "proxy.registry.io/acme/search-tool:latest"
	if ds.Integrations["search"].Image != expected {
		t.Errorf("tool image: expected %s, got %s", expected, ds.Integrations["search"].Image)
	}
}

func TestTemplate_IntegrationBuildWithoutImage(t *testing.T) {
	input := baseInput()
	input.Spec.Integrations = map[string]spec.Integration{
		"search": {
			Container: &spec.ContainerConfig{
				Build: &spec.BuildConfig{Context: ".", Dockerfile: "integrations/search/Dockerfile"},
				Port:  3000,
			},
		},
	}

	ds := mustGenerate(t, input)

	expected := "registry.example.com/dockerhub/library/my-agent-integration-search"
	if ds.Integrations["search"].Image != expected {
		t.Errorf("integration image: expected %s, got %s", expected, ds.Integrations["search"].Image)
	}
}

func TestTemplate_IngestionBuildWithoutImage(t *testing.T) {
	input := baseInput()
	input.Spec.Ingestion = map[string]spec.Ingestion{
		"crawler": {
			Container: spec.ContainerConfig{
				Build: &spec.BuildConfig{Context: ".", Dockerfile: "ingestion/crawler/Dockerfile"},
			},
			Trigger: spec.IngestionTrigger{Type: "startup"},
		},
	}

	ds := mustGenerate(t, input)

	expected := "registry.example.com/dockerhub/library/my-agent-ingestion-crawler"
	if ds.Ingestion["crawler"].Image != expected {
		t.Errorf("ingestion image: expected %s, got %s", expected, ds.Ingestion["crawler"].Image)
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

	expected := "proxy.registry.io/acme/ingestion-data:dfdddc61"
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

	prefix := "proxy.registry.io/acme/"
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
		"public": {Container: &spec.ContainerConfig{Image: "my-model:latest", Port: 8000}},
	}

	ds := mustGenerate(t, input)

	// Proxy image should be resolved
	if !strings.Contains(ds.Models["proxy"].Image, "proxy.registry.io/acme") {
		t.Errorf("proxy image should be resolved, got %s", ds.Models["proxy"].Image)
	}
	// Public image is served through the ECR pull-through cache
	expected := "123456789.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/my-model:latest"
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
				"llm":       {Container: &spec.ContainerConfig{Image: "my-model:latest", Port: 8000}},
				"embedder":  {Container: &spec.ContainerConfig{Image: "embed:latest", Port: 8000}},
				"anthropic": {Provider: "anthropic"},
			},
			Knowledge: map[string]spec.Knowledge{
				"vectors":  {Provider: "qdrant"},
				"cache":    {Provider: "redis"},
				"custom":   {Container: &spec.ContainerConfig{Image: "mydb:latest", Port: 5432}},
				"postgres": {Provider: "postgres"},
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

// ===== GET vs POST parity =====

// TestGETandPOST_ProduceSameDeploySpec verifies that a template fulfilled
// client-side (the GET path) produces the same deployment/v1 spec as the
// POST path via ShapeTemplate. This is the core backward-compat guarantee:
// switching from GET to POST must not change what gets posted to /deploy.
func TestGETandPOST_ProduceSameDeploySpec(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: true}
	base := mustGenerate(t, input)

	// Simulate user inputs: select slack+web, fill token values.
	adapterSelection := []string{"slack", "web"}
	varInputs := map[string]spec.VariableInput{
		"SLACK_BOT_TOKEN": {Value: "xoxb-test"},
		"SLACK_APP_TOKEN": {Value: "xapp-test"},
	}

	// --- POST path: ShapeTemplate does the fulfillment ---
	postResp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{Adapters: adapterSelection},
		Variables:  varInputs,
	}, nil)
	postSpec := postResp.Template

	// --- GET path: manual client-side fulfillment ---
	getSpec := deepCopySpec(base)
	getSpec.Spec = "deployment/v1"

	// Apply adapter shaping on the GET copy — same operation ShapeTemplate performs.
	// This injects slack vars, wires interfaces.environment, flips optionality, and
	// exposes the HTTP endpoint for the web adapter.
	ApplyAdapterShaping(getSpec, adapterSelection)
	if getSpec.Interfaces != nil {
		getSpec.Interfaces.Adapters = adapterSelection
		if ep, ok := getSpec.Interfaces.Endpoints["http"]; ok {
			if ep.Expose == nil {
				ep.Expose = &spec.EndpointExpose{}
			}
			ep.Expose.Enabled = true
			getSpec.Interfaces.Endpoints["http"] = ep
		}
	}

	// Fill variable values + strip template-only fields (mimics client fulfillTemplate)
	for key, v := range getSpec.Variables {
		if inp, ok := varInputs[key]; ok {
			v.Value = inp.Value
			v.Ref = inp.Ref
		}
		// Strip template-only fields
		v.Description = ""
		v.Label = ""
		v.Placeholder = ""
		v.HelpURL = ""
		v.Datatype = ""
		v.DisplayAs = ""
		v.Options = nil
		v.Fields = nil
		v.Default = ""
		getSpec.Variables[key] = v
	}

	// --- Compare ---
	getJSON, err := json.Marshal(getSpec)
	if err != nil {
		t.Fatalf("marshal GET spec: %v", err)
	}
	postJSON, err := json.Marshal(&postSpec)
	if err != nil {
		t.Fatalf("marshal POST spec: %v", err)
	}

	if string(getJSON) != string(postJSON) {
		t.Errorf("GET and POST paths produce different deploy specs.\nGET:  %s\nPOST: %s", getJSON, postJSON)
	}

	// Sanity: the spec is valid for the deploy endpoint
	if postSpec.Spec != "deployment/v1" {
		t.Errorf("POST spec version: expected deployment/v1, got %s", postSpec.Spec)
	}
}

// ===== ShapeTemplate =====

// baseTemplateForShape builds a minimal template with variables and interfaces
// suitable for testing ShapeTemplate.
func baseTemplateForShape(t *testing.T) *spec.AstroDeploymentSpec {
	t.Helper()
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: true}
	ds := mustGenerate(t, input)
	// Add a non-optional agent variable for testing required-variable validation.
	ds.Variables["MY_API_KEY"] = spec.Variable{
		Description: "API key for external service",
		Targets:     []string{"agent"},
		Secret:      true,
		Optional:    false,
	}
	return ds
}

func TestShapeTemplate_EmptyRequest(t *testing.T) {
	base := baseTemplateForShape(t)
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{}, nil)

	// Envelope spec
	if resp.Spec != "deployment-template/v1" {
		t.Errorf("resp.Spec: expected deployment-template/v1, got %s", resp.Spec)
	}

	// Template spec is deployment/v1
	if resp.Template.Spec != "deployment/v1" {
		t.Errorf("template.Spec: expected deployment/v1, got %s", resp.Template.Spec)
	}

	// Root interfaces: adapters should be empty, auth should reflect template
	if resp.Interfaces.Adapters == nil {
		t.Error("resp.Interfaces.Adapters should be non-nil (empty slice, not null)")
	}
	if len(resp.Interfaces.Adapters) != 0 {
		t.Errorf("resp.Interfaces.Adapters: expected empty, got %v", resp.Interfaces.Adapters)
	}
	if resp.Interfaces.Auth == nil || resp.Interfaces.Auth.Web == nil || resp.Interfaces.Auth.Web.Type != "oidc" {
		t.Error("resp.Interfaces.Auth should have web.type=oidc from base template")
	}

	// Root variables should have schema fields (description)
	if v, ok := resp.Variables["MY_API_KEY"]; !ok {
		t.Error("root variables missing MY_API_KEY")
	} else if v.Description == "" {
		t.Error("root variables.MY_API_KEY.Description should be populated")
	}

	// Template variables should have schema fields stripped
	if v, ok := resp.Template.Variables["MY_API_KEY"]; ok && v.Description != "" {
		t.Error("template variables.MY_API_KEY.Description should be stripped")
	}

	// Validation should report missing required vars
	if resp.Validation.Valid {
		t.Error("expected valid=false with missing required variables")
	}
	found := false
	for _, e := range resp.Validation.Errors {
		if e.Field == "variables.MY_API_KEY" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected validation error for variables.MY_API_KEY")
	}
}

func TestShapeTemplate_ResponseTimeout(t *testing.T) {
	t.Run("defaults when unset", func(t *testing.T) {
		base := baseTemplateForShape(t)
		resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{}, nil)
		if got := resp.Template.Agent.ResponseTimeout; got != spec.DefaultResponseTimeout {
			t.Errorf("template timeout: expected %s, got %q", spec.DefaultResponseTimeout, got)
		}
		if resp.Provisioning.Agent == nil || resp.Provisioning.Agent.ResponseTimeout != spec.DefaultResponseTimeout {
			t.Errorf("echoed timeout: expected %s, got %+v", spec.DefaultResponseTimeout, resp.Provisioning.Agent)
		}
	})

	t.Run("accepts a valid override", func(t *testing.T) {
		base := baseTemplateForShape(t)
		resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
			Provisioning: &spec.TemplateProvisioning{
				Agent: &spec.ComponentProvisioning{ResponseTimeout: "90s"},
			},
		}, nil)
		if got := resp.Template.Agent.ResponseTimeout; got != "90s" {
			t.Errorf("expected 90s, got %q", got)
		}
		for _, e := range resp.Validation.Errors {
			if e.Field == "agent.responseTimeout" {
				t.Errorf("unexpected validation error: %s", e.Message)
			}
		}
	})

	t.Run("rejects invalid, non-positive, and over-cap values", func(t *testing.T) {
		for _, v := range []string{"nonsense", "0s", "3m", "infinity"} {
			base := baseTemplateForShape(t)
			resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
				Provisioning: &spec.TemplateProvisioning{
					Agent: &spec.ComponentProvisioning{ResponseTimeout: v},
				},
			}, nil)
			found := false
			for _, e := range resp.Validation.Errors {
				if e.Field == "agent.responseTimeout" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("value %q: expected an agent.responseTimeout validation error", v)
			}
		}
	})
}

func TestShapeTemplate_AdapterShaping(t *testing.T) {
	base := baseTemplateForShape(t)

	// Verify Slack vars start as optional (base template default)
	if v, ok := base.Variables["SLACK_BOT_TOKEN"]; ok && !v.Optional {
		t.Fatal("precondition: SLACK_BOT_TOKEN should be optional in base template")
	}

	// Select slack adapter
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{Adapters: []string{"slack"}},
	}, nil)

	// Root schema should have slack vars as non-optional
	if v := resp.Variables["SLACK_BOT_TOKEN"]; v.Optional {
		t.Error("SLACK_BOT_TOKEN should be non-optional when slack adapter is selected")
	}
	if v := resp.Variables["SLACK_APP_TOKEN"]; v.Optional {
		t.Error("SLACK_APP_TOKEN should be non-optional when slack adapter is selected")
	}

	// Template interfaces.adapters should be set
	if resp.Template.Interfaces == nil {
		t.Fatal("template.Interfaces should not be nil")
	}
	if len(resp.Template.Interfaces.Adapters) != 1 || resp.Template.Interfaces.Adapters[0] != "slack" {
		t.Errorf("template.Interfaces.Adapters: expected [slack], got %v", resp.Template.Interfaces.Adapters)
	}

	// Root adapters should match the request selection
	if len(resp.Interfaces.Adapters) != 1 || resp.Interfaces.Adapters[0] != "slack" {
		t.Errorf("resp.Interfaces.Adapters: expected [slack], got %v", resp.Interfaces.Adapters)
	}

	// Validation should error on missing slack tokens (now non-optional due to adapter shaping)
	hasSlackErr := false
	for _, e := range resp.Validation.Errors {
		if e.Field == "variables.SLACK_BOT_TOKEN" {
			hasSlackErr = true
			break
		}
	}
	if !hasSlackErr {
		t.Error("expected validation error for SLACK_BOT_TOKEN when slack adapter is selected")
	}
}

func TestShapeTemplate_ConfiguredInlineSecrets(t *testing.T) {
	base := baseTemplateForShape(t)
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{}, &ShapeOptions{
		ConfiguredInlineSecrets: []string{"MY_API_KEY"},
	})

	if !resp.Validation.Valid {
		t.Fatalf("expected valid with configured inline secret, errors: %v", resp.Validation.Errors)
	}
	v := resp.Variables["MY_API_KEY"]
	if !v.Configured {
		t.Error("schema MY_API_KEY.Configured: expected true")
	}
	if v.Value != "" {
		t.Errorf("schema MY_API_KEY.Value: expected empty, got %q", v.Value)
	}
	if v.Ref != "" {
		t.Errorf("schema MY_API_KEY.Ref: expected empty, got %q", v.Ref)
	}
	tv := resp.Template.Variables["MY_API_KEY"]
	if tv.Configured {
		t.Error("template MY_API_KEY.Configured should be stripped")
	}
	if tv.Value != "" {
		t.Errorf("template MY_API_KEY.Value: expected empty without client input, got %q", tv.Value)
	}
}

func TestShapeTemplate_VariableFilling(t *testing.T) {
	base := baseTemplateForShape(t)
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Variables: map[string]spec.VariableInput{
			"MY_API_KEY": {Value: "sk-test-123"},
		},
	}, nil)

	// Template should have the value filled in
	if v, ok := resp.Template.Variables["MY_API_KEY"]; !ok {
		t.Error("template missing MY_API_KEY")
	} else if v.Value != "sk-test-123" {
		t.Errorf("template MY_API_KEY.Value: expected sk-test-123, got %s", v.Value)
	}

	// Root schema should also reflect the value
	if v := resp.Variables["MY_API_KEY"]; v.Value != "sk-test-123" {
		t.Errorf("root MY_API_KEY.Value: expected sk-test-123, got %s", v.Value)
	}

	// MY_API_KEY should not produce a validation error
	for _, e := range resp.Validation.Errors {
		if e.Field == "variables.MY_API_KEY" {
			t.Errorf("unexpected validation error for MY_API_KEY: %s", e.Message)
		}
	}
}

func TestShapeTemplate_VariableRef(t *testing.T) {
	base := baseTemplateForShape(t)
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Variables: map[string]spec.VariableInput{
			"MY_API_KEY": {Ref: "prod-api-key"},
		},
	}, nil)

	// Template should have the ref set, value cleared
	v := resp.Template.Variables["MY_API_KEY"]
	if v.Ref != "prod-api-key" {
		t.Errorf("template MY_API_KEY.Ref: expected prod-api-key, got %s", v.Ref)
	}
	if v.Value != "" {
		t.Errorf("template MY_API_KEY.Value should be empty when ref is set, got %s", v.Value)
	}
}

func TestShapeTemplate_FullyValid(t *testing.T) {
	base := baseTemplateForShape(t)

	// Fill all required variables
	vars := make(map[string]spec.VariableInput)
	for key, v := range base.Variables {
		if !v.Optional {
			vars[key] = spec.VariableInput{Value: "filled-" + key}
		}
	}

	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Variables: vars,
	}, nil)

	if !resp.Validation.Valid {
		errMsgs := make([]string, len(resp.Validation.Errors))
		for i, e := range resp.Validation.Errors {
			errMsgs[i] = e.Field + ": " + e.Message
		}
		t.Errorf("expected valid=true, got errors: %s", strings.Join(errMsgs, "; "))
	}
}

func TestShapeTemplate_DoesNotMutateBase(t *testing.T) {
	base := baseTemplateForShape(t)

	// Capture original state
	origJSON, _ := json.Marshal(base)

	ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{Adapters: []string{"slack"}},
		Variables:  map[string]spec.VariableInput{"MY_API_KEY": {Value: "mutated"}},
	}, nil)

	afterJSON, _ := json.Marshal(base)
	if string(origJSON) != string(afterJSON) {
		t.Error("ShapeTemplate mutated the base template")
	}
}

func TestShapeTemplate_AdaptersFromPrefill(t *testing.T) {
	base := baseTemplateForShape(t)
	// Simulate mergeDeploymentPrefill: the stored deployment had ["web", "slack"].
	base.Interfaces.Adapters = []string{"web", "slack"}

	// Initial POST with no adapters in request — response should reflect the stored adapters.
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{}, nil)
	if len(resp.Interfaces.Adapters) != 2 {
		t.Fatalf("resp.Interfaces.Adapters: expected [web slack], got %v", resp.Interfaces.Adapters)
	}
	if resp.Interfaces.Adapters[0] != "web" || resp.Interfaces.Adapters[1] != "slack" {
		t.Errorf("resp.Interfaces.Adapters: expected [web slack], got %v", resp.Interfaces.Adapters)
	}
}

func TestShapeTemplate_AdaptersReshape(t *testing.T) {
	base := baseTemplateForShape(t)
	// Simulate mergeDeploymentPrefill: the stored deployment had ["web", "slack"].
	base.Interfaces.Adapters = []string{"web", "slack"}

	// Reshape: user deselects web — response should reflect only slack.
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{Adapters: []string{"slack"}},
	}, nil)
	if len(resp.Interfaces.Adapters) != 1 || resp.Interfaces.Adapters[0] != "slack" {
		t.Errorf("resp.Interfaces.Adapters after reshape: expected [slack], got %v", resp.Interfaces.Adapters)
	}

	// Reshape: user deselects all — response should be empty slice.
	resp = ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{Adapters: []string{}},
	}, nil)
	if resp.Interfaces.Adapters == nil {
		t.Error("resp.Interfaces.Adapters should be non-nil (empty slice, not null)")
	}
	if len(resp.Interfaces.Adapters) != 0 {
		t.Errorf("resp.Interfaces.Adapters after clearing: expected empty, got %v", resp.Interfaces.Adapters)
	}
}

func TestShapeTemplate_AuthShaping(t *testing.T) {
	base := baseTemplateForShape(t)

	// Base template starts with OIDC auth
	if base.Interfaces.Auth == nil || base.Interfaces.Auth.Web == nil || base.Interfaces.Auth.Web.Type != "oidc" {
		t.Fatal("precondition: base template should have oidc auth")
	}

	// Request with auth nil — preserves base auth
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{Adapters: []string{"web"}},
	}, nil)
	if resp.Interfaces.Auth == nil || resp.Interfaces.Auth.Web == nil || resp.Interfaces.Auth.Web.Type != "oidc" {
		t.Error("expected auth preserved when request auth is nil")
	}
	if resp.Template.Interfaces.Auth == nil || resp.Template.Interfaces.Auth.Web.Type != "oidc" {
		t.Error("expected template.interfaces.auth to have oidc")
	}

	// Request explicitly sets auth — overrides base
	resp = ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{
			Adapters: []string{"web"},
			Auth:     &spec.DeploymentInterfacesAuth{},
		},
	}, nil)
	if resp.Interfaces.Auth == nil {
		t.Fatal("expected auth to be non-nil (empty struct)")
	}
	if resp.Interfaces.Auth.Web != nil {
		t.Error("expected auth.web to be nil when request clears it")
	}
	if resp.Template.Interfaces.Auth.Web != nil {
		t.Error("expected template.interfaces.auth.web to be nil when request clears it")
	}
}

func TestShapeTemplate_InterfacesJSONNeverNull(t *testing.T) {
	base := baseTemplateForShape(t)
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{}, nil)

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	// Adapters must serialize as [] not null
	if !strings.Contains(s, `"adapters":[]`) {
		t.Errorf("expected JSON to contain \"adapters\":[], got: %s", s)
	}
	// Auth should be present (base template has OIDC)
	if !strings.Contains(s, `"auth":{"web":{"type":"oidc"}}`) {
		t.Errorf("expected JSON to contain auth with oidc, got: %s", s)
	}
}

// baseTemplateWithIngestion extends baseTemplateForShape with a schedule-triggered ingestion.
func baseTemplateWithIngestion(t *testing.T) *spec.AstroDeploymentSpec {
	t.Helper()
	base := baseTemplateForShape(t)
	base.Ingestion = map[string]spec.DeploymentIngestion{
		"nightly_sync": {
			Image:   "sync:latest",
			Trigger: spec.DeploymentTrigger{Type: "schedule", Schedule: ""},
		},
		"on_demand": {
			Image:   "import:latest",
			Trigger: spec.DeploymentTrigger{Type: "manual"},
		},
	}
	return base
}

func TestShapeTemplate_ScheduleShaping(t *testing.T) {
	base := baseTemplateWithIngestion(t)
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Schedules: map[string]string{"nightly_sync": "0 3 * * *"},
	}, nil)

	// Template should have the schedule applied
	if ing, ok := resp.Template.Ingestion["nightly_sync"]; !ok {
		t.Fatal("template missing nightly_sync ingestion")
	} else if ing.Trigger.Schedule != "0 3 * * *" {
		t.Errorf("template nightly_sync schedule: expected '0 3 * * *', got '%s'", ing.Trigger.Schedule)
	}

	// Response root schedules should match
	if resp.Schedules["nightly_sync"] != "0 3 * * *" {
		t.Errorf("resp.Schedules[nightly_sync]: expected '0 3 * * *', got '%s'", resp.Schedules["nightly_sync"])
	}

	// Manual trigger should not appear in schedules
	if _, ok := resp.Schedules["on_demand"]; ok {
		t.Error("resp.Schedules should not contain non-schedule triggers")
	}

	// No schedule validation errors for nightly_sync
	for _, e := range resp.Validation.Errors {
		if e.Field == "ingestion.nightly_sync.trigger.schedule" {
			t.Errorf("unexpected validation error for nightly_sync: %s", e.Message)
		}
	}
}

func TestShapeTemplate_ScheduleValidation(t *testing.T) {
	base := baseTemplateWithIngestion(t)

	// Invalid cron should produce a validation error
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Schedules: map[string]string{"nightly_sync": "not-a-cron"},
	}, nil)
	found := false
	for _, e := range resp.Validation.Errors {
		if e.Field == "ingestion.nightly_sync.trigger.schedule" && strings.Contains(e.Message, "invalid") {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for invalid cron expression")
	}

	// Empty schedule should produce a required error
	resp = ShapeTemplate(context.Background(), base, &spec.TemplateRequest{}, nil)
	found = false
	for _, e := range resp.Validation.Errors {
		if e.Field == "ingestion.nightly_sync.trigger.schedule" && strings.Contains(e.Message, "required") {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for missing schedule")
	}
}

func TestShapeTemplate_ScheduleIgnoredForNonScheduleTrigger(t *testing.T) {
	base := baseTemplateWithIngestion(t)
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Schedules: map[string]string{"on_demand": "0 3 * * *"},
	}, nil)

	// Should not apply schedule to a manual trigger
	if ing, ok := resp.Template.Ingestion["on_demand"]; ok {
		if ing.Trigger.Schedule != "" {
			t.Errorf("manual trigger should not have schedule applied, got '%s'", ing.Trigger.Schedule)
		}
	}
}

func TestShapeTemplate_ScheduleFromPrefill(t *testing.T) {
	base := baseTemplateWithIngestion(t)
	// Simulate mergeDeploymentPrefill: stored deployment had a schedule
	if ing, ok := base.Ingestion["nightly_sync"]; ok {
		ing.Trigger.Schedule = "0 2 * * *"
		base.Ingestion["nightly_sync"] = ing
	}

	// Initial POST with no schedules in request — should reflect the stored schedule
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{}, nil)
	if resp.Schedules["nightly_sync"] != "0 2 * * *" {
		t.Errorf("resp.Schedules[nightly_sync]: expected '0 2 * * *', got '%s'", resp.Schedules["nightly_sync"])
	}

	// Reshape with new schedule — should override
	resp = ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Schedules: map[string]string{"nightly_sync": "0 4 * * *"},
	}, nil)
	if resp.Schedules["nightly_sync"] != "0 4 * * *" {
		t.Errorf("resp.Schedules[nightly_sync] after reshape: expected '0 4 * * *', got '%s'", resp.Schedules["nightly_sync"])
	}
}

// Regression: deploying with only "web" selected used to fail because the POST
// template endpoint stripped non-selected adapter variables (SLACK_*) from the
// response, but the deploy handler regenerated a fresh template with ALL
// variables and EnforceEditable rejected the submission for "removing" them.
// The fix: the deploy handler applies ApplyAdapterShaping to the
// regenerated template before validation, matching what the client received.
func TestApplyAdapterShaping_DeployRoundTrip(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: true}

	// 1. Simulate a template that was previously shaped with slack (slack vars present).
	canonical := mustGenerate(t, input)
	ApplyAdapterShaping(canonical, []string{"slack"})
	if _, ok := canonical.Variables["SLACK_BOT_TOKEN"]; !ok {
		t.Fatal("precondition: canonical template must include SLACK_BOT_TOKEN after slack shaping")
	}

	// 2. Shape with only "web" selected — mimics the POST template response.
	shaped := ShapeTemplate(context.Background(), canonical, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{Adapters: []string{"web"}},
	}, nil)
	submittedSpec := &shaped.Template

	// Verify Slack vars were stripped by ShapeTemplate.
	if _, ok := submittedSpec.Variables["SLACK_BOT_TOKEN"]; ok {
		t.Fatal("precondition: shaped template should NOT include SLACK_BOT_TOKEN when slack is not selected")
	}

	// Verify that ValidateAndResolve passes — previously the stripping left
	// dangling ${variables.SLACK_CONFIG} references in interfaces.environment.
	result, err := ValidateAndResolve(submittedSpec)
	if err != nil {
		t.Fatalf("ValidateAndResolve error: %v", err)
	}
	for _, e := range result.Errors {
		if strings.Contains(e, "not declared") {
			t.Errorf("dangling variable reference after stripping: %s", e)
		}
	}
}

// Regression: stripping adapter variables must also remove corresponding
// ${variables.KEY} references from interfaces.environment, otherwise
// ValidateReferences rejects the spec with "variable X not declared".
func TestApplyAdapterShaping_CleansEnvironmentRefs(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: true}
	ds := mustGenerate(t, input)

	// Inject slack vars first (simulates user having selected slack previously).
	ApplyAdapterShaping(ds, []string{"slack"})
	if _, ok := ds.Variables["SLACK_CONFIG"]; !ok {
		t.Fatal("precondition: SLACK_CONFIG variable should exist after slack shaping")
	}
	if ds.Interfaces == nil || ds.Interfaces.Environment["SLACK_CONFIG"] == "" {
		t.Fatal("precondition: interfaces.environment should reference SLACK_CONFIG after slack shaping")
	}

	// Now strip with web-only — slack vars and their env refs should be removed.
	ApplyAdapterShaping(ds, []string{"web"})

	if _, ok := ds.Variables["SLACK_CONFIG"]; ok {
		t.Error("SLACK_CONFIG variable should be stripped when slack is not selected")
	}
	if ref, ok := ds.Interfaces.Environment["SLACK_CONFIG"]; ok {
		t.Errorf("SLACK_CONFIG environment ref should be removed, still present: %q", ref)
	}
}

// Regression: when the template request has no `interfaces` block (the
// prefill path: POST {deployment_id} with no overrides), ShapeTemplate must
// still drop variables/env refs for non-selected adapters. Before the fix,
// shaping was gated on `req.Interfaces != nil`, so a redeploy whose stored
// adapters were ["web"] would still surface SLACK_CONFIG in the response
// template's interfaces.environment. The user observed this leaking into
// the messaging container env via the deploy roundtrip.
func TestShapeTemplate_NoRequestInterfaces_StripsNonSelectedAdapterRefs(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: true}
	canonical := mustGenerate(t, input)

	// Inject slack vars to simulate a template that was previously shaped with slack.
	ApplyAdapterShaping(canonical, []string{"slack"})
	if _, ok := canonical.Variables["SLACK_CONFIG"]; !ok {
		t.Fatal("precondition: canonical template should include SLACK_CONFIG after slack shaping")
	}
	if _, ok := canonical.Interfaces.Environment["SLACK_CONFIG"]; !ok {
		t.Fatal("precondition: canonical template should reference SLACK_CONFIG in interfaces.environment")
	}

	// Mimic the prefill path: stored deployment was web-only, base template
	// reflects that. Request body carries no interfaces overrides — the
	// caller is just asking for the current state.
	canonical.Interfaces.Adapters = []string{"web"}
	resp := ShapeTemplate(context.Background(), canonical, &spec.TemplateRequest{}, nil)

	if _, ok := resp.Template.Variables["SLACK_CONFIG"]; ok {
		t.Error("SLACK_CONFIG variable should be stripped when slack is not in shaped adapter list")
	}
	if ref, ok := resp.Template.Interfaces.Environment["SLACK_CONFIG"]; ok {
		t.Errorf("SLACK_CONFIG environment ref should be removed, still present: %q", ref)
	}
}

// Verify ApplyAdapterShaping keeps variables for selected adapters and
// variables that target non-interface components.
func TestApplyAdapterShaping_KeepsSelectedAndNonInterface(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: true}

	ds := mustGenerate(t, input)
	// Add a non-interface variable to verify it's preserved.
	ds.Variables["MY_AGENT_VAR"] = spec.Variable{
		Targets: []string{"agent"},
		Value:   "hello",
	}

	ApplyAdapterShaping(ds, []string{"slack"})

	// Slack variables should be kept.
	if _, ok := ds.Variables["SLACK_BOT_TOKEN"]; !ok {
		t.Error("SLACK_BOT_TOKEN should be kept when slack is selected")
	}
	if _, ok := ds.Variables["SLACK_APP_TOKEN"]; !ok {
		t.Error("SLACK_APP_TOKEN should be kept when slack is selected")
	}
	// Non-interface variable should be kept.
	if _, ok := ds.Variables["MY_AGENT_VAR"]; !ok {
		t.Error("MY_AGENT_VAR (targets agent, not interface) should be kept")
	}
}

// Verify ApplyAdapterShaping flips slack token optionality.
// Without this, deploying WITH slack selected would fail:
// "variables.SLACK_BOT_TOKEN.optional: server-owned field cannot be changed"
func TestApplyAdapterShaping_SlackOptionalityFlipped(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: true}
	ds := mustGenerate(t, input)

	// Raw template has no slack vars. Selecting slack injects them as required.
	ApplyAdapterShaping(ds, []string{"slack"})

	if ds.Variables["SLACK_BOT_TOKEN"].Optional {
		t.Error("SLACK_BOT_TOKEN should be required when slack is selected")
	}
	if ds.Variables["SLACK_APP_TOKEN"].Optional {
		t.Error("SLACK_APP_TOKEN should be required when slack is selected")
	}
}

// Selecting slack must flip SLACK_BOT_TOKEN.Optional to false so the shaped
// template enforces the token as required at validation time.
func TestApplyAdapterShaping_SlackSelectedFlipsOptionality(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: true}

	canonical := mustGenerate(t, input)

	shaped := ShapeTemplate(context.Background(), canonical, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{Adapters: []string{"slack"}},
		Variables: map[string]spec.VariableInput{
			"SLACK_BOT_TOKEN": {Value: "xoxb-test"},
			"SLACK_APP_TOKEN": {Value: "xapp-test"},
		},
	}, nil)

	if shaped.Variables["SLACK_BOT_TOKEN"].Optional {
		t.Error("SLACK_BOT_TOKEN should be required when slack is selected")
	}
	if shaped.Variables["SLACK_APP_TOKEN"].Optional {
		t.Error("SLACK_APP_TOKEN should be required when slack is selected")
	}
}

// Verify ApplyAdapterShaping is a no-op when spec has no interfaces.
func TestApplyAdapterShaping_NilInterfaces(t *testing.T) {
	ds := &spec.AstroDeploymentSpec{
		Variables: map[string]spec.Variable{
			"AGENT_VAR":       {Targets: []string{"agent"}, Value: "v"},
			"SLACK_BOT_TOKEN": {Targets: []string{"interface.slack"}, Secret: true},
		},
	}

	ApplyAdapterShaping(ds, nil)

	// No interfaces on the spec → nothing should be stripped.
	if len(ds.Variables) != 2 {
		t.Errorf("expected 2 variables, got %d", len(ds.Variables))
	}
}

// TestRetemplate_SlackConfigNotLeakedForWebOnly simulates the
// retemplateDeploymentSpec flow in admingrpc: generate a fresh template,
// restore the user's adapter selection (web-only), and verify that Slack
// variables don't leak into the resolved ConfigMap. Without
// ApplyAdapterShaping after re-templating, SLACK_CONFIG appears in the
// expected ConfigMap keys even though the user never enabled Slack, causing
// false-positive drift.
func TestRetemplate_SlackConfigNotLeakedForWebOnly(t *testing.T) {
	// Step 1: Generate template (same as retemplateDeploymentSpec)
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Messaging: true}
	ds := mustGenerate(t, input)

	// Step 2: Restore user adapters (web only — no slack)
	userAdapters := []string{"web"}
	ds.Interfaces.Adapters = userAdapters

	// Step 3: Apply adapter shaping — retemplateDeploymentSpec must do this
	// after re-generating the template to strip variables for non-selected
	// adapters. Without this call, SLACK_CONFIG leaks into the resolved env.
	ApplyAdapterShaping(ds, userAdapters)

	// Step 4: Resolve env (same as spec applier / normalizer)
	rctx := ResolveContext{
		Namespace:  "astro-test-0",
		AgentName:  ds.Source.Name,
		BuildID:    ds.Source.Build,
		SecretName: GenerateSecretName(ds.Source.Name, ds.Source.Build),
	}
	resolved := ResolveDeploymentSpecEnv(ds, rctx)

	// SLACK_CONFIG should NOT be in ConfigMapData — user only selected "web"
	if _, ok := resolved.ConfigMapData["SLACK_CONFIG"]; ok {
		t.Errorf("SLACK_CONFIG leaked into ConfigMapData after retemplate with web-only adapters (value=%q)", resolved.ConfigMapData["SLACK_CONFIG"])
	}

	// Slack secret vars should not be in SecretData either
	for _, key := range []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"} {
		if _, ok := resolved.SecretData[key]; ok {
			t.Errorf("%s leaked into SecretData after retemplate with web-only adapters", key)
		}
	}
}

// Regression: deploying a spec with a knowledge binding failed because the
// deploy handler regenerated a fresh template with full knowledge entries
// (image, endpoints, etc.) but the submitted spec had those fields zeroed by
// ShapeTemplate. EnforceEditable rejected binding, image, and endpoint diffs.
// The fix: ApplyBindingShaping zeroes the template's bound knowledge entries
// to match what the client received.
func TestApplyBindingShaping_DeployRoundTrip(t *testing.T) {
	bindingARN := "arn:knowledge-store:acct123:pg-store"

	template := &spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"postgres": {
				Image:      "postgres:16",
				Endpoints:  map[string]spec.Endpoint{"http": {Port: 5432, Protocol: "tcp"}},
				Persistent: true,
				Resources:  spec.DeploymentResources{CPU: "500m", Memory: "512Mi"},
			},
		},
		Variables: map[string]spec.Variable{
			"POSTGRES_USER":     {Targets: []string{"knowledge.postgres"}, Secret: true},
			"POSTGRES_PASSWORD": {Targets: []string{"knowledge.postgres"}, Secret: true},
			"AGENT_VAR":         {Targets: []string{"agent"}, Value: "v"},
		},
	}

	// Submitted spec is what ShapeTemplate would have produced — bound entry zeroed.
	submitted := &spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"postgres": {Binding: bindingARN},
		},
		Variables: map[string]spec.Variable{
			"AGENT_VAR": {Targets: []string{"agent"}, Value: "v"},
		},
	}

	ApplyBindingShaping(template, submitted)

	// Template knowledge entry should now be zeroed with only the binding ARN.
	k := template.Knowledge["postgres"]
	if k.Binding != bindingARN {
		t.Errorf("expected binding %q, got %q", bindingARN, k.Binding)
	}
	if k.Image != "" {
		t.Errorf("expected empty image, got %q", k.Image)
	}
	if len(k.Endpoints) != 0 {
		t.Errorf("expected no endpoints, got %v", k.Endpoints)
	}

	// Credential variables targeting the bound entry should be removed.
	if _, ok := template.Variables["POSTGRES_USER"]; ok {
		t.Error("expected POSTGRES_USER to be removed")
	}
	if _, ok := template.Variables["POSTGRES_PASSWORD"]; ok {
		t.Error("expected POSTGRES_PASSWORD to be removed")
	}
	// Non-bound variable should be kept.
	if _, ok := template.Variables["AGENT_VAR"]; !ok {
		t.Error("expected AGENT_VAR to be kept")
	}
}

// ApplyBindingShaping should be a no-op when no knowledge entries are bound.
func TestApplyBindingShaping_NoBoundEntries(t *testing.T) {
	template := &spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"postgres": {
				Image:     "postgres:16",
				Endpoints: map[string]spec.Endpoint{"http": {Port: 5432}},
			},
		},
		Variables: map[string]spec.Variable{
			"POSTGRES_USER": {Targets: []string{"knowledge.postgres"}, Secret: true},
		},
	}

	submitted := &spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"postgres": {
				Image:     "postgres:16",
				Endpoints: map[string]spec.Endpoint{"http": {Port: 5432}},
			},
		},
		Variables: map[string]spec.Variable{
			"POSTGRES_USER": {Targets: []string{"knowledge.postgres"}, Secret: true},
		},
	}

	ApplyBindingShaping(template, submitted)

	// Nothing should change.
	if template.Knowledge["postgres"].Image != "postgres:16" {
		t.Error("image should not have been changed")
	}
	if _, ok := template.Variables["POSTGRES_USER"]; !ok {
		t.Error("POSTGRES_USER should still be present")
	}
}

// Reproduce the user's scenario: two postgres knowledge entries ("postgres" and
// "users") plus a redis "cache". Verify that credential variables (POSTGRES_USER,
// POSTGRES_PASSWORD) are generated for both postgres entries.
func TestTemplate_MultiplePostgresKnowledge_Credentials(t *testing.T) {
	input := baseInput()
	input.AgentName = "sasbot"
	input.Spec.Name = "sasbot"
	input.Spec.Knowledge = map[string]spec.Knowledge{
		"postgres": {Provider: "postgres"},
		"users":    {Provider: "postgres"},
		"cache":    {Provider: "redis"},
	}

	ds := mustGenerate(t, input)

	// Log all variables for debugging.
	t.Log("=== Variables ===")
	for name, v := range ds.Variables {
		t.Logf("  %s  targets=%v  secret=%v", name, v.Targets, v.Secret)
	}

	// Log agent environment.
	t.Log("=== Agent Environment ===")
	for k, v := range ds.Agent.Environment {
		t.Logf("  %s = %s", k, v)
	}

	// Self-hosted credentials are platform-managed (auto-generated at deploy time),
	// so they must NOT appear in the variables map — only in agent environment.
	for key, v := range ds.Variables {
		for _, target := range v.Targets {
			if strings.HasPrefix(target, "knowledge.") {
				t.Errorf("unexpected credential variable %s with target %s — self-hosted credentials should not be in variables", key, target)
			}
		}
	}

	// --- Agent environment ---
	//
	// Credential env vars (POSTGRES_USER / _PASSWORD / _DB and the
	// per-store renamed POSTGRES_USERS_* set, plus REDIS_PASSWORD) are
	// NOT in ds.Agent.Environment. They flow via knowledgeCredEnvVars
	// at apply time as direct secretKeyRef entries on the agent
	// container, avoiding the duplicate-with-Secret pattern that
	// previously left the same name on the pod twice.
	for _, key := range []string{
		"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB",
		"POSTGRES_USERS_USER", "POSTGRES_USERS_PASSWORD", "POSTGRES_USERS_DB",
		"REDIS_PASSWORD",
	} {
		if _, exists := ds.Agent.Environment[key]; exists {
			t.Errorf("%s should not be in agent.Environment — credential env vars flow via knowledgeCredEnvVars at apply time, not through the spec resolver path", key)
		}
	}
	// The redundant qualified forms must also stay absent (RFC §8.2).
	for _, key := range []string{"POSTGRES_POSTGRES_USER", "POSTGRES_POSTGRES_PASSWORD", "POSTGRES_POSTGRES_DB"} {
		if _, exists := ds.Agent.Environment[key]; exists {
			t.Errorf("%s must not exist (entry name matches provider)", key)
		}
	}
}

// Test that RestoreBindingsFromSpec extracts bound entries from a stored
// deployment spec JSON and produces the correct TemplateBindings.
func TestRestoreBindingsFromSpec(t *testing.T) {
	storedSpec := spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"postgres": {Binding: "arn:knowledge:acct:pg-store", Provider: "postgres"},
			"cache":    {Image: "redis:7", Provider: "redis"},                             // not bound
			"users":    {Binding: "arn:knowledge:acct:users-store", Provider: "postgres"}, // bound
		},
	}
	specJSON, err := json.Marshal(storedSpec)
	if err != nil {
		t.Fatal(err)
	}

	bindings := RestoreBindingsFromSpec(nil, string(specJSON))
	if bindings == nil {
		t.Fatal("expected non-nil bindings")
	}
	if len(bindings.Knowledge) != 2 {
		t.Fatalf("expected 2 bound entries, got %d: %v", len(bindings.Knowledge), bindings.Knowledge)
	}
	if bindings.Knowledge["postgres"] != "arn:knowledge:acct:pg-store" {
		t.Errorf("postgres: got %q", bindings.Knowledge["postgres"])
	}
	if bindings.Knowledge["users"] != "arn:knowledge:acct:users-store" {
		t.Errorf("users: got %q", bindings.Knowledge["users"])
	}
	if _, ok := bindings.Knowledge["cache"]; ok {
		t.Error("cache should not be in bindings (not bound)")
	}
}

func TestRestoreBindingsFromSpec_NoBoundEntries(t *testing.T) {
	storedSpec := spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"cache": {Image: "redis:7", Provider: "redis"},
		},
	}
	specJSON, _ := json.Marshal(storedSpec)

	bindings := RestoreBindingsFromSpec(nil, string(specJSON))
	if bindings != nil {
		t.Errorf("expected nil bindings when no entries are bound, got %v", bindings)
	}
}

func TestRestoreBindingsFromSpec_EmptyJSON(t *testing.T) {
	bindings := RestoreBindingsFromSpec(nil, "")
	if bindings != nil {
		t.Errorf("expected nil for empty JSON, got %v", bindings)
	}
}

func TestRestoreBindingsFromSpec_InvalidJSON(t *testing.T) {
	bindings := RestoreBindingsFromSpec(nil, "{invalid")
	if bindings != nil {
		t.Errorf("expected nil for invalid JSON, got %v", bindings)
	}
}

// ===== YAML-driven interface / adapter tests (A1–A5) =====
//
// Each test parses an inline astropods.yml string and asserts the generated
// template has the correct interfaces and slack-variable shape.

func mustGenerateFromYAML(t *testing.T, yaml string) *spec.AstroDeploymentSpec {
	t.Helper()
	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("spec.ParseString: %v", err)
	}
	return mustGenerate(t, TemplateInput{
		Spec:        s,
		AgentName:   s.Name,
		Account:     "acme",
		BuildID:     "build1",
		RegistryURL: "registry.example.com",
	})
}

// A1: no interfaces key → messaging enabled by default; no slack variables.
func TestYAML_Interfaces_A1_DefaultNoSlackVars(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
`)

	if ds.Interfaces == nil {
		t.Fatal("interfaces: expected non-nil (messaging on by default)")
	}

	for _, key := range []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN", "SLACK_CONFIG"} {
		if _, ok := ds.Variables[key]; ok {
			t.Errorf("variables.%s: must not be present when no adapter configured", key)
		}
	}
}

// A2: interfaces.messaging: true is equivalent to the default; no slack variables.
func TestYAML_Interfaces_A2_MessagingExplicitTrueNoSlackVars(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
  interfaces:
    messaging: true
`)

	if ds.Interfaces == nil {
		t.Fatal("interfaces: expected non-nil")
	}

	for _, key := range []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN", "SLACK_CONFIG"} {
		if _, ok := ds.Variables[key]; ok {
			t.Errorf("variables.%s: must not be present when no adapter configured", key)
		}
	}
}

// A3: interfaces.messaging: false → interfaces block nil, no slack variables.
func TestYAML_Interfaces_A3_MessagingDisabled(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
  interfaces:
    messaging: false
`)

	if ds.Interfaces != nil {
		t.Error("interfaces: expected nil when messaging: false")
	}

	for _, key := range []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN", "SLACK_CONFIG"} {
		if _, ok := ds.Variables[key]; ok {
			t.Errorf("variables.%s: must not be present when messaging disabled", key)
		}
	}
}

// A6: interfaces.frontend: false + messaging: false explicitly → same result as A3;
// no interfaces block, no slack vars, agent stays on default port 8080.
func TestYAML_Interfaces_A6_BothExplicitlyFalse(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
  interfaces:
    frontend: false
    messaging: false
`)

	if ds.Interfaces != nil {
		t.Error("interfaces: expected nil when both frontend and messaging are false")
	}

	httpEp := spec.EndpointByName(ds.Agent.Endpoints, "http")
	if httpEp == nil {
		t.Fatal("agent.endpoints.http: expected non-nil")
	}
	if httpEp.Port != 8080 {
		t.Errorf("agent.endpoints.http.port: expected 8080, got %d", httpEp.Port)
	}
	if httpEp.Expose != nil && httpEp.Expose.Enabled {
		t.Error("agent.endpoints.http.expose.enabled: expected false")
	}

	for _, key := range []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN", "SLACK_CONFIG"} {
		if _, ok := ds.Variables[key]; ok {
			t.Errorf("variables.%s: must not be present", key)
		}
	}
}

// A4: interfaces.frontend: true (messaging omitted → false) → agent on port 80
// with expose enabled; no messaging block.
func TestYAML_Interfaces_A4_FrontendOnly(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
  interfaces:
    frontend: true
`)

	if ds.Interfaces != nil {
		t.Error("interfaces: expected nil (messaging omitted → false)")
	}

	httpEp := spec.EndpointByName(ds.Agent.Endpoints, "http")
	if httpEp == nil {
		t.Fatal("agent.endpoints.http: expected non-nil for frontend agent")
	}
	if httpEp.Port != 80 {
		t.Errorf("agent.endpoints.http.port: expected 80, got %d", httpEp.Port)
	}
	if httpEp.Expose == nil || !httpEp.Expose.Enabled {
		t.Error("agent.endpoints.http.expose.enabled: expected true for frontend agent")
	}
}

// Frontend agents must bind to :80; the platform injects PORT so frameworks
// reading process.env.PORT (Express, FastAPI) don't fall back to their default
// port and crash-loop behind the ingress.
func TestYAML_Interfaces_FrontendInjectsPORT(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
  interfaces:
    frontend: true
`)

	if got := ds.Agent.Environment["PORT"]; got != "80" {
		t.Errorf("agent.environment.PORT = %q, want 80", got)
	}
}

// Non-frontend agents must NOT have PORT injected — they listen on the
// messaging gRPC port via GRPC_SERVER_ADDR, not HTTP :80.
func TestYAML_Interfaces_NoFrontendNoPORT(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
`)

	if _, ok := ds.Agent.Environment["PORT"]; ok {
		t.Error("agent.environment.PORT: should not be set for non-frontend agent")
	}
}

// A5: interfaces.frontend: true + messaging: true → agent on port 80 with expose
// AND messaging block present.
func TestYAML_Interfaces_A5_FrontendAndMessaging(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
  interfaces:
    frontend: true
    messaging: true
`)

	httpEp := spec.EndpointByName(ds.Agent.Endpoints, "http")
	if httpEp == nil {
		t.Fatal("agent.endpoints.http: expected non-nil")
	}
	if httpEp.Port != 80 {
		t.Errorf("agent.endpoints.http.port: expected 80, got %d", httpEp.Port)
	}
	if httpEp.Expose == nil || !httpEp.Expose.Enabled {
		t.Error("agent.endpoints.http.expose.enabled: expected true")
	}

	if ds.Interfaces == nil {
		t.Error("interfaces: expected non-nil (messaging: true)")
	}
}

// A8: frontend + messaging both enabled → interfaces block has all correct fields
// (same as A7) independent of the agent being a frontend.
func TestYAML_Interfaces_A8_FrontendAndMessagingInterfacesBlock(t *testing.T) {
	ds := mustGenerateFromYAML(t, `
name: my-agent
agent:
  image: registry.example.com/acme/my-agent:build1
  interfaces:
    frontend: true
    messaging: true
`)

	if ds.Interfaces == nil {
		t.Fatal("interfaces: expected non-nil")
	}

	if len(ds.Interfaces.Adapters) != 0 {
		t.Errorf("interfaces.adapters: expected empty, got %v", ds.Interfaces.Adapters)
	}

	if ds.Interfaces.Image != "registry.example.com/dockerhub/astropods/messaging:latest" {
		t.Errorf("interfaces.image: got %s", ds.Interfaces.Image)
	}

	grpcEp := spec.EndpointByName(ds.Interfaces.Endpoints, "grpc")
	if grpcEp == nil || grpcEp.Port != 9090 {
		t.Errorf("interfaces.endpoints.grpc.port: expected 9090, got %v", grpcEp)
	}

	httpEp := spec.EndpointByName(ds.Interfaces.Endpoints, "http")
	if httpEp == nil || httpEp.Port != 8080 {
		t.Errorf("interfaces.endpoints.http.port: expected 8080, got %v", httpEp)
	}
	if httpEp != nil && httpEp.Expose != nil && httpEp.Expose.Enabled {
		t.Error("interfaces.endpoints.http.expose.enabled: expected false")
	}

	if ds.Interfaces.Resources != spec.MessagingResources {
		t.Errorf("interfaces.resources: expected MessagingResources, got %+v", ds.Interfaces.Resources)
	}

	if ds.Interfaces.Auth == nil || ds.Interfaces.Auth.Web == nil {
		t.Fatal("interfaces.auth.web: expected non-nil")
	}
	if ds.Interfaces.Auth.Web.Type != "oidc" {
		t.Errorf("interfaces.auth.web.type: expected oidc, got %q", ds.Interfaces.Auth.Web.Type)
	}
	if ds.Interfaces.Auth.Slack != nil {
		t.Errorf("interfaces.auth.slack: expected nil, got %+v", ds.Interfaces.Auth.Slack)
	}
}

// storedSpecWithBindings returns a marshaled spec that has two bound knowledge
// entries — used as the "deployment already has bindings" fixture.
func storedSpecWithBindings(t *testing.T) string {
	t.Helper()
	stored := spec.AstroDeploymentSpec{
		Knowledge: map[string]spec.DeploymentKnowledge{
			"postgres": {Binding: "arn:knowledge:acct:pg-store", Provider: "postgres"},
			"users":    {Binding: "arn:knowledge:acct:users-store", Provider: "postgres"},
		},
	}
	b, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// When the client sends no Bindings field at all, the stored bindings should
// be restored — that's the "open the configure panel for an existing
// deployment" case the function was designed for.
func TestApplyStoredBindingsToRequest_NilBindings_Restores(t *testing.T) {
	req := &spec.TemplateRequest{}
	ApplyStoredBindingsToRequest(nil, req, storedSpecWithBindings(t))

	if req.Bindings == nil {
		t.Fatal("expected stored bindings to be restored, got nil")
	}
	if got := req.Bindings.Knowledge["postgres"]; got != "arn:knowledge:acct:pg-store" {
		t.Errorf("postgres: got %q", got)
	}
}

// When the client sends explicit non-empty ARNs, the request must win over
// the stored bindings.
func TestApplyStoredBindingsToRequest_ExplicitNonEmpty_Wins(t *testing.T) {
	req := &spec.TemplateRequest{
		Bindings: &spec.TemplateBindings{
			Knowledge: map[string]string{"postgres": "arn:knowledge:acct:other-store"},
		},
	}
	ApplyStoredBindingsToRequest(nil, req, storedSpecWithBindings(t))

	if got := req.Bindings.Knowledge["postgres"]; got != "arn:knowledge:acct:other-store" {
		t.Errorf("postgres: got %q, want client-supplied ARN", got)
	}
	if _, ok := req.Bindings.Knowledge["users"]; ok {
		t.Errorf("users should not be present — client only sent postgres")
	}
}

// FAILING: client sends an empty Knowledge map to clear all bindings on a
// deployment that already has some. The request must be honored: the
// resulting bindings should be empty, not silently restored from the stored
// spec.
func TestApplyStoredBindingsToRequest_EmptyMap_ClearsAll(t *testing.T) {
	req := &spec.TemplateRequest{
		Bindings: &spec.TemplateBindings{Knowledge: map[string]string{}},
	}
	ApplyStoredBindingsToRequest(nil, req, storedSpecWithBindings(t))

	if req.Bindings == nil {
		t.Fatal("expected non-nil Bindings (client sent an empty map)")
	}
	if len(req.Bindings.Knowledge) != 0 {
		t.Errorf("expected empty bindings (client cleared all), got %v", req.Bindings.Knowledge)
	}
}

// FAILING: client sends explicit empty-string ARNs to unbind specific entries.
// Each "" must be preserved as an explicit unbind, not restored from the
// stored spec.
func TestApplyStoredBindingsToRequest_AllEmptyARNs_Unbinds(t *testing.T) {
	req := &spec.TemplateRequest{
		Bindings: &spec.TemplateBindings{
			Knowledge: map[string]string{"postgres": "", "users": ""},
		},
	}
	ApplyStoredBindingsToRequest(nil, req, storedSpecWithBindings(t))

	if req.Bindings == nil {
		t.Fatal("expected non-nil Bindings")
	}
	if got := req.Bindings.Knowledge["postgres"]; got != "" {
		t.Errorf("postgres: got %q, want empty (unbind)", got)
	}
	if got := req.Bindings.Knowledge["users"]; got != "" {
		t.Errorf("users: got %q, want empty (unbind)", got)
	}
}

// Mixed case: client unbinds one entry while leaving another bound. The
// unbind must stick.
func TestApplyStoredBindingsToRequest_MixedExplicit_Unbind(t *testing.T) {
	req := &spec.TemplateRequest{
		Bindings: &spec.TemplateBindings{
			Knowledge: map[string]string{
				"postgres": "arn:knowledge:acct:pg-store",
				"users":    "",
			},
		},
	}
	ApplyStoredBindingsToRequest(nil, req, storedSpecWithBindings(t))

	if got := req.Bindings.Knowledge["postgres"]; got != "arn:knowledge:acct:pg-store" {
		t.Errorf("postgres: got %q", got)
	}
	if got := req.Bindings.Knowledge["users"]; got != "" {
		t.Errorf("users: got %q, want empty (unbind)", got)
	}
}

// Regression: a frontend-only agent (astropods.yml interfaces.frontend: true)
// must deploy. The deploy form sends interfaces.auth.custom for the custom
// interface; shaping synthesizes an auth-only interfaces block (no adapters, no
// image). Previously the deploy parser rejected that with "interfaces.adapters
// must not be empty when interfaces is present".
func TestShapeTemplate_FrontendOnlyWithCustomAuthDeploys(t *testing.T) {
	input := baseInput()
	input.Spec.Agent.Interfaces = &spec.Interfaces{Frontend: true}
	base := mustGenerate(t, input)

	// Precondition: a frontend-only agent has no messaging interfaces block but
	// does expose its own http endpoint.
	if base.Interfaces != nil {
		t.Fatalf("frontend-only base should have no interfaces block, got %+v", base.Interfaces)
	}
	if spec.ExposedEndpoint(base.Agent.Endpoints) == nil {
		t.Fatal("frontend-only agent should expose its http endpoint")
	}

	// Deploy form sends interfaces.auth.custom (no messaging adapters).
	resp := ShapeTemplate(context.Background(), base, &spec.TemplateRequest{
		Interfaces: &spec.TemplateInterfaces{
			Adapters: []string{},
			Auth:     &spec.DeploymentInterfacesAuth{Custom: &spec.DeploymentCustomAuth{Public: true}},
		},
	}, nil)

	if resp.Template.Interfaces == nil || resp.Template.Interfaces.Auth == nil || resp.Template.Interfaces.Auth.Custom == nil {
		t.Fatalf("expected interfaces.auth.custom on shaped template, got %+v", resp.Template.Interfaces)
	}
	if !resp.Template.Interfaces.Auth.Custom.Public {
		t.Error("custom.public should be true")
	}
	if len(resp.Template.Interfaces.Adapters) != 0 {
		t.Errorf("no messaging adapters expected, got %v", resp.Template.Interfaces.Adapters)
	}

	// The shaped deployment spec must parse — this is the regression guard.
	raw, err := spec.SerializeDeploymentSpec(&resp.Template)
	if err != nil {
		t.Fatalf("serialize shaped template: %v", err)
	}
	if _, err := spec.ParseDeploymentSpec(raw); err != nil {
		t.Fatalf("frontend-only agent with custom auth should deploy, got: %v", err)
	}
}
