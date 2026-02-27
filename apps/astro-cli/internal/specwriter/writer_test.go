package specwriter

import (
	"os"
	"strings"
	"testing"
)

const testSpec = `spec: package/v1
name: my-agent

meta:
  description: My agent

agent:
  image: my-image:latest

models:
  claude:
    provider: anthropic

integrations:
  gh:
    provider: github
`

func TestAddEntry_preservesFormatting(t *testing.T) {
	f, err := os.CreateTemp("", "spec-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(testSpec)
	f.Close()

	if err := AddEntry(f.Name(), "models", "llama", map[string]any{
		"provider": "ollama",
		"model":    "llama3.2:1b",
	}); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(f.Name())
	result := string(data)
	t.Log("\n" + result)

	checks := []string{
		"spec: package/v1",         // top-level keys preserved
		"meta:\n  description:",    // nested structure preserved
		"  claude:\n    provider:", // existing model entry preserved
		"  llama:\n    model: llama3.2:1b\n    provider: ollama", // new entry added
		"integrations:\n  gh:", // subsequent section preserved
	}
	for _, want := range checks {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q", want)
		}
	}

	// Blank lines between top-level sections must be preserved.
	if strings.Contains(result, "anthropic\nintegrations:") {
		t.Error("blank line between models and integrations was removed")
	}
}
