package spec

import (
	"testing"
)

// ─── §8.5 SanitizeEnvName ────────────────────────────────────────────────────

func TestSanitizeEnvName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"llm", "LLM"},
		{"my_model", "MY-MODEL"},
		{"my.store", "MY-STORE"},
		{"local_llm", "LOCAL-LLM"},
		{"my-service", "MY-SERVICE"},
		{"docs_sync", "DOCS-SYNC"},
		{"a.b.c", "A-B-C"},
		{"A_B_C", "A-B-C"},    // already uppercased input still works
		{"", ""},              // empty
		{"-leading", "LEADING"},
		{"trailing-", "TRAILING"},
		{"double--hyphens", "DOUBLE-HYPHENS"},
		{"hello world", "HELLOWORLD"}, // space removed
		{"my__model", "MY-MODEL"},     // consecutive underscores → single hyphen
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := SanitizeEnvName(tt.in); got != tt.want {
				t.Errorf("SanitizeEnvName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ─── §8.1 Cloud credential keys ─────────────────────────────────────────────

func TestCloudCredentialKeys_SingleModelProvider(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"primary": {Provider: "anthropic"},
		},
	}
	keys := CloudCredentialKeys(s)
	assertCredKey(t, keys, "ANTHROPIC_API_KEY", "model", false)
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_SingleKnowledgeProvider(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"vectors": {Provider: "pinecone"},
		},
	}
	keys := CloudCredentialKeys(s)
	assertCredKey(t, keys, "PINECONE_API_KEY", "knowledge", false)
}

func TestCloudCredentialKeys_SingleToolProvider(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Tools: map[string]Tool{
			"github": {Provider: "github"},
		},
	}
	keys := CloudCredentialKeys(s)
	assertCredKey(t, keys, "GITHUB_TOKEN", "tool", false)
}

func TestCloudCredentialKeys_DuplicateModelProviders_NameMatchesPrimary(t *testing.T) {
	// Entry named "anthropic" using provider "anthropic" is primary.
	// Entry named "sonnet" using provider "anthropic" gets qualified key.
	// "anthropic" (name == provider) → bare key only, no ANTHROPIC_ANTHROPIC_API_KEY.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"anthropic": {Provider: "anthropic"},
			"sonnet":    {Provider: "anthropic"},
		},
	}
	keys := CloudCredentialKeys(s)

	// Bare key for primary.
	assertCredKey(t, keys, "ANTHROPIC_API_KEY", "model", false)
	// Qualified key for non-primary.
	assertCredKey(t, keys, "ANTHROPIC_SONNET_API_KEY", "model", false)
	// Redundant double-name form must NOT appear.
	if _, ok := keys["ANTHROPIC_ANTHROPIC_API_KEY"]; ok {
		t.Error("should not produce redundant ANTHROPIC_ANTHROPIC_API_KEY")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_DuplicateModelProviders_FirstAlphaIsPrimary(t *testing.T) {
	// No entry name matches provider; "alpha" < "beta" alphabetically, so "alpha" is primary.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"beta":  {Provider: "anthropic"},
			"alpha": {Provider: "anthropic"},
		},
	}
	keys := CloudCredentialKeys(s)

	// "alpha" is first alphabetically → gets bare key + qualified key.
	assertCredKey(t, keys, "ANTHROPIC_API_KEY", "model", false)
	assertCredKey(t, keys, "ANTHROPIC_ALPHA_API_KEY", "model", false)
	// "beta" gets qualified key only.
	assertCredKey(t, keys, "ANTHROPIC_BETA_API_KEY", "model", false)
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_MultipleProviders(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"llm": {Provider: "anthropic"},
		},
		Tools: map[string]Tool{
			"gh": {Provider: "github"},
		},
	}
	keys := CloudCredentialKeys(s)
	assertCredKey(t, keys, "ANTHROPIC_API_KEY", "model", false)
	assertCredKey(t, keys, "GITHUB_TOKEN", "tool", false)
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_SkipsCustomProviders(t *testing.T) {
	// A tool referencing a custom provider must not appear in cloud credential keys.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"myjira": {Scope: []string{"tools"}, Variables: []Input{
				{Name: "JIRA_API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Tools: map[string]Tool{
			"jira": {Provider: "myjira"},
		},
	}
	keys := CloudCredentialKeys(s)
	if len(keys) != 0 {
		t.Errorf("expected 0 cloud keys (only custom provider), got %d: %v", len(keys), keys)
	}
}

