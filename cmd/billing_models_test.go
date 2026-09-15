package cmd

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingModels(t *testing.T) {
	full := map[string]any{
		"models": []any{
			map[string]any{"model": "gpt-4o", "cost_usd": 15.0},
			map[string]any{"model": "claude-sonnet-4-5", "cost_usd": 25.5},
		},
		"cost_usd":         40.5,
		"unattributed_usd": 2.4,
	}

	cases := []struct {
		name       string
		body       any
		jsonOutput bool
		wantOut    []string
	}{
		{
			name: "sorts by cost",
			body: spendPayload(full),
			wantOut: []string{
				"claude-sonnet-4-5", msgUsageDollars(25.5),
				"gpt-4o", msgUsageDollars(15.0),
				"Total", msgUsageDollars(40.5),
				msgUsageDollars(2.4),
				msgDrillIntoModel(),
			},
		},
		{
			name:    "unavailable",
			body:    map[string]any{"available": false},
			wantOut: []string{msgBillingUnavailable()},
		},
		{
			name:    "no metered spend",
			body:    spendPayload(map[string]any{"models": []any{}}),
			wantOut: []string{msgNoModelSpend()},
		},
		{
			name:       "json",
			body:       spendPayload(full),
			jsonOutput: true,
			wantOut:    []string{`"model"`, `"unattributed_usd"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupBillingTest(t, jsonHandler(http.StatusOK, tc.body))
			if tc.jsonOutput {
				require.NoError(t, billingModelsCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { billingModelsCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			billingModelsCmd.SetOut(buf)
			billingModelsCmd.SetContext(context.Background())

			require.NoError(t, runBillingModels(billingModelsCmd, nil))
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestBillingModelByAgent(t *testing.T) {
	full := map[string]any{
		"model": "gpt-4o",
		"agents": []any{
			map[string]any{"deployment_id": "dep-1", "name": "sasbot", "cost_usd": 15.0},
			map[string]any{"deployment_id": "dep-2", "name": "old-agent", "deleted": true, "cost_usd": 0.6},
		},
		"cost_usd": 15.6,
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
				"sasbot", msgUsageDollars(15.0),
				"old-agent (deleted)", msgUsageDollars(0.6),
				"Total", msgUsageDollars(15.6),
			},
		},
		{
			name:    "unavailable",
			body:    map[string]any{"available": false},
			wantOut: []string{msgBillingUnavailable()},
		},
		{
			name:    "nothing metered for this model",
			body:    spendPayload(map[string]any{"model": "gpt-4o", "agents": []any{}}),
			wantOut: []string{msgNoModelAgentSpend("gpt-4o")},
		},
		{
			name:       "json",
			body:       spendPayload(full),
			jsonOutput: true,
			wantOut:    []string{`"deployment_id"`, `"model"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupBillingTest(t, jsonHandler(http.StatusOK, tc.body))
			if tc.jsonOutput {
				require.NoError(t, billingModelsCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { billingModelsCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			billingModelsCmd.SetOut(buf)
			billingModelsCmd.SetContext(context.Background())

			at, verbose, err := cmdAuth(billingModelsCmd)
			require.NoError(t, err)
			require.NoError(t, runBillingModelByAgent(billingModelsCmd, at, verbose, "gpt-4o"))
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}
