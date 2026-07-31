//go:build ignore

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astropods/astro-cli/internal/tui/credentials"
)

// ─── quoteDotEnvValue ─────────────────────────────────────────────────────────

func TestQuoteDotEnvValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain value", "hello", `"hello"`},
		{"value with spaces", "hello world", `"hello world"`},
		// Regression: newlines in a value would split the .env line and malform the file.
		{"newline in value", "line1\nline2", `"line1\nline2"`},
		{"carriage return", "a\rb", `"a\rb"`},
		{"crlf", "a\r\nb", `"a\r\nb"`},
		// Regression: unescaped double quotes would terminate the quoted string early.
		{"double quote in value", `say "hi"`, `"say \"hi\""`},
		// Regression: unescaped backslashes would corrupt subsequent escape sequences.
		{"backslash in value", `C:\path\to`, `"C:\\path\\to"`},
		{"backslash and newline", "a\\nb", `"a\\nb"`},
		{"empty value", "", `""`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteDotEnvValue(tt.input)
			if got != tt.want {
				t.Errorf("quoteDotEnvValue(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ─── writeEnv ────────────────────────────────────────────────────────────────

func TestWriteEnv_AppendQuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	creds := []credentials.Credential{
		{Name: "API_KEY", Secret: true},
		{Name: "BASE_URL", Secret: false},
	}
	values := credentials.Result{
		"API_KEY":  "sk-secret",
		"BASE_URL": "https://example.com",
	}

	if err := writeEnv(path, "myprovider", creds, values); err != nil {
		t.Fatalf("writeEnv: %v", err)
	}

	content, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")

	assertLinePresent(t, lines, `API_KEY="sk-secret"`)
	assertLinePresent(t, lines, `BASE_URL="https://example.com"`)
}

// Regression: a value containing a newline must not split into two .env lines,
// which would produce an unquoted fragment on the next line and malform the file.
func TestWriteEnv_NewlineInValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	creds := []credentials.Credential{{Name: "SECRET", Secret: true}}
	values := credentials.Result{"SECRET": "line1\nline2"}

	if err := writeEnv(path, "p", creds, values); err != nil {
		t.Fatalf("writeEnv: %v", err)
	}

	content, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")

	// The entire value must appear on a single line.
	assertLinePresent(t, lines, `SECRET="line1\nline2"`)
	// The raw unescaped fragment must not appear as a bare line.
	for _, l := range lines {
		if l == "line2" {
			t.Error("newline in value produced a spurious bare line 'line2'")
		}
	}
}

// Regression: a value containing double quotes must not terminate the quoted
// string early, leaving trailing content outside the quotes.
func TestWriteEnv_QuoteInValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	creds := []credentials.Credential{{Name: "MSG", Secret: false}}
	values := credentials.Result{"MSG": `say "hello"`}

	if err := writeEnv(path, "p", creds, values); err != nil {
		t.Fatalf("writeEnv: %v", err)
	}

	content, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	assertLinePresent(t, lines, `MSG="say \"hello\""`)
}

// Regression: overwriting an existing key must also quote the new value.
func TestWriteEnv_OverwriteQuotesValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	// Seed the file with an existing plain entry.
	if err := os.WriteFile(path, []byte("API_KEY=old\n"), 0600); err != nil {
		t.Fatal(err)
	}

	creds := []credentials.Credential{{Name: "API_KEY", Secret: true}}
	values := credentials.Result{"API_KEY": "new\nvalue"}

	if err := writeEnv(path, "p", creds, values); err != nil {
		t.Fatalf("writeEnv: %v", err)
	}

	content, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	assertLinePresent(t, lines, `API_KEY="new\nvalue"`)
}

func assertLinePresent(t *testing.T, lines []string, want string) {
	t.Helper()
	for _, l := range lines {
		if l == want {
			return
		}
	}
	t.Errorf("line %q not found in .env; got:\n%s", want, strings.Join(lines, "\n"))
}
