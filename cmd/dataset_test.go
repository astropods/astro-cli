package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDatasetTest(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	datasetServerURLOverride = srv.URL
	t.Cleanup(func() { datasetServerURLOverride = "" })
}

func datasetRow(i int, blueprint string) map[string]any {
	return map[string]any{
		"id": fmt.Sprintf("ds-%d", i), "dataset_name": fmt.Sprintf("eval-dep%08d", i),
		"deployment_id": fmt.Sprintf("dep%08d", i), "agent_name": blueprint, "created_at": "2026-09-01T12:00:00Z",
	}
}

func datasetListCmdForTest(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().Int("limit", 50, "")
	cmd.Flags().Int("offset", 0, "")
	cmd.Flags().Bool("json", false, "")
	for name, value := range flags {
		require.NoError(t, cmd.Flags().Set(name, value))
	}
	cmd.SetContext(context.Background())
	return cmd
}

func TestDatasetList(t *testing.T) {
	rows := []any{datasetRow(1, "support-bot"), datasetRow(2, "sales-bot")}

	cases := []struct {
		name    string
		flags   map[string]string
		total   int
		wantOut []string
		absent  []string
	}{
		{name: "table of datasets", total: 2,
			wantOut: []string{"NAME", "BLUEPRINT", "DEPLOYMENT", "eval-dep00000001", "sales-bot", "dep00000002"}, absent: []string{"Page with"}},
		{name: "json output", flags: map[string]string{"json": "true"}, total: 2,
			wantOut: []string{`"dataset_name": "eval-dep00000001"`, `"total": 2`}},
		{name: "paging hint when more remain", flags: map[string]string{"limit": "2", "offset": "4"}, total: 10,
			wantOut: []string{"Showing 5–6 of 10. Page with --offset 6."}},
		{name: "no hint on the last page", flags: map[string]string{"limit": "2", "offset": "0"}, total: 2,
			absent: []string{"Page with"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotQuery string
			setupDatasetTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
				jsonHandler(http.StatusOK, map[string]any{"datasets": rows, "total": tc.total})(w, r)
			}))
			buf := &bytes.Buffer{}
			cmd := datasetListCmdForTest(t, tc.flags)
			cmd.SetOut(buf)

			require.NoError(t, runDatasetList(cmd, nil))
			assert.Equal(t, "/api/v1/accounts/testaccount/datasets", gotPath)
			wantLimit, wantOffset := "50", "0"
			if v, ok := tc.flags["limit"]; ok {
				wantLimit = v
			}
			if v, ok := tc.flags["offset"]; ok {
				wantOffset = v
			}
			assert.Equal(t, "limit="+wantLimit+"&offset="+wantOffset, gotQuery)
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
			for _, absent := range tc.absent {
				assert.NotContains(t, buf.String(), absent)
			}
		})
	}
}

func TestDatasetListEmptyAccount(t *testing.T) {
	setupDatasetTest(t, jsonHandler(http.StatusOK, map[string]any{"datasets": []any{}, "total": 0}))
	buf := &bytes.Buffer{}
	cmd := datasetListCmdForTest(t, nil)
	cmd.SetOut(buf)

	require.NoError(t, runDatasetList(cmd, nil))
	assert.Contains(t, buf.String(), msgNoDatasets())
}

func TestDatasetListRejectsInvalidPaging(t *testing.T) {
	cases := []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{name: "zero limit", flags: map[string]string{"limit": "0"}, wantErr: errPositiveIntFlag("limit").Error()},
		{name: "negative offset", flags: map[string]string{"offset": "-1"}, wantErr: errNonNegativeIntFlag("offset").Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := datasetListCmdForTest(t, tc.flags)
			cmd.SetOut(&bytes.Buffer{})

			require.EqualError(t, runDatasetList(cmd, nil), tc.wantErr)
		})
	}
}

func TestDatasetListServerError(t *testing.T) {
	setupDatasetTest(t, jsonHandler(http.StatusInternalServerError, map[string]any{"error": "boom"}))
	cmd := datasetListCmdForTest(t, nil)
	cmd.SetOut(&bytes.Buffer{})

	err := runDatasetList(cmd, nil)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "500"), err.Error())
}

func TestDatasetListIsRegisteredOnRoot(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"dataset", "list"})
	require.NoError(t, err)
	assert.Same(t, datasetListCmd, found)
	for _, flag := range []string{"limit", "offset", "json"} {
		assert.NotNil(t, found.Flags().Lookup(flag), flag)
	}
}

