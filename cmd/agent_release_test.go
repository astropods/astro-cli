package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
	for _, tc := range []struct {
		name          string
		build         string
		readyRevision int
		wantReady     bool
	}{
		{"ready", "wanted", 2, true},
		{"rollout behind", "wanted", 1, false},
		{"target moved on", "newer", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			build := tc.build
			runtimeReads := 0
			setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/deployments/target/auto-deploy/status":
					// ready is true here so the mismatch case proves the client
					// discards it, rather than passing on an absent field.
					_, err := w.Write([]byte(`{"ready":true,"policy":{"enabled":true,"outcome":"submitted","desired_build_id":"wanted","submitted_revision":2}}`))
					require.NoError(t, err)
				case "/api/v1/deployments/target":
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"deployment": map[string]any{"id": "target", "build_id": build, "current_revision": 2, "status": "Running"}}))
				case "/api/v1/deployments/target/runtime":
					runtimeReads++
					_, err := w.Write([]byte(`{"runtime":{"ready_revision":` + strconv.Itoa(tc.readyRevision) + `}}`))
					require.NoError(t, err)
				default:
					require.FailNow(t, "unexpected request", r.URL.String())
				}
			}))
			raw, err := executeReleaseCommand(t, "status", "--id", "target", "--json")
			require.NoError(t, err)
			var result releaseStatus
			require.NoError(t, json.Unmarshal([]byte(raw), &result))
			require.Equal(t, tc.wantReady, result.Ready)
			if build == "newer" {
				require.Zero(t, runtimeReads, "a moved target is not probed for readiness")
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
				// --apply enables the reviewed plan, so the plan run comes first.
				_, err := executeReleaseCommand(t, args...)
				require.NoError(t, err)
				calls = calls[:0]
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

func TestReleaseSetupApplyRefusesAPlanThatChangedSinceReview(t *testing.T) {
	dir := t.TempDir()
	dataset := filepath.Join(dir, "dataset.json")
	rules := filepath.Join(dir, "rules.json")
	output := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(dataset, []byte(`{"name":"fixture","cases":[{"id":"one","input":true}]}`), 0600))
	require.NoError(t, os.WriteFile(rules, []byte(`[{"key":"accuracy","equals":true,"min_confidence":0.8}]`), 0600))
	version := "pinned"
	writes := 0
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/build-evaluation-preflight"):
			_, err := w.Write([]byte(`{"candidate_id":"candidate","target_id":"target","build_id":"build","revision":1,"policy":{"schema":"acceptance/v1"},"version":"` + version + `"}`))
			require.NoError(t, err)
		default:
			writes++
			w.WriteHeader(204)
		}
	}))
	args := []string{"setup", "--id", "target", "--candidate", "candidate", "--dataset", dataset, "--rules", rules, "--runner", "app:runner", "--output", output}
	_, err := executeReleaseCommand(t, args...)
	require.NoError(t, err)

	// The candidate's evaluator set moves on between review and apply.
	version = "different"
	_, err = executeReleaseCommand(t, append(args, "--apply")...)
	require.EqualError(t, err, errReleasePlanChanged(output).Error())
	require.Zero(t, writes, "nothing is enabled against an unreviewed plan")

	raw, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"pinned"`, "the reviewed plan survives a refusal")
}

func TestReleaseRetryReportsAnAlreadyAppliedRequest(t *testing.T) {
	requestID := "11111111-1111-4111-8111-111111111111"
	posts := 0
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts++
		}
		_, err := w.Write([]byte(`{"version":"` + requestID + `"}`))
		require.NoError(t, err)
	}))
	_, err := executeReleaseCommand(t, "retry", "--id", "target", "--build", "build", "--request-id", requestID)
	require.EqualError(t, err, errReleaseRetryAlreadyApplied(requestID).Error())
	require.Zero(t, posts, "a request that already landed is not sent again")
}

func TestReleaseSetupApplyRefusesAPlanReviewedForAnotherTarget(t *testing.T) {
	dir := t.TempDir()
	dataset := filepath.Join(dir, "dataset.json")
	rules := filepath.Join(dir, "rules.json")
	output := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(dataset, []byte(`{"name":"fixture","cases":[{"id":"one","input":true}]}`), 0600))
	require.NoError(t, os.WriteFile(rules, []byte(`[{"key":"accuracy","equals":true,"min_confidence":0.8}]`), 0600))
	writes := 0
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/build-evaluation-preflight") {
			writes++
			w.WriteHeader(204)
			return
		}
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		// Same candidate and policy, so version, build and revision all match.
		_, err := w.Write([]byte(`{"candidate_id":"candidate","target_id":"` + body["target_id"].(string) + `","build_id":"build","revision":1,"policy":{"schema":"acceptance/v1"},"version":"pinned"}`))
		require.NoError(t, err)
	}))
	common := []string{"--candidate", "candidate", "--dataset", dataset, "--rules", rules, "--runner", "app:runner", "--output", output}
	_, err := executeReleaseCommand(t, append([]string{"setup", "--id", "staging"}, common...)...)
	require.NoError(t, err)

	_, err = executeReleaseCommand(t, append(append([]string{"setup", "--id", "prod"}, common...), "--apply")...)
	require.EqualError(t, err, errReleasePlanChanged(output).Error())
	require.Zero(t, writes, "a target absent from the reviewed plan is never configured")
}

func TestReleaseSetupApplyStopsOnACandidateThatIsAlreadyGated(t *testing.T) {
	dir := t.TempDir()
	dataset := filepath.Join(dir, "dataset.json")
	rules := filepath.Join(dir, "rules.json")
	output := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(dataset, []byte(`{"name":"fixture","cases":[{"id":"one","input":true}]}`), 0600))
	require.NoError(t, os.WriteFile(rules, []byte(`[{"key":"accuracy","equals":true,"min_confidence":0.8}]`), 0600))
	mode := "published"
	writes := 0
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/build-evaluation-preflight"):
			_, err := w.Write([]byte(`{"candidate_id":"candidate","target_id":"target","build_id":"build","revision":1,"policy":{"schema":"acceptance/v1"},"version":"pinned"}`))
			require.NoError(t, err)
		case r.Method == http.MethodGet:
			_, err := w.Write([]byte(`{"mode":"` + mode + `"}`))
			require.NoError(t, err)
		default:
			writes++
			w.WriteHeader(204)
		}
	}))
	args := []string{"setup", "--id", "target", "--candidate", "candidate", "--dataset", dataset, "--rules", rules, "--runner", "app:runner", "--output", output}
	_, err := executeReleaseCommand(t, args...)
	require.NoError(t, err)

	mode = "accepted"
	_, err = executeReleaseCommand(t, append(args, "--apply")...)
	require.EqualError(t, err, errReleaseCandidatePolicy().Error())
	require.Zero(t, writes, "a candidate that is itself gated is refused before any write")
}

func TestReleaseSetupApplyReportsAPartialConfiguration(t *testing.T) {
	dir := t.TempDir()
	dataset := filepath.Join(dir, "dataset.json")
	rules := filepath.Join(dir, "rules.json")
	output := filepath.Join(dir, "plan.json")
	require.NoError(t, os.WriteFile(dataset, []byte(`{"name":"fixture","cases":[{"id":"one","input":true}]}`), 0600))
	require.NoError(t, os.WriteFile(rules, []byte(`[{"key":"accuracy","equals":true,"min_confidence":0.8}]`), 0600))
	apply := false
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/build-evaluation-preflight"):
			_, err := w.Write([]byte(`{"candidate_id":"candidate","target_id":"target","build_id":"build","revision":1,"policy":{"schema":"acceptance/v1"},"version":"pinned"}`))
			require.NoError(t, err)
		case r.Method == http.MethodGet:
			_, err := w.Write([]byte(`{"mode":"published"}`))
			require.NoError(t, err)
		case r.URL.Query().Get("dryrun") == "true":
			w.WriteHeader(204)
		case apply && r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/target/"):
			// The candidate is already enabled when the target write fails.
			w.WriteHeader(503)
		default:
			_, err := w.Write([]byte(`{"version":"pinned"}`))
			require.NoError(t, err)
		}
	}))
	args := []string{"setup", "--id", "target", "--candidate", "candidate", "--dataset", dataset, "--rules", rules, "--runner", "app:runner", "--output", output}
	_, err := executeReleaseCommand(t, args...)
	require.NoError(t, err)

	apply = true
	_, err = executeReleaseCommand(t, append(args, "--apply")...)
	require.Error(t, err)
	require.Contains(t, err.Error(), errReleaseSetupPartial(errors.New("x")).Error()[:40])
}

func TestReleaseRetryDoesNotAdviseResumingAfterARejection(t *testing.T) {
	version := uuid.NewString()
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusConflict)
		_, err := w.Write([]byte(`{"error":"publication superseded"}`))
		require.NoError(t, err)
	}))
	_, err := executeReleaseCommand(t, "retry", "--id", "target", "--build", "build", "--policy-version", version)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "resume with", "a rejected retry repeats; resuming is wrong advice")
	require.Contains(t, err.Error(), "publication superseded")
}

func TestReleaseRetryNamesADeploymentWithNoPolicy(t *testing.T) {
	setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		_, err := w.Write([]byte(`{"enabled":false}`))
		require.NoError(t, err)
	}))
	_, err := executeReleaseCommand(t, "retry", "--id", "target", "--build", "build")
	require.EqualError(t, err, errReleaseNoPolicyToRetry("target").Error())
}
