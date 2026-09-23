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

func TestBillingFeatures(t *testing.T) {
	full := map[string]any{
		"features": []any{
			map[string]any{"feature": "dev", "name": "Local dev", "cost_usd": 0.4},
			map[string]any{"feature": "eval-judge", "name": "Eval judge", "cost_usd": 1.2},
			map[string]any{"feature": "trace-summary", "name": "trace-summary", "cost_usd": 0.1},
		},
		"cost_usd": 1.7,
	}

	cases := []struct {
		name       string
		body       any
		jsonOutput bool
		wantOut    []string
		notOut     []string
	}{
		{
			name: "sorts by cost and names the drill-down argument",
			body: spendPayload(full),
			wantOut: []string{
				"Eval judge (eval-judge)", msgUsageDollars(1.2),
				"Local dev (dev)", msgUsageDollars(0.4),
				"Total", msgUsageDollars(1.7),
				msgDrillIntoFeature(),
			},
			notOut: []string{"trace-summary (trace-summary)"},
		},
		{
			name:    "metric not provisioned",
			body:    map[string]any{"available": false},
			wantOut: []string{msgFeatureSpendUnavailable()},
		},
		{
			name:    "no feature spend",
			body:    spendPayload(map[string]any{"features": []any{}}),
			wantOut: []string{msgNoFeatureSpend()},
		},
		{
			name:       "json",
			body:       spendPayload(full),
			jsonOutput: true,
			wantOut:    []string{`"feature"`, `"eval-judge"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			setupBillingTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				jsonHandler(http.StatusOK, tc.body)(w, r)
			}))
			require.NoError(t, billingModelsCmd.Flags().Set("features", "true"))
			t.Cleanup(func() { billingModelsCmd.Flags().Set("features", "false") }) //nolint:errcheck
			if tc.jsonOutput {
				require.NoError(t, billingModelsCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { billingModelsCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			billingModelsCmd.SetOut(buf)
			billingModelsCmd.SetContext(context.Background())

			require.NoError(t, runBillingModels(billingModelsCmd, nil))
			assert.True(t, strings.HasSuffix(gotPath, "/billing/usage/features"), "path = %s", gotPath)
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
			for _, not := range tc.notOut {
				assert.NotContains(t, buf.String(), not)
			}
		})
	}
}

func TestBillingFeatureByModel(t *testing.T) {
	full := map[string]any{
		"feature": "dev",
		"name":    "Local dev",
		"models": []any{
			map[string]any{"model": "gpt-4o", "cost_usd": 0.1},
			map[string]any{"model": "claude-sonnet-4-6", "cost_usd": 0.3},
		},
		"cost_usd": 0.4,
	}

	cases := []struct {
		name       string
		body       any
		jsonOutput bool
		wantOut    []string
	}{
		{
			name: "sorts by cost under the feature's label",
			body: spendPayload(full),
			wantOut: []string{
				"Local dev",
				"claude-sonnet-4-6", msgUsageDollars(0.3),
				"gpt-4o", msgUsageDollars(0.1),
				"Total", msgUsageDollars(0.4),
			},
		},
		{
			name:    "metric not provisioned",
			body:    map[string]any{"available": false},
			wantOut: []string{msgFeatureSpendUnavailable()},
		},
		{
			name:    "nothing metered for this feature",
			body:    spendPayload(map[string]any{"feature": "dev", "models": []any{}}),
			wantOut: []string{msgNoModelAgentSpend("dev")},
		},
		{
			name:       "json",
			body:       spendPayload(full),
			jsonOutput: true,
			wantOut:    []string{`"models"`, `"feature"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			setupBillingTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				jsonHandler(http.StatusOK, tc.body)(w, r)
			}))
			require.NoError(t, billingModelsCmd.Flags().Set("features", "true"))
			t.Cleanup(func() { billingModelsCmd.Flags().Set("features", "false") }) //nolint:errcheck
			if tc.jsonOutput {
				require.NoError(t, billingModelsCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { billingModelsCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			billingModelsCmd.SetOut(buf)
			billingModelsCmd.SetContext(context.Background())

			require.NoError(t, runBillingModels(billingModelsCmd, []string{"dev"}))
			assert.True(t, strings.HasSuffix(gotPath, "/billing/usage/features/dev/by-model"), "path = %s", gotPath)
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}
