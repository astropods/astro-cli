package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDeploySchedules(t *testing.T) {
	cases := []struct {
		name    string
		flags   []string
		want    map[string]string
		wantErr error
	}{
		{
			name:  "one ingestion",
			flags: []string{"weekly-sync=0 3 * * *"},
			want:  map[string]string{"weekly-sync": "0 3 * * *"},
		},
		{
			name:  "several ingestions",
			flags: []string{"weekly-sync=0 3 * * *", "hourly-import=0 * * * *"},
			want:  map[string]string{"weekly-sync": "0 3 * * *", "hourly-import": "0 * * * *"},
		},
		{
			name:  "surrounding spaces are trimmed",
			flags: []string{" weekly-sync = 0 3 * * * "},
			want:  map[string]string{"weekly-sync": "0 3 * * *"},
		},
		{
			name:    "no equals sign",
			flags:   []string{"weekly-sync"},
			wantErr: errInvalidSchedule("weekly-sync"),
		},
		{
			name:    "no ingestion name",
			flags:   []string{"=0 3 * * *"},
			wantErr: errInvalidSchedule("=0 3 * * *"),
		},
		{
			name:    "no cron expression",
			flags:   []string{"weekly-sync="},
			wantErr: errInvalidSchedule("weekly-sync="),
		},
		{
			name:    "same ingestion twice",
			flags:   []string{"weekly-sync=0 3 * * *", "weekly-sync=0 4 * * *"},
			wantErr: errDuplicateSchedule("weekly-sync"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDeploySchedules(tc.flags)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.wantErr.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCheckScheduleTargets(t *testing.T) {
	available := map[string]string{"weekly-sync": "0 3 * * *", "hourly-import": "0 * * * *"}

	cases := []struct {
		name      string
		requested map[string]string
		available map[string]string
		wantErr   error
	}{
		{name: "no schedules requested", available: available},
		{
			name:      "requested ingestion runs on a schedule",
			requested: map[string]string{"weekly-sync": "0 4 * * *"},
			available: available,
		},
		{
			name:      "unknown ingestion",
			requested: map[string]string{"nightly": "0 4 * * *"},
			available: available,
			wantErr:   errUnknownIngestionSchedule([]string{"nightly"}, []string{"hourly-import", "weekly-sync"}),
		},
		{
			name:      "blueprint schedules nothing",
			requested: map[string]string{"nightly": "0 4 * * *"},
			wantErr:   errUnknownIngestionSchedule([]string{"nightly"}, nil),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkScheduleTargets(tc.requested, tc.available)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.Equal(t, tc.wantErr.Error(), err.Error())
				return
			}
			assert.NoError(t, err)
		})
	}
}

func scheduleTemplateHandler(t *testing.T, deployments []agentDeployment, schedules map[string]string, captured *deployTemplateRequest, deployed *bool) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/deployments":
			jsonHandler(http.StatusOK, listDeploymentsResponse{Deployments: deployments, Count: len(deployments)})(w, r)
		case strings.HasSuffix(r.URL.Path, "/deployment-template"):
			require.NoError(t, json.NewDecoder(r.Body).Decode(captured))
			jsonHandler(http.StatusOK, map[string]any{
				"template":   json.RawMessage(`{}`),
				"validation": map[string]any{"valid": true},
				"schedules":  schedules,
			})(w, r)
		case r.URL.Path == "/api/v1/deploy":
			*deployed = true
			jsonHandler(http.StatusAccepted, map[string]any{"status": "pending", "deployment_id": "dep-1"})(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func setDeployFlag(t *testing.T, name, value string) {
	t.Helper()
	blueprintDeployCmd.ResetFlags()
	registerDeployFlags(blueprintDeployCmd)
	require.NoError(t, blueprintDeployCmd.Flags().Set(name, value))
	t.Cleanup(func() {
		blueprintDeployCmd.ResetFlags()
		registerDeployFlags(blueprintDeployCmd)
	})
}

func TestRunBlueprintDeployPassesSchedules(t *testing.T) {
	var captured deployTemplateRequest
	deployed := false
	setupBlueprintDeployTest(t, scheduleTemplateHandler(t, nil, map[string]string{"weekly-sync": "0 3 * * *"}, &captured, &deployed))

	setDeployFlag(t, "schedule", "weekly-sync=0 4 * * *")
	blueprintDeployCmd.SetOut(&bytes.Buffer{})
	blueprintDeployCmd.SetContext(context.Background())

	require.NoError(t, runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"}))
	assert.Equal(t, map[string]string{"weekly-sync": "0 4 * * *"}, captured.Schedules)
	assert.True(t, deployed)
}

func TestRunBlueprintDeployRejectsUnknownSchedule(t *testing.T) {
	var captured deployTemplateRequest
	deployed := false
	setupBlueprintDeployTest(t, scheduleTemplateHandler(t, nil, map[string]string{"weekly-sync": "0 3 * * *"}, &captured, &deployed))

	setDeployFlag(t, "schedule", "nightly-sync=0 4 * * *")
	blueprintDeployCmd.SetOut(&bytes.Buffer{})
	blueprintDeployCmd.SetContext(context.Background())

	err := runBlueprintDeploy(blueprintDeployCmd, []string{"my-agent"})
	require.Error(t, err)
	assert.Equal(t, errUnknownIngestionSchedule([]string{"nightly-sync"}, []string{"weekly-sync"}).Error(), err.Error())
	assert.False(t, deployed, "an unknown ingestion must stop the deploy")
}

func TestRunAgentRedeployPassesSchedules(t *testing.T) {
	deployments := []agentDeployment{
		{ID: "dep-1", Name: "my-bp", DisplayName: "my-agent", Status: "active"},
	}
	var captured deployTemplateRequest
	deployed := false
	setupAgentRedeployTest(t, scheduleTemplateHandler(t, deployments, map[string]string{"weekly-sync": "0 3 * * *"}, &captured, &deployed))

	setRedeployFlag(t, "schedule", "weekly-sync=30 2 * * 1")
	agentRedeployCmd.SetOut(&bytes.Buffer{})
	agentRedeployCmd.SetContext(context.Background())
	setAgentTargetName(t, agentRedeployCmd, "my-agent")

	require.NoError(t, runAgentRedeploy(agentRedeployCmd, nil))
	assert.Equal(t, map[string]string{"weekly-sync": "30 2 * * 1"}, captured.Schedules)
	assert.Equal(t, "dep-1", captured.DeploymentID)
	assert.True(t, deployed)
}
