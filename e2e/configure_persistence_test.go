//go:build integration

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Integration test for the "ast configure not persisting values after upgrade"
// regression. We build the ast binary and drive it as a subprocess so the full
// CLI path (cobra -> runConfigureFlags -> MergeProjectVars -> project-configs.json)
// is exercised end-to-end, not just the internal helpers.
//
// Covers four user scenarios tied to the fix:
//
//  1. `ast project configure --var KEY=VALUE` persists the value.
//  2. Re-running with a different key does not clobber the first.
//  3. `ast project configure --var KEY=` stores an empty string for the key.
//  4. `ast project configure --rm-var KEY` removes the key.
//  5. Running the binary from the same project via both a raw and a
//     symlink-resolved path still resolves to the same store (path-key
//     canonicalization).
//
// Run with:
//
//	go test -tags integration -run TestConfigurePersistence ./e2e/...

type projectVarsFile struct {
	Projects map[string]struct {
		Name string            `json:"name"`
		Vars map[string]string `json:"vars"`
	} `json:"projects"`
}

// buildAstBinary compiles the CLI with binaryName=ast-dev. auth.ConfigDir
// hard-codes the binary→dir mapping (ast-dev → .ast-dev), so any other
// name would silently fall back to .ast and make the test leak into the
// developer's real config.
func buildAstBinary(t *testing.T) string {
	t.Helper()
	repoRoot := findRepoRoot(t)
	cliDir := filepath.Join(repoRoot, "apps", "astro-cli")
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "ast-dev")

	cmd := exec.Command("go", "build",
		"-ldflags=-X github.com/astropods/astro-cli/internal/buildinfo.BinaryName=ast-dev",
		"-o", binPath, ".")
	cmd.Dir = cliDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return binPath
}

// astConfigSubdir is the directory auth.ConfigDir creates for the ast-dev
// binary under $HOME. Tests use this to locate project-configs.json.
const astConfigSubdir = ".ast-dev"

func findRepoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeMinimalSpec(t *testing.T, dir, name string) {
	t.Helper()
	spec := `spec: "1.0"
name: "` + name + `"
meta:
  description: integration test fixture
agent:
  image: nginx:alpine
`
	if err := os.WriteFile(filepath.Join(dir, "astropods.yml"), []byte(spec), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
}

// runAst runs the built binary from projectDir with HOME set to tmpHome and
// returns combined stdout/stderr. Fails the test if the command exits non-zero.
func runAst(t *testing.T, binPath, projectDir, tmpHome string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"HOME="+tmpHome,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ast %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func readProjectConfigs(t *testing.T, tmpHome string) projectVarsFile {
	t.Helper()
	path := filepath.Join(tmpHome, astConfigSubdir, "project-configs.json")
	data, err := os.ReadFile(path) //nolint:gosec // controlled path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cfg projectVarsFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal %s: %v\n%s", path, err, data)
	}
	return cfg
}

func projectVarsFor(t *testing.T, cfg projectVarsFile, projectDir string) map[string]string {
	t.Helper()
	// Keys are canonical (symlink-resolved) paths. Some temp dir paths are
	// already canonical; on macOS /var/folders resolves to /private/var.
	// Try canonical first, fall back to raw.
	resolved, err := filepath.EvalSymlinks(projectDir)
	if err == nil {
		if entry, ok := cfg.Projects[resolved]; ok {
			return entry.Vars
		}
	}
	if entry, ok := cfg.Projects[projectDir]; ok {
		return entry.Vars
	}
	return nil
}

// TestConfigurePersistence_SetPersistsValues locks the regression: --var must
// write the value and a subsequent --var on a different key must not clobber
// the first. This is the user's primary complaint ("ast configure not
// persisting values since upgrading the cli").
func TestConfigurePersistence_SetPersistsValues(t *testing.T) {
	binPath := buildAstBinary(t)
	tmpHome := t.TempDir()
	projectDir := t.TempDir()
	writeMinimalSpec(t, projectDir, "@org/my-agent")

	runAst(t, binPath, projectDir, tmpHome, "project", "configure", "--var", "API_KEY=sk-1")
	runAst(t, binPath, projectDir, tmpHome, "project", "configure", "--var", "OTHER_KEY=other-1")

	vars := projectVarsFor(t, readProjectConfigs(t, tmpHome), projectDir)
	if vars["API_KEY"] != "sk-1" {
		t.Errorf("API_KEY = %q, want %q", vars["API_KEY"], "sk-1")
	}
	if vars["OTHER_KEY"] != "other-1" {
		t.Errorf("OTHER_KEY = %q, want %q (second --var must not clobber first)", vars["OTHER_KEY"], "other-1")
	}
}

// TestConfigurePersistence_VarEmptyValueIsStored confirms --var KEY= stores an
// empty string for the key, replacing any existing value. This is distinct from
// --rm-var KEY which removes the key entirely.
func TestConfigurePersistence_VarEmptyValueIsStored(t *testing.T) {
	binPath := buildAstBinary(t)
	tmpHome := t.TempDir()
	projectDir := t.TempDir()
	writeMinimalSpec(t, projectDir, "@org/my-agent")

	runAst(t, binPath, projectDir, tmpHome, "project", "configure", "--var", "API_KEY=sk-1")
	runAst(t, binPath, projectDir, tmpHome, "project", "configure", "--var", "API_KEY=")

	vars := projectVarsFor(t, readProjectConfigs(t, tmpHome), projectDir)
	if v, ok := vars["API_KEY"]; !ok {
		t.Error("API_KEY should be present after --var KEY= (stored as empty string)")
	} else if v != "" {
		t.Errorf("API_KEY = %q, want empty string", v)
	}
}

// TestConfigurePersistence_UnsetRemovesKey exercises the --rm-var flag which
// explicitly removes a key from the store.
func TestConfigurePersistence_UnsetRemovesKey(t *testing.T) {
	binPath := buildAstBinary(t)
	tmpHome := t.TempDir()
	projectDir := t.TempDir()
	writeMinimalSpec(t, projectDir, "@org/my-agent")

	runAst(t, binPath, projectDir, tmpHome, "project", "configure", "--var", "API_KEY=sk-1", "--var", "OTHER=keep")
	runAst(t, binPath, projectDir, tmpHome, "project", "configure", "--rm-var", "API_KEY")

	vars := projectVarsFor(t, readProjectConfigs(t, tmpHome), projectDir)
	if _, ok := vars["API_KEY"]; ok {
		t.Errorf("API_KEY should be removed by --rm-var, still present: %q", vars["API_KEY"])
	}
	if vars["OTHER"] != "keep" {
		t.Errorf("OTHER = %q, want keep (unrelated key must survive)", vars["OTHER"])
	}
}

// TestConfigurePersistence_DoesNotEchoSecret verifies that configure output
// never contains the value passed via --var. Secrets are the primary payload
// of this store — if the happy-path banner ever leaked the value it would end
// up in terminal scrollback, shell history screenshots, and CI logs.
func TestConfigurePersistence_DoesNotEchoSecret(t *testing.T) {
	binPath := buildAstBinary(t)
	tmpHome := t.TempDir()
	projectDir := t.TempDir()
	writeMinimalSpec(t, projectDir, "@org/my-agent")

	const sentinelSecret = "sk-DO-NOT-ECHO-12345"
	out := runAst(t, binPath, projectDir, tmpHome, "project", "configure", "--var", "API_KEY="+sentinelSecret)

	if strings.Contains(out, sentinelSecret) {
		t.Errorf("configure --var echoed the secret value back to stdout/stderr:\n%s", out)
	}
}

// TestConfigurePersistence_SymlinkPathConsistency guards the macOS
// /var -> /private/var issue that previously caused `ast create`'s stored
// vars to be invisible to a subsequent `ast configure` run. The test creates
// a symlink, runs configure through one path, then through the other, and
// verifies a single store entry.
func TestConfigurePersistence_SymlinkPathConsistency(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
	binPath := buildAstBinary(t)
	tmpHome := t.TempDir()
	realDir := t.TempDir()
	writeMinimalSpec(t, realDir, "@org/my-agent")

	linkParent := t.TempDir()
	linkDir := filepath.Join(linkParent, "aliased")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	runAst(t, binPath, realDir, tmpHome, "project", "configure", "--var", "API_KEY=sk-real")
	runAst(t, binPath, linkDir, tmpHome, "project", "configure", "--var", "EXTRA=added-via-symlink")

	cfg := readProjectConfigs(t, tmpHome)
	if len(cfg.Projects) != 1 {
		t.Errorf("expected exactly one project entry after canonicalization, got %d: %v", len(cfg.Projects), cfg.Projects)
	}
	vars := projectVarsFor(t, cfg, realDir)
	if vars["API_KEY"] != "sk-real" {
		t.Errorf("API_KEY = %q, want sk-real", vars["API_KEY"])
	}
	if vars["EXTRA"] != "added-via-symlink" {
		t.Errorf("EXTRA = %q, want added-via-symlink (symlinked path must resolve to same store entry)", vars["EXTRA"])
	}
}
