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

func TestAgentUsage(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{
			"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent",
			"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active",
			"created_at": "2026-01-01T10:00:00Z",
		}},
		"count": 1,
	}
	// The server answers 30-day series oldest-first; only the tail carries usage.
	usage := map[string]any{
		"total_traces":            1204,
		"last_trace_at":           "2026-09-04T15:00:00Z",
		"request_series":          append(make([]int, 28), 12, 40),
		"token_series":            append(make([]int, 28), 900000, 1030441),
		"cost_series":             append(make([]float64, 28), 4.5, 7.91),
		"cost_usd":                12.41,
		"compute_cu_hours_series": append(make([]float64, 28), 1.5, 2.125),
		"compute_cu_hours":        3.625,
	}

	cases := []struct {
		name       string
		usage      any
		jsonOutput bool
		wantOut    []string
	}{
		{
			name:  "totals and compute",
			usage: usage,
			wantOut: []string{
				"my-agent",
				msgUsageWindow(30),
				"1,204",
				"1,930,441",
				msgUsageDollars(12.41),
				msgUsageComputeHours(3.625),
				msgUsageLastTrace("2026-09-04T15:00:00Z"),
			},
		},
		{
			name: "an agent that has served no trace still reports compute",
			usage: map[string]any{
				"total_traces": 0, "request_series": make([]int, 30), "token_series": make([]int, 30),
				"cost_series": make([]float64, 30), "cost_usd": 0,
				"compute_cu_hours_series": append(make([]float64, 29), 0.75), "compute_cu_hours": 0.75,
			},
			wantOut: []string{msgUsageComputeHours(0.75), msgUsageDollars(0)},
		},
		{
			name:       "json",
			usage:      usage,
			jsonOutput: true,
			wantOut:    []string{`"compute_cu_hours"`, `"total_traces"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/usage") {
					jsonHandler(http.StatusOK, tc.usage)(w, r)
					return
				}
				jsonHandler(http.StatusOK, list)(w, r)
			})
			setupAgentTest(t, handler)
			if tc.jsonOutput {
				require.NoError(t, agentUsageCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { agentUsageCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			agentUsageCmd.SetOut(buf)
			agentUsageCmd.SetContext(context.Background())
			setAgentTargetName(t, agentUsageCmd, "my-agent")

			require.NoError(t, runAgentUsage(agentUsageCmd, nil))
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestAgentUsageNotFound(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{
			"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent", "status": "active",
		}},
		"count": 1,
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/usage") {
			jsonHandler(http.StatusNotFound, map[string]any{"error": "not found"})(w, r)
			return
		}
		jsonHandler(http.StatusOK, list)(w, r)
	})
	setupAgentTest(t, handler)

	agentUsageCmd.SetOut(&bytes.Buffer{})
	agentUsageCmd.SetContext(context.Background())
	setAgentTargetName(t, agentUsageCmd, "my-agent")

	err := runAgentUsage(agentUsageCmd, nil)
	require.Error(t, err)
	assert.Equal(t, errAgentDeploymentNotFound("my-agent").Error(), err.Error())
}

func TestThousands(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1,000"},
		{1930441, "1,930,441"},
		{-4200, "-4,200"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, thousands(tc.in))
	}
}
