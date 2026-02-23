package create

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/postman/astro/apps/astro-cli/internal/scaffold"
)

// keyFromString returns a tea.Msg that matches the key comparisons in the create TUI (msg.String()).
func keyFromString(s string) tea.Msg {
	switch s {
	case "enter":
		return tea.KeyMsg(tea.Key{Type: tea.KeyEnter})
	case "up":
		return tea.KeyMsg(tea.Key{Type: tea.KeyUp})
	case "down":
		return tea.KeyMsg(tea.Key{Type: tea.KeyDown})
	case " ":
		return tea.KeyMsg(tea.Key{Type: tea.KeySpace})
	default:
		// Single rune or runes (e.g. "j", "k", "y", "n", "llama3")
		runes := []rune(s)
		if len(runes) == 0 {
			return tea.KeyMsg(tea.Key{Type: tea.KeyEnter})
		}
		return tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: runes})
	}
}

// runWithKeys drives the create TUI model with a sequence of key strings and returns the final config.
// Use key names: "enter", "up", "down", " ", and single runes like "j", "k", "y", "n", or type "llama3" as multiple keys.
// Returns (config, done, quitting). If done is true, user confirmed; if quitting is true, user cancelled.
func runWithKeys(name string, keys []string) (scaffold.ScaffoldConfig, bool, bool) {
	m := initialModel(name)
	for _, k := range keys {
		var cmd tea.Cmd
		var next tea.Model
		next, cmd = m.Update(keyFromString(k))
		_ = cmd
		m = next.(model)
		if m.done || m.quitting {
			break
		}
	}
	return m.config, m.done, m.quitting
}

func TestRunWithKeys_DefaultsToOllamaThenConfirm(t *testing.T) {
	// Flow: description -> interface -> model (Ollama pre-selected) enter -> Ollama model (first = llama3.2) enter -> knowledge -> tools -> ingestion -> y
	keys := []string{
		"enter", "enter", // description, interface
		"enter",           // model (Ollama selected)
		"enter",           // Ollama model pick (cursor 0 = llama3.2)
		"enter", "enter", "enter", // knowledge, tools, ingestion (nothing pre-selected; enter with no selection)
		"y",
	}
	cfg, done, quitting := runWithKeys("test-agent", keys)
	if quitting {
		t.Fatal("expected done, not quitting")
	}
	if !done {
		t.Fatal("expected done=true")
	}
	if cfg.Name != "test-agent" {
		t.Errorf("config.Name = %q, want test-agent", cfg.Name)
	}
	if cfg.ModelProvider != "ollama" {
		t.Errorf("config.ModelProvider = %q, want ollama", cfg.ModelProvider)
	}
	if cfg.Model != "llama3.2:1b" {
		t.Errorf("config.Model = %q, want llama3.2:1b", cfg.Model)
	}
	if len(cfg.Knowledge) != 0 {
		t.Errorf("config.Knowledge = %v, want []", cfg.Knowledge)
	}
	if len(cfg.Integrations) != 0 {
		t.Errorf("config.Integrations = %v, want []", cfg.Integrations)
	}
}

func TestRunWithKeys_ModelNone(t *testing.T) {
	// Model: unselect Ollama (no selection), enter -> no model, go to Knowledge
	keys := []string{
		"enter", "enter",
		" ", "enter", // unselect Ollama, confirm (nothing selected)
		"enter", "enter", "enter",
		"y",
	}
	cfg, done, quitting := runWithKeys("no-model-agent", keys)
	if !done || quitting {
		t.Fatalf("done=%v quitting=%v", done, quitting)
	}
	if cfg.ModelProvider != "" {
		t.Errorf("config.ModelProvider = %q, want empty", cfg.ModelProvider)
	}
	if len(cfg.Integrations) != 0 {
		t.Errorf("config.Integrations = %v, want []", cfg.Integrations)
	}
}

func TestRunWithKeys_ModelAnthropic(t *testing.T) {
	// Model: unselect Ollama, select Anthropic only (multi-select)
	keys := []string{
		"enter", "enter",
		" ", "down", " ", "enter", // unselect Ollama, select Anthropic, confirm
		"enter", "enter",
		"enter", "enter", "y", // skip API key, ingestion, confirm
	}
	cfg, done, quitting := runWithKeys("claude-agent", keys)
	if !done || quitting {
		t.Fatalf("done=%v quitting=%v", done, quitting)
	}
	if cfg.ModelProvider != "" {
		t.Errorf("config.ModelProvider = %q, want empty", cfg.ModelProvider)
	}
	if len(cfg.Integrations) != 1 || cfg.Integrations[0] != "anthropic" {
		t.Errorf("config.Integrations = %v, want [anthropic]", cfg.Integrations)
	}
}

