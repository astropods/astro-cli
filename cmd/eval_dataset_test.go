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

func TestEvalDataset(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{
			"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent", "status": "active",
		}},
		"count": 1,
	}
	full := map[string]any{
		"deployment": map[string]any{
			"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent",
			"status": "active", "eval_dataset_id": "ds-1",
		},
	}
	dataset := map[string]any{
		"id": "ds-1", "dataset_name": "my-agent evaluation", "item_count": 42,
		"evaluators": []any{
			map[string]any{"key": "correctness", "label": "Correctness", "distribution": []any{
				map[string]any{"value": true, "count": 30},
				map[string]any{"value": false, "count": 12},
			}},
		},
	}
	items := map[string]any{
		"items": []any{
			map[string]any{"id": "item-1", "source_trace_id": "trace-1", "created_at": "2026-09-01T00:00:00Z", "outdated": true},
		},
		"page": 1, "limit": 20, "total_items": 42, "total_pages": 3,
	}

	cases := []struct {
		name       string
		withItems  bool
		jsonOutput bool
		wantOut    []string
	}{
		{
			name:    "summary",
			wantOut: []string{"my-agent evaluation", "42", "Correctness", "30", "12"},
		},
		{
			name:      "items",
			withItems: true,
			wantOut:   []string{"trace-1", "(outdated)", "page 1 of 3"},
		},
		{
			name:       "json",
			jsonOutput: true,
			wantOut:    []string{`"dataset_name"`, `"item_count"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/items"):
					jsonHandler(http.StatusOK, items)(w, r)
				case strings.Contains(r.URL.Path, "/datasets/"):
					jsonHandler(http.StatusOK, dataset)(w, r)
				case strings.Contains(r.URL.Path, "/deployments/dep-abc-123"):
					jsonHandler(http.StatusOK, full)(w, r)
				default:
					jsonHandler(http.StatusOK, list)(w, r)
				}
			})
			setupAgentTest(t, handler)
			evalServerURLOverride = agentServerURLOverride
			t.Cleanup(func() { evalServerURLOverride = "" })

			require.NoError(t, evalDatasetCmd.Flags().Set("items", boolFlagValue(tc.withItems)))
			t.Cleanup(func() { evalDatasetCmd.Flags().Set("items", "false") }) //nolint:errcheck
			if tc.jsonOutput {
				require.NoError(t, evalDatasetCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { evalDatasetCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			evalDatasetCmd.SetOut(buf)
			evalDatasetCmd.SetContext(context.Background())
			setAgentTargetName(t, evalDatasetCmd, "my-agent")

			require.NoError(t, runEvalDataset(evalDatasetCmd, nil))
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestEvalDatasetNoDataset(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{"id": "dep-abc-123", "name": "my-agent", "status": "active"}},
		"count":       1,
	}
	full := map[string]any{
		"deployment": map[string]any{"id": "dep-abc-123", "name": "my-agent", "status": "active"},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/deployments/dep-abc-123") {
			jsonHandler(http.StatusOK, full)(w, r)
			return
		}
		jsonHandler(http.StatusOK, list)(w, r)
	})
	setupAgentTest(t, handler)

	evalDatasetCmd.SetOut(&bytes.Buffer{})
	evalDatasetCmd.SetContext(context.Background())
	setAgentTargetName(t, evalDatasetCmd, "my-agent")

	err := runEvalDataset(evalDatasetCmd, nil)
	require.Error(t, err)
	assert.Equal(t, errNoEvalDataset("my-agent").Error(), err.Error())
}

func boolFlagValue(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
