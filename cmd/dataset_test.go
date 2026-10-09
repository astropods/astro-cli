package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		{name: "limit above the server max", flags: map[string]string{"limit": "101"}, wantErr: errDatasetLimit(100).Error()},
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
		{name: "limit above the server max", flags: map[string]string{"limit": "101"}, wantErr: errDatasetLimit(100).Error()},
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

func datasetAddCmdForTest(t *testing.T, traceID string, jsonOut bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "add"}
	cmd.Flags().StringP("trace-id", "t", "", "")
	cmd.Flags().Bool("json", false, "")
	if traceID != "" {
		require.NoError(t, cmd.Flags().Set("trace-id", traceID))
	}
	if jsonOut {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	cmd.SetContext(context.Background())
	return cmd
}

func TestDatasetAdd(t *testing.T) {
	const name = "eval-dep00000001"
	created := map[string]any{"eval_dataset_id": "ds-1", "trace_id": "trace-abc", "evaluation_ref": "ref-1"}
	match := map[string]any{"datasets": []any{datasetRow(1, "support-bot")}, "total": 1}

	cases := []struct {
		name       string
		traceID    string
		jsonOutput bool
		status     int
		body       any
		wantErr    string
		wantOut    string
		wantPosted bool
	}{
		{name: "adds the trace", traceID: "trace-abc", status: http.StatusCreated, body: created,
			wantOut: msgDatasetAdded("trace-abc", name), wantPosted: true},
		{name: "json output", traceID: "trace-abc", jsonOutput: true, status: http.StatusCreated, body: created,
			wantOut: `"evaluation_ref": "ref-1"`, wantPosted: true},
		{name: "trace from another deployment", traceID: "trace-abc", status: http.StatusForbidden,
			body: map[string]any{"error": "trace does not belong to this deployment"}, wantPosted: true,
			wantErr: errDatasetAddWrongDeployment("trace-abc", name).Error()},
		{name: "trace without input", traceID: "trace-abc", status: http.StatusUnprocessableEntity,
			body: map[string]any{"error": "trace has no input"}, wantPosted: true,
			wantErr: errDatasetAddNoInput("trace-abc").Error()},
		{name: "trace already in the dataset", traceID: "trace-abc", status: http.StatusConflict,
			body: map[string]any{"error": "trace already in the dataset"}, wantPosted: true,
			wantErr: errDatasetAddAlreadyAdded("trace-abc", name).Error()},
		{name: "unknown trace", traceID: "trace-abc", status: http.StatusNotFound,
			body: map[string]any{"error": "trace not found"}, wantPosted: true,
			wantErr: errDatasetTraceNotFound("trace-abc", name).Error()},
		{name: "rejected request prints the server message", traceID: "trace-abc", status: http.StatusBadRequest,
			body: map[string]any{"error": "trace_id is required"}, wantPosted: true,
			wantErr: errDatasetAddRejected("trace_id is required").Error()},
		{name: "trace id is required", wantErr: errTraceIDRequired().Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var posted bool
			var gotPath string
			var gotBody map[string]any
			setupDatasetTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posted, gotPath = true, r.URL.Path
					_ = json.NewDecoder(r.Body).Decode(&gotBody)
					jsonHandler(tc.status, tc.body)(w, r)
					return
				}
				jsonHandler(http.StatusOK, match)(w, r)
			}))
			buf := &bytes.Buffer{}
			cmd := datasetAddCmdForTest(t, tc.traceID, tc.jsonOutput)
			cmd.SetOut(buf)

			err := runDatasetAdd(cmd, []string{name})
			assert.Equal(t, tc.wantPosted, posted)
			if tc.wantPosted {
				assert.Equal(t, "/api/v1/datasets/ds-1/items", gotPath)
				assert.Equal(t, map[string]any{"trace_id": "trace-abc"}, gotBody)
			}
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, buf.String(), tc.wantOut)
		})
	}
}

func TestDatasetAddIsRegisteredOnRoot(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"dataset", "add", "some-name"})
	require.NoError(t, err)
	assert.Same(t, datasetAddCmd, found)
	assert.NotNil(t, found.Flags().ShorthandLookup("t"), "-t selects the trace")
	assert.NotNil(t, found.Flags().Lookup("json"))
	require.Error(t, found.Args(found, nil), "a dataset name is required")
}

func datasetEditCmdForTest(t *testing.T, traceID string, sets, setStrings []string, jsonOut bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "edit"}
	cmd.Flags().StringP("trace-id", "t", "", "")
	registerEvalValueFlags(cmd)
	cmd.Flags().Bool("json", false, "")
	if traceID != "" {
		require.NoError(t, cmd.Flags().Set("trace-id", traceID))
	}
	for _, s := range sets {
		require.NoError(t, cmd.Flags().Set("set", s))
	}
	for _, s := range setStrings {
		require.NoError(t, cmd.Flags().Set("set-string", s))
	}
	if jsonOut {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	cmd.SetContext(context.Background())
	return cmd
}

