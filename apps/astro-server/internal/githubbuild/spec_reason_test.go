package githubbuild

import (
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The reason a reader sees for a YAML problem must read as one sentence with the
// line number in the prose, not as the parser's "line N:" behind a second colon.
func TestYAMLSyntaxReason(t *testing.T) {
	var out map[string]any
	err := yaml.Unmarshal([]byte("name: a\n  bad: indent\n"), &out)
	if err == nil {
		t.Fatal("want a parse error from malformed YAML")
	}
	got := yamlSyntaxReason(err)
	if !strings.HasPrefix(got, "astropods.yml has a syntax error on line ") {
		t.Fatalf("reason should lift the line number into the prose, got %q", got)
	}
	if strings.Contains(got, "yaml:") || strings.Count(got, ":") != 1 {
		t.Fatalf("reason should carry one colon and drop the parser prefix, got %q", got)
	}
	if !strings.HasSuffix(got, ".") {
		t.Fatalf("reason should end in a period, got %q", got)
	}
}

// The lead sentence ends in a period, so the validator's own "field: message"
// colon is the only one the reader meets.
func TestSpecInvalidReason(t *testing.T) {
	one := specInvalidReason([]string{"agent: must specify either image or build"})
	if one != "astropods.yml is invalid. agent: must specify either image or build." {
		t.Fatalf("unexpected reason: %q", one)
	}

	// Past the cap, the count of omitted problems points at the log.
	many := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		many = append(many, "agent: must specify either image or build")
	}
	got := specInvalidReason(many)
	if !strings.HasSuffix(got, "The build log lists 4 more.") {
		t.Fatalf("want the omitted count, got %q", got)
	}
	if strings.Count(got, "agent:") != specProblemLimit {
		t.Fatalf("want %d problems listed, got %q", specProblemLimit, got)
	}
}

// A type error never reaches the reader as parser text. yaml.v3 reports a
// top-level shape mismatch with a Go type, a YAML tag, and a slice of the
// reader's own file, none of which belong in an inbox.
func TestYAMLSyntaxReasonDropsParserInternals(t *testing.T) {
	var out map[string]any
	err := yaml.Unmarshal([]byte("just a string\n"), &out)
	if err == nil {
		t.Fatal("want a type error from a scalar at the top level")
	}
	// Guard the premise: this test is worthless if the parser stops emitting the
	// internals it emits today.
	for _, leak := range []string{"map[string]interface {}", "!!str", "just a "} {
		if !strings.Contains(err.Error(), leak) {
			t.Fatalf("parser no longer reports %q, so this test proves nothing: %q", leak, err.Error())
		}
	}

	got := yamlSyntaxReason(err)
	for _, leak := range []string{"map[string]interface {}", "interface {}", "!!str", "unmarshal", "just a "} {
		if strings.Contains(got, leak) {
			t.Fatalf("reason leaks %q to the reader: %q", leak, got)
		}
	}
	if got != "astropods.yml is not valid YAML. Check line 1." {
		t.Fatalf("want the line number and nothing else, got %q", got)
	}
}

// A message with no line number anywhere falls back to a sentence that still
// tells the reader which file to look at.
func TestYAMLSyntaxReasonWithoutALineNumber(t *testing.T) {
	got := yamlSyntaxReason(errors.New("yaml: control characters are not allowed"))
	if got != "astropods.yml is not valid YAML." {
		t.Fatalf("unexpected fallback reason: %q", got)
	}
	if strings.ContainsAny(got, "\n") {
		t.Fatalf("reason must be a single line, got %q", got)
	}
}

// The reason must never quote the reader's own spec. Several yaml.v3 errors embed
// a token from the file: a duplicate mapping key names the key, an unknown anchor
// names the anchor, and a top-level scalar echoes its value. Each case asserts the
// parser still embeds the token, so an upgrade that changes the message cannot let
// this test pass while proving nothing.
func TestYAMLSyntaxReasonNeverQuotesTheSpec(t *testing.T) {
	cases := []struct {
		name  string
		spec  string
		token string // the reader's content, which must not reach the reason
	}{
		{"duplicate mapping key", "agent: a\nagent: b\n", "agent"},
		{"duplicate nested key", "models:\n  gpt: 1\n  gpt: 2\n", "gpt"},
		{"unknown anchor", "agent: *missing\n", "missing"},
		{"scalar at top level", "notaspec\n", "notaspec"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out map[string]any
			err := yaml.Unmarshal([]byte(tc.spec), &out)
			if err == nil {
				t.Fatalf("want a parse error for %q", tc.spec)
			}
			if !strings.Contains(err.Error(), tc.token) {
				t.Fatalf("parser no longer embeds %q, so this case proves nothing: %q", tc.token, err.Error())
			}
			if got := yamlSyntaxReason(err); strings.Contains(got, tc.token) {
				t.Fatalf("reason quotes the reader's spec (%q): %q", tc.token, got)
			}
		})
	}
}
