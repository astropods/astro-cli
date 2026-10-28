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
	setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	err := runDeployWithRequest(cmd, AccountToken{Account: "testaccount", Token: "test-token"}, false, "testaccount", "dummy", "Dummy", nil, deployTemplateRequest{DeploymentID: "dep", ExpectedRevision: 3}, false)
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
