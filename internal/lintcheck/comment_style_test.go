// Package lintcheck enforces repo-wide style rules that aren't covered by
// the standard Go linters. Currently: ban `/* */` comments in favor of `//`.
package lintcheck

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestNoBlockCommentsInCLI walks apps/astro-cli and fails if any Go source
// file contains a `/* ... */` comment. `//` is the house style for both
// single-line and multi-line comments; this test is the enforcement because
// no standard Go linter catches it.
//
// Rule is scoped to apps/astro-cli for now; extend the walk root once other
// Go modules adopt it.
func TestNoBlockCommentsInCLI(t *testing.T) {
	repoRoot := findRepoRoot(t)
	target := filepath.Join(repoRoot, "apps", "astro-cli")

	fset := token.NewFileSet()
	var offenders []string

	walkErr := filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return nil
		}
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(c.Text, "/*") {
					pos := fset.Position(c.Pos())
					rel, relErr := filepath.Rel(repoRoot, pos.Filename)
					if relErr != nil {
						rel = pos.Filename
					}
					offenders = append(offenders, rel+":"+strconv.Itoa(pos.Line))
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if len(offenders) > 0 {
		t.Errorf(
			"block comments (/* ... */) are banned in apps/astro-cli; use // instead.\n"+
				"Offenders:\n  %s",
			strings.Join(offenders, "\n  "),
		)
	}
}

func skipDir(name string) bool {
	switch name {
	case "vendor", "bin", "node_modules", ".moon":
		return true
	}
	return false
}

func findRepoRoot(t *testing.T) string {
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
	t.Fatalf("could not locate repo root from cwd")
	return ""
}
