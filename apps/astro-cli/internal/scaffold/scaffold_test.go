package scaffold

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// MemFs is an in-memory Fs implementation for tests.
type MemFs struct {
	Files map[string][]byte
	Dirs  map[string]struct{}
}

func newMemFs() *MemFs {
	return &MemFs{Files: map[string][]byte{}, Dirs: map[string]struct{}{}}
}

func (m *MemFs) MkdirAll(path string, _ fs.FileMode) error {
	m.Dirs[path] = struct{}{}
	return nil
}

func (m *MemFs) WriteFile(path string, data []byte, _ fs.FileMode) error {
	m.Files[path] = data
	return nil
}

func (m *MemFs) HasFile(path string) bool {
	_, ok := m.Files[path]
	return ok
}

func (m *MemFs) HasDir(path string) bool {
	_, ok := m.Dirs[path]
	return ok
}

// renderAstroYml uses the same rendering path as the CLI (RenderTemplate) so tests assert real behavior.
func renderAstroYml(t *testing.T, config ScaffoldConfig) string {
	t.Helper()
	paths, err := GetTemplatePaths("mastra")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}
	yaml, err := RenderTemplate(paths.AstroYml, config)
	if err != nil {
		t.Fatalf("RenderTemplate: %v", err)
	}
	return yaml
}

func TestAstroYml_GitHubUnderTools(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web"},
		Integrations:    []string{"github"},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestions:      []string{},
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if _, ok := s.Integrations["github"]; !ok {
		t.Errorf("expected github under tools, got tools=%v", s.Integrations)
	}
	if len(s.Providers) != 0 {
		t.Errorf("expected no integrations, got %v", s.Providers)
	}
}

func TestAstroYml_KnowledgeMapping(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web"},
		Integrations:    []string{},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{"qdrant", "redis", "neo4j"},
		Ingestions:      []string{},
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if _, ok := s.Knowledge["docs"]; !ok {
		t.Errorf("expected qdrant mapped to 'docs', got knowledge=%v", s.Knowledge)
	}
	if _, ok := s.Knowledge["cache"]; !ok {
		t.Errorf("expected redis mapped to 'cache', got knowledge=%v", s.Knowledge)
	}
	if _, ok := s.Knowledge["graph"]; !ok {
		t.Errorf("expected neo4j mapped to 'graph', got knowledge=%v", s.Knowledge)
	}
}

func TestAstroYml_FullInfrastructure(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web", "slack"},
		ModelProvider:   "ollama",
		Model:           "mistral",
		Integrations:    []string{"anthropic", "openai", "github"},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{"qdrant", "redis"},
		Ingestions:      []string{"schedule"},
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	// Models: ollama + anthropic + openai
	if len(s.Models) != 3 {
		t.Errorf("expected 3 models, got %d: %v", len(s.Models), s.Models)
	}
	if s.Models["ollama"].Provider != "ollama" {
		t.Errorf("expected ollama model provider=ollama, got %q", s.Models["ollama"].Provider)
	}
	if s.Models["anthropic"].Provider != "anthropic" {
		t.Errorf("expected anthropic model provider=anthropic, got %q", s.Models["anthropic"].Provider)
	}
	if s.Models["openai"].Provider != "openai" {
		t.Errorf("expected openai model provider=openai, got %q", s.Models["openai"].Provider)
	}

	// Tools: github
	if _, ok := s.Integrations["github"]; !ok {
		t.Errorf("expected github under tools, got %v", s.Integrations)
	}

	// Knowledge: qdrant + redis
	if len(s.Knowledge) != 2 {
		t.Errorf("expected 2 knowledge stores, got %d: %v", len(s.Knowledge), s.Knowledge)
	}

	// Integrations: none (all mapped to models/tools)
	if len(s.Providers) != 0 {
		t.Errorf("expected no integrations, got %v", s.Providers)
	}

	// Ingestion
	if len(s.Ingestion) != 1 {
		t.Errorf("expected 1 ingestion, got %d", len(s.Ingestion))
	}
	if _, ok := s.Ingestion["schedule"]; !ok {
		t.Errorf("expected ingestion key 'schedule', got %v", s.Ingestion)
	}
}

func TestAstroYml_MinimalConfig(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "bare-agent",
		Description:     "minimal",
		Interfaces:      []string{"web"},
		Integrations:    []string{},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestions:      []string{},
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if s.Name != "bare-agent" {
		t.Errorf("expected name=bare-agent, got %q", s.Name)
	}
	if len(s.Models) != 0 {
		t.Errorf("expected no models, got %v", s.Models)
	}
	if len(s.Knowledge) != 0 {
		t.Errorf("expected no knowledge, got %v", s.Knowledge)
	}
	if len(s.Integrations) != 0 {
		t.Errorf("expected no tools, got %v", s.Integrations)
	}
	if len(s.Providers) != 0 {
		t.Errorf("expected no integrations, got %v", s.Providers)
	}
}

