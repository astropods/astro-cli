package auth

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
Provenance guard for the keyring escape hatch.

The env var literal "ASTRO_NO_KEYRING" must ONLY appear in files on the
allowlist below. In particular:

  - It must not be referenced from any production command handler (cmd/).
  - It must not be set by the CLI itself, only read.
  - Test-only references live either in this auth package or under e2e/.

If someone later adds an implicit `os.Setenv("ASTRO_NO_KEYRING", "1")` in a
production path — or reads it from a second place where the strict-match
rules don't apply — this test fails and forces a deliberate update of the
allowlist.
*/

var allowedEscapeHatchFiles = map[string]struct{}{
	/* production read site, with strict "1" matching */
	"apps/astro-cli/internal/auth/storage.go": {},
	/* this provenance test itself */
	"apps/astro-cli/internal/auth/keyring_escape_hatch_provenance_test.go": {},
	/* integration tests that set the hatch when driving the built binary */
	"apps/astro-cli/e2e/configure_persistence_test.go": {},
}

func TestKeyringEscapeHatchProvenance(t *testing.T) {
	repoRoot := findAstroRepoRoot(t)
	astroCLI := filepath.Join(repoRoot, "apps", "astro-cli")

	var offenders []string
	err := filepath.WalkDir(astroCLI, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "vendor" || d.Name() == "bin" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), "ASTRO_NO_KEYRING") {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			rel = path
		}
		if _, ok := allowedEscapeHatchFiles[filepath.ToSlash(rel)]; !ok {
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Errorf(
			"ASTRO_NO_KEYRING must only appear in the allowlist. "+
				"If you intentionally added a new reference, update allowedEscapeHatchFiles. "+
				"Unexpected files: %v",
			offenders,
		)
	}
}

// findAstroRepoRoot walks up from the test's working directory looking for
// the apps/astro-cli directory. Keeps this test runnable from any package
// entry point without hardcoding an absolute path.
func findAstroRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "apps", "astro-cli", "moon.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate astro repo root from cwd")
	return ""
}
