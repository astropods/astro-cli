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

func TestAgentNetwork(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{
			"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent",
			"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active",
			"created_at": "2026-01-01T10:00:00Z",
		}},
		"count": 1,
	}
	summary := map[string]any{
		"inbound": map[string]any{
			"request_count": 1204, "error_count": 3, "error_rate": 0.0025,
			"latency_p95_ms": 42.0, "unique_peer_count": 2, "bytes_total": 1048576,
		},
		"outbound": map[string]any{
			"request_count": 30, "error_count": 0, "error_rate": 0.0,
			"latency_p95_ms": nil, "unique_peer_count": 1, "bytes_total": 2048,
		},
		"database": map[string]any{
			"request_count": 500, "error_count": 0, "error_rate": 0.0,
			"latency_p95_ms": 8.0, "unique_peer_count": 1, "bytes_total": 512000,
		},
	}
	flows := map[string]any{
		"direction": "outbound",
		"flows": []any{
			map[string]any{
				"peer": "api.openai.com", "peer_kind": "address", "request_count": 30,
				"error_count": 0, "error_rate": 0.0, "latency_p95_ms": 210.0,
				"bytes_total": 2048, "registrable_domain": "openai.com",
			},
		},
	}

	cases := []struct {
		name       string
		direction  string
		jsonOutput bool
		wantOut    []string
	}{
		{
			name: "summary for all directions",
			wantOut: []string{
				"my-agent", "Inbound", "Outbound", "Database",
				"1,204", formatErrorRate(0.0025), formatLatencyMs(ptr(42.0)),
				"n/a", // outbound has no latency
				formatBytes(1048576),
			},
		},
		{
			name:      "direction lists top peers",
			direction: "outbound",
			wantOut:   []string{"Top outbound peers", "openai.com"},
		},
		{
			name:       "json",
			jsonOutput: true,
			wantOut:    []string{`"request_count"`, `"unique_peer_count"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/network/summary"):
					jsonHandler(http.StatusOK, summary)(w, r)
				case strings.HasSuffix(r.URL.Path, "/network/flows"):
					jsonHandler(http.StatusOK, flows)(w, r)
				default:
					jsonHandler(http.StatusOK, list)(w, r)
				}
			})
			setupAgentTest(t, handler)

			require.NoError(t, agentNetworkCmd.Flags().Set("direction", tc.direction))
			t.Cleanup(func() { agentNetworkCmd.Flags().Set("direction", "") }) //nolint:errcheck
			if tc.jsonOutput {
				require.NoError(t, agentNetworkCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { agentNetworkCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			agentNetworkCmd.SetOut(buf)
			agentNetworkCmd.SetContext(context.Background())
			setAgentTargetName(t, agentNetworkCmd, "my-agent")

			require.NoError(t, runAgentNetwork(agentNetworkCmd, nil))
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestAgentNetworkUnknownDirection(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{"id": "dep-abc-123", "name": "my-agent", "status": "active"}},
		"count":       1,
	}
	setupAgentTest(t, jsonHandler(http.StatusOK, list))

	require.NoError(t, agentNetworkCmd.Flags().Set("direction", "sideways"))
	t.Cleanup(func() { agentNetworkCmd.Flags().Set("direction", "") }) //nolint:errcheck

	agentNetworkCmd.SetOut(&bytes.Buffer{})
	agentNetworkCmd.SetContext(context.Background())
	setAgentTargetName(t, agentNetworkCmd, "my-agent")

	err := runAgentNetwork(agentNetworkCmd, nil)
	require.Error(t, err)
	assert.Equal(t, errUnknownNetworkDirection("sideways").Error(), err.Error())
}

func ptr(f float64) *float64 { return &f }