// TestAstroYml_ModelDeclarationPerModelChoice ensures the spec includes the correct
// models block for each model selection (ollama+name, anthropic, openai, combined).
func TestAstroYml_ModelDeclarationPerModelChoice(t *testing.T) {
	tests := []struct {
		name       string
		config     ScaffoldConfig
		wantModels []string // expected keys under models: (e.g. "ollama", "anthropic")
		wantModel  string   // if non-empty, expected model name on the ModelProvider key
	}{
		{
			name: "ollama with model name",
			config: ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				ModelProvider: "ollama", Model: "llama3",
				Integrations: nil, IntegrationKeys: map[string]string{}, Knowledge: nil, Ingestions: []string{},
			},
			wantModels: []string{"ollama"},
			wantModel:  "llama3",
		},
		{
			name: "ollama without model name",
			config: ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				ModelProvider: "ollama", Model: "",
				Integrations: nil, IntegrationKeys: map[string]string{}, Knowledge: nil, Ingestions: []string{},
			},
			wantModels: []string{"ollama"},
		},
		{
			name: "anthropic only",
			config: ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				ModelProvider: "", Model: "",
				Integrations: []string{"anthropic"}, IntegrationKeys: map[string]string{}, Knowledge: nil, Ingestions: []string{},
			},
			wantModels: []string{"anthropic"},
		},
		{
			name: "openai only",
			config: ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				ModelProvider: "", Model: "",
				Integrations: []string{"openai"}, IntegrationKeys: map[string]string{}, Knowledge: nil, Ingestions: []string{},
			},
			wantModels: []string{"openai"},
		},
		{
			name: "ollama and anthropic",
			config: ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				ModelProvider: "ollama", Model: "mistral",
				Integrations: []string{"anthropic"}, IntegrationKeys: map[string]string{}, Knowledge: nil, Ingestions: []string{},
			},
			wantModels: []string{"ollama", "anthropic"},
			wantModel:  "mistral",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := renderAstroYml(t, tt.config)
			if !strings.Contains(yaml, "models:") {
				t.Errorf("generated spec missing 'models:' section:\n%s", yaml)
			}
			s, err := spec.ParseString(yaml)
			if err != nil {
				t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
			}
			for _, key := range tt.wantModels {
				if _, ok := s.Models[key]; !ok {
					t.Errorf("expected models to contain %q, got %v", key, s.Models)
				}
			}
			if len(s.Models) != len(tt.wantModels) {
				t.Errorf("expected %d model(s), got %d: %v", len(tt.wantModels), len(s.Models), s.Models)
			}
			if tt.config.ModelProvider != "" {
				m, ok := s.Models[tt.config.ModelProvider]
				if !ok {
					t.Errorf("expected model key %q, got %v", tt.config.ModelProvider, s.Models)
				} else if m.Provider != tt.config.ModelProvider {
					t.Errorf("model %q provider = %q, want %q", tt.config.ModelProvider, m.Provider, tt.config.ModelProvider)
				}
				if tt.wantModel != "" {
					resolved := m.ResolvedModels()
					if len(resolved) == 0 || resolved[0] != tt.wantModel {
						t.Errorf("model %q models = %v, want %q", tt.config.ModelProvider, resolved, tt.wantModel)
					}
				}
			}
		})
	}
}

// TestAstroYml_ModelKeyMatchesProvider ensures every model key in the generated
// spec matches its provider name — no model should be keyed as "default".
func TestAstroYml_ModelKeyMatchesProvider(t *testing.T) {
	configs := []ScaffoldConfig{
		{
			Name: "a", Description: "d", Interfaces: []string{"web"},
			ModelProvider: "ollama", Model: "llama3",
			Integrations: []string{"anthropic", "openai"}, IntegrationKeys: map[string]string{},
			Knowledge: nil, Ingestions: []string{},
		},
		{
			Name: "a", Description: "d", Interfaces: []string{"web"},
			ModelProvider: "ollama", Model: "",
			Integrations: nil, IntegrationKeys: map[string]string{},
			Knowledge: nil, Ingestions: []string{},
		},
		{
			Name: "a", Description: "d", Interfaces: []string{"web"},
			Integrations: []string{"anthropic"}, IntegrationKeys: map[string]string{},
			Knowledge: nil, Ingestions: []string{},
		},
	}
	for _, cfg := range configs {
		yaml := renderAstroYml(t, cfg)
		s, err := spec.ParseString(yaml)
		if err != nil {
			t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
		}
		for key, m := range s.Models {
			if key != m.Provider {
				t.Errorf("model key %q does not match provider %q", key, m.Provider)
			}
		}
	}
}

func TestAstroYml_MultipleIngestions(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name:            "test-agent",
		Description:     "test",
		Interfaces:      []string{"web"},
		Integrations:    []string{},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestions:      []string{"schedule", "webhook"},
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if len(s.Ingestion) != 2 {
		t.Errorf("expected 2 ingestion jobs, got %d: %v", len(s.Ingestion), s.Ingestion)
	}
	if _, ok := s.Ingestion["schedule"]; !ok {
		t.Errorf("expected ingestion key 'schedule', got %v", s.Ingestion)
	}
	if _, ok := s.Ingestion["webhook"]; !ok {
		t.Errorf("expected ingestion key 'webhook', got %v", s.Ingestion)
	}
}