func TestDatasetEdit(t *testing.T) {
	const name = "eval-dep00000001"
	updated := map[string]any{
		"eval_dataset_id": "ds-1", "trace_id": "trace-abc", "evaluation_ref": "ref-1", "verified_by_user_id": "user-1",
		"evaluator_outputs": []any{map[string]any{"key": "helpful", "value": false}, map[string]any{"key": "note", "value": "42"}},
	}
	match := map[string]any{"datasets": []any{datasetRow(1, "support-bot")}, "total": 1}

	cases := []struct {
		name       string
		traceID    string
		sets       []string
		setStrings []string
		jsonOutput bool
		status     int
		body       any
		wantPut    bool
		wantValues []any
		wantErr    string
		wantOut    string
	}{
		{name: "replaces the item's values", traceID: "trace-abc", sets: []string{"helpful=false"}, setStrings: []string{"note=42"},
			status: http.StatusOK, body: updated, wantPut: true,
			wantValues: []any{map[string]any{"key": "helpful", "value": false}, map[string]any{"key": "note", "value": "42"}},
			wantOut:    msgDatasetEdited("trace-abc", name, 2)},
		{name: "json output", traceID: "trace-abc", sets: []string{"helpful=false"}, jsonOutput: true,
			status: http.StatusOK, body: updated, wantPut: true,
			wantValues: []any{map[string]any{"key": "helpful", "value": false}},
			wantOut:    `"verified_by_user_id": "user-1"`},
		{name: "item not in the dataset", traceID: "trace-abc", sets: []string{"helpful=true"}, status: http.StatusNotFound,
			body: map[string]any{"error": "dataset item not found"}, wantPut: true,
			wantValues: []any{map[string]any{"key": "helpful", "value": true}},
			wantErr:    errDatasetItemNotFound("trace-abc", name).Error()},
		{name: "item on an older evaluation set", traceID: "trace-abc", sets: []string{"helpful=true"}, status: http.StatusConflict,
			body: map[string]any{"error": "dataset item does not use the active evaluation set"}, wantPut: true,
			wantValues: []any{map[string]any{"key": "helpful", "value": true}},
			wantErr:    errDatasetEditOutdated("trace-abc", name).Error()},
		{name: "invalid value prints the server message", traceID: "trace-abc", sets: []string{"tone=lukewarm"}, status: http.StatusBadRequest,
			body: map[string]any{"error": `evaluator "tone": not an option`}, wantPut: true,
			wantValues: []any{map[string]any{"key": "tone", "value": "lukewarm"}},
			wantErr:    errDatasetEditRejected(`evaluator "tone": not an option`).Error()},
		{name: "trace id is required", sets: []string{"helpful=true"}, wantErr: errTraceIDRequired().Error()},
		{name: "at least one value is required", traceID: "trace-abc", wantErr: errSetValueRequired().Error()},
		{name: "duplicate keys are rejected before any request", traceID: "trace-abc", sets: []string{"a=1"}, setStrings: []string{"a=2"},
			wantErr: errEvalSetFlagDuplicate("a").Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var put bool
			var gotPath string
			var gotBody map[string]any
			setupDatasetTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					put, gotPath = true, r.URL.Path
					_ = json.NewDecoder(r.Body).Decode(&gotBody)
					jsonHandler(tc.status, tc.body)(w, r)
					return
				}
				jsonHandler(http.StatusOK, match)(w, r)
			}))
			buf := &bytes.Buffer{}
			cmd := datasetEditCmdForTest(t, tc.traceID, tc.sets, tc.setStrings, tc.jsonOutput)
			cmd.SetOut(buf)

			err := runDatasetEdit(cmd, []string{name})
			assert.Equal(t, tc.wantPut, put)
			if tc.wantPut {
				assert.Equal(t, "/api/v1/datasets/ds-1/items/trace-abc/evaluator-outputs", gotPath)
				assert.Equal(t, map[string]any{"values": tc.wantValues}, gotBody)
			}
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, buf.String(), tc.wantOut)
		})
	}
}

func TestDatasetEditIsRegisteredOnRoot(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"dataset", "edit", "some-name"})
	require.NoError(t, err)
	assert.Same(t, datasetEditCmd, found)
	assert.NotNil(t, found.Flags().ShorthandLookup("t"), "-t selects the trace")
	for _, flag := range []string{"set", "set-string", "json"} {
		assert.NotNil(t, found.Flags().Lookup(flag), flag)
	}
	require.Error(t, found.Args(found, nil), "a dataset name is required")
}

