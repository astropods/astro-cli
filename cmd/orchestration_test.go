package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func fastOrchestration(t *testing.T) {
	t.Helper()
	previous := orchestrationPollInterval
	orchestrationPollInterval = time.Millisecond
	t.Cleanup(func() { orchestrationPollInterval = previous })
}

func TestCommitDeployWaitsForPublishedBuildAndReadyRevisionWithJSON(t *testing.T) {
	fastOrchestration(t)
	sha := strings.Repeat("a", 40)
	buildPolls, runtimePolls, deployPosts := 0, 0, 0
	reads := make(map[string]int)
	setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			reads[r.URL.Path]++
			if reads[r.URL.Path] == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		}
		switch r.URL.Path {
		case "/api/v1/agents/testaccount/dummy/builds":
			require.Equal(t, sha, r.URL.Query().Get("commit"))
			buildPolls++
			status := "building"
			if buildPolls > 1 {
				status = "registered"
				require.Equal(t, "exact-build", r.URL.Query().Get("build_id"))
			}
			_ = json.NewEncoder(w).Encode(blueprintBuildsResponse{Builds: []blueprintBuild{{BuildID: "exact-build", CommitSHA: sha, Status: status, Source: "github"}}})
		case "/api/v1/agents/testaccount/dummy/deployment-template":
			var req deployTemplateRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "exact-build", req.Build)
			require.Nil(t, req.Interfaces)
			_ = json.NewEncoder(w).Encode(deployTemplateResponse{Template: json.RawMessage(`{"spec":"deployment/v1","source":{"build":"exact-build"}}`), Validation: deployTemplateValidation{Valid: true}, Signature: "signature"})
		case "/api/v1/deploy":
			deployPosts++
			require.Equal(t, `"3"`, r.Header.Get("If-Match"))
			require.Equal(t, "signature", r.Header.Get("X-Template-Signature"))
			_ = json.NewEncoder(w).Encode(agentDeployResult{DeploymentID: "dep", BuildID: "exact-build", Revision: 4, Status: "pending"})
		case "/api/v1/deployments/dep":
			_ = json.NewEncoder(w).Encode(agentDeploymentFullResponse{Deployment: agentDeploymentFull{ID: "dep", BuildID: "exact-build", CurrentRevision: 4, Status: "Running"}})
		case "/api/v1/deployments/dep/runtime":
			runtimePolls++
			revision := 3
			if runtimePolls > 1 {
				revision = 4
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"runtime": map[string]any{"ready_revision": revision}})
		default:
			require.Failf(t, "unexpected request", "%s", r.URL)
			http.NotFound(w, r)
		}
	}))
	cmd := &cobra.Command{}
	registerDeployCommonFlags(cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set("commit", sha))
	require.NoError(t, cmd.Flags().Set("wait", "true"))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := runDeployWithRequest(cmd, AccountToken{Account: "testaccount", Token: "test-token"}, false, deployOptions{
		TemplateAccount: "testaccount", Blueprint: "dummy", DisplayName: "Dummy", ExpectedRevision: 3,
		Request: deployTemplateRequest{DeploymentID: "dep"},
	})
	require.NoError(t, err)
	var result agentDeployResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	require.Equal(t, "active", result.Status)
	require.Equal(t, sha, result.CommitSHA)
	require.Equal(t, 4, result.Revision)
	require.Equal(t, 1, deployPosts)
	require.Equal(t, 2, runtimePolls)
}

func TestBuildWaitRejectsTerminalAndMismatchedBuilds(t *testing.T) {
	sha := strings.Repeat("b", 40)
	for _, status := range []string{"failed", "cancelled", "skipped"} {
		t.Run(status, func(t *testing.T) {
			setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(blueprintBuildsResponse{Builds: []blueprintBuild{{BuildID: "build", CommitSHA: sha, Status: status, Source: "github"}}})
			}))
			_, err := waitForBuild(context.Background(), AccountToken{Account: "testaccount"}, "testaccount", "dummy", "", sha, false, io.Discard)
			require.EqualError(t, err, errBuildNotPublished("build", status, "").Error())
		})
	}
	t.Run("server ignores selector", func(t *testing.T) {
		setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(blueprintBuildsResponse{Builds: []blueprintBuild{{BuildID: "wrong", CommitSHA: strings.Repeat("c", 40), Status: "registered"}}})
		}))
		_, err := waitForBuild(context.Background(), AccountToken{Account: "testaccount"}, "testaccount", "dummy", "", sha, false, io.Discard)
		require.EqualError(t, err, errBuildSelectorUnsupported().Error())
	})
}