func TestCloudCredentialKeys_OptionalCredential(t *testing.T) {
	// gemini has optional=false by default; validate the Optional field is carried through.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"ai": {Provider: "openai"},
		},
	}
	keys := CloudCredentialKeys(s)
	m, ok := keys["OPENAI_API_KEY"]
	if !ok {
		t.Fatal("OPENAI_API_KEY not found")
	}
	if m.Optional {
		t.Error("expected Optional=false for OPENAI_API_KEY")
	}
}

// ─── Custom provider credential keys ────────────────────────────────────────

func TestCustomProviderCredentialKeys_SecretVariables(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"myjira": {Scope: []string{"tools"}, Variables: []Input{
				{Name: "JIRA_API_KEY", Datatype: "string", Secret: true, Description: "Jira key"},
				{Name: "JIRA_URL", Datatype: "string", Secret: false},     // non-secret: not included
				{Name: "JIRA_TOKEN", Datatype: "string", Secret: true, Optional: true},
			}},
		},
		Tools: map[string]Tool{
			"jira": {Provider: "myjira"},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	assertCredKey(t, keys, "JIRA_API_KEY", "provider", false)
	assertCredKey(t, keys, "JIRA_TOKEN", "provider", true)
	if _, ok := keys["JIRA_URL"]; ok {
		t.Error("JIRA_URL (non-secret) must not appear in credential keys")
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
}

func TestCustomProviderCredentialKeys_UnreferencedProviderExcluded(t *testing.T) {
	// A provider defined but not referenced by any component is excluded.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"unused": {Scope: []string{"tools"}, Variables: []Input{
				{Name: "UNUSED_KEY", Datatype: "string", Secret: true},
			}},
		},
	}
	keys := CustomProviderCredentialKeys(s)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys for unreferenced provider, got %d: %v", len(keys), keys)
	}
}

// ─── §8.2 Self-hosted provider connection wiring ─────────────────────────────

func TestAgentConnectionKeys_SelfHostedModel(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"local": {Provider: "ollama", Model: "llama3.2"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.local": {Host: "model-local", Port: "11434", URL: "http://model-local:11434", BaseURL: "http://model-local:11434/api"},
	}
	env := AgentConnectionKeys(s, addrs)

	assertEnv(t, env, "OLLAMA_HOST", "model-local")
	assertEnv(t, env, "OLLAMA_PORT", "11434")
	assertEnv(t, env, "OLLAMA_URL", "http://model-local:11434")
	assertEnv(t, env, "OLLAMA_BASE_URL", "http://model-local:11434/api")
	assertEnv(t, env, "OLLAMA_MODEL", "llama3.2")
}

func TestAgentConnectionKeys_SelfHostedModel_NoModel(t *testing.T) {
	// When model name is not set, OLLAMA_MODEL must not be injected.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"local": {Provider: "ollama"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.local": {Host: "h", Port: "11434", URL: "http://h:11434", BaseURL: "http://h:11434/api"},
	}
	env := AgentConnectionKeys(s, addrs)
	if _, ok := env["OLLAMA_MODEL"]; ok {
		t.Error("OLLAMA_MODEL must not be set when model name is empty")
	}
}

