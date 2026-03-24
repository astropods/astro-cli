//go:build integration

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-cli/internal/scaffold"
)

// TestCreatePython_GeneratedFiles verifies that GenerateFiles for a Python agent
// produces the correct file tree and omits TypeScript-only files.
func TestCreatePython_GeneratedFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-py-agent")

	config := scaffold.ScaffoldConfig{
		Name:            "my-py-agent",
		Description:     "Integration test Python agent",
		Interfaces:      []string{"web"},
		Integrations:    []string{"anthropic"},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{"redis"},
		Ingestions:      []string{"webhook", "startup"},
	}

	if err := scaffold.GenerateFiles(target, config, "langchain"); err != nil {
		t.Fatalf("GenerateFiles: %v", err)
	}

	mustExist := []string{
		"agent/main.py",
		"requirements.txt",
		"Dockerfile",
		"astropods.yml",
		".gitignore",
		".dockerignore",
		"README.md",
		"AGENTS.md",
		"CLAUDE.md",
		"ingestion/webhook/main.py",
		"ingestion/webhook/requirements.txt",
		"ingestion/webhook/Dockerfile",
		"ingestion/startup/main.py",
		"ingestion/startup/requirements.txt",
		"ingestion/startup/Dockerfile",
		"postman/collections/messaging.postman_collection.json",
		"postman/collections/webhook.postman_collection.json",
	}
	for _, f := range mustExist {
		if _, err := os.Stat(filepath.Join(target, f)); os.IsNotExist(err) {
			t.Errorf("expected file missing: %s", f)
		}
	}

	mustNotExist := []string{
		"package.json",
		"tsconfig.json",
		"agent/index.ts",
	}
	for _, f := range mustNotExist {
		if _, err := os.Stat(filepath.Join(target, f)); !os.IsNotExist(err) {
			t.Errorf("unexpected TypeScript file present: %s", f)
		}
	}
}

// TestCreatePython_DockerBuild generates a Python agent scaffold and runs
// `docker build` to confirm the generated Dockerfile is valid.
func TestCreatePython_DockerBuild(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "my-py-agent")

	config := scaffold.ScaffoldConfig{
		Name:            "my-py-agent",
		Description:     "Integration test Python agent",
		Interfaces:      []string{"web"},
		Integrations:    []string{"anthropic"},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestions:      []string{},
	}

	if err := scaffold.GenerateFiles(target, config, "langchain"); err != nil {
		t.Fatalf("GenerateFiles: %v", err)
	}

	tag := "ast-integration-test-py:latest"
	cmd := exec.Command("docker", "build", "-t", tag, "--no-cache", ".")
	cmd.Dir = target
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker build failed:\n%s", out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rmi", tag).Run() })
}

// TestCreatePython_IngestionDockerBuild generates a Python agent with webhook
// ingestion and confirms the ingestion Dockerfile builds.
func TestCreatePython_IngestionDockerBuild(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "my-py-agent")

	config := scaffold.ScaffoldConfig{
		Name:            "my-py-agent",
		Description:     "Integration test Python agent",
		Interfaces:      []string{"web"},
		Integrations:    []string{},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestions:      []string{"webhook"},
	}

	if err := scaffold.GenerateFiles(target, config, "langchain"); err != nil {
		t.Fatalf("GenerateFiles: %v", err)
	}

	tag := "ast-integration-test-py-ingestion:latest"
	cmd := exec.Command("docker", "build", "-t", tag, "--no-cache",
		"-f", filepath.Join("ingestion", "webhook", "Dockerfile"), ".")
	cmd.Dir = target
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker build (ingestion/webhook/Dockerfile) failed:\n%s", out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rmi", tag).Run() })
}

// TestCreatePython_RequirementsTxt checks that the generated requirements.txt
// contains the langchain adapter dependency.
func TestCreatePython_RequirementsTxt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "my-py-agent")

	config := scaffold.ScaffoldConfig{
		Name:            "my-py-agent",
		Description:     "Integration test Python agent",
		Interfaces:      []string{"web"},
		Integrations:    []string{"anthropic"},
		IntegrationKeys: map[string]string{},
		Knowledge:       []string{},
		Ingestions:      []string{},
	}

	if err := scaffold.GenerateFiles(target, config, "langchain"); err != nil {
		t.Fatalf("GenerateFiles: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "requirements.txt"))
	if err != nil {
		t.Fatalf("read requirements.txt: %v", err)
	}
	content := string(data)

	for _, dep := range []string{"langchain", "astropods-adapter-langchain", "langchain-anthropic"} {
		if !strings.Contains(content, dep) {
			t.Errorf("requirements.txt missing %q:\n%s", dep, content)
		}
	}
}
