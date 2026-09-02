package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEvalTest(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	evalServerURLOverride = srv.URL
	t.Cleanup(func() { evalServerURLOverride = "" })
}

func writeEvalProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, contents := range files {
		full := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(contents), 0o600))
	}
	return dir
}

func evalCmdWithSpecFile(t *testing.T, use string, dir string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: use}
	cmd.Flags().StringP("file", "f", "", "")
	require.NoError(t, cmd.Flags().Set("file", filepath.Join(dir, "astropods.yml")))
	cmd.SetContext(context.Background())
	return cmd
}

func evalPushCmdWithSpecFile(t *testing.T, dir string) *cobra.Command {
	return evalCmdWithSpecFile(t, "push", dir)
}

func evalValidateCmdWithSpecFile(t *testing.T, dir string) *cobra.Command {
	return evalCmdWithSpecFile(t, "validate", dir)
}

func TestEvalPush_ActivatesFromFile(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"EVALUATION.yaml": "schema: evaluation/v1\n" +
			"evaluators:\n" +
			"  - ref: preset/exposed-pii\n" +
			"  - key: response_quality\n" +
			"    label: Response quality\n" +
			"    type: llm\n" +
			"    prompt_file: evaluation/response-quality.md\n" +
			"    output:\n" +
			"      type: number\n",
		"evaluation/response-quality.md": "Assess the overall quality of the response.",
	})

	var gotBody map[string]any
	called := false
	setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, http.MethodPut, r.Method)
		assert.True(t, strings.HasSuffix(r.URL.Path, "/agents/testaccount/my-agent/evaluation-set"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"evaluation_ref": "agent/abc123"}) //nolint:errcheck
	}))

	err := runEvalPush(evalPushCmdWithSpecFile(t, dir), []string{"my-agent"})
	require.NoError(t, err)
	require.True(t, called)

	promptFiles, ok := gotBody["prompt_files"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Assess the overall quality of the response.", promptFiles["evaluation/response-quality.md"])
	assert.Contains(t, gotBody["evaluation_yaml"], "preset/exposed-pii")
}

func TestEvalPush_AcceptsYmlAlias(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"EVALUATION.yml": "schema: evaluation/v1\nevaluators:\n  - ref: preset/exposed-pii\n",
	})

	called := false
	setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"evaluation_ref": "agent/abc123"}) //nolint:errcheck
	}))

	err := runEvalPush(evalPushCmdWithSpecFile(t, dir), []string{"my-agent"})
	require.NoError(t, err)
	require.True(t, called)
}

func TestEvalPush_DerivesNameFromSpec(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"astropods.yml":   "name: spec-agent\nagent:\n  image: agent:latest\n",
		"EVALUATION.yaml": "schema: evaluation/v1\nevaluators:\n  - ref: preset/exposed-pii\n",
	})

	called := false
	setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.True(t, strings.HasSuffix(r.URL.Path, "/agents/testaccount/spec-agent/evaluation-set"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"evaluation_ref": "agent/abc123"}) //nolint:errcheck
	}))

	err := runEvalPush(evalPushCmdWithSpecFile(t, dir), nil)
	require.NoError(t, err)
	require.True(t, called)
}

func TestEvalPush_ArgOverridesSpecName(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"astropods.yml":   "name: spec-agent\nagent:\n  image: agent:latest\n",
		"EVALUATION.yaml": "schema: evaluation/v1\nevaluators:\n  - ref: preset/exposed-pii\n",
	})

	called := false
	setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.True(t, strings.HasSuffix(r.URL.Path, "/agents/testaccount/override-agent/evaluation-set"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"evaluation_ref": "agent/abc123"}) //nolint:errcheck
	}))

	err := runEvalPush(evalPushCmdWithSpecFile(t, dir), []string{"override-agent"})
	require.NoError(t, err)
	require.True(t, called)
}

func TestEvalPush_NoNameNoArgFails(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"astropods.yml":   "agent:\n  image: agent:latest\n",
		"EVALUATION.yaml": "schema: evaluation/v1\nevaluators:\n  - ref: preset/exposed-pii\n",
	})

	called := false
	setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	err := runEvalPush(evalPushCmdWithSpecFile(t, dir), nil)
	require.Error(t, err)
	assert.False(t, called)
}

func TestEvalPush_MissingFile(t *testing.T) {
	dir := writeEvalProject(t, nil)

	called := false
	setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	err := runEvalPush(evalPushCmdWithSpecFile(t, dir), []string{"my-agent"})
	require.Error(t, err)
	assert.Equal(t, errNoEvaluationFile(), err)
	assert.False(t, called)
}

func TestEvalPush_MissingPromptFile(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"EVALUATION.yaml": "schema: evaluation/v1\n" +
			"evaluators:\n" +
			"  - key: response_quality\n" +
			"    label: Response quality\n" +
			"    type: llm\n" +
			"    prompt_file: evaluation/missing.md\n" +
			"    output:\n" +
			"      type: number\n",
	})

	called := false
	setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	err := runEvalPush(evalPushCmdWithSpecFile(t, dir), []string{"my-agent"})
	require.Error(t, err)
	assert.False(t, called)
}

func TestEvalPush_ServerRejectsInvalidContent(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"EVALUATION.yaml": "schema: evaluation/v2\nevaluators:\n  - ref: preset/exposed-pii\n",
	})

	setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"error":   "Invalid evaluation configuration",
			"details": "invalid evaluation document: schema must be \"evaluation/v1\", got \"evaluation/v2\"",
		})
	}))

	err := runEvalPush(evalPushCmdWithSpecFile(t, dir), []string{"my-agent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema must be")
}

func TestEvalPush_NotFound(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"EVALUATION.yaml": "schema: evaluation/v1\nevaluators:\n  - ref: preset/exposed-pii\n",
	})

	setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "not found"}) //nolint:errcheck
	}))

	err := runEvalPush(evalPushCmdWithSpecFile(t, dir), []string{"ghost"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `agent "ghost" not found`)
}

func TestEvalValidate_AcceptsValidFile(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"EVALUATION.yaml": "schema: evaluation/v1\nevaluators:\n  - ref: preset/exposed-pii\n",
	})

	err := runEvalValidate(evalValidateCmdWithSpecFile(t, dir), nil)
	require.NoError(t, err)
}

func TestEvalValidate_RejectsInvalidContent(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"EVALUATION.yaml": "schema: evaluation/v2\nevaluators:\n  - ref: preset/exposed-pii\n",
	})

	err := runEvalValidate(evalValidateCmdWithSpecFile(t, dir), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema must be")
}

func TestEvalValidate_RejectsAnUnknownPresetRef(t *testing.T) {
	dir := writeEvalProject(t, map[string]string{
		"EVALUATION.yaml": "schema: evaluation/v1\nevaluators:\n  - ref: preset/does-not-exist\n",
	})

	err := runEvalValidate(evalValidateCmdWithSpecFile(t, dir), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown preset reference")
}

func TestEvalValidate_MissingFile(t *testing.T) {
	dir := writeEvalProject(t, nil)

	err := runEvalValidate(evalValidateCmdWithSpecFile(t, dir), nil)
	require.Error(t, err)
	assert.Equal(t, errNoEvaluationFile(), err)
}