func TestAgentConnectionKeys_DuplicateSelfHostedModelProvider(t *testing.T) {
	// Two entries both using ollama → qualified keys + bare keys for the first alphabetically.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"large": {Provider: "ollama", Model: "llama3.2:70b"},
			"small": {Provider: "ollama", Model: "llama3.2"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.large": {Host: "model-large", Port: "11434", URL: "http://model-large:11434", BaseURL: "http://model-large:11434/api"},
		"models.small": {Host: "model-small", Port: "11435", URL: "http://model-small:11435", BaseURL: "http://model-small:11435/api"},
	}
	env := AgentConnectionKeys(s, addrs)

	// "large" < "small" alphabetically → "large" is first → gets bare + qualified.
	assertEnv(t, env, "OLLAMA_HOST", "model-large")           // bare, first
	assertEnv(t, env, "OLLAMA_LARGE_HOST", "model-large")     // qualified
	assertEnv(t, env, "OLLAMA_SMALL_HOST", "model-small")     // qualified only
	assertEnv(t, env, "OLLAMA_LARGE_MODEL", "llama3.2:70b")
	assertEnv(t, env, "OLLAMA_SMALL_MODEL", "llama3.2")
	// Bare MODEL key goes to the first (alphabetically) entry — "large".
	assertEnv(t, env, "OLLAMA_MODEL", "llama3.2:70b")
}

// ─── §8.2 Self-hosted knowledge provider connection wiring ───────────────────

func TestAgentConnectionKeys_SelfHostedKnowledge(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"docs": {Provider: "qdrant", Persistent: true},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.docs": {Host: "knowledge-docs", Port: "6333", URL: "http://knowledge-docs:6333"},
	}
	env := AgentConnectionKeys(s, addrs)

	assertEnv(t, env, "QDRANT_HOST", "knowledge-docs")
	assertEnv(t, env, "QDRANT_PORT", "6333")
	assertEnv(t, env, "QDRANT_URL", "http://knowledge-docs:6333")
}

func TestAgentConnectionKeys_SelfHostedKnowledge_WithRedisURL(t *testing.T) {
	// Redis has URLScheme="redis" → URL key IS injected.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"cache": {Provider: "redis"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.cache": {Host: "knowledge-cache", Port: "6379", URL: "redis://knowledge-cache:6379"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "REDIS_HOST", "knowledge-cache")
	assertEnv(t, env, "REDIS_PORT", "6379")
	assertEnv(t, env, "REDIS_URL", "redis://knowledge-cache:6379")
}

func TestAgentConnectionKeys_SelfHostedKnowledge_NoURL(t *testing.T) {
	// Postgres provider has no URLScheme → URL key must not be injected.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"db": {Provider: "postgres"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.db": {Host: "knowledge-db", Port: "5432"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "POSTGRES_HOST", "knowledge-db")
	assertEnv(t, env, "POSTGRES_PORT", "5432")
	if _, ok := env["POSTGRES_URL"]; ok {
		t.Error("POSTGRES_URL must not be injected (postgres has no URLScheme)")
	}
}

func TestAgentConnectionKeys_DuplicateSelfHostedKnowledgeProvider(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"docs":   {Provider: "qdrant"},
			"images": {Provider: "qdrant"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.docs":   {Host: "kn-docs", Port: "6333", URL: "http://kn-docs:6333"},
		"knowledge.images": {Host: "kn-images", Port: "6334", URL: "http://kn-images:6334"},
	}
	env := AgentConnectionKeys(s, addrs)

	// "docs" < "images" → bare keys go to docs.
	assertEnv(t, env, "QDRANT_HOST", "kn-docs")
	assertEnv(t, env, "QDRANT_DOCS_HOST", "kn-docs")
	assertEnv(t, env, "QDRANT_IMAGES_HOST", "kn-images")
}

// ─── §8.3 Container-mode connection wiring ───────────────────────────────────

func TestAgentConnectionKeys_ContainerModeModel(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"embedder": {Container: &ContainerConfig{Image: "embed:latest", Port: 8000}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.embedder": {Host: "model-embedder", Port: "8000", URL: "http://model-embedder:8000"},
	}
	env := AgentConnectionKeys(s, addrs)

	assertEnv(t, env, "MODEL_EMBEDDER_HOST", "model-embedder")
	assertEnv(t, env, "MODEL_EMBEDDER_PORT", "8000")
	assertEnv(t, env, "MODEL_EMBEDDER_URL", "http://model-embedder:8000")
}

