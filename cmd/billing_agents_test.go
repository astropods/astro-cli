package cmd

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingAgents(t *testing.T) {
	full := map[string]any{
		"agents": []any{
			map[string]any{"deployment_id": "dep-1", "name": "sasbot", "cu_hours": 25.5, "cost_usd": 15.0},
			map[string]any{"deployment_id": "dep-2", "name": "old-agent", "deleted": true, "cu_hours": 1.0, "cost_usd": 0.6},
		},
		"cu_hours":         26.5,
		"cost_usd":         15.6,
		"unattributed_usd": 2.4,
	}

	cases := []struct {
		name       string
		body       any
		jsonOutput bool
		wantOut    []string
	}{
		{
			name: "sorts by cost and shows deleted agents",
			body: spendPayload(full),
			wantOut: []string{
				"sasbot", msgUsageComputeHours(25.5), msgUsageDollars(15.0),
				"old-agent (deleted)", msgUsageDollars(0.6),
				"Total", msgUsageComputeHours(26.5), msgUsageDollars(15.6),
				msgUsageDollars(2.4),
			},
		},
		{
			name:    "unavailable",
			body:    map[string]any{"available": false},
			wantOut: []string{msgBillingUnavailable()},
		},
		{
			name:    "no metered spend",
			body:    spendPayload(map[string]any{"agents": []any{}}),
			wantOut: []string{msgNoAgentSpend()},
		},
		{
			name:       "json",
			body:       spendPayload(full),
			jsonOutput: true,
			wantOut:    []string{`"deployment_id"`, `"unattributed_usd"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupBillingTest(t, jsonHandler(http.StatusOK, tc.body))
			if tc.jsonOutput {
				require.NoError(t, billingAgentsCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { billingAgentsCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			billingAgentsCmd.SetOut(buf)
			billingAgentsCmd.SetContext(context.Background())

			require.NoError(t, runBillingAgents(billingAgentsCmd, nil))
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}
