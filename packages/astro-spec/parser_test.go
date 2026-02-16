package spec

import (
	"os"
	"path/filepath"
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
spec: astro/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if s.Spec != "astro/v1" {
					t.Errorf("Spec = %q, want %q", s.Spec, "astro/v1")
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
spec: astro/v1
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
			name: "spec with integrations flat map",
			yaml: `
spec: astro/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  primary:
    provider: anthropic
    type: model
  backup:
    provider: openai
    type: model
  github:
    provider: github
    type: tool
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if len(s.Integrations) != 3 {
					t.Fatalf("len(Integrations) = %d, want 3", len(s.Integrations))
				}
				primary, ok := s.Integrations["primary"]
				if !ok {
					t.Fatal("Integrations[primary] not found")
				}
				if primary.Provider != "anthropic" {
					t.Errorf("Integrations[primary].Provider = %q, want %q", primary.Provider, "anthropic")
				}
				if primary.Type != "model" {
					t.Errorf("Integrations[primary].Type = %q, want %q", primary.Type, "model")
				}
				backup, ok := s.Integrations["backup"]
				if !ok {
					t.Fatal("Integrations[backup] not found")
				}
				if backup.Provider != "openai" {
					t.Errorf("Integrations[backup].Provider = %q, want %q", backup.Provider, "openai")
				}
				gh, ok := s.Integrations["github"]
				if !ok {
					t.Fatal("Integrations[github] not found")
				}
				if gh.Provider != "github" {
					t.Errorf("Integrations[github].Provider = %q, want %q", gh.Provider, "github")
				}
			},
		},
		{
			name: "spec with knowledge stores - provider mode",
			yaml: `
spec: astro/v1
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
spec: astro/v1
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
spec: astro/v1
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
spec: astro/v1
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
spec: astro/v1
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
spec: astro/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
dev:
  interfaces: [slack, web]
  schedules:
    docs-sync: "0 */4 * * *"
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if s.Dev == nil {
					t.Fatal("Dev is nil")
				}
				if len(s.Dev.Interfaces) != 2 {
					t.Fatalf("len(Dev.Interfaces) = %d, want 2", len(s.Dev.Interfaces))
				}
				if s.Dev.Interfaces[0] != "slack" {
					t.Errorf("Dev.Interfaces[0] = %q, want %q", s.Dev.Interfaces[0], "slack")
				}
				if s.Dev.Interfaces[1] != "web" {
					t.Errorf("Dev.Interfaces[1] = %q, want %q", s.Dev.Interfaces[1], "web")
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
			name: "spec with custom integration provider",
			yaml: `
spec: astro/v1
name: test-agent
meta:
  version: 1.0.0
agent:
  image: test:latest
integrations:
  my-service:
    provider: custom
    type: tool
    credentials:
      - suffix: API_KEY
        description: API key for my-service
      - suffix: SECRET
        description: Shared secret
        optional: true
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if len(s.Integrations) != 1 {
					t.Fatalf("len(Integrations) = %d, want 1", len(s.Integrations))
				}
				svc, ok := s.Integrations["my-service"]
				if !ok {
					t.Fatal("Integrations[my-service] not found")
				}
				if svc.Provider != "custom" {
					t.Errorf("Provider = %q, want %q", svc.Provider, "custom")
				}
				if svc.Type != "tool" {
					t.Errorf("Type = %q, want %q", svc.Type, "tool")
				}
				if len(svc.Credentials) != 2 {
					t.Fatalf("len(Credentials) = %d, want 2", len(svc.Credentials))
				}
				if svc.Credentials[0].Suffix != "API_KEY" {
					t.Errorf("Credentials[0].Suffix = %q, want %q", svc.Credentials[0].Suffix, "API_KEY")
				}
				if svc.Credentials[0].Description != "API key for my-service" {
					t.Errorf("Credentials[0].Description = %q, want %q", svc.Credentials[0].Description, "API key for my-service")
				}
				if svc.Credentials[1].Suffix != "SECRET" {
					t.Errorf("Credentials[1].Suffix = %q, want %q", svc.Credentials[1].Suffix, "SECRET")
				}
				if !svc.Credentials[1].Optional {
					t.Error("Credentials[1].Optional = false, want true")
				}
			},
		},
		{
			name:    "invalid yaml",
			yaml:    `invalid: [yaml`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := Parse([]byte(tt.yaml))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, spec)
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
spec: astro/v1
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
spec: astro/v1
name: test-agent
meta:
  version: 1.0.0
`,
			wantErr: "agent.build or agent.image is required",
		},
		{
			name: "valid spec passes validation",
			yaml: `
spec: astro/v1
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
spec: astro/v1
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
spec: astro/v1
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
spec: astro/v1
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
spec: astro/v1
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
spec: astro/v1
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write yaml to temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "astro.yml")
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
spec: astro/v1
name: file-test
meta:
  version: 1.0.0
agent:
  image: test:latest
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "astro.yml")
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
	_, err := ParseFile("/nonexistent/path/astro.yml")
	if err == nil {
		t.Error("ParseFile() expected error for nonexistent file, got nil")
	}
}

func TestParseString(t *testing.T) {
	yaml := `
spec: astro/v1
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
