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
	specPath := writeSpecFile(t, "spec: blueprint/v1\nname: demo\nmeta: {}\nagent:\n  image: demo:latest\n")

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
	// Missing top-level `agent` (required by schema)
	specPath := writeSpecFile(t, "spec: blueprint/v1\nname: demo\n")

	var gotErr error
	out := captureStdout(t, func() {
		_, gotErr = validateSpecFile(specPath)
	})

	if gotErr == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(out, "agent") {
		t.Errorf("expected 'agent' in error output, got: %q", out)
	}
}

func TestValidateSpecFile_SemanticError(t *testing.T) {
	// Agent with both image and build is semantically invalid (mutually exclusive)
	specPath := writeSpecFile(t, `spec: blueprint/v1
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
	specPath := writeSpecFile(t, "spec: blueprint/v1\nname: demo\nmeta: {}\nagent:\n  image: [unclosed\n")

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

// The name check lives in astro-spec's ParseSpec, so it only reaches the CLI
// through go.mod. A pin that predates it compiles and passes every other test
// while letting an invalid name through to a 400 after build and push.
func TestValidateSpecFile_RejectsInvalidAgentName(t *testing.T) {
	// validateSpecFile prints the rule and the offending line to stdout and
	// returns a generic error, so the assertion is that it refuses at all.
	tests := []struct {
		name string
		spec string
	}{
		{name: "underscore and punctuation", spec: "spec: blueprint/v1\nname: \"Bad_Name!\"\nagent:\n  image: x\n"},
		{name: "single character", spec: "spec: blueprint/v1\nname: \"a\"\nagent:\n  image: x\n"},
		{name: "leading digit", spec: "spec: blueprint/v1\nname: \"1abc\"\nagent:\n  image: x\n"},
		{name: "trailing hyphen", spec: "spec: blueprint/v1\nname: \"abc-\"\nagent:\n  image: x\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateSpecFile(writeSpecFile(t, tc.spec)); err == nil {
				t.Fatal("an invalid name must be rejected before any build or push")
			}
		})
	}
}

func TestValidateSpecFile_AcceptsValidAgentName(t *testing.T) {
	for _, name := range []string{"good-name", "ok-2", "ab"} {
		spec := "spec: blueprint/v1\nname: \"" + name + "\"\nagent:\n  image: x\n"
		if _, err := validateSpecFile(writeSpecFile(t, spec)); err != nil {
			t.Errorf("%q is a valid name but was rejected: %v", name, err)
		}
	}
}