func TestAstroYml_SingleIngestion(t *testing.T) {
	for _, ingType := range []string{"schedule", "webhook", "manual", "startup"} {
		t.Run(ingType, func(t *testing.T) {
			yaml := renderAstroYml(t, ScaffoldConfig{
				Name:            "test-agent",
				Description:     "test",
				Interfaces:      []string{"web"},
				Integrations:    []string{},
				IntegrationKeys: map[string]string{},
				Knowledge:       []string{},
				Ingestions:      []string{ingType},
			})

			s, err := spec.ParseString(yaml)
			if err != nil {
				t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
			}

			if len(s.Ingestion) != 1 {
				t.Errorf("expected 1 ingestion job, got %d: %v", len(s.Ingestion), s.Ingestion)
			}
			if _, ok := s.Ingestion[ingType]; !ok {
				t.Errorf("expected ingestion key %q, got %v", ingType, s.Ingestion)
			}
			if s.Ingestion[ingType].Trigger.Type != ingType {
				t.Errorf("trigger type = %q, want %q", s.Ingestion[ingType].Trigger.Type, ingType)
			}
		})
	}
}

// subsets returns all 2^n subsets of items (including empty).
func subsets(items []string) [][]string {
	result := make([][]string, 1<<len(items))
	for mask := range result {
		var s []string
		for i, v := range items {
			if mask&(1<<i) != 0 {
				s = append(s, v)
			}
		}
		result[mask] = s
	}
	return result
}

// TestAllTemplatesRender renders every template against every combination of
// interfaces, model state, integrations, knowledge, and ingestion selections.
// This catches stale field references the moment a ScaffoldConfig field is renamed.
func TestAllTemplatesRender(t *testing.T) {
	paths, err := GetTemplatePaths("mastra")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}

	// Standard templates rendered with ScaffoldConfig.
	standardTemplates := []struct {
		name string
		path string
	}{
		{"astropods.yml", paths.AstroYml},
		{"Dockerfile", paths.Dockerfile},
		{"package.json", paths.PackageJson},
		{"tsconfig.json", paths.Tsconfig},
		{"gitignore", paths.Gitignore},
		{"dockerignore", paths.Dockerignore},
		{"agent/index.ts", paths.AgentIndex},
		{"ingestion/<type>/index.ts", paths.IngestionIndex},
		{"ingestion/webhook/index.ts", paths.IngestionWebhookIndex},
		{"agents.md", paths.LlmMd},
		{"AGENT.md", paths.AgentMd},
		{"README.md", paths.Readme},
	}

	// Ingestion Dockerfile is rendered with ingestionDockerfileData (needs IngestionType).
	ingestionDockerfileTypes := []string{"schedule", "webhook", "manual", "startup"}

	// All subsets of each multi-valued field.
	interfaceSubsets := subsets([]string{"web", "slack"})                             // 2^2 = 4
	integrationSubsets := subsets([]string{"anthropic", "openai", "github"})          // 2^3 = 8
	knowledgeSubsets := subsets([]string{"qdrant", "redis", "neo4j"})                 // 2^3 = 8
	ingestionSubsets := subsets([]string{"schedule", "webhook", "manual", "startup"}) // 2^4 = 16

	// Model states: none, provider-only, provider+model.
	type modelState struct{ provider, model string }
	modelStates := []modelState{
		{"", ""},
		{"ollama", ""},
		{"ollama", "llama3.2:1b"},
	}

	for _, tmpl := range standardTemplates {
		t.Run(tmpl.name, func(t *testing.T) {
			for _, ifaces := range interfaceSubsets {
				for _, ms := range modelStates {
					for _, integs := range integrationSubsets {
						for _, know := range knowledgeSubsets {
							for _, ings := range ingestionSubsets {
								cfg := ScaffoldConfig{
									Name:            "a",
									Description:     "d",
									Interfaces:      ifaces,
									ModelProvider:   ms.provider,
									Model:           ms.model,
									Integrations:    integs,
									IntegrationKeys: map[string]string{},
									Knowledge:       know,
									Ingestions:      ings,
								}
								if _, err := RenderTemplate(tmpl.path, cfg); err != nil {
									t.Errorf(
										"interfaces=%v model=%q/%q integrations=%v knowledge=%v ingestions=%v: %v",
										ifaces, ms.provider, ms.model, integs, know, ings, err,
									)
								}
							}
						}
					}
				}
			}
		})
	}

	// Ingestion Dockerfile is rendered with RenderIngestionDockerfile (requires IngestionType).
	t.Run("ingestion/<type>/Dockerfile", func(t *testing.T) {
		cfg := ScaffoldConfig{Name: "a", Description: "d", IntegrationKeys: map[string]string{}}
		for _, ingType := range ingestionDockerfileTypes {
			if _, err := RenderIngestionDockerfile(paths.DockerfileIngestion, cfg, ingType); err != nil {
				t.Errorf("ingestionType=%q: %v", ingType, err)
			}
		}
	})
}