func datasetRemoveCmdForTest(t *testing.T, traceID, confirm string, jsonOut bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "remove"}
	cmd.Flags().StringP("trace-id", "t", "", "")
	cmd.Flags().String("confirm", "", "")
	cmd.Flags().Bool("json", false, "")
	if traceID != "" {
		require.NoError(t, cmd.Flags().Set("trace-id", traceID))
	}
	if confirm != "" {
		require.NoError(t, cmd.Flags().Set("confirm", confirm))
	}
	if jsonOut {
		require.NoError(t, cmd.Flags().Set("json", "true"))
	}
	cmd.SetContext(context.Background())
	return cmd
}

func TestDatasetRemove(t *testing.T) {
	const name = "eval-dep00000001"
	removed := map[string]any{"eval_dataset_id": "ds-1", "trace_id": "trace-abc", "evaluation_ref": "ref-1"}
	match := map[string]any{"datasets": []any{datasetRow(1, "support-bot")}, "total": 1}

	cases := []struct {
		name        string
		traceID     string
		confirm     string
		jsonOutput  bool
		status      int
		body        any
		wantDeleted bool
		wantErr     string
		wantOut     string
	}{
		{name: "removes the item once confirmed", traceID: "trace-abc", confirm: "trace-abc",
			status: http.StatusOK, body: removed, wantDeleted: true, wantOut: msgDatasetRemoved("trace-abc", name)},
		{name: "json output", traceID: "trace-abc", confirm: "trace-abc", jsonOutput: true,
			status: http.StatusOK, body: removed, wantDeleted: true, wantOut: `"evaluation_ref": "ref-1"`},
		{name: "a confirmation that does not match cancels without deleting", traceID: "trace-abc", confirm: "other-trace",
			wantOut: "Confirmation does not match"},
		{name: "item not in the dataset", traceID: "trace-abc", confirm: "trace-abc", status: http.StatusNotFound,
			body: map[string]any{"error": "trace is not in the dataset"}, wantDeleted: true,
			wantErr: errDatasetItemNotFound("trace-abc", name).Error()},
		{name: "trace id is required", confirm: "trace-abc", wantErr: errTraceIDRequired().Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var deleted bool
			var gotPath string
			setupDatasetTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deleted, gotPath = true, r.URL.Path
					jsonHandler(tc.status, tc.body)(w, r)
					return
				}
				jsonHandler(http.StatusOK, match)(w, r)
			}))
			buf := &bytes.Buffer{}
			cmd := datasetRemoveCmdForTest(t, tc.traceID, tc.confirm, tc.jsonOutput)
			cmd.SetOut(buf)

			err := runDatasetRemove(cmd, []string{name})
			assert.Equal(t, tc.wantDeleted, deleted)
			if tc.wantDeleted {
				assert.Equal(t, "/api/v1/datasets/ds-1/items/trace-abc", gotPath)
			}
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, buf.String(), tc.wantOut)
		})
	}
}

func TestDatasetRemoveChecksTheDatasetBeforeAskingToConfirm(t *testing.T) {
	setupDatasetTest(t, jsonHandler(http.StatusOK, map[string]any{"datasets": []any{}, "total": 0}))
	cmd := datasetRemoveCmdForTest(t, "trace-abc", "", false)
	cmd.SetOut(&bytes.Buffer{})

	require.EqualError(t, runDatasetRemove(cmd, []string{"ghost"}), errDatasetNotFound("ghost").Error(),
		"an unknown dataset fails before any prompt")
}

func TestDatasetRemoveIsRegisteredOnRoot(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"dataset", "remove", "some-name"})
	require.NoError(t, err)
	assert.Same(t, datasetRemoveCmd, found)
	assert.NotNil(t, found.Flags().ShorthandLookup("t"), "-t selects the trace")
	for _, flag := range []string{"confirm", "json"} {
		assert.NotNil(t, found.Flags().Lookup(flag), flag)
	}
	require.Error(t, found.Args(found, nil), "a dataset name is required")
}

func datasetDownloadCmdForTest(t *testing.T, output string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "download"}
	cmd.Flags().StringP("output", "o", "", "")
	if output != "" {
		require.NoError(t, cmd.Flags().Set("output", output))
	}
	cmd.SetContext(context.Background())
	return cmd
}

func datasetDownloadServer(t *testing.T, download http.HandlerFunc) {
	t.Helper()
	match := map[string]any{"datasets": []any{datasetRow(1, "support-bot")}, "total": 1}
	setupDatasetTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/datasets/ds-1/download" {
			download(w, r)
			return
		}
		jsonHandler(http.StatusOK, match)(w, r)
	}))
}

