package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSpecFile creates a spec file in a tempdir and returns its path.
func writeSpecFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "astropods.yml")
	if err := os.WriteFile(specPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return specPath
}

func TestValidateSpecFile_Valid(t *testing.T) {
	specPath := writeSpecFile(t, "spec: package/v1\nname: demo\nmeta: {}\nagent:\n  image: demo:latest\n")

	var (
		gotErr error
		name   string
	)
	out := captureStdout(t, func() {
		p, err := validateSpecFile(specPath)
		gotErr = err
		if p != nil {
			name = p.Name
		}
	})

	if gotErr != nil {
		t.Fatalf("expected no error, got: %v", gotErr)
	}
	if name != "demo" {
		t.Errorf("expected name 'demo', got %q", name)
	}
	if out != "" {
		t.Errorf("expected no output on success, got: %q", out)
	}
}

func TestValidateSpecFile_MissingRequiredField(t *testing.T) {
	// Missing top-level `meta` (required by schema)
	specPath := writeSpecFile(t, "spec: package/v1\nname: demo\nagent:\n  image: demo:latest\n")

	var gotErr error
	out := captureStdout(t, func() {
		_, gotErr = validateSpecFile(specPath)
	})

	if gotErr == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(out, "meta") {
		t.Errorf("expected 'meta' in error output, got: %q", out)
	}
}

func TestValidateSpecFile_SemanticError(t *testing.T) {
	// Agent with both image and build is semantically invalid (mutually exclusive)
	specPath := writeSpecFile(t, `spec: package/v1
name: demo
meta: {}
agent:
  image: demo:latest
  build:
    context: .
    dockerfile: Dockerfile
`)

	var gotErr error
	out := captureStdout(t, func() {
		_, gotErr = validateSpecFile(specPath)
	})

	if gotErr == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(out, "mutually exclusive") {
		t.Errorf("expected mutual-exclusion error in output, got: %q", out)
	}
}

func TestValidateSpecFile_YAMLSyntaxError(t *testing.T) {
	// Unterminated flow sequence — YAML parse must fail.
	specPath := writeSpecFile(t, "spec: package/v1\nname: demo\nmeta: {}\nagent:\n  image: [unclosed\n")

	var gotErr error
	out := captureStdout(t, func() {
		_, gotErr = validateSpecFile(specPath)
	})

	if gotErr == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(out, "YAML syntax error") {
		t.Errorf("expected YAML syntax error in output, got: %q", out)
	}
}