// generateWithMemFs runs generateFiles with an in-memory filesystem and returns it.
func generateWithMemFs(t *testing.T, config ScaffoldConfig) *MemFs {
	t.Helper()
	memfs := newMemFs()
	if err := generateFiles(memfs, "/proj", config, "mastra"); err != nil {
		t.Fatalf("generateFiles: %v", err)
	}
	return memfs
}

func TestGenerateFiles_IngestionPerTypeFolderStructure(t *testing.T) {
	for _, ingType := range []string{"schedule", "webhook", "manual", "startup"} {
		t.Run(ingType, func(t *testing.T) {
			memfs := generateWithMemFs(t, ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				Integrations: []string{}, IntegrationKeys: map[string]string{},
				Knowledge: []string{}, Ingestions: []string{ingType},
			})

			if !memfs.HasDir(filepath.Join("/proj", "ingestion", ingType)) {
				t.Errorf("expected dir ingestion/%s", ingType)
			}
			if !memfs.HasFile(filepath.Join("/proj", "ingestion", ingType, "Dockerfile")) {
				t.Errorf("expected file ingestion/%s/Dockerfile", ingType)
			}
			if !memfs.HasFile(filepath.Join("/proj", "ingestion", ingType, "index.ts")) {
				t.Errorf("expected file ingestion/%s/index.ts", ingType)
			}
			if memfs.HasFile(filepath.Join("/proj", "Dockerfile.ingestion")) {
				t.Errorf("unexpected top-level Dockerfile.ingestion")
			}
		})
	}
}

func TestGenerateFiles_MultipleIngestions_EachGetsOwnFolder(t *testing.T) {
	ingestions := []string{"schedule", "webhook", "manual", "startup"}
	memfs := generateWithMemFs(t, ScaffoldConfig{
		Name: "a", Description: "d", Interfaces: []string{"web"},
		Integrations: []string{}, IntegrationKeys: map[string]string{},
		Knowledge: []string{}, Ingestions: ingestions,
	})

	for _, ing := range ingestions {
		if !memfs.HasDir(filepath.Join("/proj", "ingestion", ing)) {
			t.Errorf("expected dir ingestion/%s", ing)
		}
		if !memfs.HasFile(filepath.Join("/proj", "ingestion", ing, "Dockerfile")) {
			t.Errorf("expected file ingestion/%s/Dockerfile", ing)
		}
		if !memfs.HasFile(filepath.Join("/proj", "ingestion", ing, "index.ts")) {
			t.Errorf("expected file ingestion/%s/index.ts", ing)
		}
	}
	if memfs.HasFile(filepath.Join("/proj", "Dockerfile.ingestion")) {
		t.Errorf("unexpected top-level Dockerfile.ingestion")
	}
}

func TestGenerateFiles_NoIngestion_NoIngestionSubdirs(t *testing.T) {
	memfs := generateWithMemFs(t, ScaffoldConfig{
		Name: "a", Description: "d", Interfaces: []string{"web"},
		Integrations: []string{}, IntegrationKeys: map[string]string{},
		Knowledge: []string{}, Ingestions: []string{},
	})

	for path := range memfs.Dirs {
		if filepath.Dir(path) == filepath.Join("/proj", "ingestion") {
			t.Errorf("unexpected ingestion subdir: %s", path)
		}
	}
}

func TestAstroYml_IngestionDockerfilePath(t *testing.T) {
	for _, ingType := range []string{"schedule", "webhook", "manual", "startup"} {
		t.Run(ingType, func(t *testing.T) {
			yaml := renderAstroYml(t, ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				Integrations: []string{}, IntegrationKeys: map[string]string{},
				Knowledge: []string{}, Ingestions: []string{ingType},
			})

			s, err := spec.ParseString(yaml)
			if err != nil {
				t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
			}

			ing, ok := s.Ingestion[ingType]
			if !ok {
				t.Fatalf("expected ingestion key %q, got %v", ingType, s.Ingestion)
			}
			wantDockerfile := filepath.Join("ingestion", ingType, "Dockerfile")
			if ing.Container.Build == nil {
				t.Fatalf("ingestion[%q].container.build is nil", ingType)
			}
			if ing.Container.Build.Dockerfile != wantDockerfile {
				t.Errorf("dockerfile = %q, want %q", ing.Container.Build.Dockerfile, wantDockerfile)
			}
		})
	}
}

func TestIngestionDockerfile_CorrectPathsPerType(t *testing.T) {
	paths, err := GetTemplatePaths("mastra")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}
	cfg := ScaffoldConfig{Name: "a", Description: "d", IntegrationKeys: map[string]string{}}

	for _, ingType := range []string{"schedule", "webhook", "manual", "startup"} {
		t.Run(ingType, func(t *testing.T) {
			content, err := RenderIngestionDockerfile(paths.DockerfileIngestion, cfg, ingType)
			if err != nil {
				t.Fatalf("RenderIngestionDockerfile: %v", err)
			}
			wantCopy := "COPY ingestion/" + ingType
			wantCmd := `"ingestion/` + ingType + `/index.ts"`
			if !strings.Contains(content, wantCopy) {
				t.Errorf("expected COPY referencing %q in:\n%s", wantCopy, content)
			}
			if !strings.Contains(content, wantCmd) {
				t.Errorf("expected CMD referencing %q in:\n%s", wantCmd, content)
			}
			if strings.Contains(content, "ingestion/index.ts") {
				t.Errorf("found stale ingestion/index.ts path in:\n%s", content)
			}
		})
	}
}