func TestDeploymentWaitTimesOutWithoutRevisionReadinessAndDetectsReplacement(t *testing.T) {
	fastOrchestration(t)
	expected := agentDeployResult{DeploymentID: "dep", BuildID: "build", Revision: 2}
	for _, revision := range []int{2, 3} {
		t.Run(string(rune('0'+revision)), func(t *testing.T) {
			setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/runtime") {
					_, _ = io.WriteString(w, `{"runtime":{"ready_revision":1}}`)
					return
				}
				_ = json.NewEncoder(w).Encode(agentDeploymentFullResponse{Deployment: agentDeploymentFull{ID: "dep", BuildID: "build", CurrentRevision: revision, Status: "Running"}})
			}))
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			err := waitForDeployment(ctx, expected, AccountToken{Account: "testaccount"}, false, io.Discard)
			if revision == 3 {
				require.EqualError(t, err, errDeploymentSuperseded(expected).Error())
			} else {
				require.True(t, errors.Is(err, context.DeadlineExceeded), "%v", err)
			}
		})
	}
}

func TestHeadlessAccountTokenNeedsNoStoredProfile(t *testing.T) {
	if os.Getenv("ASTRO_ORCHESTRATION_AUTH_CHILD") == "1" {
		at, err := getCurrentAccountToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, "ci-account", at.Account)
		require.Equal(t, "synthetic-test-token", at.Token)
		require.Empty(t, os.Getenv("ASTRO_ACCESS_TOKEN"))
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	child := exec.Command(executable, "-test.run=^TestHeadlessAccountTokenNeedsNoStoredProfile$")
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "HOME=") && !strings.HasPrefix(v, "ASTRO_ACCOUNT=") && !strings.HasPrefix(v, "ASTRO_ACCESS_TOKEN=") {
			child.Env = append(child.Env, v)
		}
	}
	child.Env = append(child.Env, "HOME="+t.TempDir(), "ASTRO_ORCHESTRATION_AUTH_CHILD=1", "ASTRO_ACCOUNT=ci-account", "ASTRO_ACCESS_TOKEN=synthetic-test-token")
	output, err := child.CombinedOutput()
	require.NoError(t, err, "%s", output)
}

func TestMachineTokenWithoutAccountIsRejected(t *testing.T) {
	if os.Getenv("ASTRO_ORCHESTRATION_AUTH_CHILD") == "1" {
		t.Setenv("HOME", t.TempDir())
		writeAccountTestCredentials(t, accountTestCreds("acme-corp"))
		_, err := getCurrentAccountToken(context.Background())
		require.EqualError(t, err, errMachineTokenNeedsAccount().Error())
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	child := exec.Command(executable, "-test.run=^TestMachineTokenWithoutAccountIsRejected$")
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "ASTRO_ACCOUNT=") && !strings.HasPrefix(v, "ASTRO_ACCESS_TOKEN=") {
			child.Env = append(child.Env, v)
		}
	}
	child.Env = append(child.Env, "ASTRO_ORCHESTRATION_AUTH_CHILD=1", "ASTRO_ACCESS_TOKEN=synthetic-test-token")
	output, err := child.CombinedOutput()
	require.NoError(t, err, "%s", output)
}

func TestDeployWaitSucceedsWhenTheServerReportsNoRevision(t *testing.T) {
	fastOrchestration(t)
	deployPosts, runtimePolls := 0, 0
	setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agents/testaccount/dummy/deployment-template":
			_ = json.NewEncoder(w).Encode(deployTemplateResponse{Template: json.RawMessage(`{"spec":"deployment/v1"}`), Validation: deployTemplateValidation{Valid: true}, Signature: "signature"})
		case "/api/v1/deploy":
			deployPosts++
			_ = json.NewEncoder(w).Encode(agentDeployResult{DeploymentID: "dep", BuildID: "exact-build", Status: "pending"})
		case "/api/v1/deployments/dep/runtime":
			runtimePolls++
			_ = json.NewEncoder(w).Encode(map[string]any{"runtime": map[string]any{}})
		default:
			require.Failf(t, "unexpected request", "%s", r.URL)
		}
	}))
	cmd := &cobra.Command{}
	registerDeployCommonFlags(cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set("wait", "true"))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := runDeployWithRequest(cmd, AccountToken{Account: "testaccount", Token: "test-token"}, false, deployOptions{
		TemplateAccount: "testaccount", Blueprint: "dummy", DisplayName: "Dummy",
	})

	require.NoError(t, err, "a submitted deployment must not fail the command")
	require.Equal(t, 1, deployPosts)
	require.Zero(t, runtimePolls, "no revision to wait for")
	require.Contains(t, stderr.String(), msgDeploymentWaitUnsupported("dep"))
	var result agentDeployResult
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	require.Equal(t, "dep", result.DeploymentID)
}

