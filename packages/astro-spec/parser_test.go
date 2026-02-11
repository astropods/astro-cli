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
agent: test-agent
meta:
  version: 1.0.0
container:
  image: test:latest
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if s.Spec != "astro/v1" {
					t.Errorf("Spec = %q, want %q", s.Spec, "astro/v1")
				}
				if s.Agent != "test-agent" {
					t.Errorf("Agent = %q, want %q", s.Agent, "test-agent")
				}
				if s.Meta.Version != "1.0.0" {
					t.Errorf("Meta.Version = %q, want %q", s.Meta.Version, "1.0.0")
				}
				if s.Container.Image != "test:latest" {
					t.Errorf("Container.Image = %q, want %q", s.Container.Image, "test:latest")
				}
			},
		},
		{
			name: "spec with build config",
			yaml: `
spec: astro/v1
agent: my-agent
meta:
  version: 0.1.0
container:
  build:
    context: .
    dockerfile: Dockerfile
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if s.Container.Build == nil {
					t.Fatal("Container.Build is nil")
				}
				if s.Container.Build.Context != "." {
					t.Errorf("Container.Build.Context = %q, want %q", s.Container.Build.Context, ".")
				}
				if s.Container.Build.Dockerfile != "Dockerfile" {
					t.Errorf("Container.Build.Dockerfile = %q, want %q", s.Container.Build.Dockerfile, "Dockerfile")
				}
			},
		},
		{
			name: "spec with integrations array format",
			yaml: `
spec: astro/v1
agent: test-agent
meta:
  version: 1.0.0
container:
  image: test:latest
integrations:
  models:
    - name: primary
      provider: anthropic
    - name: backup
      provider: openai
      env:
        prefix: BACKUP_
  tools:
    - name: github
      provider: github
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if len(s.Integrations.Models) != 2 {
					t.Fatalf("len(Integrations.Models) = %d, want 2", len(s.Integrations.Models))
				}
				if s.Integrations.Models[0].Name != "primary" {
					t.Errorf("Integrations.Models[0].Name = %q, want %q", s.Integrations.Models[0].Name, "primary")
				}
				if s.Integrations.Models[0].Provider != "anthropic" {
					t.Errorf("Integrations.Models[0].Provider = %q, want %q", s.Integrations.Models[0].Provider, "anthropic")
				}
				if s.Integrations.Models[1].Name != "backup" {
					t.Errorf("Integrations.Models[1].Name = %q, want %q", s.Integrations.Models[1].Name, "backup")
				}
				if s.Integrations.Models[1].Env == nil {
					t.Fatal("Integrations.Models[1].Env is nil")
				}
				if s.Integrations.Models[1].Env.Prefix != "BACKUP_" {
					t.Errorf("Integrations.Models[1].Env.Prefix = %q, want %q", s.Integrations.Models[1].Env.Prefix, "BACKUP_")
				}
				if len(s.Integrations.Tools) != 1 {
					t.Fatalf("len(Integrations.Tools) = %d, want 1", len(s.Integrations.Tools))
				}
				if s.Integrations.Tools[0].Name != "github" {
					t.Errorf("Integrations.Tools[0].Name = %q, want %q", s.Integrations.Tools[0].Name, "github")
				}
			},
		},
		{
			name: "spec with knowledge stores",
			yaml: `
spec: astro/v1
agent: test-agent
meta:
  version: 1.0.0
container:
  image: test:latest
knowledge:
  cache:
    type: kv
    provider: redis
    container:
      image: redis:7-alpine
      environment:
        REDIS_PASSWORD: secret
  docs:
    type: vector
    provider: qdrant
    container:
      image: qdrant/qdrant:latest
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
				if cache.Type != "kv" {
					t.Errorf("Knowledge[cache].Type = %q, want %q", cache.Type, "kv")
				}
				if cache.Provider != "redis" {
					t.Errorf("Knowledge[cache].Provider = %q, want %q", cache.Provider, "redis")
				}
				if cache.Container.Environment["REDIS_PASSWORD"] != "secret" {
					t.Errorf("Knowledge[cache].Container.Environment[REDIS_PASSWORD] = %q, want %q", cache.Container.Environment["REDIS_PASSWORD"], "secret")
				}
				docs, ok := s.Knowledge["docs"]
				if !ok {
					t.Fatal("Knowledge[docs] not found")
				}
				if !docs.Container.Persistent {
					t.Error("Knowledge[docs].Container.Persistent = false, want true")
				}
			},
		},
		{
			name: "spec with interfaces",
			yaml: `
spec: astro/v1
agent: test-agent
meta:
  version: 1.0.0
container:
  image: test:latest
interfaces:
  api:
    type: http
  messaging:
    type: slack
`,
			wantErr: false,
			check: func(t *testing.T, s *AstroSpec) {
				if len(s.Interfaces) != 2 {
					t.Fatalf("len(Interfaces) = %d, want 2", len(s.Interfaces))
				}
				api, ok := s.Interfaces["api"]
				if !ok {
					t.Fatal("Interfaces[api] not found")
				}
				if api.Type != "http" {
					t.Errorf("Interfaces[api].Type = %q, want %q", api.Type, "http")
				}
				messaging, ok := s.Interfaces["messaging"]
				if !ok {
					t.Fatal("Interfaces[messaging] not found")
				}
				if messaging.Type != "slack" {
					t.Errorf("Interfaces[messaging].Type = %q, want %q", messaging.Type, "slack")
				}
			},
		},
		{
			name: "spec with ingestion",
			yaml: `
spec: astro/v1
agent: test-agent
meta:
  version: 1.0.0
container:
  image: test:latest
ingestion:
  docs-sync:
    container:
      image: my-ingest-worker:latest
      environment:
        SOURCE_REPO: owner/repo
    trigger:
      type: schedule
      schedule: "0 0 * * *"
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
				if ing.Trigger.Schedule != "0 0 * * *" {
					t.Errorf("Ingestion[docs-sync].Trigger.Schedule = %q, want %q", ing.Trigger.Schedule, "0 0 * * *")
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
agent: test-agent
meta:
  version: 1.0.0
container:
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
container:
  image: test:latest
`,
			wantErr: "agent name is required",
		},
		{
			name: "missing container config",
			yaml: `
spec: astro/v1
agent: test-agent
meta:
  version: 1.0.0
`,
			wantErr: "container.build or container.image is required",
		},
		{
			name: "valid spec passes validation",
			yaml: `
spec: astro/v1
agent: test-agent
meta:
  version: 1.0.0
container:
  image: test:latest
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
agent: file-test
meta:
  version: 1.0.0
container:
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
	if spec.Agent != "file-test" {
		t.Errorf("Agent = %q, want %q", spec.Agent, "file-test")
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
agent: string-test
meta:
  version: 1.0.0
container:
  image: test:latest
`
	spec, err := ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	if spec.Agent != "string-test" {
		t.Errorf("Agent = %q, want %q", spec.Agent, "string-test")
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
