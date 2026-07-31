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

// TestNoBlockCommentsInCLI walks the astro-cli module and fails if any Go
// source file contains a `/* ... */` comment. `//` is the house style for
// both single-line and multi-line comments; this test is the enforcement
// because no standard Go linter catches it.
func TestNoBlockCommentsInCLI(t *testing.T) {
	root := findModuleRoot(t)
	target := root

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
					rel, relErr := filepath.Rel(root, pos.Filename)
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
			"block comments (/* ... */) are banned in the astro-cli module; use // instead.\n"+
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

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate module root from cwd")
	return ""
}