func datasetGetCmdForTest(t *testing.T, jsonOut bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "get"}
	cmd.Flags().Bool("json", false, "")
	if jsonOut {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	cmd.SetContext(context.Background())
	return cmd
}

func TestDatasetGet(t *testing.T) {
	summary := map[string]any{
		"id": "ds-1", "dataset_name": "eval-dep00000001", "item_count": 12,
		"evaluators": []any{
			map[string]any{"key": "helpful", "label": "Helpful", "distribution": []any{
				map[string]any{"value": true, "count": 8},
				map[string]any{"value": false, "count": 4},
			}},
			map[string]any{"key": "tone", "label": "Tone", "distribution": []any{}},
		},
	}
	match := map[string]any{"datasets": []any{datasetRow(1, "support-bot")}, "total": 1}

	cases := []struct {
		name       string
		jsonOutput bool
		listBody   any
		getStatus  int
		wantErr    string
		wantOut    []string
	}{
		{name: "prints the item count and value distribution", listBody: match, getStatus: http.StatusOK,
			wantOut: []string{"eval-dep00000001", "Blueprint:  support-bot", "Deployment: dep00000001", "Items:      12",
				"Helpful (helpful)", "true", "8", "false", "4", "Tone (tone)", "No values yet"}},
		{name: "json output", jsonOutput: true, listBody: match, getStatus: http.StatusOK,
			wantOut: []string{`"item_count": 12`}},
		{name: "unknown name suggests dataset list", listBody: map[string]any{"datasets": []any{}, "total": 0},
			wantErr: errDatasetNotFound("eval-dep00000001").Error()},
		{name: "a name matching two datasets is ambiguous",
			listBody: map[string]any{"datasets": []any{datasetRow(1, "a"), datasetRow(2, "b")}, "total": 2},
			wantErr:  errDatasetAmbiguous("eval-dep00000001", []string{"ds-1", "ds-2"}).Error()},
		{name: "dataset removed between lookup and summary", listBody: match, getStatus: http.StatusNotFound,
			wantErr: errDatasetNotFound("eval-dep00000001").Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var listQuery, getPath string
			setupDatasetTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/v1/datasets/") {
					getPath = r.URL.Path
					jsonHandler(tc.getStatus, summary)(w, r)
					return
				}
				listQuery = r.URL.RawQuery
				jsonHandler(http.StatusOK, tc.listBody)(w, r)
			}))
			buf := &bytes.Buffer{}
			cmd := datasetGetCmdForTest(t, tc.jsonOutput)
			cmd.SetOut(buf)

			err := runDatasetGet(cmd, []string{"eval-dep00000001"})
			assert.Contains(t, listQuery, "name=eval-dep00000001", "the name is resolved through the list endpoint")
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "/api/v1/datasets/ds-1", getPath)
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestDatasetGetIsRegisteredOnRoot(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"dataset", "get", "some-name"})
	require.NoError(t, err)
	assert.Same(t, datasetGetCmd, found)
	assert.NotNil(t, found.Flags().Lookup("json"))
	require.Error(t, found.Args(found, nil), "a dataset name is required")
}

func datasetItemsCmdForTest(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "items"}
	cmd.Flags().Int("limit", 50, "")
	cmd.Flags().Int("offset", 0, "")
	cmd.Flags().Bool("json", false, "")
	for name, value := range flags {
		require.NoError(t, cmd.Flags().Set(name, value))
	}
	cmd.SetContext(context.Background())
	return cmd
}

