package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	spec "github.com/postman/astro/packages/astro-spec"
)

func TestUpdateSpecVersion(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name        string
		initialYAML string
		version     string
		wantVersion string
	}{
		{
			name: "no meta creates meta with version",
			initialYAML: `
spec: astro/v1
agent: my-agent
container:
  image: myimg:latest
`,
			version:     "0.2.0",
			wantVersion: "0.2.0",
		},
		{
			name: "existing meta version is updated",
			initialYAML: `
spec: astro/v1
agent: my-agent
meta:
  version: "0.1.0"
  description: My agent
container:
  image: myimg:latest
`,
			version:     "0.2.0",
			wantVersion: "0.2.0",
		},
		{
			name: "meta with other fields preserves them",
			initialYAML: `
spec: astro/v1
agent: my-agent
meta:
  version: "0.1.0"
  description: My agent
  owner: myteam
container:
  image: myimg:latest
`,
			version:     "1.0.0",
			wantVersion: "1.0.0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specPath := filepath.Join(dir, "astro.yml")
			if err := os.WriteFile(specPath, []byte(tt.initialYAML), 0644); err != nil {
				t.Fatal(err)
			}
			if err := updateSpecVersion(specPath, tt.version); err != nil {
				t.Fatalf("updateSpecVersion() error = %v", err)
			}
			parsed, err := spec.ParseSpec(specPath)
			if err != nil {
				t.Fatalf("ParseSpec after update: %v", err)
			}
			if parsed.Meta.Version != tt.wantVersion {
				t.Errorf("meta.version = %q, want %q", parsed.Meta.Version, tt.wantVersion)
			}
		})
	}
}

func TestUpdateSpecVersion_invalidPath(t *testing.T) {
	err := updateSpecVersion(filepath.Join(t.TempDir(), "nonexistent", "astro.yml"), "0.1")
	if err == nil {
		t.Error("updateSpecVersion with invalid path should error")
	}
}

// TestUpdateSpecVersion_withAutoTag verifies that when we set version to baseVersion + "-ast_" + tag
// (as with ast publish --version auto), the spec meta.version is updated.
func TestUpdateSpecVersion_withAutoTag(t *testing.T) {
	specYAML := `
spec: astro/v1
agent: my-agent
meta:
  version: "0.1"
  description: My agent
container:
  image: myimg:latest
`
	dateOnlyRE := regexp.MustCompile(`^0\.1-ast_\d{6}\.\d{6}$`)

	dir := t.TempDir()
	specPath := filepath.Join(dir, "astro.yml")
	if err := os.WriteFile(specPath, []byte(specYAML), 0644); err != nil {
		t.Fatal(err)
	}
	parsed, err := spec.ParseSpec(specPath)
	if err != nil {
		t.Fatal(err)
	}
	effectiveVersion := baseVersion(parsed.Meta.Version) + "-ast_" + defaultPublishTag(dir)
	if err := updateSpecVersion(specPath, effectiveVersion); err != nil {
		t.Fatalf("updateSpecVersion() error = %v", err)
	}
	parsed, err = spec.ParseSpec(specPath)
	if err != nil {
		t.Fatalf("ParseSpec after update: %v", err)
	}
	if parsed.Meta.Version != effectiveVersion {
		t.Errorf("meta.version = %q, want %q", parsed.Meta.Version, effectiveVersion)
	}
	if !dateOnlyRE.MatchString(parsed.Meta.Version) {
		t.Errorf("meta.version = %q, want match 0.1-ast_yymmdd.hhmmss", parsed.Meta.Version)
	}
}

func TestBaseVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"0.1", "0.1"},
		{"0.1.0", "0.1.0"},
		{"0.1-abc-123", "0.1-abc-123"},
		{"0.1-ast_260209.135935", "0.1"},
		{"0.1.0-ast_abc1234.260209.135935.dirty", "0.1.0"},
		{"1.2.3-ast_260209.135935", "1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := baseVersion(tt.version)
			if got != tt.want {
				t.Errorf("baseVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

// TestUpdateSpecVersion_autoDoesNotGrow verifies that running --version auto twice does not grow the version string.
func TestUpdateSpecVersion_autoDoesNotGrow(t *testing.T) {
	specYAML := `
spec: astro/v1
agent: my-agent
meta:
  version: "0.1"
container:
  image: myimg:latest
`
	dir := t.TempDir()
	specPath := filepath.Join(dir, "astro.yml")
	if err := os.WriteFile(specPath, []byte(specYAML), 0644); err != nil {
		t.Fatal(err)
	}
	parsed, _ := spec.ParseSpec(specPath)
	v1 := baseVersion(parsed.Meta.Version) + "-ast_" + defaultPublishTag(dir)
	if err := updateSpecVersion(specPath, v1); err != nil {
		t.Fatal(err)
	}
	parsed, _ = spec.ParseSpec(specPath)
	v2 := baseVersion(parsed.Meta.Version) + "-ast_" + defaultPublishTag(dir)
	if err := updateSpecVersion(specPath, v2); err != nil {
		t.Fatal(err)
	}
	parsed, _ = spec.ParseSpec(specPath)
	// Version must be 0.1-ast_<single tag>, not 0.1-ast_<old>-ast_<new>
	if !regexp.MustCompile(`^0\.1-ast_\d{6}\.\d{6}$`).MatchString(parsed.Meta.Version) &&
		!regexp.MustCompile(`^0\.1-ast_[a-f0-9]+\.\d{6}\.\d{6}(\.dirty)?$`).MatchString(parsed.Meta.Version) {
		t.Errorf("meta.version = %q: expected 0.1-ast_<tag> (single -ast_ suffix)", parsed.Meta.Version)
	}
}

func TestDefaultPublishTag(t *testing.T) {
	// Auto tag uses "." to avoid ambiguity with "-" in version (yymmdd.hhmmss, hash.date, hash.date.dirty)
	dateOnlyRE := regexp.MustCompile(`^\d{6}\.\d{6}$`)
	hashDateRE := regexp.MustCompile(`^[a-f0-9]+\.\d{6}\.\d{6}$`)
	hashDateDirtyRE := regexp.MustCompile(`^[a-f0-9]+\.\d{6}\.\d{6}\.dirty$`)

	t.Run("no_git_dir_returns_date_only", func(t *testing.T) {
		dir := t.TempDir()
		got := defaultPublishTag(dir)
		if got == "" {
			t.Fatal("defaultPublishTag() returned empty string")
		}
		if !dateOnlyRE.MatchString(got) {
			t.Errorf("defaultPublishTag() = %q, want match %s (yymmdd.hhmmss)", got, dateOnlyRE)
		}
	})

	t.Run("git_clean_returns_hash_date", func(t *testing.T) {
		dir := t.TempDir()
		gitInit(t, dir)
		gitCommit(t, dir, "init")
		got := defaultPublishTag(dir)
		if got == "" {
			t.Fatal("defaultPublishTag() returned empty string")
		}
		if !hashDateRE.MatchString(got) {
			t.Errorf("defaultPublishTag() = %q, want match %s (shortHash.yymmdd.hhmmss)", got, hashDateRE)
		}
	})

	t.Run("git_dirty_returns_hash_date_dirty", func(t *testing.T) {
		dir := t.TempDir()
		gitInit(t, dir)
		gitCommit(t, dir, "init")
		// Make working tree dirty (modify tracked file; status -uno ignores untracked)
		if err := os.WriteFile(filepath.Join(dir, "file"), []byte("modified"), 0644); err != nil {
			t.Fatal(err)
		}
		got := defaultPublishTag(dir)
		if got == "" {
			t.Fatal("defaultPublishTag() returned empty string")
		}
		if !hashDateDirtyRE.MatchString(got) {
			t.Errorf("defaultPublishTag() = %q, want match %s (shortHash.yymmdd.hhmmss.dirty)", got, hashDateDirtyRE)
		}
	})
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	// Required for commit in some environments
	cmd = exec.Command("git", "config", "user.email", "test@test")
	cmd.Dir = dir
	cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = dir
	cmd.Run()
}

func gitCommit(t *testing.T, dir, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	execIn(t, dir, "git", "add", "file")
	execIn(t, dir, "git", "commit", "-m", message)
}

func execIn(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v %s", name, args, err, out)
	}
}