func TestAgentConnectionKeys_ContainerModeModel_NameSanitized(t *testing.T) {
	// Entry name "my_embedder" → key prefix "MODEL_MY-EMBEDDER_*"
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"my_embedder": {Container: &ContainerConfig{Image: "embed:latest", Port: 8000}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.my_embedder": {Host: "model-my-embedder", Port: "8000", URL: "http://model-my-embedder:8000"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "MODEL_MY-EMBEDDER_HOST", "model-my-embedder")
}

func TestAgentConnectionKeys_ContainerModeKnowledge(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"custom": {Container: &ContainerConfig{Image: "mydb:latest", Port: 5432}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"knowledge.custom": {Host: "knowledge-custom", Port: "5432"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "KNOWLEDGE_CUSTOM_HOST", "knowledge-custom")
	assertEnv(t, env, "KNOWLEDGE_CUSTOM_PORT", "5432")
	// Container-mode knowledge does NOT inject URL.
	if _, ok := env["KNOWLEDGE_CUSTOM_URL"]; ok {
		t.Error("KNOWLEDGE_CUSTOM_URL must not be injected for container-mode knowledge")
	}
}

func TestAgentConnectionKeys_ContainerModeTool(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Tools: map[string]Tool{
			"search": {Container: &ContainerConfig{Image: "search:latest", Port: 3000}},
		},
	}
	addrs := map[string]ConnectionAddress{
		"tools.search": {Host: "tool-search", Port: "3000", URL: "http://tool-search:3000"},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "TOOL_SEARCH_HOST", "tool-search")
	assertEnv(t, env, "TOOL_SEARCH_PORT", "3000")
	assertEnv(t, env, "TOOL_SEARCH_URL", "http://tool-search:3000")
}

func TestAgentConnectionKeys_CloudProviderSkipped(t *testing.T) {
	// Cloud model, knowledge, and tool providers produce no connection keys.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"ai": {Provider: "anthropic"},
		},
		Knowledge: map[string]Knowledge{
			"vec": {Provider: "pinecone"},
		},
		Tools: map[string]Tool{
			"gh": {Provider: "github"},
		},
	}
	env := AgentConnectionKeys(s, nil)
	if len(env) != 0 {
		t.Errorf("cloud-only providers should produce no connection keys, got %v", env)
	}
}

func TestAgentConnectionKeys_CustomProviderToolSkipped(t *testing.T) {
	// Custom provider referenced by a tool → no connection wiring.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Providers: map[string]CustomProvider{
			"myjira": {Scope: []string{"tools"}, Variables: []Input{
				{Name: "JIRA_API_KEY", Datatype: "string", Secret: true},
			}},
		},
		Tools: map[string]Tool{
			"jira": {Provider: "myjira"},
		},
	}
	env := AgentConnectionKeys(s, nil)
	if len(env) != 0 {
		t.Errorf("custom provider tool should produce no connection keys, got %v", env)
	}
}

// ─── §8.4 Input injection ─────────────────────────────────────────────────────

func TestResolveEnvVars_TopLevelInputs(t *testing.T) {
	// Top-level inputs are injected into ALL containers.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Inputs: map[string]Input{
			"LOG_LEVEL": {Name: "LOG_LEVEL", Datatype: "string", Default: "info"},
		},
		Models: map[string]Model{
			"llm": {Provider: "ollama"},
		},
		Knowledge: map[string]Knowledge{
			"docs": {Provider: "qdrant"},
		},
		Ingestion: map[string]Ingestion{
			"sync": {Container: ContainerConfig{Image: "sync:latest"}, Trigger: IngestionTrigger{Type: "schedule"}},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)

	assertEnv(t, res.Agent, "LOG_LEVEL", "info")
	assertEnv(t, res.Models["llm"], "LOG_LEVEL", "info")
	assertEnv(t, res.Knowledge["docs"], "LOG_LEVEL", "info")
	assertEnv(t, res.Ingestion["sync"], "LOG_LEVEL", "info")
}

func TestResolveEnvVars_TopLevelInputs_UserOverridesDefault(t *testing.T) {
	s := &AstroSpec{
		Name:   "agent",
		Agent:  Container{Image: "a:1"},
		Inputs: map[string]Input{
			"LOG_LEVEL": {Name: "LOG_LEVEL", Datatype: "string", Default: "info"},
		},
	}
	res := ResolveEnvVars(s, nil, nil, map[string]string{"LOG_LEVEL": "debug"})
	assertEnv(t, res.Agent, "LOG_LEVEL", "debug")
}