func serveZipBytes(payload string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte(payload))
	}
}

func TestDatasetDownload(t *testing.T) {
	const name = "eval-dep00000001"
	const payload = "PK-fake-zip-bytes"

	t.Run("writes <dataset-name>.zip to the current directory by default", func(t *testing.T) {
		datasetDownloadServer(t, serveZipBytes(payload))
		dir := t.TempDir()
		t.Chdir(dir)
		buf := &bytes.Buffer{}
		cmd := datasetDownloadCmdForTest(t, "")
		cmd.SetOut(buf)

		require.NoError(t, runDatasetDownload(cmd, []string{name}))

		got, err := os.ReadFile(filepath.Join(dir, name+".zip"))
		require.NoError(t, err)
		assert.Equal(t, payload, string(got))
		assert.Contains(t, buf.String(), msgDatasetDownloaded(name, name+".zip", int64(len(payload))))
	})

	t.Run("output names the file", func(t *testing.T) {
		datasetDownloadServer(t, serveZipBytes(payload))
		target := filepath.Join(t.TempDir(), "export.zip")
		cmd := datasetDownloadCmdForTest(t, target)
		cmd.SetOut(&bytes.Buffer{})

		require.NoError(t, runDatasetDownload(cmd, []string{name}))

		got, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, payload, string(got))
	})

	t.Run("output that is a directory gets <dataset-name>.zip inside it", func(t *testing.T) {
		datasetDownloadServer(t, serveZipBytes(payload))
		dir := t.TempDir()
		cmd := datasetDownloadCmdForTest(t, dir)
		cmd.SetOut(&bytes.Buffer{})

		require.NoError(t, runDatasetDownload(cmd, []string{name}))

		_, err := os.Stat(filepath.Join(dir, name+".zip"))
		assert.NoError(t, err)
	})

	t.Run("output - writes only the zip to stdout", func(t *testing.T) {
		datasetDownloadServer(t, serveZipBytes(payload))
		t.Chdir(t.TempDir())
		buf := &bytes.Buffer{}
		cmd := datasetDownloadCmdForTest(t, "-")
		cmd.SetOut(buf)

		require.NoError(t, runDatasetDownload(cmd, []string{name}))

		assert.Equal(t, payload, buf.String(), "no status line is mixed into the zip")
		entries, err := os.ReadDir(".")
		require.NoError(t, err)
		assert.Empty(t, entries, "nothing is written to disk")
	})

	t.Run("a dropped connection returns an error and keeps the existing file", func(t *testing.T) {
		datasetDownloadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write([]byte("PK-partial"))
		})
		target := filepath.Join(t.TempDir(), "export.zip")
		require.NoError(t, os.WriteFile(target, []byte("previous export"), 0o600))
		cmd := datasetDownloadCmdForTest(t, target)
		cmd.SetOut(&bytes.Buffer{})

		err := runDatasetDownload(cmd, []string{name})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "download failed")
		got, readErr := os.ReadFile(target)
		require.NoError(t, readErr)
		assert.Equal(t, "previous export", string(got))
		entries, dirErr := os.ReadDir(filepath.Dir(target))
		require.NoError(t, dirErr)
		assert.Len(t, entries, 1, "no temporary file is left behind")
	})

	t.Run("dataset removed between lookup and download", func(t *testing.T) {
		datasetDownloadServer(t, jsonHandler(http.StatusNotFound, map[string]any{"error": "dataset not found"}))
		cmd := datasetDownloadCmdForTest(t, filepath.Join(t.TempDir(), "x.zip"))
		cmd.SetOut(&bytes.Buffer{})

		require.EqualError(t, runDatasetDownload(cmd, []string{name}), errDatasetNotFound(name).Error())
	})

	t.Run("unknown dataset name", func(t *testing.T) {
		setupDatasetTest(t, jsonHandler(http.StatusOK, map[string]any{"datasets": []any{}, "total": 0}))
		cmd := datasetDownloadCmdForTest(t, "-")
		cmd.SetOut(&bytes.Buffer{})

		require.EqualError(t, runDatasetDownload(cmd, []string{"ghost"}), errDatasetNotFound("ghost").Error())
	})
}

func TestDatasetDownloadIsRegisteredOnRoot(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"dataset", "download", "some-name"})
	require.NoError(t, err)
	assert.Same(t, datasetDownloadCmd, found)
	assert.NotNil(t, found.Flags().ShorthandLookup("o"), "-o selects the output")
	require.Error(t, found.Args(found, nil), "a dataset name is required")
}
