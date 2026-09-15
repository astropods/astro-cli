package cmd

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIngestionJobsFromWorkloads(t *testing.T) {
	cases := []struct {
		name      string
		workloads []workloadDetail
		want      []ingestionJob
	}{
		{
			name: "only ingestion components, sorted by name",
			workloads: []workloadDetail{
				{Component: "agent", Name: "my-agent"},
				{Component: "ingestion-metrics_rollup", Name: "my-agent-ingestion-metrics-rollup", Schedule: "0 9 * * 1"},
				{Component: "collector", Name: "my-agent-collector"},
				{Component: "ingestion-docs_sync", Name: "my-agent-ingestion-docs-sync", Schedule: "*/15 * * * *"},
			},
			want: []ingestionJob{
				{name: "docs_sync", schedule: "*/15 * * * *"},
				{name: "metrics_rollup", schedule: "0 9 * * 1"},
			},
		},
		{
			name: "a job with no cron is still triggerable",
			workloads: []workloadDetail{
				{Component: "ingestion-on_demand", Name: "my-agent-ingestion-on-demand"},
			},
			want: []ingestionJob{{name: "on_demand"}},
		},
		{
			name: "an agent with no ingestion yields none",
			workloads: []workloadDetail{
				{Component: "agent", Name: "my-agent"},
			},
			want: []ingestionJob{},
		},
		{
			name: "a component merely containing the prefix is not a job",
			workloads: []workloadDetail{
				{Component: "pre-ingestion-hook", Name: "my-agent-pre-ingestion-hook"},
			},
			want: []ingestionJob{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ingestionJobsFromWorkloads(tc.workloads))
		})
	}
}

func TestIngestionJobNames(t *testing.T) {
	jobs := []ingestionJob{{name: "docs_sync"}, {name: "metrics_rollup"}}

	assert.Equal(t, []string{"docs_sync", "metrics_rollup"}, ingestionJobNames(jobs))
}

func TestAgentTriggerArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr error
	}{
		{name: "no job lists them", args: nil},
		{name: "one job", args: []string{"docs_sync"}},
		{
			name:    "a second positional is rejected",
			args:    []string{"docs_sync", "metrics_rollup"},
			wantErr: errAgentUnexpectedArgument("metrics_rollup"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := agentTriggerArgs(nil, tc.args)
			if tc.wantErr != nil {
				assert.EqualError(t, err, tc.wantErr.Error())
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestRunAgentTriggerJobSurface(t *testing.T) {
	deployment := map[string]any{
		"id":           "dep-abc-123",
		"name":         "my-bp",
		"display_name": "my-agent",
		"status":       "active",
	}

	cases := []struct {
		name      string
		workloads []any
		args      []string
		wantErr   error
		wantOut   string
	}{
		{
			name: "listing prints the jobs header",
			workloads: []any{
				map[string]any{"name": "my-agent-agent", "component": "agent"},
				map[string]any{"name": "my-agent-ingestion-docs-sync", "component": "ingestion-docs_sync", "schedule": "*/15 * * * *"},
			},
			wantOut: msgAgentJobsHeader("my-agent"),
		},
		{
			name:      "an agent that runs none says so",
			workloads: []any{map[string]any{"name": "my-agent-agent", "component": "agent"}},
			wantErr:   errAgentNoJobs("my-agent"),
		},
		{
			name: "an unmatched name lists what is available",
			workloads: []any{
				map[string]any{"name": "my-agent-ingestion-docs-sync", "component": "ingestion-docs_sync"},
			},
			args:    []string{"nightly"},
			wantErr: errAgentUnknownJob("nightly", []string{"docs_sync"}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupAgentTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/deployments/dep-abc-123") {
					jsonHandler(http.StatusOK, map[string]any{
						"deployment": map[string]any{"id": "dep-abc-123", "workloads": tc.workloads},
					})(w, r)
					return
				}
				jsonHandler(http.StatusOK, map[string]any{"deployments": []any{deployment}, "count": 1})(w, r)
			}))

			buf := &bytes.Buffer{}
			agentTriggerCmd.SetOut(buf)
			agentTriggerCmd.SetContext(context.Background())
			setAgentTargetName(t, agentTriggerCmd, "my-agent")

			err := runAgentTrigger(agentTriggerCmd, tc.args)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
				return
			}
			require.NoError(t, err)
			assert.Contains(t, buf.String(), tc.wantOut, "the listing must name jobs, not ingestion jobs")
		})
	}
}
