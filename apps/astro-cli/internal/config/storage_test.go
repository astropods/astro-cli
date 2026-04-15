package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeProjectVars_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	// Point the binary name to a temp dir so config writes go there.
	binaryName := filepath.Base(dir)

	// Patch ConfigsPath by setting HOME so auth.ConfigDir resolves into our temp dir.
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	err := MergeProjectVars(binaryName, "/fake/project", "my-agent", map[string]string{
		"API_KEY":     "  sk-abc123  ",
		"SLACK_TOKEN": "\tbot-token\n",
		"EMPTY":       "   ",
	})
	if err != nil {
		t.Fatalf("MergeProjectVars error: %v", err)
	}

	vars := GetProjectVars(binaryName, "/fake/project")

	if got, want := vars["API_KEY"], "sk-abc123"; got != want {
		t.Errorf("API_KEY = %q, want %q", got, want)
	}
	if got, want := vars["SLACK_TOKEN"], "bot-token"; got != want {
		t.Errorf("SLACK_TOKEN = %q, want %q", got, want)
	}
	if _, ok := vars["EMPTY"]; ok {
		t.Error("whitespace-only value should not be stored")
	}
}
