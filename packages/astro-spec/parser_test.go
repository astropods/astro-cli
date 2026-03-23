package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		check   func(*testing.T, *AstroSpec)
	}{
		{
			name: "minimal valid spec",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if s.Spec != "package/v1" {
					t.Errorf("Spec = %q, want %q", s.Spec, "package/v1")
				}
				if s.Name != "test-agent" {
					t.Errorf("Name = %q, want %q", s.Name, "test-agent")
				}
				if s.Agent.Image != "test:latest" {
					t.Errorf("Agent.Image = %q, want %q", s.Agent.Image, "test:latest")
				}
			},
		},
		{
			name: "spec with build config",
			yaml: `
spec: package/v1
name: my-agent
meta:
  version: 0.1.0
agent:
  build:
    context: .
    dockerfile: Dockerfile
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if s.Agent.Build == nil {
					t.Fatal("Agent.Build is nil")
				}
				if s.Agent.Build.Context != "." {
					t.Errorf("Agent.Build.Context = %q, want %q", s.Agent.Build.Context, ".")
				}
				if s.Agent.Build.Dockerfile != "Dockerfile" {
					t.Errorf("Agent.Build.Dockerfile = %q, want %q", s.Agent.Build.Dockerfile, "Dockerfile")
				}
			},
		},
		{
			name: "spec with cloud providers in models and tools",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
models:
  primary:
    provider: anthropic
  backup:
    provider: openai
integrations:
  github:
    provider: github
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if len(s.Models) != 2 {
					t.Fatalf("len(Models) = %d, want 2", len(s.Models))
				}
				primary, ok := s.Models["primary"]
				if !ok {
					t.Fatal("Models[primary] not found")
				}
				if primary.Provider != "anthropic" {
					t.Errorf("Models[primary].Provider = %q, want %q", primary.Provider, "anthropic")
				}
				backup, ok := s.Models["backup"]
				if !ok {
					t.Fatal("Models[backup] not found")
				}
				if backup.Provider != "openai" {
					t.Errorf("Models[backup].Provider = %q, want %q", backup.Provider, "openai")
				}
				gh, ok := s.Tools["github"]
				if !ok {
					t.Fatal("Tools[github] not found")
				}
				if gh.Provider != "github" {
					t.Errorf("Tools[github].Provider = %q, want %q", gh.Provider, "github")
				}
			},
		},
		{
			name: "spec with knowledge stores - provider mode",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
knowledge:
  cache:
    provider: redis
  docs:
    provider: qdrant
    persistent: true
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if len(s.Knowledge) != 2 {
					t.Fatalf("len(Knowledge) = %d, want 2", len(s.Knowledge))
				}
				cache, ok := s.Knowledge["cache"]
				if !ok {
					t.Fatal("Knowledge[cache] not found")
				}
				if cache.Provider != "redis" {
					t.Errorf("Knowledge[cache].Provider = %q, want %q", cache.Provider, "redis")
				}
				// Provider mode: container should be nil
				if cache.Container != nil {
					t.Error("Knowledge[cache].Container should be nil in provider mode")
				}
				// ResolvedContainer should fill in image from registry
				rc := cache.ResolvedContainer()
				if rc.Image != "redis:7-alpine" {
					t.Errorf("Knowledge[cache].ResolvedContainer().Image = %q, want %q", rc.Image, "redis:7-alpine")
				}
				docs, ok := s.Knowledge["docs"]
				if !ok {
					t.Fatal("Knowledge[docs] not found")
				}
				if !docs.Persistent {
					t.Error("Knowledge[docs].Persistent = false, want true")
				}
				dc := docs.ResolvedContainer()
				if !dc.Persistent {
					t.Error("Knowledge[docs].ResolvedContainer().Persistent = false, want true")
				}
			},
		},
		{
			name: "spec with knowledge stores - container mode",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
knowledge:
  custom_store:
    container:
      image: my-store:latest
      port: 5000
      environment:
        STORE_PASSWORD: secret
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				store, ok := s.Knowledge["custom_store"]
				if !ok {
					t.Fatal("Knowledge[custom_store] not found")
				}
				if store.Provider != "" {
					t.Errorf("Knowledge[custom_store].Provider = %q, want empty", store.Provider)
				}
				if store.Container == nil {
					t.Fatal("Knowledge[custom_store].Container is nil")
				}
				if store.Container.Image != "my-store:latest" {
					t.Errorf("Container.Image = %q, want %q", store.Container.Image, "my-store:latest")
				}
				rc := store.ResolvedContainer()
				if rc.Port != 5000 {
					t.Errorf("ResolvedContainer().Port = %d, want 5000", rc.Port)
				}
			},
		},
		{
			name: "spec with models - provider mode",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
models:
  local_llm:
    provider: ollama
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if len(s.Models) != 1 {
					t.Fatalf("len(Models) = %d, want 1", len(s.Models))
				}
				llm, ok := s.Models["local_llm"]
				if !ok {
					t.Fatal("Models[local_llm] not found")
				}
				if llm.Provider != "ollama" {
					t.Errorf("Models[local_llm].Provider = %q, want %q", llm.Provider, "ollama")
				}
				if llm.Container != nil {
					t.Error("Models[local_llm].Container should be nil in provider mode")
				}
				rc := llm.ResolvedContainer()
				if rc.Image != "ollama/ollama:latest" {
					t.Errorf("ResolvedContainer().Image = %q, want %q", rc.Image, "ollama/ollama:latest")
				}
				if rc.Port != 11434 {
					t.Errorf("ResolvedContainer().Port = %d, want 11434", rc.Port)
				}
			},
		},
		{
			name: "spec with models - container mode",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
models:
  embedder:
    container:
      image: my-model:latest
      port: 8000
      gpu:
        vram: 24Gi
        runtime: cuda
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				model, ok := s.Models["embedder"]
				if !ok {
					t.Fatal("Models[embedder] not found")
				}
				if model.Provider != "" {
					t.Errorf("Models[embedder].Provider = %q, want empty", model.Provider)
				}
				if model.Container == nil {
					t.Fatal("Models[embedder].Container is nil")
				}
				if model.Container.Image != "my-model:latest" {
					t.Errorf("Container.Image = %q, want %q", model.Container.Image, "my-model:latest")
				}
				rc := model.ResolvedContainer()
				if rc.Port != 8000 {
					t.Errorf("ResolvedContainer().Port = %d, want 8000", rc.Port)
				}
				if rc.GPU == nil {
					t.Fatal("ResolvedContainer().GPU is nil, want non-nil")
				}
				if rc.GPU.VRAM != "24Gi" {
					t.Errorf("ResolvedContainer().GPU.VRAM = %q, want %q", rc.GPU.VRAM, "24Gi")
				}
				if rc.GPU.Runtime != "cuda" {
					t.Errorf("ResolvedContainer().GPU.Runtime = %q, want %q", rc.GPU.Runtime, "cuda")
				}
			},
		},
		{
			name: "spec with ingestion",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
ingestion:
  docs-sync:
    container:
      image: my-ingest-worker:latest
      environment:
        SOURCE_REPO: owner/repo
    trigger:
      type: schedule
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if len(s.Ingestion) != 1 {
					t.Fatalf("len(Ingestion) = %d, want 1", len(s.Ingestion))
				}
				ing, ok := s.Ingestion["docs-sync"]
				if !ok {
					t.Fatal("Ingestion[docs-sync] not found")
				}
				if ing.Container.Image != "my-ingest-worker:latest" {
					t.Errorf("Ingestion[docs-sync].Container.Image = %q, want %q", ing.Container.Image, "my-ingest-worker:latest")
				}
				if ing.Container.Environment["SOURCE_REPO"] != "owner/repo" {
					t.Errorf("Ingestion[docs-sync].Container.Environment[SOURCE_REPO] = %q, want %q", ing.Container.Environment["SOURCE_REPO"], "owner/repo")
				}
				if ing.Trigger.Type != "schedule" {
					t.Errorf("Ingestion[docs-sync].Trigger.Type = %q, want %q", ing.Trigger.Type, "schedule")
				}
			},
		},
		{
			name: "spec with dev section",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
dev:
  interfaces:
    frontend:
      port: 3000
    messaging:
      adapters: [slack, web]
  schedules:
    docs-sync: "0 */4 * * *"
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if s.Dev == nil {
					t.Fatal("Dev is nil")
				}
				if s.Dev.Interfaces == nil {
					t.Fatal("Dev.Interfaces is nil")
				}
				if s.Dev.Interfaces.Frontend == nil {
					t.Fatal("Dev.Interfaces.Frontend is nil")
				}
				if s.Dev.Interfaces.Frontend.Port != 3000 {
					t.Errorf("Dev.Interfaces.Frontend.Port = %d, want 3000", s.Dev.Interfaces.Frontend.Port)
				}
				if s.Dev.Interfaces.Messaging == nil {
					t.Fatal("Dev.Interfaces.Messaging is nil")
				}
				if len(s.Dev.Interfaces.Messaging.Adapters) != 2 {
					t.Fatalf("len(Dev.Interfaces.Messaging.Adapters) = %d, want 2", len(s.Dev.Interfaces.Messaging.Adapters))
				}
				if s.Dev.Interfaces.Messaging.Adapters[0] != "slack" {
					t.Errorf("Dev.Interfaces.Messaging.Adapters[0] = %q, want %q", s.Dev.Interfaces.Messaging.Adapters[0], "slack")
				}
				if len(s.Dev.Schedules) != 1 {
					t.Fatalf("len(Dev.Schedules) = %d, want 1", len(s.Dev.Schedules))
				}
				if s.Dev.Schedules["docs-sync"] != "0 */4 * * *" {
					t.Errorf("Dev.Schedules[docs-sync] = %q, want %q", s.Dev.Schedules["docs-sync"], "0 */4 * * *")
				}
			},
		},
		{
			name: "spec with legacy dev interfaces (string array)",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
dev:
  interfaces: [slack, web]
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if s.Dev == nil {
					t.Fatal("Dev is nil")
				}
				if s.Dev.Interfaces == nil {
					t.Fatal("Dev.Interfaces is nil")
				}
				if s.Dev.Interfaces.Frontend != nil {
					t.Error("Dev.Interfaces.Frontend should be nil for legacy format")
				}
				if s.Dev.Interfaces.Messaging == nil {
					t.Fatal("Dev.Interfaces.Messaging is nil")
				}
				if len(s.Dev.Interfaces.Messaging.Adapters) != 2 {
					t.Fatalf("len(Dev.Interfaces.Messaging.Adapters) = %d, want 2", len(s.Dev.Interfaces.Messaging.Adapters))
				}
				if s.Dev.Interfaces.Messaging.Adapters[0] != "slack" {
					t.Errorf("Adapters[0] = %q, want %q", s.Dev.Interfaces.Messaging.Adapters[0], "slack")
				}
				if s.Dev.Interfaces.Messaging.Adapters[1] != "web" {
					t.Errorf("Adapters[1] = %q, want %q", s.Dev.Interfaces.Messaging.Adapters[1], "web")
				}
			},
		},
		{
			name: "spec with custom provider",
			yaml: `
spec: package/v1
name: test-agent
meta:
  description: test
agent:
  image: test:latest
providers:
  my-service:
    scope: [integrations]
    variables:
      - name: MY_SERVICE_API_KEY
        datatype: string
        secret: true
        description: API key for my-service
      - name: MY_SERVICE_SECRET
        datatype: string
        secret: true
        description: Shared secret
        optional: true
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if len(s.Providers) != 1 {
					t.Fatalf("len(Providers) = %d, want 1", len(s.Providers))
				}
				prov, ok := s.Providers["my-service"]
				if !ok {
					t.Fatal("Providers[my-service] not found")
				}
				if len(prov.Scope) != 1 || prov.Scope[0] != "integrations" {
					t.Errorf("Scope = %v, want [integrations]", prov.Scope)
				}
				if len(prov.Variables) != 2 {
					t.Fatalf("len(Variables) = %d, want 2", len(prov.Variables))
				}
				if prov.Variables[0].Name != "MY_SERVICE_API_KEY" {
					t.Errorf("Variables[0].Name = %q, want MY_SERVICE_API_KEY", prov.Variables[0].Name)
				}
				if prov.Variables[0].Description != "API key for my-service" {
					t.Errorf("Variables[0].Description = %q", prov.Variables[0].Description)
				}
				if !prov.Variables[1].Optional {
					t.Error("Variables[1].Optional = false, want true")
				}
			},
		},
		{
			name:    "invalid yaml",
			yaml:    `invalid: [yaml`,
			wantErr: true,
		},
		{
			name: "unquoted @ in name gives helpful error",
			yaml: `
spec: package/v1
name: @pirates/my-agent
agent:
  image: test:latest
`,
			wantErr: true,
			check: func(t *testing.T, _ *AstroSpec) {
				t.Error("should not reach check on error case")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := Parse([]byte(tt.yaml))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				// Check for helpful error on unquoted @ case
				if tt.name == "unquoted @ in name gives helpful error" {
					if !strings.Contains(err.Error(), "must be quoted") {
						t.Errorf("expected helpful error about quoting @, got: %v", err)
					}
				}
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, spec)
			}
		})
	}
}