func TestResolveEnvVars_TopLevelInputs_EmptyDefaultAndNoValue(t *testing.T) {
	// When default is empty and no user value, the key must not appear.
	s := &AstroSpec{
		Name:   "agent",
		Agent:  Container{Image: "a:1"},
		Inputs: map[string]Input{
			"OPTIONAL_KEY": {Name: "OPTIONAL_KEY", Datatype: "string"},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	if _, ok := res.Agent["OPTIONAL_KEY"]; ok {
		t.Error("OPTIONAL_KEY with no default and no value must not appear in env")
	}
}

func TestResolveEnvVars_AgentInputs(t *testing.T) {
	// Agent-specific inputs go only to the agent.
	s := &AstroSpec{
		Name: "agent",
		Agent: Container{
			Image: "a:1",
			Inputs: []Input{
				{Name: "LOG_LEVEL", Datatype: "string", Default: "warn"},
			},
		},
		Models: map[string]Model{
			"llm": {Provider: "ollama"},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Agent, "LOG_LEVEL", "warn")
	if _, ok := res.Models["llm"]["LOG_LEVEL"]; ok {
		t.Error("agent-specific input must not appear in model container")
	}
}

func TestResolveEnvVars_ModelInputs(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"llm": {
				Provider: "ollama",
				Inputs: []Input{
					{Name: "BATCH_SIZE", Datatype: "number", Default: "32"},
				},
			},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Models["llm"], "BATCH_SIZE", "32")
	if _, ok := res.Agent["BATCH_SIZE"]; ok {
		t.Error("model-specific input must not appear in agent container")
	}
}

func TestResolveEnvVars_KnowledgeInputs(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Knowledge: map[string]Knowledge{
			"docs": {
				Provider: "qdrant",
				Inputs: []Input{
					{Name: "COLLECTION_NAME", Datatype: "string", Default: "embeddings"},
				},
			},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Knowledge["docs"], "COLLECTION_NAME", "embeddings")
	if _, ok := res.Agent["COLLECTION_NAME"]; ok {
		t.Error("knowledge-specific input must not appear in agent container")
	}
}

func TestResolveEnvVars_ToolInputs(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Tools: map[string]Tool{
			"search": {
				Container: &ContainerConfig{Image: "search:latest", Port: 3000},
				Inputs: []Input{
					{Name: "RESULT_LIMIT", Datatype: "number", Default: "10"},
				},
			},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Tools["search"], "RESULT_LIMIT", "10")
	if _, ok := res.Agent["RESULT_LIMIT"]; ok {
		t.Error("tool-specific input must not appear in agent container")
	}
}

func TestResolveEnvVars_IngestionInputs(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Ingestion: map[string]Ingestion{
			"sync": {
				Container: ContainerConfig{Image: "sync:latest"},
				Trigger:   IngestionTrigger{Type: "schedule"},
				Inputs: []Input{
					{Name: "BATCH_SIZE", Datatype: "number", Default: "100"},
				},
			},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	assertEnv(t, res.Ingestion["sync"], "BATCH_SIZE", "100")
	if _, ok := res.Agent["BATCH_SIZE"]; ok {
		t.Error("ingestion-specific input must not appear in agent container")
	}
}

func TestResolveEnvVars_InputScopeIsolation(t *testing.T) {
	// Each component type must only receive inputs declared for it.
	// (No cross-contamination between model, knowledge, tool, ingestion inputs.)
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"llm": {Provider: "ollama", Inputs: []Input{{Name: "MODEL_FLAG", Datatype: "string", Default: "x"}}},
		},
		Knowledge: map[string]Knowledge{
			"docs": {Provider: "qdrant", Inputs: []Input{{Name: "K_FLAG", Datatype: "string", Default: "y"}}},
		},
		Tools: map[string]Tool{
			"srch": {Container: &ContainerConfig{Image: "s:1"}, Inputs: []Input{{Name: "TOOL_FLAG", Datatype: "string", Default: "z"}}},
		},
	}
	res := ResolveEnvVars(s, nil, nil, nil)

	// model input only in model container
	assertEnv(t, res.Models["llm"], "MODEL_FLAG", "x")
	if _, ok := res.Knowledge["docs"]["MODEL_FLAG"]; ok {
		t.Error("model input leaked into knowledge container")
	}
	if _, ok := res.Tools["srch"]["MODEL_FLAG"]; ok {
		t.Error("model input leaked into tool container")
	}

	// knowledge input only in knowledge container
	assertEnv(t, res.Knowledge["docs"], "K_FLAG", "y")
	if _, ok := res.Models["llm"]["K_FLAG"]; ok {
		t.Error("knowledge input leaked into model container")
	}

	// tool input only in tool container
	assertEnv(t, res.Tools["srch"], "TOOL_FLAG", "z")
	if _, ok := res.Models["llm"]["TOOL_FLAG"]; ok {
		t.Error("tool input leaked into model container")
	}
}

// ─── Full spec: combined injection ───────────────────────────────────────────

func TestResolveEnvVars_FullSpec(t *testing.T) {
	s := &AstroSpec{
		Name: "agent",
		Agent: Container{
			Image: "agent:latest",
			Inputs: []Input{
				{Name: "LOG_LEVEL", Datatype: "string", Default: "info"},
			},
		},
		Inputs: map[string]Input{
			"ALLOWED_ORIGINS": {Name: "ALLOWED_ORIGINS", Datatype: "string", Default: "http://localhost"},
		},
		Models: map[string]Model{
			"llm":       {Provider: "ollama", Model: "llama3.2"},
			"embedder":  {Container: &ContainerConfig{Image: "embed:latest", Port: 8000}},
			"anthropic": {Provider: "anthropic"},
		},
		Knowledge: map[string]Knowledge{
			"docs":  {Provider: "qdrant", Persistent: true},
			"cache": {Provider: "redis"},
		},
		Tools: map[string]Tool{
			"github": {Provider: "github"},
			"search": {Container: &ContainerConfig{Image: "search:latest", Port: 3000}},
		},
		Providers: map[string]CustomProvider{
			"myjira": {Scope: []string{"tools"}, Variables: []Input{
				{Name: "JIRA_API_KEY", Datatype: "string", Secret: true},
			}},
		},
	}

	addrs := map[string]ConnectionAddress{
		"models.llm":       {Host: "model-llm", Port: "11434", URL: "http://model-llm:11434", BaseURL: "http://model-llm:11434/api"},
		"models.embedder":  {Host: "model-embedder", Port: "8000", URL: "http://model-embedder:8000"},
		"knowledge.docs":   {Host: "knowledge-docs", Port: "6333", URL: "http://knowledge-docs:6333"},
		"knowledge.cache":  {Host: "knowledge-cache", Port: "6379"},
		"tools.search":     {Host: "tool-search", Port: "3000", URL: "http://tool-search:3000"},
	}
	creds := map[string]string{
		"ANTHROPIC_API_KEY": "sk-ant-test",
		"GITHUB_TOKEN":      "ghp_test",
	}

	res := ResolveEnvVars(s, addrs, creds, nil)

	// Self-hosted model connections
	assertEnv(t, res.Agent, "OLLAMA_HOST", "model-llm")
	assertEnv(t, res.Agent, "OLLAMA_MODEL", "llama3.2")
	assertEnv(t, res.Agent, "OLLAMA_BASE_URL", "http://model-llm:11434/api")

	// Container-mode model connections
	assertEnv(t, res.Agent, "MODEL_EMBEDDER_HOST", "model-embedder")
	assertEnv(t, res.Agent, "MODEL_EMBEDDER_URL", "http://model-embedder:8000")

	// Self-hosted knowledge connections
	assertEnv(t, res.Agent, "QDRANT_HOST", "knowledge-docs")
	assertEnv(t, res.Agent, "QDRANT_URL", "http://knowledge-docs:6333")
	assertEnv(t, res.Agent, "REDIS_HOST", "knowledge-cache")

	// Container-mode tool connections
	assertEnv(t, res.Agent, "TOOL_SEARCH_HOST", "tool-search")
	assertEnv(t, res.Agent, "TOOL_SEARCH_URL", "http://tool-search:3000")

	// Cloud credentials
	assertEnv(t, res.Agent, "ANTHROPIC_API_KEY", "sk-ant-test")
	assertEnv(t, res.Agent, "GITHUB_TOKEN", "ghp_test")

	// Inputs
	assertEnv(t, res.Agent, "LOG_LEVEL", "info")
	assertEnv(t, res.Agent, "ALLOWED_ORIGINS", "http://localhost")

	// Top-level input in all containers
	assertEnv(t, res.Models["llm"], "ALLOWED_ORIGINS", "http://localhost")
	assertEnv(t, res.Knowledge["docs"], "ALLOWED_ORIGINS", "http://localhost")
	assertEnv(t, res.Tools["search"], "ALLOWED_ORIGINS", "http://localhost")

	// Agent-specific input not in other containers
	if _, ok := res.Models["llm"]["LOG_LEVEL"]; ok {
		t.Error("agent input LOG_LEVEL must not appear in model container")
	}

	// Cloud model/tool providers produce no connection keys
	if _, ok := res.Agent["MODEL_ANTHROPIC_HOST"]; ok {
		t.Error("cloud provider anthropic must not produce connection keys")
	}
	if _, ok := res.Agent["TOOL_GITHUB_HOST"]; ok {
		t.Error("cloud provider github must not produce connection keys")
	}
}

func TestResolveEnvVars_EmptySpec(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
	}
	res := ResolveEnvVars(s, nil, nil, nil)
	if len(res.Agent) != 0 {
		t.Errorf("empty spec should produce empty agent env, got %v", res.Agent)
	}
}