func TestRunWithKeys_ModelOpenAI(t *testing.T) {
	// Model: unselect Ollama, select OpenAI only
	keys := []string{
		"enter", "enter",
		" ", "down", "down", " ", "enter",
		"enter", "enter",
		"enter", "enter", "y",
	}
	cfg, done, quitting := runWithKeys("gpt-agent", keys)
	if !done || quitting {
		t.Fatalf("done=%v quitting=%v", done, quitting)
	}
	if len(cfg.Integrations) != 1 || cfg.Integrations[0] != "openai" {
		t.Errorf("config.Integrations = %v, want [openai]", cfg.Integrations)
	}
}

func TestRunWithKeys_OllamaWithModelName(t *testing.T) {
	// Model Ollama selected, enter -> Ollama model: unselect first, select mistral:7b (index 2), enter
	keys := []string{
		"enter", "enter",
		"enter",                    // model (Ollama selected)
		" ", "down", "down", " ", "enter", // Ollama model: toggle off first, toggle mistral:7b, confirm
		"enter", "enter", "enter",
		"y",
	}
	cfg, done, quitting := runWithKeys("ollama-agent", keys)
	if !done || quitting {
		t.Fatalf("done=%v quitting=%v", done, quitting)
	}
	if cfg.ModelProvider != "ollama" {
		t.Errorf("config.ModelProvider = %q, want ollama", cfg.ModelProvider)
	}
	if cfg.Model != "mistral:7b" {
		t.Errorf("config.Model = %q, want mistral:7b", cfg.Model)
	}
}

func TestRunWithKeys_KnowledgeQdrant(t *testing.T) {
	// Model Ollama -> Ollama model enter -> Knowledge: select Qdrant (cursor 0), enter
	keys := []string{
		"enter", "enter", "enter", "enter", // to knowledge (nothing pre-selected)
		" ", "enter", // toggle Qdrant, confirm
		"enter", "enter", "y",
	}
	cfg, done, quitting := runWithKeys("qdrant-agent", keys)
	if !done || quitting {
		t.Fatalf("done=%v quitting=%v", done, quitting)
	}
	if len(cfg.Knowledge) != 1 || cfg.Knowledge[0] != "qdrant" {
		t.Errorf("config.Knowledge = %v, want [qdrant]", cfg.Knowledge)
	}
}

func TestRunWithKeys_ToolsGitHub(t *testing.T) {
	// Model: unselect Ollama, enter; then Tools: select GitHub (only option), enter
	keys := []string{
		"enter", "enter",
		" ", "enter", // model: nothing selected
		"enter",             // knowledge
		" ", "enter",        // tools: toggle GitHub, confirm
		"enter", "enter", "y",
	}
	cfg, done, quitting := runWithKeys("github-agent", keys)
	if !done || quitting {
		t.Fatalf("done=%v quitting=%v", done, quitting)
	}
	if len(cfg.Integrations) != 1 || cfg.Integrations[0] != "github" {
		t.Errorf("config.Integrations = %v, want [github]", cfg.Integrations)
	}
}

func TestRunWithKeys_CancelWithQ(t *testing.T) {
	keys := []string{
		"enter", "enter",
		"q",
	}
	_, done, quitting := runWithKeys("cancelled", keys)
	if !quitting || done {
		t.Errorf("expected quitting=true, done=false; got quitting=%v done=%v", quitting, done)
	}
}

func TestRunWithKeys_InterfaceRequiresSelection(t *testing.T) {
	// Drive the model manually so we can inspect intermediate state.
	m := initialModel("test-agent")

	// Advance past description.
	next, _ := m.Update(keyFromString("enter"))
	m = next.(model)

	// Now on screenInterface with web pre-selected. Deselect it, then press enter.
	next, _ = m.Update(keyFromString(" ")) // deselect web
	m = next.(model)
	next, _ = m.Update(keyFromString("enter")) // attempt confirm with nothing selected
	m = next.(model)

	if m.screen != screenInterface {
		t.Errorf("expected to stay on screenInterface, got screen=%d", m.screen)
	}
	if m.err == "" {
		t.Errorf("expected validation error, got empty err")
	}

	// Now re-select web and confirm — should advance.
	next, _ = m.Update(keyFromString(" ")) // select web
	m = next.(model)
	next, _ = m.Update(keyFromString("enter"))
	m = next.(model)

	if m.screen == screenInterface {
		t.Errorf("expected to advance past screenInterface after valid selection")
	}
	if m.err != "" {
		t.Errorf("expected err to be cleared after valid selection, got %q", m.err)
	}
}