func TestContainer_HasMessaging(t *testing.T) {
	tests := []struct {
		name       string
		interfaces *Interfaces
		want       bool
	}{
		{"nil interfaces (backward compat)", nil, true},
		{"messaging true", &Interfaces{Messaging: true}, true},
		{"messaging false", &Interfaces{Messaging: false}, false},
		{"frontend only", &Interfaces{Frontend: true}, false},
		{"both enabled", &Interfaces{Frontend: true, Messaging: true}, true},
		{"empty interfaces", &Interfaces{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Container{Interfaces: tt.interfaces}
			if got := c.HasMessaging(); got != tt.want {
				t.Errorf("HasMessaging() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainer_HasFrontend(t *testing.T) {
	tests := []struct {
		name       string
		interfaces *Interfaces
		want       bool
	}{
		{"nil interfaces", nil, false},
		{"frontend true", &Interfaces{Frontend: true}, true},
		{"frontend false", &Interfaces{Frontend: false}, false},
		{"messaging only", &Interfaces{Messaging: true}, false},
		{"both enabled", &Interfaces{Frontend: true, Messaging: true}, true},
		{"empty interfaces", &Interfaces{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Container{Interfaces: tt.interfaces}
			if got := c.HasFrontend(); got != tt.want {
				t.Errorf("HasFrontend() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSpec_Validation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing spec version",
			yaml: `
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
`,
			wantErr: "spec version is required",
		},
		{
			name: "missing agent name",
			yaml: `
spec: package/v1
meta:
  version: 1.0.0
agent:
  image: test:latest
`,
			wantErr: "agent name is required",
		},
		{
			name: "missing container config",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
`,
			wantErr: "agent.build or agent.image is required",
		},
		{
			name: "valid spec passes validation",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
`,
			wantErr: "",
		},
		{
			name: "knowledge with both provider and container",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
knowledge:
  docs:
    provider: qdrant
    container:
      image: qdrant/qdrant:latest
`,
			wantErr: "provider and container are mutually exclusive",
		},
		{
			name: "knowledge with neither provider nor container",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
knowledge:
  docs: {}
`,
			wantErr: "either provider or container is required",
		},
		{
			name: "model with both provider and container",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
models:
  llm:
    provider: ollama
    container:
      image: ollama/ollama:latest
`,
			wantErr: "provider and container are mutually exclusive",
		},
		{
			name: "model with neither provider nor container",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
models:
  llm: {}
`,
			wantErr: "either provider or container is required",
		},
		{
			name: "valid model with provider",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
models:
  llm:
    provider: ollama
`,
			wantErr: "",
		},
		{
			name: "tool with both provider and container",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  gh:
    provider: github
    container:
      image: tool:latest
`,
			wantErr: "provider and container are mutually exclusive",
		},
		{
			name: "tool with neither provider nor container",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  empty: {}
`,
			wantErr: "either provider or container is required",
		},
		{
			name: "custom provider without variables rejected",
			yaml: `
spec: package/v1
name: test-agent
meta:
  description: test
agent:
  image: test:latest
providers:
  my-service:
    scope: [integrations]
    variables: []
`,
			wantErr: "variables is required and must contain at least one entry",
		},
		{
			name: "valid cloud model provider",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
models:
  anthropic:
    provider: anthropic
`,
			wantErr: "",
		},
		{
			name: "valid cloud tool provider",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  github:
    provider: github
`,
			wantErr: "",
		},
		// Fix 1: agent.image and agent.build are mutually exclusive
		{
			name: "agent with both image and build rejected",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
  build:
    context: .
    dockerfile: Dockerfile
`,
			wantErr: "image and build are mutually exclusive",
		},
		// Fix 2: integrations container.build must have context and dockerfile
		{
			name: "tool container build missing context",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  mytool:
    container:
      build:
        dockerfile: Dockerfile
`,
			wantErr: "integrations.mytool.container.build.context is required",
		},
		{
			name: "tool container build missing dockerfile",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  mytool:
    container:
      build:
        context: .
`,
			wantErr: "integrations.mytool.container.build.dockerfile is required",
		},
		{
			name: "tool container build valid",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  mytool:
    container:
      build:
        context: .
        dockerfile: Dockerfile
`,
			wantErr: "",
		},
		// Fix 3: gpu.runtime must be cuda or rocm — integrations and knowledge
		{
			name: "tool container invalid gpu runtime",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  mytool:
    container:
      image: tool:latest
      gpu:
        runtime: metal
`,
			wantErr: "integrations.mytool.container.gpu.runtime: must be one of cuda or rocm",
		},
		{
			name: "tool container valid gpu runtime rocm",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  mytool:
    container:
      image: tool:latest
      gpu:
        runtime: rocm
`,
			wantErr: "",
		},
		{
			name: "knowledge container invalid gpu runtime",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
knowledge:
  store:
    container:
      image: store:latest
      gpu:
        runtime: directx
`,
			wantErr: "knowledge.store.container.gpu.runtime: must be one of cuda or rocm",
		},
		{
			name: "knowledge container valid gpu runtime cuda",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
knowledge:
  store:
    container:
      image: store:latest
      gpu:
        runtime: cuda
`,
			wantErr: "",
		},
		{
			name: "model with both models and model rejected",
			yaml: `
spec: package/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
models:
  llm:
    provider: ollama
    model: llama3.2
    models:
      - llama3.2
      - mistral
`,
			wantErr: "models and model are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write yaml to temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "astropods.yml")
			if err := os.WriteFile(tmpFile, []byte(tt.yaml), 0644); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}

			_, err := ParseSpec(tmpFile)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ParseSpec() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("ParseSpec() expected error containing %q, got nil", tt.wantErr)
				} else if !contains(err.Error(), tt.wantErr) {
					t.Errorf("ParseSpec() error = %q, want error containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestParseFile(t *testing.T) {
	yaml := `
spec: package/v1
name: file-test
meta:
  version: 1.0.0
agent:
  image: test:latest
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(tmpFile, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	spec, err := ParseFile(tmpFile)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	if spec.Name != "file-test" {
		t.Errorf("Name = %q, want %q", spec.Name, "file-test")
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/astropods.yml")
	if err == nil {
		t.Error("ParseFile() expected error for nonexistent file, got nil")
	}
}

func TestParseString(t *testing.T) {
	yaml := `
spec: package/v1
name: string-test
meta:
  version: 1.0.0
agent:
  image: test:latest
`
	spec, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	if spec.Name != "string-test" {
		t.Errorf("Name = %q, want %q", spec.Name, "string-test")
	}
}

func TestSlackConfig(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }

	tests := []struct {
		name string
		dev  *Dev
		want *SlackAdapterConfig
	}{
		{"nil dev", nil, nil},
		{"no interfaces", &Dev{}, nil},
		{"no messaging", &Dev{Interfaces: &DevInterfaces{}}, nil},
		{"no slack config", &Dev{Interfaces: &DevInterfaces{Messaging: &DevMessaging{Adapters: []string{"slack"}}}}, nil},
		{"empty reactions", &Dev{Interfaces: &DevInterfaces{Messaging: &DevMessaging{
			Slack: &SlackAdapterConfig{ActionableReactions: []string{}},
		}}}, &SlackAdapterConfig{ActionableReactions: []string{}}},
		{"configured reactions", &Dev{Interfaces: &DevInterfaces{Messaging: &DevMessaging{
			Slack: &SlackAdapterConfig{ActionableReactions: []string{"ticket", "bug"}},
		}}}, &SlackAdapterConfig{ActionableReactions: []string{"ticket", "bug"}}},
		{"full config", &Dev{Interfaces: &DevInterfaces{Messaging: &DevMessaging{
			Slack: &SlackAdapterConfig{
				ActionableReactions: []string{"ticket"},
				AllowedChannelIDs:   []string{"C123", "C999"},
				AllowedUserIDs:      []string{"U123", "U999"},
				SocketMode:          boolPtr(false),
				AutoThread:          boolPtr(true),
			},
		}}}, &SlackAdapterConfig{
			ActionableReactions: []string{"ticket"},
			AllowedChannelIDs:   []string{"C123", "C999"},
			AllowedUserIDs:      []string{"U123", "U999"},
			SocketMode:          boolPtr(false),
			AutoThread:          boolPtr(true),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dev.SlackConfig()
			if tt.want == nil {
				if got != nil {
					t.Errorf("SlackConfig() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("SlackConfig() = nil, want non-nil")
			}
			if len(got.ActionableReactions) != len(tt.want.ActionableReactions) {
				t.Fatalf("ActionableReactions len = %d, want %d", len(got.ActionableReactions), len(tt.want.ActionableReactions))
			}
			for i := range tt.want.ActionableReactions {
				if got.ActionableReactions[i] != tt.want.ActionableReactions[i] {
					t.Errorf("ActionableReactions[%d] = %q, want %q", i, got.ActionableReactions[i], tt.want.ActionableReactions[i])
				}
			}
			if len(got.AllowedChannelIDs) != len(tt.want.AllowedChannelIDs) {
				t.Fatalf("AllowedChannelIDs len = %d, want %d", len(got.AllowedChannelIDs), len(tt.want.AllowedChannelIDs))
			}
			for i := range tt.want.AllowedChannelIDs {
				if got.AllowedChannelIDs[i] != tt.want.AllowedChannelIDs[i] {
					t.Errorf("AllowedChannelIDs[%d] = %q, want %q", i, got.AllowedChannelIDs[i], tt.want.AllowedChannelIDs[i])
				}
			}
			if len(got.AllowedUserIDs) != len(tt.want.AllowedUserIDs) {
				t.Fatalf("AllowedUserIDs len = %d, want %d", len(got.AllowedUserIDs), len(tt.want.AllowedUserIDs))
			}
			for i := range tt.want.AllowedUserIDs {
				if got.AllowedUserIDs[i] != tt.want.AllowedUserIDs[i] {
					t.Errorf("AllowedUserIDs[%d] = %q, want %q", i, got.AllowedUserIDs[i], tt.want.AllowedUserIDs[i])
				}
			}
			if tt.want.SocketMode != nil {
				if got.SocketMode == nil || *got.SocketMode != *tt.want.SocketMode {
					t.Errorf("SocketMode = %v, want %v", got.SocketMode, *tt.want.SocketMode)
				}
			}
			if tt.want.AutoThread != nil {
				if got.AutoThread == nil || *got.AutoThread != *tt.want.AutoThread {
					t.Errorf("AutoThread = %v, want %v", got.AutoThread, *tt.want.AutoThread)
				}
			}
		})
	}
}

func TestParseSpec_StructuredSlackConfig(t *testing.T) {
	yaml := `
spec: package/v1
name: test-agent
agent:
  image: test:latest
dev:
  interfaces:
    messaging:
      adapters: [slack, web]
      slack:
        actionable_reactions: [ticket, bug]
        allowed_channel_ids: [C123, C999]
        allowed_user_ids: [U123, U999]
        socket_mode: false
        auto_thread: true
`
	spec, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	cfg := spec.Dev.SlackConfig()
	if cfg == nil {
		t.Fatal("SlackConfig() = nil, want non-nil")
	}
	if len(cfg.ActionableReactions) != 2 || cfg.ActionableReactions[0] != "ticket" || cfg.ActionableReactions[1] != "bug" {
		t.Errorf("ActionableReactions = %v, want [ticket bug]", cfg.ActionableReactions)
	}
	if len(cfg.AllowedChannelIDs) != 2 || cfg.AllowedChannelIDs[0] != "C123" || cfg.AllowedChannelIDs[1] != "C999" {
		t.Errorf("AllowedChannelIDs = %v, want [C123 C999]", cfg.AllowedChannelIDs)
	}
	if len(cfg.AllowedUserIDs) != 2 || cfg.AllowedUserIDs[0] != "U123" || cfg.AllowedUserIDs[1] != "U999" {
		t.Errorf("AllowedUserIDs = %v, want [U123 U999]", cfg.AllowedUserIDs)
	}
	if cfg.SocketMode == nil || *cfg.SocketMode != false {
		t.Errorf("SocketMode = %v, want false", cfg.SocketMode)
	}
	if cfg.AutoThread == nil || *cfg.AutoThread != true {
		t.Errorf("AutoThread = %v, want true", cfg.AutoThread)
	}
	adapters := spec.Dev.MessagingAdapters()
	if len(adapters) != 2 || adapters[0] != "slack" || adapters[1] != "web" {
		t.Errorf("MessagingAdapters() = %v, want [slack web]", adapters)
	}
}

func TestParseSpec_LegacyInterfacesNoSlackConfig(t *testing.T) {
	yaml := `
spec: package/v1
name: test-agent
agent:
  image: test:latest
dev:
  interfaces: [slack, web]
`
	spec, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	if spec.Dev.SlackConfig() != nil {
		t.Error("legacy format should have nil SlackConfig")
	}
	adapters := spec.Dev.MessagingAdapters()
	if len(adapters) != 2 {
		t.Fatalf("MessagingAdapters() len = %d, want 2", len(adapters))
	}
}

func TestParseSpec_StructuredNoSlackBlock(t *testing.T) {
	yaml := `
spec: package/v1
name: test-agent
agent:
  image: test:latest
dev:
  interfaces:
    messaging:
      adapters: [slack]
`
	spec, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	if spec.Dev.SlackConfig() != nil {
		t.Error("structured format without slack block should have nil SlackConfig")
	}
}

func TestParseSpec_StructuredSlackConfigDefaults(t *testing.T) {
	yaml := `
spec: package/v1
name: test-agent
agent:
  image: test:latest
dev:
  interfaces:
    messaging:
      adapters: [slack]
      slack:
        actionable_reactions: [ticket]
`
	spec, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	cfg := spec.Dev.SlackConfig()
	if cfg == nil {
		t.Fatal("SlackConfig() = nil, want non-nil")
	}
	if len(cfg.ActionableReactions) != 1 || cfg.ActionableReactions[0] != "ticket" {
		t.Errorf("ActionableReactions = %v, want [ticket]", cfg.ActionableReactions)
	}
	if cfg.SocketMode != nil {
		t.Errorf("SocketMode should be nil (not specified), got %v", *cfg.SocketMode)
	}
	if cfg.AutoThread != nil {
		t.Errorf("AutoThread should be nil (not specified), got %v", *cfg.AutoThread)
	}
}

func TestSecretDefaultViolations(t *testing.T) {
	s := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Inputs: map[string]Input{
			"api_key":   {Name: "API_KEY", Secret: true, Default: "sk-secret"},
			"log_level": {Name: "LOG_LEVEL", Default: "debug"},
		},
		Providers: map[string]CustomProvider{
			"jira": {Scope: []string{"integrations"}, Variables: []Input{
				{Name: "JIRA_TOKEN", Secret: true, Default: "jira-secret"},
				{Name: "JIRA_URL", Default: "https://jira.example.com"},
			}},
		},
	}

	violations := SecretDefaultViolations(s)
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations, got %d: %v", len(violations), violations)
	}

	// Clean spec should have zero violations
	clean := &AstroSpec{
		Name:  "agent",
		Agent: Container{Image: "a:1"},
		Inputs: map[string]Input{
			"api_key": {Name: "API_KEY", Secret: true}, // no default
		},
	}
	if v := SecretDefaultViolations(clean); len(v) != 0 {
		t.Errorf("expected 0 violations for clean spec, got %d: %v", len(v), v)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
