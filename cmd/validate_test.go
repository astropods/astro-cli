package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
//
// Asserting on the printed rule as well as the error matters: validateSpecFile
// returns a bare "validation failed" for every reason it has, so err != nil
// alone keeps passing for a spec broken in some way unrelated to the name.
func TestValidateSpecFile_RejectsInvalidAgentName(t *testing.T) {
	tests := []struct {
		name     string
		spec     string
		wantRule string
	}{
		{
			name:     "underscore and punctuation",
			spec:     "spec: blueprint/v1\nname: \"Bad_Name!\"\nagent:\n  image: x\n",
			wantRule: "lowercase alphanumeric with hyphens",
		},
		{
			name:     "single character",
			spec:     "spec: blueprint/v1\nname: \"a\"\nagent:\n  image: x\n",
			wantRule: "at least 2 characters",
		},
		{
			name:     "leading digit",
			spec:     "spec: blueprint/v1\nname: \"1abc\"\nagent:\n  image: x\n",
			wantRule: "start with a letter",
		},
		{
			name:     "trailing hyphen",
			spec:     "spec: blueprint/v1\nname: \"abc-\"\nagent:\n  image: x\n",
			wantRule: "end with alphanumeric",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			specPath := writeSpecFile(t, tc.spec)

			var gotErr error
			out := captureStdout(t, func() {
				_, gotErr = validateSpecFile(specPath)
			})

			require.Error(t, gotErr, "an invalid name must be rejected before any build or push")
			assert.Contains(t, out, tc.wantRule,
				"the output must name the rule the author broke, not just fail")
			assert.Contains(t, out, "does not match pattern",
				"the schema pattern anchors the error to the offending line")
		})
	}
}

func TestValidateSpecFile_AcceptsValidAgentName(t *testing.T) {
	for _, name := range []string{"good-name", "ok-2", "ab"} {
		t.Run(name, func(t *testing.T) {
			specPath := writeSpecFile(t,
				"spec: blueprint/v1\nname: \""+name+"\"\nagent:\n  image: x\n")

			var gotErr error
			out := captureStdout(t, func() {
				_, gotErr = validateSpecFile(specPath)
			})

			assert.NoError(t, gotErr, "%q is a valid name", name)
			assert.Empty(t, out, "a valid spec prints nothing")
		})
	}
}