func TestAstroYml_MultipleIngestions_DockerfilePaths(t *testing.T) {
	ingestions := []string{"schedule", "webhook", "manual", "startup"}
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name: "a", Description: "d", Interfaces: []string{"web"},
		Integrations: []string{}, IntegrationKeys: map[string]string{},
		Knowledge: []string{}, Ingestions: ingestions,
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	for _, ingType := range ingestions {
		ing, ok := s.Ingestion[ingType]
		if !ok {
			t.Errorf("expected ingestion key %q", ingType)
			continue
		}
		wantDockerfile := filepath.Join("ingestion", ingType, "Dockerfile")
		if ing.Container.Build == nil {
			t.Errorf("ingestion[%q].container.build is nil", ingType)
			continue
		}
		if ing.Container.Build.Dockerfile != wantDockerfile {
			t.Errorf("ingestion[%q] dockerfile = %q, want %q", ingType, ing.Container.Build.Dockerfile, wantDockerfile)
		}
	}
}

// validateWithParseSpec writes yaml to a temp file and runs the same semantic
// validation that `ast validate` uses (spec.ParseSpec). This catches errors that
// spec.ParseString silently ignores (missing required fields, mutual exclusions, etc.).
func validateWithParseSpec(t *testing.T, yaml string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "astropods-*.yml")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(yaml); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := spec.ParseSpec(f.Name()); err != nil {
		t.Errorf("ast validate (ParseSpec) failed:\n%s\nerror: %v", yaml, err)
	}
}

// TestAstroYml_PassesSpecValidate checks that the generated spec passes
// the same semantic validation run by `ast validate` for representative configs.
func TestAstroYml_PassesSpecValidate(t *testing.T) {
	tests := []struct {
		name   string
		config ScaffoldConfig
	}{
		{
			name: "minimal",
			config: ScaffoldConfig{
				Name: "bare-agent", Description: "minimal",
				Interfaces: []string{"web"}, Integrations: []string{},
				IntegrationKeys: map[string]string{}, Knowledge: []string{}, Ingestions: []string{},
			},
		},
		{
			name: "full infrastructure",
			config: ScaffoldConfig{
				Name: "full-agent", Description: "full",
				Interfaces:    []string{"web", "slack"},
				ModelProvider: "ollama", Model: "mistral",
				Integrations:    []string{"anthropic", "openai", "github"},
				IntegrationKeys: map[string]string{},
				Knowledge:       []string{"qdrant", "redis", "neo4j"},
				Ingestions:      []string{"schedule", "webhook"},
			},
		},
		{
			name: "anthropic only",
			config: ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				Integrations: []string{"anthropic"}, IntegrationKeys: map[string]string{},
				Knowledge: []string{}, Ingestions: []string{},
			},
		},
		{
			name: "all ingestion types",
			config: ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				Integrations: []string{}, IntegrationKeys: map[string]string{},
				Knowledge: []string{}, Ingestions: []string{"schedule", "webhook", "manual", "startup"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yaml := renderAstroYml(t, tt.config)
			validateWithParseSpec(t, yaml)
		})
	}
}

// TestAgentEnvVars_AlwaysIncludesGRPC verifies GRPC_SERVER_ADDR is always present.
func TestAgentEnvVars_AlwaysIncludesGRPC(t *testing.T) {
	cfg := ScaffoldConfig{Name: "a", Description: "d", IntegrationKeys: map[string]string{}}
	found := false
	for _, v := range cfg.AgentEnvVars() {
		if v.Key == "GRPC_SERVER_ADDR" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AgentEnvVars should always include GRPC_SERVER_ADDR")
	}
}

// TestAgentEnvVars_CloudCredentials checks that cloud integrations produce credential keys.
func TestAgentEnvVars_CloudCredentials(t *testing.T) {
	tests := []struct {
		name         string
		integrations []string
		wantKey      string
	}{
		{"anthropic", []string{"anthropic"}, "ANTHROPIC_API_KEY"},
		{"openai", []string{"openai"}, "OPENAI_API_KEY"},
		{"github", []string{"github"}, "GITHUB_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ScaffoldConfig{
				Name:            "a",
				Integrations:    tt.integrations,
				IntegrationKeys: map[string]string{},
			}
			found := false
			for _, v := range cfg.AgentEnvVars() {
				if v.Key == tt.wantKey {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("AgentEnvVars with integrations=%v should include %s", tt.integrations, tt.wantKey)
			}
		})
	}
}

