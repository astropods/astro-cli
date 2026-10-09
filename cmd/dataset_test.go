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
