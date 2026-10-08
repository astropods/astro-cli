package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	evalspec "github.com/astropods/astro-spec/eval"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

func TestEvalRun(t *testing.T) {
	dep := map[string]any{
		"id": "dep-abc-123", "name": "coach", "display_name": "coach-dev",
		"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}
	ids := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("t%d", i)
		}
		return out
	}

	cases := []struct {
		name            string
		traceID         string
		includeOutdated bool
		jsonOutput      bool
		statusCode      int
		body            any
		wantPath        string
		wantBody        map[string]any
		wantErr         string
		wantOut         string
	}{
		{name: "batch queues recent traces", statusCode: http.StatusAccepted,
			body:     map[string]any{"enqueued_trace_ids": ids(3), "failed_trace_ids": []string{}},
			wantPath: "/agents/testaccount/coach/evaluations", wantBody: map[string]any{"deployment_id": "dep-abc-123", "include_outdated_runs": false},
			wantOut: msgEvalRunQueued(3, 0, evalRunBatchLimit)},
		{name: "batch reports failures", statusCode: http.StatusAccepted,
			body:     map[string]any{"enqueued_trace_ids": ids(2), "failed_trace_ids": []string{"bad"}},
			wantPath: "/agents/testaccount/coach/evaluations", wantBody: map[string]any{"deployment_id": "dep-abc-123", "include_outdated_runs": false},
			wantOut: msgEvalRunQueued(2, 1, evalRunBatchLimit)},
		{name: "batch at the limit says more may remain", statusCode: http.StatusAccepted,
			body:     map[string]any{"enqueued_trace_ids": ids(evalRunBatchLimit), "failed_trace_ids": []string{}},
			wantPath: "/agents/testaccount/coach/evaluations", wantBody: map[string]any{"deployment_id": "dep-abc-123", "include_outdated_runs": false},
			wantOut: "run it again"},
		{name: "include outdated is sent", includeOutdated: true, statusCode: http.StatusAccepted,
			body:     map[string]any{"enqueued_trace_ids": ids(1), "failed_trace_ids": []string{}},
			wantPath: "/agents/testaccount/coach/evaluations", wantBody: map[string]any{"deployment_id": "dep-abc-123", "include_outdated_runs": true},
			wantOut: msgEvalRunQueued(1, 0, evalRunBatchLimit)},
		{name: "nothing to evaluate", statusCode: http.StatusAccepted,
			body:     map[string]any{"enqueued_trace_ids": []string{}, "failed_trace_ids": []string{}},
			wantPath: "/agents/testaccount/coach/evaluations", wantBody: map[string]any{"deployment_id": "dep-abc-123", "include_outdated_runs": false},
			wantOut: msgEvalRunQueued(0, 0, evalRunBatchLimit)},
		{name: "batch json output", jsonOutput: true, statusCode: http.StatusAccepted,
			body:     map[string]any{"enqueued_trace_ids": ids(1), "failed_trace_ids": []string{}},
			wantPath: "/agents/testaccount/coach/evaluations", wantBody: map[string]any{"deployment_id": "dep-abc-123", "include_outdated_runs": false},
			wantOut: `"enqueued_trace_ids"`},
		{name: "single trace", traceID: "trace-abc", statusCode: http.StatusAccepted,
			body:     map[string]any{"evaluation_run_id": "run-1", "status": "queued"},
			wantPath: "/agents/testaccount/coach/evaluations/trace-abc", wantBody: map[string]any{"deployment_id": "dep-abc-123"},
			wantOut: msgEvalRunTraceQueued("trace-abc", "run-1", "queued", "dep-abc-123")},
		{name: "single trace already running", traceID: "trace-abc", statusCode: http.StatusConflict,
			body: map[string]any{"error": "evaluation is already running"}, wantErr: errEvalRunAlreadyActive("trace-abc").Error()},
		{name: "single trace not found", traceID: "trace-abc", statusCode: http.StatusNotFound,
			body: map[string]any{"error": "trace not found"}, wantErr: errAgentTraceNotFound("trace-abc", "coach-dev").Error()},
		{name: "not configured", statusCode: http.StatusServiceUnavailable,
			body: map[string]any{"error": "evaluation is not configured"}, wantErr: errEvaluationNotConfigured().Error()},
		{name: "billing suspended surfaces the pay reason", statusCode: http.StatusPaymentRequired,
			body:    map[string]any{"error": "Billing suspended", "code": "BILLING_SUSPENDED", "reason": "no_card", "action": "add_card"},
			wantErr: newAPIError(http.StatusPaymentRequired, []byte(`{"error":"Billing suspended","code":"BILLING_SUSPENDED","reason":"no_card","action":"add_card"}`)).Error()},
		{name: "trace id with include outdated is rejected", traceID: "trace-abc", includeOutdated: true,
			wantErr: errEvalRunTraceWithOutdated().Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			var gotBody map[string]any
			setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/evaluations") {
					gotPath = r.URL.Path
					_ = json.NewDecoder(r.Body).Decode(&gotBody)
					jsonHandler(tc.statusCode, tc.body)(w, r)
					return
				}
				jsonHandler(http.StatusOK, listPayload)(w, r)
			}))
			evalServerURLOverride = agentServerURLOverride
			t.Cleanup(func() { evalServerURLOverride = "" })

			setAgentTargetName(t, evalRunCmd, "coach")
			setEvalRunFlag(t, "trace-id", tc.traceID, "")
			if tc.includeOutdated {
				setEvalRunFlag(t, "include-outdated", "true", "false")
			}
			if tc.jsonOutput {
				setEvalRunFlag(t, "json", "true", "false")
			}
			buf := &bytes.Buffer{}
			evalRunCmd.SetOut(buf)
			evalRunCmd.SetContext(context.Background())

			err := runEvalRun(evalRunCmd, nil)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, strings.HasSuffix(gotPath, tc.wantPath), "path %q", gotPath)
			assert.Equal(t, tc.wantBody, gotBody)
			assert.Contains(t, buf.String(), tc.wantOut)
		})
	}
}

