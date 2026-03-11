package cmd

import (
	"slices"
	"strings"
	"testing"
)

func TestComposeBuildArgs(t *testing.T) {
	tests := []struct {
		name          string
		rebuild       bool
		noPull        bool
		wantPull      bool
		wantPullFalse bool
		wantNoCache   bool
	}{
		{
			name:          "normal mode pulls base images",
			rebuild:       false,
			noPull:        false,
			wantPull:      true,
			wantPullFalse: false,
			wantNoCache:   false,
		},
		{
			name:          "local mode explicitly disables pull",
			rebuild:       false,
			noPull:        true,
			wantPull:      false,
			wantPullFalse: true,
			wantNoCache:   false,
		},
		{
			name:          "rebuild disables cache and pulls",
			rebuild:       true,
			noPull:        false,
			wantPull:      true,
			wantPullFalse: false,
			wantNoCache:   true,
		},
		{
			name:          "rebuild + no-pull disables cache and pull",
			rebuild:       true,
			noPull:        true,
			wantPull:      false,
			wantPullFalse: true,
			wantNoCache:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := composeBuildArgs("test-compose.yml", tt.rebuild, tt.noPull)

			hasPull := slices.Contains(args, "--pull")
			if hasPull != tt.wantPull {
				t.Errorf("--pull present = %v, want %v (args: %v)", hasPull, tt.wantPull, args)
			}

			hasPullFalse := slices.Contains(args, "--pull=false")
			if hasPullFalse != tt.wantPullFalse {
				t.Errorf("--pull=false present = %v, want %v (args: %v)", hasPullFalse, tt.wantPullFalse, args)
			}

			if hasPull && hasPullFalse {
				t.Errorf("--pull and --pull=false are mutually exclusive (args: %v)", args)
			}

			hasNoCache := slices.Contains(args, "--no-cache")
			if hasNoCache != tt.wantNoCache {
				t.Errorf("--no-cache present = %v, want %v (args: %v)", hasNoCache, tt.wantNoCache, args)
			}
		})
	}
}

func TestDevLogsArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		all         bool
		wantService string
	}{
		{
			name:        "default tails agent",
			args:        nil,
			all:         false,
			wantService: "agent",
		},
		{
			name:        "all flag tails everything",
			args:        nil,
			all:         true,
			wantService: "",
		},
		{
			name:        "explicit service overrides default",
			args:        []string{"astro-messaging"},
			all:         false,
			wantService: "astro-messaging",
		},
		{
			name:        "explicit service overrides all",
			args:        []string{"playground"},
			all:         true,
			wantService: "playground",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := devLogsArgs("test-compose.yml", tt.args, tt.all)

			baseArgs := []string{"compose", "-f", "test-compose.yml", "logs", "-f"}
			for _, base := range baseArgs {
				if !slices.Contains(args, base) {
					t.Errorf("missing base arg %q in %v", base, args)
				}
			}

			lastArg := args[len(args)-1]
			if tt.wantService == "" {
				if lastArg != "-f" {
					t.Errorf("--all should not append a service, got trailing arg %q (args: %v)", lastArg, args)
				}
			} else {
				if lastArg != tt.wantService {
					t.Errorf("last arg = %q, want %q (args: %v)", lastArg, tt.wantService, args)
				}
			}
		})
	}
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
	for _, img := range localDockerImages {
		t.Run(img.tag, func(t *testing.T) {
			if !strings.HasSuffix(img.tag, ":latest") {
				t.Errorf("tag %q should end with :latest", img.tag)
			}

			if !strings.HasPrefix(img.dockerfile, "modules/") {
				t.Errorf("dockerfile %q should start with modules/", img.dockerfile)
			}

			if !strings.HasPrefix(img.context, "modules/") {
				t.Errorf("context %q should start with modules/", img.context)
			}

			if !strings.HasPrefix(img.dockerfile, img.context) {
				t.Errorf("dockerfile %q should be inside context %q", img.dockerfile, img.context)
			}

			tagName := strings.TrimSuffix(img.tag, ":latest")
			contextDir := img.context[strings.LastIndex(img.context, "/")+1:]
			if tagName != contextDir {
				t.Errorf("tag name %q doesn't match context dir %q", tagName, contextDir)
			}
		})
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
