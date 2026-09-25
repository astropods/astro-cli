package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	composeBuilder "github.com/astropods/astro-cli/internal/compose"
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
		p, _, err := validateSpecFile(os.Stdout, specPath)
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
		_, _, gotErr = validateSpecFile(os.Stdout, specPath)
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
		_, _, gotErr = validateSpecFile(os.Stdout, specPath)
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
		_, _, gotErr = validateSpecFile(os.Stdout, specPath)
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
				_, _, gotErr = validateSpecFile(os.Stdout, specPath)
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
				_, _, gotErr = validateSpecFile(os.Stdout, specPath)
			})

			assert.NoError(t, gotErr, "%q is a valid name", name)
			assert.Empty(t, out, "a valid spec prints nothing")
		})
	}
}

// watchSpec writes a spec whose dev.watch is a block list, returning the spec
// path and the line number of its first entry. No directories are created:
// validate must not care. Entries are quoted, so a line's text is the YAML
// spelling rather than the parsed value; the line number is what a caller can
// assert against either way.
func watchSpec(t *testing.T, watch ...string) (string, int) {
	t.Helper()
	var b strings.Builder
	b.WriteString("spec: blueprint/v1\nname: demo\nmeta: {}\nagent:\n  image: demo:latest\ndev:\n  watch:\n")
	for _, w := range watch {
		fmt.Fprintf(&b, "    - %q\n", w)
	}
	content := b.String()

	firstEntry := 0
	for i, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "    - ") {
			firstEntry = i + 1
			break
		}
	}
	require.NotZero(t, firstEntry, "the helper must be able to find its own first entry")
	return writeSpecFile(t, content), firstEntry
}

// validateWatch runs the full validate path against a spec and returns its
// output. The writer is a buffer, so nothing has to intercept stdout.
func validateWatch(t *testing.T, specPath string) (string, error) {
	t.Helper()
	var out strings.Builder
	err := runValidate(&out, specPath)
	return out.String(), err
}

func TestRunValidate_DoesNotCheckWhetherAWatchDirExists(t *testing.T) {
	specPath, _ := watchSpec(t, "agent", "packages/core", "srcc")

	out, err := validateWatch(t, specPath)

	require.NoError(t, err,
		"whether a directory exists is a property of the checkout, not of the spec")
	assert.Contains(t, out, "is valid")
	assert.NotContains(t, out, "srcc",
		"a directory absent from this tree is 'project start' business, not validate's")
}

func TestRunValidate_RejectsAMalformedWatchEntry(t *testing.T) {
	tests := []struct {
		name   string
		entry  string
		reason string
	}{
		{name: "a backslash separator", entry: `packages\core`, reason: `a backslash is not a path separator here, use /`},
		{name: "an absolute path", entry: "/etc", reason: "an absolute path would mount something outside the project"},
		{name: "a climb out", entry: "../sibling", reason: "the path escapes the project"},
		{name: "the project root", entry: ".", reason: "the project root would hide everything the image built"},
		{name: "a dot component", entry: "./agent", reason: "a . path component is not allowed, name the directory directly"},
		{name: "an empty component", entry: "a//b", reason: "an empty path component names no directory"},
		{name: "a drive letter", entry: "C:/x", reason: "a drive letter cannot be mounted"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specPath, firstEntry := watchSpec(t, "agent", tt.entry)

			out, err := validateWatch(t, specPath)

			require.Error(t, err, "a malformed entry cannot work in any checkout, so it fails")
			assert.Contains(t, out, composeBuilder.MsgWatchDirRejected(tt.entry, tt.reason),
				"the reason comes from the builder, so one rule keeps one wording")
			assert.Regexp(t, fmt.Sprintf(`>\s+%d │`, firstEntry+1), out,
				"the highlighted line must be the offending entry's own, not the valid one above it")
			assert.NotContains(t, out, "is valid")
		})
	}
}

func TestRunValidate_WarnsWithoutFailingOnARepeatedWatchEntry(t *testing.T) {
	specPath, _ := watchSpec(t, "agent", "agent", "packages/core/")

	out, err := validateWatch(t, specPath)

	require.NoError(t, err, "the builder mounts a repeat once, so it is not a failure")
	assert.Contains(t, out, composeBuilder.MsgWatchDirDuplicate("agent"))
	assert.Contains(t, out, "is valid")
}

func TestRunValidate_IgnoresASpecThatNamesNoWatchDir(t *testing.T) {
	specPath := writeSpecFile(t, "spec: blueprint/v1\nname: demo\nmeta: {}\nagent:\n  image: demo:latest\n")

	out, err := validateWatch(t, specPath)

	require.NoError(t, err)
	assert.NotContains(t, out, "dev.watch",
		"the implicit agent/ default is not the author's claim, so it is not checked")
}