func setEvalRunFlag(t *testing.T, name, value, reset string) {
	t.Helper()
	require.NoError(t, evalRunCmd.Flags().Set(name, value))
	t.Cleanup(func() { _ = evalRunCmd.Flags().Set(name, reset) })
}

func TestEvalRunRejectsPositionalArgs(t *testing.T) {
	require.EqualError(t, agentTargetArgs(evalRunCmd, []string{"coach"}), errAgentUnexpectedArgument("coach").Error())
}

func reviewTestSet() []evalSetEvaluator {
	one, five := 1.0, 5.0
	return []evalSetEvaluator{
		{Key: "helpful", Output: evalspec.Output{Type: evalspec.OutputBoolean}},
		{Key: "tone", Output: evalspec.Output{Type: evalspec.OutputEnum, Options: []string{"warm", "cold"}}},
		{Key: "score", Output: evalspec.Output{Type: evalspec.OutputNumber, Minimum: &one, Maximum: &five}},
		{Key: "note", Output: evalspec.Output{Type: evalspec.OutputString}},
	}
}

func TestParseEvalSetFlags(t *testing.T) {
	cases := []struct {
		name    string
		raw     []string
		want    map[string]string
		wantErr string
	}{
		{name: "types values by evaluator output",
			raw:  []string{"helpful=true", "tone=warm", "score=4.5", "note=a=b"},
			want: map[string]string{"helpful": "true", "tone": `"warm"`, "score": "4.5", "note": `"a=b"`}},
		{name: "unknown key is sent as a string for the server to reject",
			raw: []string{"mystery=1"}, want: map[string]string{"mystery": `"1"`}},
		{name: "missing equals", raw: []string{"helpful"}, wantErr: errEvalSetFlagFormat("helpful").Error()},
		{name: "empty key", raw: []string{"=true"}, wantErr: errEvalSetFlagFormat("=true").Error()},
		{name: "duplicate key", raw: []string{"tone=warm", "tone=cold"}, wantErr: errEvalSetFlagDuplicate("tone").Error()},
		{name: "bad boolean", raw: []string{"helpful=maybe"},
			wantErr: errEvalSetFlagValue("helpful", errors.New(`"maybe" is not true or false`)).Error()},
		{name: "bad number", raw: []string{"score=high"},
			wantErr: errEvalSetFlagValue("score", errors.New(`"high" is not a number`)).Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEvalSetFlags(tc.raw, reviewTestSet())
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, got, len(tc.want))
			for k, v := range tc.want {
				assert.JSONEq(t, v, string(got[k]), k)
			}
		})
	}
}

