package config

import (
	"os"
	"path/filepath"
	"testing"
)

// isolatedConfig sets up a temp HOME so config writes land in a sandbox, and
// returns the binaryName that resolves under it.
func isolatedConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	binaryName := filepath.Base(dir)
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	t.Cleanup(func() { _ = os.Setenv("HOME", origHome) })
	return binaryName
}

func TestMergeProjectVars_TrimsWhitespace(t *testing.T) {
	binaryName := isolatedConfig(t)

	err := MergeProjectVars(binaryName, "/fake/project", "my-agent", map[string]string{
		"API_KEY":     "  sk-abc123  ",
		"SLACK_TOKEN": "\tbot-token\n",
	})
	if err != nil {
		t.Fatalf("MergeProjectVars error: %v", err)
	}

	vars := GetProjectVars(binaryName, "/fake/project")

	if got, want := vars["API_KEY"], "sk-abc123"; got != want {
		t.Errorf("API_KEY = %q, want %q", got, want)
	}
	if got, want := vars["SLACK_TOKEN"], "bot-token"; got != want {
		t.Errorf("SLACK_TOKEN = %q, want %q", got, want)
	}
}

// TestMergeProjectVars_SkipEmpty locks the regression fix: a configure submit
// of an empty or whitespace-only field must NOT clobber the stored value.
// The interactive form always submits every field, so this is what keeps
// untouched/blanked secrets around across repeated configure runs.
func TestMergeProjectVars_SkipEmpty(t *testing.T) {
	binaryName := isolatedConfig(t)

	if err := MergeProjectVars(binaryName, "/fake/project", "my-agent", map[string]string{
		"FOO": "bar",
		"BAZ": "qux",
	}); err != nil {
		t.Fatalf("initial MergeProjectVars error: %v", err)
	}

	if err := MergeProjectVars(binaryName, "/fake/project", "my-agent", map[string]string{
		"FOO":   "",      // untouched secret -> must preserve
		"BAZ":   "   \n", // whitespace-only -> must preserve
		"OTHER": "",      // new key with empty value -> must NOT be created
	}); err != nil {
		t.Fatalf("second MergeProjectVars error: %v", err)
	}

	vars := GetProjectVars(binaryName, "/fake/project")
	if got, want := vars["FOO"], "bar"; got != want {
		t.Errorf("FOO = %q, want %q (empty submit must not overwrite)", got, want)
	}
	if got, want := vars["BAZ"], "qux"; got != want {
		t.Errorf("BAZ = %q, want %q (whitespace-only submit must not overwrite)", got, want)
	}
	if _, ok := vars["OTHER"]; ok {
		t.Errorf("OTHER should not be stored when submitted as empty, got %q", vars["OTHER"])
	}
}

// TestUnsetProjectVars_ExplicitUnset exercises the explicit unset path that
// replaces the old "empty value clears key" behavior.
func TestUnsetProjectVars_ExplicitUnset(t *testing.T) {
	binaryName := isolatedConfig(t)

	if err := MergeProjectVars(binaryName, "/fake/project", "my-agent", map[string]string{
		"FOO": "1",
		"BAR": "2",
	}); err != nil {
		t.Fatalf("MergeProjectVars error: %v", err)
	}

	if err := UnsetProjectVars(binaryName, "/fake/project", []string{"FOO"}); err != nil {
		t.Fatalf("UnsetProjectVars error: %v", err)
	}

	vars := GetProjectVars(binaryName, "/fake/project")
	if _, ok := vars["FOO"]; ok {
		t.Errorf("FOO should have been removed, got %q", vars["FOO"])
	}
	if vars["BAR"] != "2" {
		t.Errorf("BAR = %q, want 2 (unrelated key should be preserved)", vars["BAR"])
	}
}