// TestAgentEnvVars_SelfHostedKnowledgeConnections checks connection keys for self-hosted providers.
func TestAgentEnvVars_SelfHostedKnowledgeConnections(t *testing.T) {
	tests := []struct {
		knowledge []string
		wantKey   string
	}{
		{[]string{"qdrant"}, "QDRANT_HOST"},
		{[]string{"qdrant"}, "QDRANT_PORT"},
		{[]string{"redis"}, "REDIS_HOST"},
		{[]string{"redis"}, "REDIS_PORT"},
		{[]string{"neo4j"}, "NEO4J_HOST"},
		{[]string{"neo4j"}, "NEO4J_PORT"},
	}
	for _, tt := range tests {
		t.Run(tt.wantKey, func(t *testing.T) {
			cfg := ScaffoldConfig{
				Name:            "a",
				Knowledge:       tt.knowledge,
				IntegrationKeys: map[string]string{},
			}
			found := false
			for _, v := range cfg.AgentEnvVars() {
				if v.Key == tt.wantKey {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("AgentEnvVars with knowledge=%v should include %s", tt.knowledge, tt.wantKey)
			}
		})
	}
}

// TestAgentEnvVars_OllamaModelProvider checks connection keys for self-hosted model providers.
func TestAgentEnvVars_OllamaModelProvider(t *testing.T) {
	cfg := ScaffoldConfig{
		Name:            "a",
		ModelProvider:   "ollama",
		Model:           "llama3",
		IntegrationKeys: map[string]string{},
	}
	wantKeys := []string{"OLLAMA_HOST", "OLLAMA_PORT", "OLLAMA_URL"}
	for _, wantKey := range wantKeys {
		found := false
		for _, v := range cfg.AgentEnvVars() {
			if v.Key == wantKey {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AgentEnvVars with ollama model should include %s", wantKey)
		}
	}
}

// TestAgentEnvVars_AllHaveDescriptions verifies every returned var has a non-empty description.
func TestAgentEnvVars_AllHaveDescriptions(t *testing.T) {
	cfg := ScaffoldConfig{
		Name:            "a",
		ModelProvider:   "ollama",
		Model:           "llama3",
		Integrations:    []string{"anthropic", "openai", "github"},
		Knowledge:       []string{"qdrant", "redis", "neo4j"},
		IntegrationKeys: map[string]string{},
	}
	for _, v := range cfg.AgentEnvVars() {
		if v.Description == "" {
			t.Errorf("env var %q has no description", v.Key)
		}
	}
}

// TestAgentEnvVars_MatchesSpecEnvResolver verifies that AgentEnvVars covers all
// keys that AllAgentAutoEnvKeys returns for the rendered spec.
func TestAgentEnvVars_MatchesSpecEnvResolver(t *testing.T) {
	cfg := ScaffoldConfig{
		Name:            "a",
		Integrations:    []string{"anthropic", "openai", "github"},
		Knowledge:       []string{"qdrant", "redis"},
		IntegrationKeys: map[string]string{},
	}

	s, err := cfg.specFromTemplate()
	if err != nil {
		t.Fatalf("specFromTemplate: %v", err)
	}

	vars := cfg.AgentEnvVars()
	varSet := make(map[string]bool, len(vars))
	for _, v := range vars {
		varSet[v.Key] = true
	}

	for k := range spec.AllAgentAutoEnvKeys(s) {
		if !varSet[k] {
			t.Errorf("AgentEnvVars missing key %q from AllAgentAutoEnvKeys", k)
		}
	}
}

// TestMastraTemplate_AgentIndex_EnvVarsInComment checks that the rendered template
// includes the computed env vars with their descriptions in the JSDoc comment.
func TestMastraTemplate_AgentIndex_EnvVarsInComment(t *testing.T) {
	paths, _ := GetTemplatePaths("mastra")
	cfg := ScaffoldConfig{
		Name:            "test-agent",
		Description:     "A test agent",
		Integrations:    []string{"anthropic"},
		Knowledge:       []string{"qdrant"},
		IntegrationKeys: map[string]string{},
	}
	content := renderTemplate(t, paths.AgentIndex, cfg)

	// Key and description pairs expected in the comment.
	wantLines := []string{
		"GRPC_SERVER_ADDR - injected by Astro messaging service",
		"ANTHROPIC_API_KEY - injected by anthropic model",
		"QDRANT_HOST - injected by qdrant knowledge store host",
		"QDRANT_PORT - injected by qdrant knowledge store port",
	}
	for _, want := range wantLines {
		if !strings.Contains(content, want) {
			t.Errorf("rendered index.ts comment should contain %q, got:\n%s", want, content)
		}
	}
}

// generateWithPyMemFs runs generateFiles with lang=py against an in-memory filesystem.
func generateWithPyMemFs(t *testing.T, config ScaffoldConfig) *MemFs {
	t.Helper()
	memfs := newMemFs()
	if err := generateFiles(memfs, "/proj", config, "langchain"); err != nil {
		t.Fatalf("generateFiles: %v", err)
	}
	return memfs
}

// TestGetTemplatePaths_Python_HasIngestionRequirementsTxt ensures the IngestionRequirementsTxt
// field is set for the Python langchain template so the scaffold can generate requirements.txt.
func TestGetTemplatePaths_Python_HasIngestionRequirementsTxt(t *testing.T) {
	paths, err := GetTemplatePaths("langchain")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}
	if paths.IngestionRequirementsTxt == "" {
		t.Error("IngestionRequirementsTxt should be set for Python langchain template")
	}
}

// TestGenerateFiles_Python_AgentMainPy verifies that Python scaffolds produce agent/main.py
// and not the TypeScript agent/index.ts.
func TestGenerateFiles_Python_AgentMainPy(t *testing.T) {
	memfs := generateWithPyMemFs(t, ScaffoldConfig{
		Name: "a", Description: "d", Interfaces: []string{"web"},
		Integrations: []string{}, IntegrationKeys: map[string]string{},
		Knowledge: []string{}, Ingestions: []string{},
	})

	if !memfs.HasFile(filepath.Join("/proj", "agent", "main.py")) {
		t.Error("expected agent/main.py for Python scaffold")
	}
	if memfs.HasFile(filepath.Join("/proj", "agent", "index.ts")) {
		t.Error("unexpected agent/index.ts for Python scaffold")
	}
}

// TestGenerateFiles_Python_NoTypescriptFiles verifies that TypeScript-only files are
// not generated for Python scaffolds.
func TestGenerateFiles_Python_NoTypescriptFiles(t *testing.T) {
	memfs := generateWithPyMemFs(t, ScaffoldConfig{
		Name: "a", Description: "d", Interfaces: []string{"web"},
		Integrations: []string{}, IntegrationKeys: map[string]string{},
		Knowledge: []string{}, Ingestions: []string{},
	})

	for _, tsFile := range []string{"package.json", "tsconfig.json"} {
		if memfs.HasFile(filepath.Join("/proj", tsFile)) {
			t.Errorf("unexpected TypeScript file %s in Python scaffold", tsFile)
		}
	}
}

// TestGenerateFiles_Python_IngestionRequirementsTxt verifies that a requirements.txt
// is generated alongside main.py for each ingestion type in a Python scaffold.
func TestGenerateFiles_Python_IngestionRequirementsTxt(t *testing.T) {
	for _, ingType := range []string{"schedule", "webhook", "manual", "startup"} {
		t.Run(ingType, func(t *testing.T) {
			memfs := generateWithPyMemFs(t, ScaffoldConfig{
				Name: "a", Description: "d", Interfaces: []string{"web"},
				Integrations: []string{}, IntegrationKeys: map[string]string{},
				Knowledge: []string{}, Ingestions: []string{ingType},
			})

			mainPy := filepath.Join("/proj", "ingestion", ingType, "main.py")
			reqsTxt := filepath.Join("/proj", "ingestion", ingType, "requirements.txt")

			if !memfs.HasFile(mainPy) {
				t.Errorf("expected ingestion/%s/main.py", ingType)
			}
			if !memfs.HasFile(reqsTxt) {
				t.Errorf("expected ingestion/%s/requirements.txt alongside main.py", ingType)
			}
			if memfs.HasFile(filepath.Join("/proj", "ingestion", ingType, "index.ts")) {
				t.Errorf("unexpected ingestion/%s/index.ts in Python scaffold", ingType)
			}
		})
	}
}

// TestPythonIngestionDockerfile_CorrectPathsPerType verifies each rendered Python ingestion
// Dockerfile references the correct ingestion type in its COPY and CMD instructions.
func TestPythonIngestionDockerfile_CorrectPathsPerType(t *testing.T) {
	paths, err := GetTemplatePaths("langchain")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}
	cfg := ScaffoldConfig{Name: "a", Description: "d", IntegrationKeys: map[string]string{}}

	for _, ingType := range []string{"schedule", "webhook", "manual", "startup"} {
		t.Run(ingType, func(t *testing.T) {
			content, err := RenderIngestionDockerfile(paths.DockerfileIngestion, cfg, ingType)
			if err != nil {
				t.Fatalf("RenderIngestionDockerfile: %v", err)
			}
			wantCopy := "COPY ingestion/" + ingType
			wantCmd := `"ingestion/` + ingType + `/main.py"`
			if !strings.Contains(content, wantCopy) {
				t.Errorf("expected COPY referencing %q in:\n%s", wantCopy, content)
			}
			if !strings.Contains(content, wantCmd) {
				t.Errorf("expected CMD referencing %q in:\n%s", wantCmd, content)
			}
			if strings.Contains(content, "index.ts") {
				t.Errorf("found TypeScript path index.ts in Python Dockerfile:\n%s", content)
			}
		})
	}
}

