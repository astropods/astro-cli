package spec

import (
	"fmt"
	"sort"
	"testing"
)

// --- CollectComponents ---

func TestCollectComponents_AgentOnly(t *testing.T) {
	s := &AstroSpec{
		Agent: Container{Build: &BuildConfig{Context: ".", Dockerfile: "Dockerfile"}},
	}
	got := CollectComponents(s, "my-agent")
	if len(got) != 1 {
		t.Fatalf("got %d components, want 1", len(got))
	}
	c := got[0]
	if c.Kind != ComponentAgent {
		t.Errorf("Kind = %q, want %q", c.Kind, ComponentAgent)
	}
	if c.ImageName != "my-agent" {
		t.Errorf("ImageName = %q, want %q", c.ImageName, "my-agent")
	}
	if c.Name != "" {
		t.Errorf("Name = %q, want empty", c.Name)
	}
	if c.Build == nil {
		t.Error("Build is nil")
	}
}

func TestCollectComponents_AgentWithoutBuild(t *testing.T) {
	s := &AstroSpec{
		Agent: Container{Image: "prebuilt:latest"},
	}
	got := CollectComponents(s, "my-agent")
	if len(got) != 0 {
		t.Fatalf("got %d components, want 0 (agent without build should be omitted)", len(got))
	}
}

func TestCollectComponents_AllTypes(t *testing.T) {
	s := &AstroSpec{
		Agent: Container{Build: &BuildConfig{Context: "."}},
		Models: map[string]Model{
			"llm": {Container: &ContainerConfig{Build: &BuildConfig{Context: "models/llm"}}},
		},
		Knowledge: map[string]Knowledge{
			"docs": {Container: &ContainerConfig{Build: &BuildConfig{Context: "knowledge/docs"}}},
		},
		Integrations: map[string]Integration{
			"search": {Container: &ContainerConfig{Build: &BuildConfig{Context: "integrations/search"}}},
		},
		Ingestion: map[string]Ingestion{
			"crawler": {Container: ContainerConfig{Build: &BuildConfig{Context: "ingestion/crawler"}}},
		},
	}

	got := CollectComponents(s, "my-agent")
	if len(got) != 5 {
		t.Fatalf("got %d components, want 5", len(got))
	}

	byKind := map[ComponentKind]Component{}
	for _, c := range got {
		byKind[c.Kind] = c
	}

	expect := map[ComponentKind]string{
		ComponentAgent:       "my-agent",
		ComponentModel:       "my-agent-model-llm",
		ComponentKnowledge:   "my-agent-knowledge-docs",
		ComponentIntegration: "my-agent-integration-search",
		ComponentIngestion:   "my-agent-ingestion-crawler",
	}
	for kind, wantImage := range expect {
		c, ok := byKind[kind]
		if !ok {
			t.Errorf("missing component of kind %q", kind)
			continue
		}
		if c.ImageName != wantImage {
			t.Errorf("%s ImageName = %q, want %q", kind, c.ImageName, wantImage)
		}
	}
}

func TestCollectComponents_IntegrationNaming(t *testing.T) {
	s := &AstroSpec{
		Integrations: map[string]Integration{
			"github": {Container: &ContainerConfig{Build: &BuildConfig{Context: "."}}},
		},
	}
	got := CollectComponents(s, "my-agent")
	if len(got) != 1 {
		t.Fatalf("got %d components, want 1", len(got))
	}
	if got[0].ImageName != "my-agent-integration-github" {
		t.Errorf("ImageName = %q, want %q (must use -integration-, not -tool-)", got[0].ImageName, "my-agent-integration-github")
	}
}

func TestCollectComponents_MixedBuildAndImage(t *testing.T) {
	s := &AstroSpec{
		Agent: Container{Build: &BuildConfig{Context: "."}},
		Models: map[string]Model{
			"cloud":    {Provider: "anthropic"},                                                      // no build
			"custom":   {Container: &ContainerConfig{Build: &BuildConfig{Context: "models/custom"}}}, // has build
			"prebuilt": {Container: &ContainerConfig{Image: "prebuilt:latest"}},                      // image only
		},
		Knowledge: map[string]Knowledge{
			"managed": {Provider: "qdrant"}, // provider, no build
		},
	}
	got := CollectComponents(s, "my-agent")

	// Only agent + custom model should be collected
	if len(got) != 2 {
		t.Fatalf("got %d components, want 2", len(got))
	}

	names := make([]string, len(got))
	for i, c := range got {
		names[i] = c.ImageName
	}
	sort.Strings(names)
	if names[0] != "my-agent" || names[1] != "my-agent-model-custom" {
		t.Errorf("unexpected components: %v", names)
	}
}