// TestProjectConfigs_FilePermissions locks the file mode of
// project-configs.json to owner read/write only. The file holds user
// secrets (API keys, tokens) in plaintext on disk; a permissions regression
// would expose them to other local users on shared machines.
func TestProjectConfigs_FilePermissions(t *testing.T) {
	binaryName := isolatedConfig(t)

	if err := MergeProjectVars(binaryName, "/some/project", "my-agent", map[string]string{
		"API_KEY": "sk-secret",
	}); err != nil {
		t.Fatalf("MergeProjectVars: %v", err)
	}

	path, err := ConfigsPath(binaryName)
	if err != nil {
		t.Fatalf("ConfigsPath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if perms := info.Mode().Perm(); perms != 0o600 {
		t.Errorf("project-configs.json perms = %04o, want 0600 (owner read/write only)", perms)
	}
}

// TestProjectPath_LegacyUnnormalizedKeyReadable guards the backward-compat
// path: config files written by older CLIs may contain un-canonicalized path
// keys (e.g. "/var/folders/..."). A fresh `ast configure` that canonicalizes
// to "/private/var/..." must still find them via the fallback lookup.
func TestProjectPath_LegacyUnnormalizedKeyReadable(t *testing.T) {
	binaryName := isolatedConfig(t)

	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	if resolved == dir {
		t.Skip("temp dir is already canonical (no symlink component) — cannot exercise fallback")
	}

	if err := writeRawProjectConfig(binaryName, dir, "my-agent", map[string]string{
		"LEGACY_KEY": "hello",
	}); err != nil {
		t.Fatalf("seed legacy config: %v", err)
	}

	if got := GetProjectVars(binaryName, resolved)["LEGACY_KEY"]; got != "hello" {
		t.Errorf("fallback lookup for legacy key failed: got %q, want %q", got, "hello")
	}

	if err := MergeProjectVars(binaryName, resolved, "my-agent", map[string]string{
		"NEW_KEY": "world",
	}); err != nil {
		t.Fatalf("MergeProjectVars: %v", err)
	}

	vars := GetProjectVars(binaryName, resolved)
	if vars["LEGACY_KEY"] != "hello" {
		t.Errorf("LEGACY_KEY lost after merge: %q", vars["LEGACY_KEY"])
	}
	if vars["NEW_KEY"] != "world" {
		t.Errorf("NEW_KEY not stored: %q", vars["NEW_KEY"])
	}
}

// writeRawProjectConfig seeds project-configs.json with a specific (raw,
// unnormalized) key so tests can simulate entries written by earlier CLIs.
func writeRawProjectConfig(binaryName, rawKey, agentName string, vars map[string]string) error {
	cfg, err := LoadProjectConfigs(binaryName)
	if err != nil {
		return err
	}
	cfg.Projects[rawKey] = &ProjectConfig{Name: agentName, Vars: vars}
	return SaveProjectConfigs(binaryName, cfg)
}

// TestProjectPath_CreateMatchesConfigure exercises the create→configure hand-off:
// `ast create` stores vars keyed by filepath.Abs(targetDir), `ast configure` later
// reads/writes them keyed by os.Getwd(). On a real user flow the two must resolve
// to the same string so configure sees (and preserves) what create saved.
//
// Regression guard: if the two ever drift (symlink resolution, trailing slash,
// double-slash collapsing, etc.) configure would start with an empty store and
// appear to "lose" the values set during create.
func TestProjectPath_CreateMatchesConfigure(t *testing.T) {
	binaryName := isolatedConfig(t)

	// Build a parent dir + child dir, mirroring `ast create --path <parent> <name>`.
	parent := t.TempDir()
	targetDir := filepath.Join(parent, "my-agent")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Simulate create: resolve targetDir via filepath.Abs (as create.go does).
	createKey, err := filepath.Abs(targetDir)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if err := MergeProjectVars(binaryName, createKey, "my-agent", map[string]string{
		"API_KEY": "sk-from-create",
	}); err != nil {
		t.Fatalf("create MergeProjectVars: %v", err)
	}

	// Simulate configure: cd into the project directory and use os.Getwd().
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origCwd) })
	if err := os.Chdir(targetDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	configureKey, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd after chdir: %v", err)
	}

	if got := GetProjectVars(binaryName, configureKey)["API_KEY"]; got != "sk-from-create" {
		t.Errorf("configure read API_KEY = %q, want %q (create=%q configure=%q)",
			got, "sk-from-create", createKey, configureKey)
	}
}