func TestDeployWaitWithAnExplicitBuildSkipsThePublishersBuildHistory(t *testing.T) {
	fastOrchestration(t)
	buildReads, runtimePolls := 0, 0
	setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/builds"):
			// Another account's build history answers 404 to a reader.
			buildReads++
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/deployment-template"):
			_ = json.NewEncoder(w).Encode(deployTemplateResponse{Template: json.RawMessage(`{"spec":"deployment/v1"}`), Validation: deployTemplateValidation{Valid: true}, Signature: "signature"})
		case r.URL.Path == "/api/v1/deploy":
			_ = json.NewEncoder(w).Encode(agentDeployResult{DeploymentID: "dep", BuildID: "exact-build", Revision: 4, Status: "pending"})
		case r.URL.Path == "/api/v1/deployments/dep":
			_ = json.NewEncoder(w).Encode(agentDeploymentFullResponse{Deployment: agentDeploymentFull{ID: "dep", BuildID: "exact-build", CurrentRevision: 4, Status: "Running"}})
		case r.URL.Path == "/api/v1/deployments/dep/runtime":
			runtimePolls++
			_ = json.NewEncoder(w).Encode(map[string]any{"runtime": map[string]any{"ready_revision": 4}})
		default:
			require.Failf(t, "unexpected request", "%s", r.URL)
		}
	}))
	cmd := &cobra.Command{}
	registerDeployCommonFlags(cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Set("wait", "true"))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := runDeployWithRequest(cmd, AccountToken{Account: "mine", Token: "test-token"}, false, deployOptions{
		TemplateAccount: "acme", Blueprint: "dummy", DisplayName: "Dummy",
		Request: deployTemplateRequest{DeploymentID: "dep", Build: "exact-build"},
	})

	require.NoError(t, err, "a cross-account deploy with an explicit build must not need build history")
	require.Zero(t, buildReads, "an already published build is not waited for")
	require.Equal(t, 1, runtimePolls)
}

func TestDeploymentWaitStopsOnAFailedRollout(t *testing.T) {
	fastOrchestration(t)
	reads := 0
	setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/deployments/dep", r.URL.Path)
		reads++
		_ = json.NewEncoder(w).Encode(agentDeploymentFullResponse{Deployment: agentDeploymentFull{
			ID: "dep", BuildID: "one", CurrentRevision: 2, Status: "failed", ErrorMessage: "image pull failed",
		}})
	}))
	err := waitForDeployment(context.Background(), agentDeployResult{DeploymentID: "dep", BuildID: "one", Revision: 2},
		AccountToken{Account: "testaccount", Token: "test-token"}, false, io.Discard)
	require.EqualError(t, err, errDeploymentFailed("dep", "failed", "image pull failed").Error())
	require.Equal(t, 1, reads, "a failed rollout is not polled until the deadline")
}

func TestRedeployWaitDowngradesOnAServerWithoutRevisions(t *testing.T) {
	fastOrchestration(t)
	deployPosts := 0
	setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/deployment-template"):
			_ = json.NewEncoder(w).Encode(deployTemplateResponse{Template: json.RawMessage(`{"spec":"deployment/v1"}`), Validation: deployTemplateValidation{Valid: true}, Signature: "signature"})
		case r.URL.Path == "/api/v1/deploy":
			deployPosts++
			_ = json.NewEncoder(w).Encode(agentDeployResult{DeploymentID: "dep", BuildID: "one", Status: "pending"})
		case r.URL.Path == "/api/v1/deployments/dep":
			// A pre-revision server reports no current_revision.
			_ = json.NewEncoder(w).Encode(agentDeploymentFullResponse{Deployment: agentDeploymentFull{ID: "dep", Name: "dummy", BuildID: "one", Status: "Running"}})
		default:
			require.Failf(t, "unexpected request", "%s", r.URL)
		}
	}))
	cmd := &cobra.Command{}
	registerDeployCommonFlags(cmd)
	registerAgentTargetFlags(cmd)
	cmd.Flags().Bool("latest", false, "")
	cmd.SetContext(context.Background())
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	require.NoError(t, cmd.Flags().Set("id", "dep"))
	require.NoError(t, cmd.Flags().Set("wait", "true"))

	require.NoError(t, runAgentRedeploy(cmd, nil), "a bare --wait matches blueprint deploy and does not fail before submitting")
	require.Equal(t, 1, deployPosts)
}

func TestMachineTokenRejectsAnotherAccount(t *testing.T) {
	if os.Getenv("ASTRO_ORCHESTRATION_AUTH_CHILD") == "1" {
		t.Setenv("HOME", t.TempDir())
		writeAccountTestCredentials(t, accountTestCreds("acme-corp"))
		_, err := getAccountToken(context.Background(), "other-org")
		require.EqualError(t, err, errMachineTokenAccountMismatch("ci-account", "other-org").Error())
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	child := exec.Command(executable, "-test.run=^TestMachineTokenRejectsAnotherAccount$")
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "ASTRO_ACCOUNT=") && !strings.HasPrefix(v, "ASTRO_ACCESS_TOKEN=") {
			child.Env = append(child.Env, v)
		}
	}
	child.Env = append(child.Env, "ASTRO_ORCHESTRATION_AUTH_CHILD=1", "ASTRO_ACCOUNT=ci-account", "ASTRO_ACCESS_TOKEN=synthetic-test-token")
	output, err := child.CombinedOutput()
	require.NoError(t, err, "%s", output)
}
