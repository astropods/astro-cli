package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAgentRedeployTest(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	agentServerURLOverride = srv.URL
	blueprintServerURLOverride = srv.URL
	t.Cleanup(func() {
		agentServerURLOverride = ""
		blueprintServerURLOverride = ""
	})
}

func setRedeployFlag(t *testing.T, name, value string) {
	t.Helper()
	flags := agentRedeployCmd.Flags()
	require.NoError(t, flags.Set(name, value))
	t.Cleanup(func() {
		f := flags.Lookup(name)
		if sv, ok := f.Value.(interface{ Replace([]string) error }); ok {
			require.NoError(t, sv.Replace(nil))
		} else {
			require.NoError(t, f.Value.Set(f.DefValue))
		}
		f.Changed = false
	})
}

func TestRunAgentRedeploy(t *testing.T) {
	deployments := []agentDeployment{
		{ID: "dep-123", Name: "weather-bp", DisplayName: "weather-agent", BuildID: "build-abc", Status: "active"},
	}
	listResp := listDeploymentsResponse{Deployments: deployments, Count: 1}

	validTemplate := json.RawMessage(`{"spec":"deployment/v1","source":{"name":"weather-bp"}}`)
	validTmplResp := map[string]any{
		"template":   json.RawMessage(validTemplate),
		"validation": map[string]any{"valid": true},
	}
	deployResp := map[string]any{
		"status":        "pending",
		"deployment_id": "dep-123",
		"name":          "weather-agent",
		"build_id":      "build-abc",
	}

	cases := []struct {
		name       string
		tmplStatus int
		tmplResp   any
		deplStatus int
		deplResp   any
		dryRun     bool
		jsonOut    bool
		wantErr    string
		wantOut    string
		wantNoDep  bool
	}{
		{
			name:       "happy path redeployed",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusAccepted,
			deplResp:   deployResp,
			wantOut:    "✓ deployed",
		},
		{
			name:       "banner says Redeploying",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusAccepted,
			deplResp:   deployResp,
			wantOut:    "Redeploying",
		},
		{
			name:       "banner includes blueprint and agent names",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusAccepted,
			deplResp:   deployResp,
			wantOut:    "weather-bp",
		},
		{
			name:       "dry run does not call deploy",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			dryRun:     true,
			wantOut:    "template valid",
			wantNoDep:  true,
		},
		{
			name:       "json output",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusAccepted,
			deplResp:   deployResp,
			jsonOut:    true,
			wantOut:    `"deployment_id"`,
		},
		{
			name:       "agent not found",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			wantErr:    errAgentDeploymentNotFound("unknown-agent").Error(),
			wantNoDep:  true,
		},
		{
			name:       "deploy endpoint 404 reports deployment no longer exists",
			tmplStatus: http.StatusOK,
			tmplResp:   validTmplResp,
			deplStatus: http.StatusNotFound,
			deplResp:   map[string]any{"error": "not found"},
			wantErr:    `agent deployment "weather-agent" no longer exists`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deployCalled := false

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api/v1/deployments":
					if tc.name == "agent not found" {
						jsonHandler(http.StatusOK, listDeploymentsResponse{})(w, r)
					} else {
						jsonHandler(http.StatusOK, listResp)(w, r)
					}
				case strings.HasSuffix(r.URL.Path, "/deployment-template"):
					jsonHandler(tc.tmplStatus, tc.tmplResp)(w, r)
				case r.URL.Path == "/api/v1/deploy":
					deployCalled = true
					jsonHandler(tc.deplStatus, tc.deplResp)(w, r)
				default:
					http.NotFound(w, r)
				}
			})
			setupAgentRedeployTest(t, handler)

			if tc.dryRun {
				require.NoError(t, agentRedeployCmd.Flags().Set("dry-run", "true"))
				t.Cleanup(func() { agentRedeployCmd.Flags().Set("dry-run", "false") }) //nolint:errcheck
			}
			if tc.jsonOut {
				require.NoError(t, agentRedeployCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { agentRedeployCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			agentRedeployCmd.SetOut(buf)
			agentRedeployCmd.SetContext(context.Background())

			name := "weather-agent"
			if tc.name == "agent not found" {
				name = "unknown-agent"
			}
			setAgentTargetName(t, agentRedeployCmd, name)
			err := runAgentRedeploy(agentRedeployCmd, nil)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tc.wantOut != "" {
				assert.Contains(t, stripANSI(buf.String()), tc.wantOut)
			}
			if tc.wantNoDep {
				assert.False(t, deployCalled, "deploy POST should not have been called")
			}
		})
	}
}

