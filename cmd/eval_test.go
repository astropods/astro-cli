package cmd

import (
	"bytes"
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

func evalReadCmd(t *testing.T, use string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: use}
	cmd.Flags().Bool("json", false, "")
	cmd.SetContext(context.Background())
	return cmd
}

func TestEvalGet(t *testing.T) {
	setPayload := map[string]any{
		"evaluation_ref": "ref-1",
		"evaluators": []any{
			map[string]any{"key": "helpful", "label": "Helpful", "type": "llm_judge", "output": map[string]any{"type": "boolean"}},
			map[string]any{"key": "tone", "label": "Tone", "type": "llm_judge", "output": map[string]any{"type": "enum", "options": []string{"warm", "cold"}}},
			map[string]any{"key": "score", "label": "Score", "type": "llm_judge", "output": map[string]any{"type": "number", "minimum": 1, "maximum": 5}},
		},
	}
	cases := []struct {
		name       string
		statusCode int
		body       any
		jsonOutput bool
		wantErr    string
		wantOut    []string
	}{
		{name: "table of evaluators", statusCode: http.StatusOK, body: setPayload,
			wantOut: []string{"ref-1", "helpful", "true, false", "warm, cold", "1 to 5"}},
		{name: "json output", statusCode: http.StatusOK, body: setPayload, jsonOutput: true,
			wantOut: []string{`"evaluation_ref": "ref-1"`}},
		{name: "empty set", statusCode: http.StatusOK, body: map[string]any{"evaluation_ref": "r", "evaluators": []any{}},
			wantOut: []string{msgNoEvaluators("coach")}},
		{name: "not found", statusCode: http.StatusNotFound, body: map[string]any{"error": "agent not found"},
			wantErr: errEvalSetNotFound("coach", "testaccount").Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			setupEvalTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				jsonHandler(tc.statusCode, tc.body)(w, r)
			}))
			cmd := evalReadCmd(t, "get")
			if tc.jsonOutput {
				require.NoError(t, cmd.Flags().Set("json", "true"))
			}
			buf := &bytes.Buffer{}
			cmd.SetOut(buf)

			err := runEvalGet(cmd, []string{"coach"})
			assert.True(t, strings.HasSuffix(gotPath, "/agents/testaccount/coach/evaluation-set") ||
				strings.HasSuffix(gotPath, "/coach/evaluation-set"), "path %q", gotPath)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestEvalStatus(t *testing.T) {
	dep := map[string]any{
		"id": "dep-abc-123", "name": "coach", "display_name": "coach",
		"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}
	summary := map[string]any{"queued": 1, "in_progress": 2, "completed": 30, "failed": 4, "outdated_count": 5}

	cases := []struct {
		name       string
		statusCode int
		body       any
		jsonOutput bool
		wantErr    string
		wantOut    []string
	}{
		{name: "prints counts", statusCode: http.StatusOK, body: summary,
			wantOut: []string{"Queued:       1", "In progress:  2", "Completed:    30", "Failed:       4", "Outdated:     5"}},
		{name: "json output", statusCode: http.StatusOK, body: summary, jsonOutput: true,
			wantOut: []string{`"outdated_count": 5`}},
		{name: "not configured", statusCode: http.StatusServiceUnavailable, body: map[string]any{"error": "x"},
			wantErr: errEvaluationNotConfigured().Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var summaryPath string
			setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/evaluations/summary") {
					summaryPath = r.URL.Path
					jsonHandler(tc.statusCode, tc.body)(w, r)
					return
				}
				jsonHandler(http.StatusOK, listPayload)(w, r)
			}))
			setAgentTargetName(t, evalStatusCmd, "coach")
			if tc.jsonOutput {
				require.NoError(t, evalStatusCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { _ = evalStatusCmd.Flags().Set("json", "false") })
			}
			buf := &bytes.Buffer{}
			evalStatusCmd.SetOut(buf)
			evalStatusCmd.SetContext(context.Background())

			err := runEvalStatus(evalStatusCmd, nil)
			assert.Equal(t, "/api/v1/deployments/dep-abc-123/evaluations/summary", summaryPath)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}
