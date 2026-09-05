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

func TestRunValidate_ReportsAgentCardAttribution(t *testing.T) {
	const validSpec = "spec: blueprint/v1\nname: demo\nmeta: {}\nagent:\n  image: demo:latest\n"

	t.Run("warns when the card carries no attribution", func(t *testing.T) {
		specPath := writeSpecFile(t, validSpec)
		workingDir := filepath.Dir(specPath)
		if err := os.WriteFile(filepath.Join(workingDir, "AGENT.md"), []byte("---\ndescription: demo\n---\nBody.\n"), 0600); err != nil {
			t.Fatal(err)
		}

		out := captureStdout(t, func() {
			if err := runValidate(specPath, workingDir); err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})

		if !strings.Contains(out, msgAgentCardMissingAuthors()) {
			t.Errorf("expected the authors warning, got: %q", out)
		}
		if !strings.Contains(out, msgAgentCardMissingRepository()) {
			t.Errorf("expected the repository warning, got: %q", out)
		}
	})

	t.Run("stays quiet when the card is complete", func(t *testing.T) {
		specPath := writeSpecFile(t, validSpec)
		workingDir := filepath.Dir(specPath)
		card := "---\ndescription: demo\nauthors:\n  - name: Jane Doe\n    account: janedoe\nrepository: github:astropods/demo\n---\nBody.\n"
		if err := os.WriteFile(filepath.Join(workingDir, "AGENT.md"), []byte(card), 0600); err != nil {
			t.Fatal(err)
		}

		out := captureStdout(t, func() {
			if err := runValidate(specPath, workingDir); err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})

		if strings.Contains(out, "AGENT.md") {
			t.Errorf("expected no AGENT.md warning, got: %q", out)
		}
	})
}
