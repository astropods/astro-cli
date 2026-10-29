package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRedeployPreservesTheTemplateNameAfterConcurrentConfigurationChanges(t *testing.T) {
	reads := 0
	var submittedName, expectedRevision string
	setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/deployments/dep":
			reads++
			name, revision := "original", 4
			if reads > 1 {
				revision = 5
			}
			_ = json.NewEncoder(w).Encode(agentDeploymentFullResponse{Deployment: agentDeploymentFull{ID: "dep", Name: "dummy", DisplayName: name, BuildID: "build", CurrentRevision: revision, SourceAccount: "testaccount", Status: "Running"}})
		case "/api/v1/agents/testaccount/dummy/deployment-template":
			_ = json.NewEncoder(w).Encode(deployTemplateResponse{Template: json.RawMessage(`{"target":{"display_name":"renamed"},"source":{"build":"build"}}`), Validation: deployTemplateValidation{Valid: true}, Signature: "signature"})
		case "/api/v1/deploy":
			var payload struct {
				Target struct {
					DisplayName string `json:"display_name"`
				} `json:"target"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			submittedName = payload.Target.DisplayName
			expectedRevision = r.Header.Get("If-Match")
			_ = json.NewEncoder(w).Encode(agentDeployResult{DeploymentID: "dep", BuildID: "build", Revision: 5, Status: "pending"})
		case "/api/v1/deployments/dep/runtime":
			_, _ = io.WriteString(w, `{"runtime":{"ready_revision":5}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	command := &cobra.Command{}
	registerDeployCommonFlags(command)
	registerAgentTargetFlags(command)
	command.Flags().Bool("latest", false, "")
	command.SetContext(context.Background())
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	require.NoError(t, command.Flags().Set("id", "dep"))
	require.NoError(t, command.Flags().Set("wait", "true"))
	require.NoError(t, runAgentRedeploy(command, nil))
	require.Equal(t, `"4"`, expectedRevision)
	require.Equal(t, "renamed", submittedName)
	require.Equal(t, 2, reads, "read the deployment once before submission and once while waiting")
}