func makeRedeployCapturingHandler(t *testing.T, deployments []agentDeployment, captured *deployTemplateRequest) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/deployments":
			jsonHandler(http.StatusOK, listDeploymentsResponse{Deployments: deployments, Count: len(deployments)})(w, r)
		case strings.HasSuffix(r.URL.Path, "/deployment-template"):
			if err := json.NewDecoder(r.Body).Decode(captured); err != nil {
				http.Error(w, "bad body", http.StatusBadRequest)
				return
			}
			jsonHandler(http.StatusOK, map[string]any{
				"template":   json.RawMessage(`{}`),
				"validation": map[string]any{"valid": true},
			})(w, r)
		case r.URL.Path == "/api/v1/deploy":
			jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": deployments[0].ID})(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func TestRunAgentRedeployPassesDeploymentID(t *testing.T) {
	deployments := []agentDeployment{
		{ID: "dep-xyz", Name: "my-bp", DisplayName: "my-agent", BuildID: "build-1", Status: "active"},
	}
	var captured deployTemplateRequest
	setupAgentRedeployTest(t, makeRedeployCapturingHandler(t, deployments, &captured))

	buf := &bytes.Buffer{}
	agentRedeployCmd.SetOut(buf)
	agentRedeployCmd.SetContext(context.Background())

	setAgentTargetName(t, agentRedeployCmd, "my-agent")
	require.NoError(t, runAgentRedeploy(agentRedeployCmd, nil))
	assert.Equal(t, "dep-xyz", captured.DeploymentID)
}

func TestRunAgentRedeployDefaultsToWebOIDC(t *testing.T) {
	deployments := []agentDeployment{
		{ID: "dep-1", Name: "my-bp", DisplayName: "my-agent", Status: "active"},
	}
	var captured deployTemplateRequest
	setupAgentRedeployTest(t, makeRedeployCapturingHandler(t, deployments, &captured))

	buf := &bytes.Buffer{}
	agentRedeployCmd.SetOut(buf)
	agentRedeployCmd.SetContext(context.Background())

	setAgentTargetName(t, agentRedeployCmd, "my-agent")
	require.NoError(t, runAgentRedeploy(agentRedeployCmd, nil))
	require.NotNil(t, captured.Interfaces)
	assert.Equal(t, []string{"web"}, captured.Interfaces.Adapters)
	require.NotNil(t, captured.Interfaces.Auth)
	assert.Equal(t, "oidc", captured.Interfaces.Auth.Web.Type)
}

func TestRunAgentRedeployAdapterOverride(t *testing.T) {
	deployments := []agentDeployment{
		{ID: "dep-1", Name: "my-bp", DisplayName: "my-agent", Status: "active"},
	}
	var captured deployTemplateRequest
	setupAgentRedeployTest(t, makeRedeployCapturingHandler(t, deployments, &captured))

	setRedeployFlag(t, "adapter", "slack")

	buf := &bytes.Buffer{}
	agentRedeployCmd.SetOut(buf)
	agentRedeployCmd.SetContext(context.Background())

	setAgentTargetName(t, agentRedeployCmd, "my-agent")
	require.NoError(t, runAgentRedeploy(agentRedeployCmd, nil))
	require.NotNil(t, captured.Interfaces)
	assert.Equal(t, []string{"slack"}, captured.Interfaces.Adapters)
	assert.Nil(t, captured.Interfaces.Auth, "slack adapter should have no auth")
}

func TestRunAgentRedeployPassesCluster(t *testing.T) {
	deployments := []agentDeployment{
		{ID: "dep-1", Name: "my-bp", DisplayName: "my-agent", Status: "active"},
	}
	var captured deployTemplateRequest
	setupAgentRedeployTest(t, makeRedeployCapturingHandler(t, deployments, &captured))

	setRedeployFlag(t, "cluster", "us-east-1-managed")

	agentRedeployCmd.SetOut(&bytes.Buffer{})
	agentRedeployCmd.SetContext(context.Background())

	setAgentTargetName(t, agentRedeployCmd, "my-agent")
	require.NoError(t, runAgentRedeploy(agentRedeployCmd, nil))
	assert.Equal(t, "us-east-1-managed", captured.ClusterID)
}

func TestRunAgentRedeployKeepsCurrentClusterByDefault(t *testing.T) {
	deployments := []agentDeployment{
		{ID: "dep-1", Name: "my-bp", DisplayName: "my-agent", Status: "active"},
	}
	var captured deployTemplateRequest
	setupAgentRedeployTest(t, makeRedeployCapturingHandler(t, deployments, &captured))
	stubInteractiveTerminal(t, true)

	agentRedeployCmd.SetOut(&bytes.Buffer{})
	agentRedeployCmd.SetContext(context.Background())

	setAgentTargetName(t, agentRedeployCmd, "my-agent")
	require.NoError(t, runAgentRedeploy(agentRedeployCmd, nil))
	assert.Empty(t, captured.ClusterID)
}

func TestRunAgentRedeployInvalidAdapterBeforeAPICall(t *testing.T) {
	apiCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		http.NotFound(w, r)
	})
	setupAgentRedeployTest(t, handler)

	setRedeployFlag(t, "adapter", "xyz")

	agentRedeployCmd.SetOut(&bytes.Buffer{})
	agentRedeployCmd.SetContext(context.Background())

	setAgentTargetName(t, agentRedeployCmd, "weather-agent")
	err := runAgentRedeploy(agentRedeployCmd, nil)
	require.ErrorContains(t, err, "unknown adapter")
	assert.False(t, apiCalled, "API should not be called when adapter is invalid")
}