func TestResolveEnvVars_CredentialsWiredIntoAgent(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
	}
	creds := map[string]string{
		"MY_API_KEY": "secret123",
	}
	res := ResolveEnvVars(s, nil, creds, nil)
	assertEnv(t, res.Agent, "MY_API_KEY", "secret123")
	// Credentials must not leak into other (non-existent) containers.
	if len(res.Models) != 0 || len(res.Knowledge) != 0 {
		t.Error("credential must not create phantom component entries")
	}
}

// ─── Deferred placeholder values (deployment template use-case) ──────────────

func TestAgentConnectionKeys_DeferredPlaceholders(t *testing.T) {
	// Simulate the deployment template use-case: addrs hold ${...} references.
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Models: map[string]Model{
			"llm": {Provider: "ollama"},
		},
	}
	addrs := map[string]ConnectionAddress{
		"models.llm": {
			Host:    "${models.llm.host}",
			Port:    "${models.llm.port}",
			URL:     "${models.llm.url}",
			BaseURL: "${models.llm.url}/api",
		},
	}
	env := AgentConnectionKeys(s, addrs)
	assertEnv(t, env, "OLLAMA_HOST", "${models.llm.host}")
	assertEnv(t, env, "OLLAMA_PORT", "${models.llm.port}")
	assertEnv(t, env, "OLLAMA_URL", "${models.llm.url}")
	assertEnv(t, env, "OLLAMA_BASE_URL", "${models.llm.url}/api")
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func assertEnv(t *testing.T, env map[string]string, key, want string) {
	t.Helper()
	got, ok := env[key]
	if !ok {
		t.Errorf("env[%q] missing (want %q)", key, want)
		return
	}
	if got != want {
		t.Errorf("env[%q] = %q, want %q", key, got, want)
	}
}

func assertCredKey(t *testing.T, keys map[string]CredentialMeta, key, wantCategory string, wantOptional bool) {
	t.Helper()
	m, ok := keys[key]
	if !ok {
		t.Errorf("credential key %q missing", key)
		return
	}
	if m.Category != wantCategory {
		t.Errorf("key %q: category = %q, want %q", key, m.Category, wantCategory)
	}
	if m.Optional != wantOptional {
		t.Errorf("key %q: Optional = %v, want %v", key, m.Optional, wantOptional)
	}
}