// TestPythonIngestionDockerfile_HasRequirementsInstall verifies the ingestion Dockerfile
// installs dependencies from requirements.txt.
func TestPythonIngestionDockerfile_HasRequirementsInstall(t *testing.T) {
	paths, err := GetTemplatePaths("langchain")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}
	cfg := ScaffoldConfig{Name: "a", Description: "d", IntegrationKeys: map[string]string{}}

	content, err := RenderIngestionDockerfile(paths.DockerfileIngestion, cfg, "schedule")
	if err != nil {
		t.Fatalf("RenderIngestionDockerfile: %v", err)
	}
	if !strings.Contains(content, "pip install") {
		t.Errorf("Python ingestion Dockerfile should install pip dependencies, got:\n%s", content)
	}
	if !strings.Contains(content, "requirements.txt") {
		t.Errorf("Python ingestion Dockerfile should reference requirements.txt, got:\n%s", content)
	}
}

// TestPythonIngestionDockerfile_HasPythonUnbuffered verifies the ingestion Dockerfile
// sets PYTHONUNBUFFERED=1 so logs are visible in Docker without buffering.
func TestPythonIngestionDockerfile_HasPythonUnbuffered(t *testing.T) {
	paths, err := GetTemplatePaths("langchain")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}
	cfg := ScaffoldConfig{Name: "a", Description: "d", IntegrationKeys: map[string]string{}}

	content, err := RenderIngestionDockerfile(paths.DockerfileIngestion, cfg, "schedule")
	if err != nil {
		t.Fatalf("RenderIngestionDockerfile: %v", err)
	}
	if !strings.Contains(content, "PYTHONUNBUFFERED=1") {
		t.Errorf("Python ingestion Dockerfile should set PYTHONUNBUFFERED=1, got:\n%s", content)
	}
}

