package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	spec "github.com/astropods/astro/packages/astro-spec"
)

func TestDevStatePath(t *testing.T) {
	path, err := devStatePath()
	if err != nil {
		t.Fatalf("devStatePath() error = %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join(".ast", ".running")) {
		t.Errorf("devStatePath() = %q, want suffix %q", path, filepath.Join(".ast", ".running"))
	}
}

func TestReadDevProjectName(t *testing.T) {
	t.Run("reads project name from file", func(t *testing.T) {
		dir := t.TempDir()
		statePath := filepath.Join(dir, ".running")
		if err := os.WriteFile(statePath, []byte("my-agent\n"), 0600); err != nil {
			t.Fatal(err)
		}
		name, err := readDevProjectName(statePath, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if name != "my-agent" {
			t.Errorf("name = %q, want %q", name, "my-agent")
		}
	})

	t.Run("returns error when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		_, err := readDevProjectName(filepath.Join(dir, ".running"), nil)
		if err == nil {
			t.Fatal("expected error when state file is missing")
		}
	})
}

func TestLocalAstroPackagesPointToModules(t *testing.T) {
	for _, pkg := range localAstroPackages {
		t.Run(pkg.scope+"/"+pkg.name, func(t *testing.T) {
			if !strings.HasPrefix(pkg.path, "modules/") {
				t.Errorf("path %q should start with modules/ (not packages/)", pkg.path)
			}
		})
	}
}

func TestLocalDockerImagesConsistency(t *testing.T) {
	for _, img := range devLocalImages {
		t.Run(img.tag, func(t *testing.T) {
			if !strings.HasSuffix(img.tag, ":latest") {
				t.Errorf("tag %q should end with :latest", img.tag)
			}

			if !strings.HasPrefix(img.dockerfile, "modules/") && !strings.HasPrefix(img.dockerfile, "deployment/") {
				t.Errorf("dockerfile %q should start with modules/ or deployment/", img.dockerfile)
			}

			if img.context != "." && !strings.HasPrefix(img.context, "modules/") {
				t.Errorf("context %q should be '.' or start with modules/", img.context)
			}

			if img.context != "." && !strings.HasPrefix(img.dockerfile, img.context) {
				t.Errorf("dockerfile %q should be inside context %q", img.dockerfile, img.context)
			}

			if img.context != "." {
				tagName := strings.TrimSuffix(img.tag, ":latest")
				contextDir := img.context[strings.LastIndex(img.context, "/")+1:]
				if tagName != contextDir {
					t.Errorf("tag name %q doesn't match context dir %q", tagName, contextDir)
				}
			}
		})
	}
}

func TestRewriteDockerHostsToLocalhost(t *testing.T) {
	tests := []struct {
		name    string
		spec    *spec.AstroSpec
		envMap  map[string]string
		wantEnv map[string]string
	}{
		{
			name: "rewrites knowledge host to localhost",
			spec: &spec.AstroSpec{
				Knowledge: map[string]spec.Knowledge{
					"graph": {Provider: "neo4j"},
				},
			},
			envMap: map[string]string{
				"NEO4J_HOST": "knowledge-graph",
				"NEO4J_PORT": "7474",
				"OTHER_VAR":  "untouched",
			},
			wantEnv: map[string]string{
				"NEO4J_HOST": "localhost",
				"NEO4J_PORT": "7474",
				"OTHER_VAR":  "untouched",
			},
		},
		{
			name: "rewrites model host and embedded URLs",
			spec: &spec.AstroSpec{
				Models: map[string]spec.Model{
					"llm": {Provider: "ollama"},
				},
			},
			envMap: map[string]string{
				"OLLAMA_HOST":     "model-llm",
				"OLLAMA_URL":      "http://model-llm:11434",
				"OLLAMA_BASE_URL": "http://model-llm:11434/api",
			},
			wantEnv: map[string]string{
				"OLLAMA_HOST":     "localhost",
				"OLLAMA_URL":      "http://localhost:11434",
				"OLLAMA_BASE_URL": "http://localhost:11434/api",
			},
		},
		{
			name: "skips cloud providers that dont deploy containers",
			spec: &spec.AstroSpec{
				Models: map[string]spec.Model{
					"claude": {Provider: "anthropic"},
				},
			},
			envMap: map[string]string{
				"ANTHROPIC_API_KEY": "sk-test",
			},
			wantEnv: map[string]string{
				"ANTHROPIC_API_KEY": "sk-test",
			},
		},
		{
			name: "rewrites multiple services simultaneously",
			spec: &spec.AstroSpec{
				Knowledge: map[string]spec.Knowledge{
					"graph": {Provider: "neo4j"},
					"cache": {Provider: "redis"},
				},
			},
			envMap: map[string]string{
				"NEO4J_HOST": "knowledge-graph",
				"REDIS_HOST": "knowledge-cache",
			},
			wantEnv: map[string]string{
				"NEO4J_HOST": "localhost",
				"REDIS_HOST": "localhost",
			},
		},
		{
			name: "no-op when spec has no container services",
			spec: &spec.AstroSpec{},
			envMap: map[string]string{
				"HOME": "/Users/test",
			},
			wantEnv: map[string]string{
				"HOME": "/Users/test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.spec.Providers == nil {
				tt.spec.Providers = map[string]spec.CustomProvider{}
			}
			rewriteDockerHostsToLocalhost(tt.spec, tt.envMap)
			for k, want := range tt.wantEnv {
				if got := tt.envMap[k]; got != want {
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestLocalAstroPythonPackagesPointToModules(t *testing.T) {
	for _, pkg := range localAstroPythonPackages {
		t.Run(pkg.name, func(t *testing.T) {
			if !strings.HasPrefix(pkg.path, "modules/") {
				t.Errorf("path %q should start with modules/ (not packages/)", pkg.path)
			}
		})
	}
}

func TestLocalAstroPythonPackagesDependencyOrder(t *testing.T) {
	// messaging must be installed before adapter-core, and adapter-core before langchain.
	indexOf := func(name string) int {
		for i, pkg := range localAstroPythonPackages {
			if pkg.name == name {
				return i
			}
		}
		return -1
	}
	msgIdx := indexOf("astropods-messaging")
	coreIdx := indexOf("astropods-adapter-core")
	langchainIdx := indexOf("astropods-adapter-langchain")

	if msgIdx < 0 {
		t.Fatal("astropods-messaging not found in localAstroPythonPackages")
	}
	if coreIdx < 0 {
		t.Fatal("astropods-adapter-core not found in localAstroPythonPackages")
	}
	if langchainIdx < 0 {
		t.Fatal("astropods-adapter-langchain not found in localAstroPythonPackages")
	}
	if msgIdx >= coreIdx {
		t.Errorf("astropods-messaging (index %d) must come before astropods-adapter-core (index %d)", msgIdx, coreIdx)
	}
	if coreIdx >= langchainIdx {
		t.Errorf("astropods-adapter-core (index %d) must come before astropods-adapter-langchain (index %d)", coreIdx, langchainIdx)
	}
}

func TestResolveAstroSourceRoot(t *testing.T) {
	t.Run("missing env var returns error with guidance", func(t *testing.T) {
		t.Setenv("ASTRO_ROOT", "")
		_, err := resolveAstroSourceRoot()
		if err == nil {
			t.Fatal("expected error when ASTRO_ROOT is empty")
		}
		if !strings.Contains(err.Error(), "export ASTRO_ROOT") {
			t.Errorf("error should include export example, got: %v", err)
		}
	})

	t.Run("set env var returns cleaned path", func(t *testing.T) {
		t.Setenv("ASTRO_ROOT", "/home/user/astro/astro/")
		root, err := resolveAstroSourceRoot()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.HasSuffix(root, "/") {
			t.Errorf("path should be cleaned (no trailing slash), got: %q", root)
		}
		if root != "/home/user/astro/astro" {
			t.Errorf("got %q, want /home/user/astro/astro", root)
		}
	})
}
