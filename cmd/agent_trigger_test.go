package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
