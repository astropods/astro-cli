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

func TestEvalQueue(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{
			"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent", "status": "active",
		}},
		"count": 1,
	}
	status := map[string]any{
		"queued": 2, "in_progress": 1, "completed": 30, "failed": 1, "outdated_count": 3,
	}
	queue := map[string]any{
		"items": []any{
			map[string]any{
				"trace_id": "trace-1", "timestamp": "2026-09-05T00:00:00Z",
				"latency_ms": 420, "total_cost": 0.02,
				"run": map[string]any{"status": "completed"},
			},
			map[string]any{
				"trace_id": "trace-2", "timestamp": "2026-09-04T00:00:00Z",
				"latency_ms": 100, "total_cost": 0.01, "run": nil,
			},
		},
		"next_cursor": "cursor-2",
	}

	cases := []struct {
		name       string
		jsonOutput bool
		wantOut    []string
	}{
		{
			name: "status and items",
			wantOut: []string{
				"my-agent", "2", "30", "1",
				"trace-1", "completed", "trace-2", "not evaluated",
				msgMoreQueueItems(),
			},
		},
		{
			name:       "json",
			jsonOutput: true,
			wantOut:    []string{`"trace_id"`, `"in_progress"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/review-queue/status"):
					jsonHandler(http.StatusOK, status)(w, r)
				case strings.HasSuffix(r.URL.Path, "/review-queue"):
					jsonHandler(http.StatusOK, queue)(w, r)
				default:
					jsonHandler(http.StatusOK, list)(w, r)
				}
			})
			setupAgentTest(t, handler)
			evalServerURLOverride = agentServerURLOverride
			t.Cleanup(func() { evalServerURLOverride = "" })

			if tc.jsonOutput {
				require.NoError(t, evalQueueCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { evalQueueCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			evalQueueCmd.SetOut(buf)
			evalQueueCmd.SetContext(context.Background())
			setAgentTargetName(t, evalQueueCmd, "my-agent")

			require.NoError(t, runEvalQueue(evalQueueCmd, nil))
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestEvalQueueUnknownFilter(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{"id": "dep-abc-123", "name": "my-agent", "status": "active"}},
		"count":       1,
	}
	setupAgentTest(t, jsonHandler(http.StatusOK, list))
	evalServerURLOverride = agentServerURLOverride
	t.Cleanup(func() { evalServerURLOverride = "" })

	require.NoError(t, evalQueueCmd.Flags().Set("evaluation", "somehow"))
	t.Cleanup(func() { evalQueueCmd.Flags().Set("evaluation", "") }) //nolint:errcheck

	evalQueueCmd.SetOut(&bytes.Buffer{})
	evalQueueCmd.SetContext(context.Background())
	setAgentTargetName(t, evalQueueCmd, "my-agent")

	err := runEvalQueue(evalQueueCmd, nil)
	require.Error(t, err)
	assert.Equal(t, errUnknownEvaluationFilter("somehow").Error(), err.Error())
}