func TestCollectComponents_KnowledgeResolvedContainer(t *testing.T) {
	// Knowledge with a Container field (not provider mode)
	s := &AstroSpec{
		Knowledge: map[string]Knowledge{
			"local": {
				Container: &ContainerConfig{
					Build: &BuildConfig{Context: "knowledge/local"},
				},
			},
		},
	}
	got := CollectComponents(s, "my-agent")
	if len(got) != 1 {
		t.Fatalf("got %d components, want 1", len(got))
	}
	if got[0].Kind != ComponentKnowledge {
		t.Errorf("Kind = %q, want %q", got[0].Kind, ComponentKnowledge)
	}
	if got[0].Build.Context != "knowledge/local" {
		t.Errorf("Build.Context = %q, want %q", got[0].Build.Context, "knowledge/local")
	}
}

// --- Component.Suffix ---

func TestComponentSuffix(t *testing.T) {
	tests := []struct {
		comp Component
		want string
	}{
		{Component{Kind: ComponentAgent}, "agent"},
		{Component{Kind: ComponentModel, Name: "llm"}, "model-llm"},
		{Component{Kind: ComponentKnowledge, Name: "docs"}, "knowledge-docs"},
		{Component{Kind: ComponentIntegration, Name: "search"}, "integration-search"},
		{Component{Kind: ComponentIngestion, Name: "crawler"}, "ingestion-crawler"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.comp.Suffix(); got != tt.want {
				t.Errorf("Suffix() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- TransformSpecForRegistry ---

func TestTransformSpecForRegistry_AgentBuild(t *testing.T) {
	specObj := map[string]any{
		"name": "old-name",
		"agent": map[string]any{
			"build": map[string]any{"context": "."},
		},
	}

	imageRefFn := func(name string) string { return "registry.io/acct/" + name + ":abc123" }
	result := TransformSpecForRegistry(specObj, "new-name", imageRefFn)

	agent := result["agent"].(map[string]any)
	if _, hasBuild := agent["build"]; hasBuild {
		t.Error("agent.build should be deleted")
	}
	if agent["image"] != "registry.io/acct/new-name:abc123" {
		t.Errorf("agent.image = %q, want %q", agent["image"], "registry.io/acct/new-name:abc123")
	}
	if result["name"] != "new-name" {
		t.Errorf("name = %q, want %q", result["name"], "new-name")
	}
}

func TestTransformSpecForRegistry_AgentWithoutBuild(t *testing.T) {
	specObj := map[string]any{
		"name": "my-agent",
		"agent": map[string]any{
			"image": "prebuilt:latest",
		},
	}

	called := false
	imageRefFn := func(name string) string {
		called = true
		return "registry.io/acct/" + name + ":tag"
	}
	result := TransformSpecForRegistry(specObj, "my-agent", imageRefFn)

	agent := result["agent"].(map[string]any)
	if agent["image"] != "prebuilt:latest" {
		t.Errorf("agent.image should be unchanged, got %q", agent["image"])
	}
	// imageRefFn may be called for other sections but agent shouldn't trigger it
	// (no assertion on called since other sections may call it)
	_ = called
}

func TestTransformSpecForRegistry_AllSections(t *testing.T) {
	specObj := map[string]any{
		"name": "my-agent",
		"agent": map[string]any{
			"build": map[string]any{"context": "."},
		},
		"models": map[string]any{
			"llm": map[string]any{
				"container": map[string]any{
					"build": map[string]any{"context": "models/llm"},
				},
			},
		},
		"knowledge": map[string]any{
			"docs": map[string]any{
				"container": map[string]any{
					"build": map[string]any{"context": "knowledge/docs"},
				},
			},
		},
		"integrations": map[string]any{
			"search": map[string]any{
				"container": map[string]any{
					"build": map[string]any{"context": "integrations/search"},
				},
			},
		},
		"ingestion": map[string]any{
			"crawler": map[string]any{
				"container": map[string]any{
					"build": map[string]any{"context": "ingestion/crawler"},
				},
			},
		},
	}

	var calledWith []string
	imageRefFn := func(name string) string {
		calledWith = append(calledWith, name)
		return fmt.Sprintf("r.io/%s:tag", name)
	}
	TransformSpecForRegistry(specObj, "my-agent", imageRefFn)

	// Verify all build blocks removed and images set
	checks := []struct {
		path    string
		get     func() map[string]any
		wantImg string
	}{
		{"agent", func() map[string]any { return specObj["agent"].(map[string]any) }, "r.io/my-agent:tag"},
		{"models.llm.container", func() map[string]any {
			return specObj["models"].(map[string]any)["llm"].(map[string]any)["container"].(map[string]any)
		}, "r.io/my-agent-model-llm:tag"},
		{"knowledge.docs.container", func() map[string]any {
			return specObj["knowledge"].(map[string]any)["docs"].(map[string]any)["container"].(map[string]any)
		}, "r.io/my-agent-knowledge-docs:tag"},
		{"integrations.search.container", func() map[string]any {
			return specObj["integrations"].(map[string]any)["search"].(map[string]any)["container"].(map[string]any)
		}, "r.io/my-agent-integration-search:tag"},
		{"ingestion.crawler.container", func() map[string]any {
			return specObj["ingestion"].(map[string]any)["crawler"].(map[string]any)["container"].(map[string]any)
		}, "r.io/my-agent-ingestion-crawler:tag"},
	}

	for _, c := range checks {
		t.Run(c.path, func(t *testing.T) {
			m := c.get()
			if _, hasBuild := m["build"]; hasBuild {
				t.Errorf("%s: build should be deleted", c.path)
			}
			if m["image"] != c.wantImg {
				t.Errorf("%s: image = %q, want %q", c.path, m["image"], c.wantImg)
			}
		})
	}

	// Verify imageRefFn received correct image names
	sort.Strings(calledWith)
	expected := []string{
		"my-agent",
		"my-agent-ingestion-crawler",
		"my-agent-integration-search",
		"my-agent-knowledge-docs",
		"my-agent-model-llm",
	}
	if len(calledWith) != len(expected) {
		t.Fatalf("imageRefFn called %d times, want %d: %v", len(calledWith), len(expected), calledWith)
	}
	for i := range expected {
		if calledWith[i] != expected[i] {
			t.Errorf("imageRefFn call %d = %q, want %q", i, calledWith[i], expected[i])
		}
	}
}

func TestTransformSpecForRegistry_NoBuildBlocks(t *testing.T) {
	specObj := map[string]any{
		"name": "my-agent",
		"agent": map[string]any{
			"image": "prebuilt:latest",
		},
		"models": map[string]any{
			"llm": map[string]any{
				"container": map[string]any{
					"image": "model:latest",
				},
			},
		},
	}

	called := false
	imageRefFn := func(name string) string {
		called = true
		return "should-not-appear"
	}
	TransformSpecForRegistry(specObj, "my-agent", imageRefFn)

	if called {
		t.Error("imageRefFn should not be called when there are no build blocks")
	}
	if specObj["agent"].(map[string]any)["image"] != "prebuilt:latest" {
		t.Error("agent image should be unchanged")
	}
}

func TestTransformSpecForRegistry_NameNormalization(t *testing.T) {
	tests := []struct {
		specName  string
		agentName string
		wantName  string
	}{
		{"@example/foobar", "foobar", "foobar"},
		{"@example/foobar", "barbat", "barbat"},
		{"my-agent", "my-agent", "my-agent"},
	}
	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			specObj := map[string]any{
				"name":  tt.specName,
				"agent": map[string]any{"image": "x:latest"},
			}
			result := TransformSpecForRegistry(specObj, tt.agentName, func(string) string { return "" })
			if result["name"] != tt.wantName {
				t.Errorf("name = %q, want %q", result["name"], tt.wantName)
			}
		})
	}
}

// --- StripSecretDefaults ---

func TestStripSecretDefaults(t *testing.T) {
	specObj := map[string]any{
		"name": "test-agent",
		"inputs": map[string]any{
			"api_key":   map[string]any{"name": "API_KEY", "secret": true, "default": "sk-secret"},
			"log_level": map[string]any{"name": "LOG_LEVEL", "default": "debug"},
		},
		"agent": map[string]any{
			"image": "test:latest",
			"inputs": []any{
				map[string]any{"name": "AGENT_SECRET", "secret": true, "default": "agent-val"},
				map[string]any{"name": "AGENT_PLAIN", "default": "plain-val"},
			},
		},
		"models": map[string]any{
			"llm": map[string]any{
				"inputs": []any{
					map[string]any{"name": "MODEL_KEY", "secret": true, "default": "model-secret"},
				},
			},
		},
		"knowledge": map[string]any{
			"docs": map[string]any{
				"inputs": []any{
					map[string]any{"name": "KB_KEY", "secret": true, "default": "kb-secret"},
				},
			},
		},
		"integrations": map[string]any{
			"search": map[string]any{
				"inputs": []any{
					map[string]any{"name": "SEARCH_KEY", "secret": true, "default": "search-secret"},
				},
			},
		},
		"ingestion": map[string]any{
			"crawler": map[string]any{
				"inputs": []any{
					map[string]any{"name": "CRAWL_KEY", "secret": true, "default": "crawl-secret"},
				},
			},
		},
		"providers": map[string]any{
			"anthropic": map[string]any{
				"variables": []any{
					map[string]any{"name": "ANTHROPIC_API_KEY", "secret": true, "default": "sk-ant-test"},
					map[string]any{"name": "ANTHROPIC_ORG", "default": "org-123"},
				},
			},
		},
	}

	StripSecretDefaults(specObj)

	// Secret defaults should be stripped
	apiKey := specObj["inputs"].(map[string]any)["api_key"].(map[string]any)
	if _, ok := apiKey["default"]; ok {
		t.Error("secret input API_KEY should have default stripped")
	}

	// Non-secret defaults should be preserved
	logLevel := specObj["inputs"].(map[string]any)["log_level"].(map[string]any)
	if logLevel["default"] != "debug" {
		t.Error("non-secret LOG_LEVEL default should be preserved")
	}

	// Agent secret stripped, non-secret preserved
	agentInputs := specObj["agent"].(map[string]any)["inputs"].([]any)
	if _, ok := agentInputs[0].(map[string]any)["default"]; ok {
		t.Error("agent secret input should have default stripped")
	}
	if agentInputs[1].(map[string]any)["default"] != "plain-val" {
		t.Error("agent non-secret input default should be preserved")
	}

	// Model secret stripped
	modelInputs := specObj["models"].(map[string]any)["llm"].(map[string]any)["inputs"].([]any)
	if _, ok := modelInputs[0].(map[string]any)["default"]; ok {
		t.Error("model secret input should have default stripped")
	}

	// Knowledge secret stripped
	kbInputs := specObj["knowledge"].(map[string]any)["docs"].(map[string]any)["inputs"].([]any)
	if _, ok := kbInputs[0].(map[string]any)["default"]; ok {
		t.Error("knowledge secret input should have default stripped")
	}

	// Integration secret stripped
	searchInputs := specObj["integrations"].(map[string]any)["search"].(map[string]any)["inputs"].([]any)
	if _, ok := searchInputs[0].(map[string]any)["default"]; ok {
		t.Error("integration secret input should have default stripped")
	}

	// Ingestion secret stripped
	crawlInputs := specObj["ingestion"].(map[string]any)["crawler"].(map[string]any)["inputs"].([]any)
	if _, ok := crawlInputs[0].(map[string]any)["default"]; ok {
		t.Error("ingestion secret input should have default stripped")
	}

	// Provider secret stripped, non-secret preserved
	provVars := specObj["providers"].(map[string]any)["anthropic"].(map[string]any)["variables"].([]any)
	if _, ok := provVars[0].(map[string]any)["default"]; ok {
		t.Error("provider secret variable should have default stripped")
	}
	if provVars[1].(map[string]any)["default"] != "org-123" {
		t.Error("provider non-secret variable default should be preserved")
	}
}

func TestStripSecretDefaults_EmptySpec(t *testing.T) {
	specObj := map[string]any{"name": "test-agent"}
	// Should not panic on empty spec
	StripSecretDefaults(specObj)
}
