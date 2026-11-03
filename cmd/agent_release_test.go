package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func executeReleaseCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := newAgentReleaseCommand()
	command.SetArgs(args)
	command.SetContext(t.Context())
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&out)
	command.SilenceUsage, command.SilenceErrors = true, true
	err := command.Execute()
	return out.String(), err
}

func TestReleaseStatusRequiresTheSubmittedBuildAndReadyRevision(t *testing.T) {
	for _, build := range []string{"wanted", "newer"} {
		t.Run(build, func(t *testing.T) {
			runtimeReads := 0
			setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/deployments/target/auto-deploy/status":
					_, err := w.Write([]byte(`{"policy":{"enabled":true,"outcome":"submitted","desired_build_id":"wanted","submitted_revision":2}}`))
					require.NoError(t, err)
				case "/api/v1/deployments/target":
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"deployment": map[string]any{"id": "target", "build_id": build, "current_revision": 2, "status": "Running"}}))
				case "/api/v1/deployments/target/runtime":
					runtimeReads++
					_, err := w.Write([]byte(`{"runtime":{"ready_revision":2}}`))
					require.NoError(t, err)
				default:
					require.FailNow(t, "unexpected request", r.URL.String())
				}
			}))
			raw, err := executeReleaseCommand(t, "status", "--id", "target", "--json")
			require.NoError(t, err)
			var result releaseStatus
			require.NoError(t, json.Unmarshal([]byte(raw), &result))
			require.Equal(t, build == "wanted", result.Ready)
			if build == "newer" {
				require.Zero(t, runtimeReads)
			}
		})
	}
}

func TestReleaseHistoryPreservesTheCursorAndFullJSONMetadata(t *testing.T) {
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/deployments/candidate/build-acceptance-policies", r.URL.Path)
		require.Equal(t, "cursor+value", r.URL.Query().Get("cursor"))
		require.Equal(t, "2", r.URL.Query().Get("limit"))
		_, err := w.Write([]byte(`{"items":[{"version":"policy","dataset_hash":"hash"}],"next_cursor":"next"}`))
		require.NoError(t, err)
	}))
	raw, err := executeReleaseCommand(t, "history", "--id", "candidate", "--policies", "--limit", "2", "--cursor", "cursor+value", "--json")
	require.NoError(t, err)
	require.JSONEq(t, `{"items":[{"version":"policy","dataset_hash":"hash"}],"next_cursor":"next"}`, raw)
}

func TestReleaseRetryPreservesExplicitPreconditionsAndMakesOneMutation(t *testing.T) {
	version, requestID := uuid.NewString(), uuid.NewString()
	calls := 0
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/deployments/target/auto-deploy/retry", r.URL.Path)
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, map[string]string{"policy_version": version, "request_id": requestID, "build_id": "build"}, body)
		require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"version": requestID, "desired_build_id": "build", "outcome": "pending"}))
	}))
	raw, err := executeReleaseCommand(t, "retry", "--id", "target", "--build", "build", "--policy-version", version, "--request-id", requestID)
	require.NoError(t, err)
	require.Equal(t, msgReleaseRetry(releasePolicy{Version: requestID, DesiredBuild: "build", Outcome: "pending"})+"\n", raw)
	require.Equal(t, 1, calls)
}

func TestReleaseCommandsValidateBeforeAuthentication(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want error
	}{
		{[]string{"status"}, errReleaseDeploymentRequired()},
		{[]string{"retry", "--id", "target"}, errReleaseBuildRequired()},
		{[]string{"history", "--id", "candidate", "--limit", "51"}, errReleaseHistoryLimit()},
	} {
		t.Run(tc.args[0], func(t *testing.T) {
			_, err := executeReleaseCommand(t, tc.args...)
			require.EqualError(t, err, tc.want.Error())
		})
	}
}

func TestReleaseSetupValidatesWithoutApplyingAndChecksPermissionsBeforeWrites(t *testing.T) {
	for _, tc := range []struct {
		name           string
		apply, allowed bool
	}{{"plan", false, true}, {"denied", true, false}, {"apply", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dataset := filepath.Join(dir, "dataset.json")
			rules := filepath.Join(dir, "rules.json")
			output := filepath.Join(dir, "plan.json")
			require.NoError(t, os.WriteFile(dataset, []byte(`{"name":"fixture","cases":[{"id":"one","input":true}]}`), 0600))
			require.NoError(t, os.WriteFile(rules, []byte(`[{"key":"accuracy","equals":true,"min_confidence":0.8}]`), 0600))
			calls := []string{}
			setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.Method+" "+r.URL.RequestURI())
				switch {
				case strings.HasSuffix(r.URL.Path, "/build-evaluation-preflight"):
					_, err := w.Write([]byte(`{"candidate_id":"candidate","target_id":"target","build_id":"build","revision":1,"policy":{"schema":"acceptance/v1"},"version":"pinned"}`))
					require.NoError(t, err)
				case r.Method == http.MethodGet:
					_, err := w.Write([]byte(`{"mode":"published"}`))
					require.NoError(t, err)
				case r.URL.Query().Get("dryrun") == "true":
					if strings.Contains(r.URL.Path, "target") && !tc.allowed {
						w.WriteHeader(403)
					} else {
						w.WriteHeader(204)
					}
				default:
					require.True(t, tc.apply && tc.allowed)
					require.GreaterOrEqual(t, len(calls), 6)
					_, err := w.Write([]byte(`{"version":"pinned"}`))
					require.NoError(t, err)
				}
			}))
			args := []string{"setup", "--id", "target", "--candidate", "candidate", "--dataset", dataset, "--rules", rules, "--runner", "app:runner", "--output", output}
			if tc.apply {
				args = append(args, "--apply")
			}
			_, err := executeReleaseCommand(t, args...)
			if !tc.allowed {
				require.Error(t, err)
				require.Len(t, calls, 5)
			} else {
				require.NoError(t, err)
				if tc.apply {
					require.Len(t, calls, 8)
				} else {
					require.Len(t, calls, 1)
				}
			}
			raw, err := os.ReadFile(output)
			require.NoError(t, err)
			var plan releasePlan
			require.NoError(t, json.Unmarshal(raw, &plan))
			require.Equal(t, "pinned", plan.Version)
			info, err := os.Stat(output)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0600), info.Mode().Perm())
		})
	}
}

func TestReleaseComparePreservesTheFullComparisonAndUsesTheSelectedRuns(t *testing.T) {
	response := `{"baseline_build_id":"old","current_build_id":"new","changed":1,"improved":0,"regressed":1,"values":[{"case_id":"one","key":"accurate","baseline":true,"current":false}]}`
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/deployments/candidate/build-evaluations/current/compare", r.URL.Path)
		require.Equal(t, "previous", r.URL.Query().Get("baseline"))
		_, err := w.Write([]byte(response))
		require.NoError(t, err)
	}))
	args := []string{"compare", "--id", "candidate", "--run", "current", "--baseline", "previous"}
	out, err := executeReleaseCommand(t, append(args, "--json")...)
	require.NoError(t, err)
	require.JSONEq(t, response, out)
	out, err = executeReleaseCommand(t, args...)
	require.NoError(t, err)
	require.Equal(t, msgReleaseComparison(releaseComparison{BaselineBuildID: "old", CurrentBuildID: "new", Changed: 1, Regressed: 1})+"\n", out)
}