func TestEvalOutputValues(t *testing.T) {
	got := evalOutputValues(map[string]json.RawMessage{
		"tone":    json.RawMessage(`"warm"`),
		"helpful": json.RawMessage(`true`),
	})
	require.Len(t, got, 2)
	assert.Equal(t, "helpful", got[0].Key, "outputs are ordered by key")
	assert.Equal(t, "tone", got[1].Key)
}

func TestEvalReview(t *testing.T) {
	dep := map[string]any{
		"id": "dep-abc-123", "name": "coach", "display_name": "coach-dev",
		"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}
	setPayload := map[string]any{"evaluation_ref": "ref-2", "evaluators": []any{
		map[string]any{"key": "helpful", "label": "Helpful", "type": "llm_judge", "output": map[string]any{"type": "boolean"}},
		map[string]any{"key": "tone", "label": "Tone", "type": "llm_judge", "output": map[string]any{"type": "enum", "options": []string{"warm", "cold"}}},
	}}
	withRun := map[string]any{"evaluation": map[string]any{
		"evaluation_ref": "ref-1", "outdated": false, "status": "completed",
		"run": map[string]any{"id": "run-1", "status": "completed", "evaluators": []any{
			map[string]any{"key": "helpful", "status": "completed", "value": true},
			map[string]any{"key": "tone", "status": "completed", "value": "cold"},
		}},
	}}
	noRun := map[string]any{"evaluation": map[string]any{
		"evaluation_ref": "ref-2", "outdated": false, "status": "none", "run": nil, "human_review": nil,
	}}
	saved := map[string]any{
		"human_review":             map[string]any{"evaluator_outputs": []any{map[string]any{"key": "helpful", "value": false}, map[string]any{"key": "tone", "value": "cold"}}},
		"dataset_snapshot_updated": true,
	}

	cases := []struct {
		name          string
		sets          []string
		traceID       string
		updateDataset bool
		current       map[string]any
		currentStatus int
		putStatus     int
		putBody       any
		wantBody      map[string]any
		wantErr       string
		wantOut       string
	}{
		{name: "sends only the given values with the run id", sets: []string{"helpful=false"}, traceID: "trace-abc",
			current: withRun, putStatus: http.StatusOK, putBody: saved,
			wantBody: map[string]any{
				"deployment_id": "dep-abc-123", "evaluation_ref": "ref-1", "evaluation_run_id": "run-1",
				"evaluator_outputs":       []any{map[string]any{"key": "helpful", "value": false}},
				"update_dataset_snapshot": false,
			},
			wantOut: msgEvalReviewSaved("trace-abc", 2, false, true)},
		{name: "unevaluated trace sends the active ref without a run id", sets: []string{"helpful=true"}, traceID: "trace-abc",
			current: noRun, putStatus: http.StatusOK, putBody: saved,
			wantBody: map[string]any{
				"deployment_id": "dep-abc-123", "evaluation_ref": "ref-2",
				"evaluator_outputs":       []any{map[string]any{"key": "helpful", "value": true}},
				"update_dataset_snapshot": false,
			},
			wantOut: msgEvalReviewSaved("trace-abc", 2, false, true)},
		{name: "update dataset is sent", sets: []string{"helpful=true"}, traceID: "trace-abc", updateDataset: true,
			current: noRun, putStatus: http.StatusOK, putBody: map[string]any{"human_review": map[string]any{"evaluator_outputs": []any{}}, "dataset_snapshot_updated": false},
			wantBody: map[string]any{
				"deployment_id": "dep-abc-123", "evaluation_ref": "ref-2",
				"evaluator_outputs":       []any{map[string]any{"key": "helpful", "value": true}},
				"update_dataset_snapshot": true,
			},
			wantOut: msgEvalReviewSaved("trace-abc", 0, true, false)},
		{name: "invalid value prints the server message", sets: []string{"tone=lukewarm"}, traceID: "trace-abc",
			current: withRun, putStatus: http.StatusBadRequest, putBody: map[string]any{"error": `evaluator "tone": not an option`},
			wantErr: errEvalReviewInvalid(`evaluator "tone": not an option`).Error()},
		{name: "conflict suggests re-running the trace", sets: []string{"tone=warm"}, traceID: "trace-abc",
			current: withRun, putStatus: http.StatusConflict, putBody: map[string]any{"error": "evaluation set changed"},
			wantErr: errEvalReviewConflict("evaluation set changed", "trace-abc").Error()},
		{name: "unknown trace is reported before any review is sent", sets: []string{"tone=warm"}, traceID: "trace-abc",
			currentStatus: http.StatusNotFound, current: map[string]any{"error": "trace not found"},
			wantErr: errAgentTraceNotFound("trace-abc", "coach-dev").Error()},
		{name: "trace id is required", sets: []string{"tone=warm"}, wantErr: errEvalReviewTraceRequired().Error()},
		{name: "at least one set is required", traceID: "trace-abc", wantErr: errEvalReviewSetRequired().Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]any
			var gotPath string
			setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/human-review"):
					gotPath = r.URL.Path
					_ = json.NewDecoder(r.Body).Decode(&gotBody)
					jsonHandler(tc.putStatus, tc.putBody)(w, r)
				case strings.HasSuffix(r.URL.Path, "/evaluation-set"):
					jsonHandler(http.StatusOK, setPayload)(w, r)
				case strings.Contains(r.URL.Path, "/trace-evaluations/"):
					status := tc.currentStatus
					if status == 0 {
						status = http.StatusOK
					}
					jsonHandler(status, tc.current)(w, r)
				default:
					jsonHandler(http.StatusOK, listPayload)(w, r)
				}
			}))
			evalServerURLOverride = agentServerURLOverride
			t.Cleanup(func() { evalServerURLOverride = "" })

			setAgentTargetName(t, evalReviewCmd, "coach")
			if tc.traceID != "" {
				require.NoError(t, evalReviewCmd.Flags().Set("trace-id", tc.traceID))
				t.Cleanup(func() { _ = evalReviewCmd.Flags().Set("trace-id", "") })
			}
			for _, s := range tc.sets {
				require.NoError(t, evalReviewCmd.Flags().Set("set", s))
			}
			t.Cleanup(func() { resetStringArrayFlag(t, evalReviewCmd, "set") })
			if tc.updateDataset {
				require.NoError(t, evalReviewCmd.Flags().Set("update-dataset", "true"))
				t.Cleanup(func() { _ = evalReviewCmd.Flags().Set("update-dataset", "false") })
			}
			buf := &bytes.Buffer{}
			evalReviewCmd.SetOut(buf)
			evalReviewCmd.SetContext(context.Background())

			err := runEvalReview(evalReviewCmd, nil)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.True(t, strings.HasSuffix(gotPath, "/agents/testaccount/coach/evaluations/trace-abc/human-review"), gotPath)
			assert.Equal(t, tc.wantBody, gotBody)
			assert.Contains(t, buf.String(), tc.wantOut)
		})
	}
}

func resetStringArrayFlag(t *testing.T, cmd *cobra.Command, name string) {
	t.Helper()
	f := cmd.Flags().Lookup(name)
	require.NotNil(t, f)
	require.NoError(t, f.Value.(pflag.SliceValue).Replace(nil))
	f.Changed = false
}
