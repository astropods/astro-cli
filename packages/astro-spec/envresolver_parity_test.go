package spec

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoLocalCredentialNameDerivation_OutsideSpecPackage is a structural guard
// that fails CI if either astro-server or astro-cli reintroduces local
// credential env-var name derivation outside the spec package. The pattern
// being banned is the §8.1-shaped string concat:
//
//	... + cs.Suffix
//	... + "_" + cred.Suffix
//
// Anything outside packages/astro-spec/ that wants credential env-var names
// MUST call CloudCredentialKeys or CustomProviderCredentialKeys. No
// exceptions — that's how dev/prod parity is enforced.
//
// Test is skipped when the repo layout isn't visible (e.g. installed-module
// builds). It runs in repo-checkout CI and that's where drift matters.
func TestNoLocalCredentialNameDerivation_OutsideSpecPackage(t *testing.T) {
	repoRoot, ok := findRepoRoot()
	if !ok {
		t.Skip("repo root not visible (running outside checkout); skipping structural guard")
	}

	banned := []*regexp.Regexp{
		regexp.MustCompile(`\+\s*cs\.Suffix\b`),
		regexp.MustCompile(`\+\s*[a-zA-Z_]+\.Suffix\b`),
	}

	roots := []string{
		filepath.Join(repoRoot, "apps", "astro-server"),
		filepath.Join(repoRoot, "apps", "astro-cli"),
	}

	type hit struct {
		path string
		line int
		text string
		pat  string
	}
	var hits []hit

	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				base := filepath.Base(path)
				if base == "vendor" || base == ".git" || base == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, readErr := os.ReadFile(path) //nolint:gosec
			if readErr != nil {
				return readErr
			}
			for i, line := range strings.Split(string(data), "\n") {
				for _, re := range banned {
					if re.MatchString(line) {
						hits = append(hits, hit{
							path: path, line: i + 1, text: strings.TrimSpace(line), pat: re.String(),
						})
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	if len(hits) > 0 {
		t.Errorf("found %d local credential-name derivation site(s) outside packages/astro-spec/. Use CloudCredentialKeys or CustomProviderCredentialKeys instead:", len(hits))
		for _, h := range hits {
			rel, _ := filepath.Rel(repoRoot, h.path)
			t.Errorf("  %s:%d (matched /%s/): %s", rel, h.line, h.pat, h.text)
		}
	}
}

// findRepoRoot walks up from the test working directory looking for the
// monorepo's marker layout (both apps/astro-server/ and apps/astro-cli/
// present under a common parent). Returns the parent if found.
func findRepoRoot() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := cwd
	for {
		_, serverErr := os.Stat(filepath.Join(dir, "apps", "astro-server"))
		_, cliErr := os.Stat(filepath.Join(dir, "apps", "astro-cli"))
		if serverErr == nil && cliErr == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