// TestAllPythonTemplatesRender renders every Python template against a representative
// set of config combinations to catch stale field references.
func TestAllPythonTemplatesRender(t *testing.T) {
	paths, err := GetTemplatePaths("langchain")
	if err != nil {
		t.Fatalf("GetTemplatePaths: %v", err)
	}

	standardTemplates := []struct {
		name string
		path string
	}{
		{"astropods.yml", paths.AstroYml},
		{"Dockerfile", paths.Dockerfile},
		{"gitignore", paths.Gitignore},
		{"dockerignore", paths.Dockerignore},
		{"agent/main.py", paths.AgentMain},
		{"requirements.txt", paths.RequirementsTxt},
		{"ingestion/main.py", paths.IngestionMain},
		{"ingestion/webhook.py", paths.IngestionWebhookPy},
		{"agents.md", paths.LlmMd},
		{"AGENT.md", paths.AgentMd},
		{"README.md", paths.Readme},
	}

	ingestionDockerfileTypes := []string{"schedule", "webhook", "manual", "startup"}
	interfaceSubsets := subsets([]string{"web", "slack"})
	integrationSubsets := subsets([]string{"anthropic", "openai", "github"})
	knowledgeSubsets := subsets([]string{"qdrant", "redis", "neo4j"})
	ingestionSubsets := subsets([]string{"schedule", "webhook", "manual", "startup"})

	type modelState struct{ provider, model string }
	modelStates := []modelState{
		{"", ""},
		{"ollama", ""},
		{"ollama", "llama3.2:1b"},
	}

	for _, tmpl := range standardTemplates {
		t.Run(tmpl.name, func(t *testing.T) {
			for _, ifaces := range interfaceSubsets {
				for _, ms := range modelStates {
					for _, integs := range integrationSubsets {
						for _, know := range knowledgeSubsets {
							for _, ings := range ingestionSubsets {
								cfg := ScaffoldConfig{
									Name:            "a",
									Description:     "d",
									Interfaces:      ifaces,
									ModelProvider:   ms.provider,
									Model:           ms.model,
									Integrations:    integs,
									IntegrationKeys: map[string]string{},
									Knowledge:       know,
									Ingestions:      ings,
								}
								if _, err := RenderTemplate(tmpl.path, cfg); err != nil {
									t.Errorf(
										"interfaces=%v model=%q/%q integrations=%v knowledge=%v ingestions=%v: %v",
										ifaces, ms.provider, ms.model, integs, know, ings, err,
									)
								}
							}
						}
					}
				}
			}
		})
	}

	t.Run("ingestion/<type>/Dockerfile", func(t *testing.T) {
		cfg := ScaffoldConfig{Name: "a", Description: "d", IntegrationKeys: map[string]string{}}
		for _, ingType := range ingestionDockerfileTypes {
			if _, err := RenderIngestionDockerfile(paths.DockerfileIngestion, cfg, ingType); err != nil {
				t.Errorf("ingestionType=%q: %v", ingType, err)
			}
		}
	})
}

// TestAstroYml_ScheduleIngestion_NoDevSchedules ensures the generated spec does
// not include a dev.schedules block — triggering is handled via `ast dev trigger`.
func TestAstroYml_ScheduleIngestion_NoDevSchedules(t *testing.T) {
	yaml := renderAstroYml(t, ScaffoldConfig{
		Name: "a", Description: "d", Interfaces: []string{"web"},
		Integrations: []string{}, IntegrationKeys: map[string]string{},
		Knowledge: []string{}, Ingestions: []string{"schedule"},
	})

	s, err := spec.ParseString(yaml)
	if err != nil {
		t.Fatalf("ParseString failed:\n%s\nerror: %v", yaml, err)
	}

	if s.Dev != nil && len(s.Dev.Schedules) > 0 {
		t.Errorf("expected no dev.schedules, got %v", s.Dev.Schedules)
	}
}
