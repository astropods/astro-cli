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
	for _, tc := range []struct {
		name, flag, value string
		byName, stale     bool
	}{
		{name: "default redeploy"},
		{name: "explicit build", flag: "build", value: "build"},
		{name: "wait", flag: "wait", value: "true"},
		{name: "name target", byName: true},
		{name: "stale revision", stale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			var submittedName, expectedRevision string
			conflict := []byte(`{"error":"deployment revision changed"}`)
			setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/deployments":
					_ = json.NewEncoder(w).Encode(listDeploymentsResponse{Deployments: []agentDeployment{{ID: "dep", Name: "dummy", DisplayName: "original"}}})
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
					if tc.stale {
						w.WriteHeader(http.StatusPreconditionFailed)
						_, _ = w.Write(conflict)
						return
					}
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
			if tc.byName {
				require.NoError(t, command.Flags().Set("name", "original"))
			} else {
				require.NoError(t, command.Flags().Set("id", "dep"))
			}
			if tc.flag != "" {
				require.NoError(t, command.Flags().Set(tc.flag, tc.value))
			}
			err := runAgentRedeploy(command, nil)
			if tc.stale {
				require.EqualError(t, err, errDeploymentRevisionConflict(4).Error())
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, `"4"`, expectedRevision)
			require.Equal(t, "renamed", submittedName)
			wantReads := 1
			if tc.flag == "wait" {
				wantReads++
			}
			require.Equal(t, wantReads, reads, "read one configuration snapshot before submission")
		})
	}
}