func TestDatasetItems(t *testing.T) {
	items := []any{
		map[string]any{
			"id": "item-1", "input": map[string]any{"q": "hi\nthere"}, "expected_output": "hello",
			"source_trace_id": "trace-aaa", "created_at": "2026-09-01T12:00:00Z", "outdated": false,
			"evaluator_outputs": []any{
				map[string]any{"key": "helpful", "label": "Helpful", "value": true},
				map[string]any{"key": "tone", "label": "Tone", "value": "warm"},
			},
		},
		map[string]any{
			"id": "item-2", "input": "plain text", "expected_output": nil,
			"source_trace_id": "trace-bbb", "created_at": "2026-09-01T12:01:00Z", "outdated": true,
			"evaluator_outputs": []any{map[string]any{"key": "helpful", "label": "Helpful", "value": false}},
		},
		map[string]any{
			"id": "item-3", "input": "unreviewed", "expected_output": nil,
			"source_trace_id": "trace-ccc", "created_at": "2026-09-01T12:02:00Z", "outdated": true,
			"evaluator_outputs": []any{},
		},
	}
	page := func(total int) map[string]any {
		return map[string]any{"items": items, "page": 1, "limit": 50, "total_items": total, "total_pages": 1}
	}
	match := map[string]any{"datasets": []any{datasetRow(1, "support-bot")}, "total": 1}

	cases := []struct {
		name      string
		flags     map[string]string
		body      any
		getStatus int
		wantQuery string
		wantErr   string
		wantOut   []string
		absent    []string
	}{
		{name: "table of items", body: page(3), getStatus: http.StatusOK, wantQuery: "limit=50&page=1",
			wantOut: []string{"TRACE ID", "trace-aaa", `{"q":"hi\nthere"}`, "hello", "helpful=true tone=warm", "plain text", "helpful=false (outdated)", "trace-ccc"},
			absent:  []string{"Page with", "unreviewed (outdated)"}},
		{name: "offset maps to a page", flags: map[string]string{"limit": "50", "offset": "100"}, body: page(3),
			getStatus: http.StatusOK, wantQuery: "limit=50&page=3"},
		{name: "hint when more items remain", flags: map[string]string{"limit": "3"}, body: page(10),
			getStatus: http.StatusOK, wantQuery: "limit=3&page=1", wantOut: []string{"Showing 1–3 of 10. Page with --offset 3."}},
		{name: "json output", flags: map[string]string{"json": "true"}, body: page(3), getStatus: http.StatusOK,
			wantQuery: "limit=50&page=1", wantOut: []string{`"total_items": 3`}},
		{name: "empty dataset", body: map[string]any{"items": []any{}, "page": 1, "limit": 50, "total_items": 0, "total_pages": 0},
			getStatus: http.StatusOK, wantQuery: "limit=50&page=1", wantOut: []string{msgNoDatasetItems("eval-dep00000001")}},
		{name: "dataset removed between lookup and items", body: page(3), getStatus: http.StatusNotFound,
			wantQuery: "limit=50&page=1", wantErr: errDatasetNotFound("eval-dep00000001").Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotQuery string
			setupDatasetTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/v1/datasets/") {
					gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
					jsonHandler(tc.getStatus, tc.body)(w, r)
					return
				}
				jsonHandler(http.StatusOK, match)(w, r)
			}))
			buf := &bytes.Buffer{}
			cmd := datasetItemsCmdForTest(t, tc.flags)
			cmd.SetOut(buf)

			err := runDatasetItems(cmd, []string{"eval-dep00000001"})
			assert.Equal(t, "/api/v1/datasets/ds-1/items", gotPath)
			assert.Equal(t, tc.wantQuery, gotQuery)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
			for _, absent := range tc.absent {
				assert.NotContains(t, buf.String(), absent)
			}
		})
	}
}

func TestDatasetItemsRejectsInvalidPaging(t *testing.T) {
	cases := []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{name: "zero limit", flags: map[string]string{"limit": "0"}, wantErr: errPositiveIntFlag("limit").Error()},
		{name: "negative offset", flags: map[string]string{"offset": "-1"}, wantErr: errNonNegativeIntFlag("offset").Error()},
		{name: "limit above the server max", flags: map[string]string{"limit": "101"}, wantErr: errDatasetItemsLimit(100).Error()},
		{name: "offset not a multiple of limit", flags: map[string]string{"limit": "50", "offset": "25"}, wantErr: errDatasetItemsOffset(50).Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := datasetItemsCmdForTest(t, tc.flags)
			cmd.SetOut(&bytes.Buffer{})

			require.EqualError(t, runDatasetItems(cmd, []string{"eval-dep00000001"}), tc.wantErr)
		})
	}
}

func TestDatasetItemsIsRegisteredOnRoot(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"dataset", "items", "some-name"})
	require.NoError(t, err)
	assert.Same(t, datasetItemsCmd, found)
	for _, flag := range []string{"limit", "offset", "json"} {
		assert.NotNil(t, found.Flags().Lookup(flag), flag)
	}
	require.Error(t, found.Args(found, nil), "a dataset name is required")
}
