package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetAccountSetFlags restores accountSetCmd's shared flags to their
// pristine, unset state, both before and after the calling test, so neither
// a leftover from an earlier test nor this test's own flags leak
// Changed=true into a test run after it.
func resetAccountSetFlags(t *testing.T) {
	t.Helper()
	reset := func() {
		for _, s := range accountBoolSettings {
			flag := accountSetCmd.Flags().Lookup(s.flag)
			flag.Changed = false
			flag.Value.Set("false") //nolint:errcheck
		}
	}
	reset()
	t.Cleanup(reset)
}

func newAccountSetTestServer(t *testing.T, received *[]*http.Request, bodies *[]map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		*received = append(*received, r)
		*bodies = append(*bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": body["enabled"]})
	}))
	t.Cleanup(srv.Close)
	accountServerURLOverride = srv.URL
	t.Cleanup(func() { accountServerURLOverride = "" })
	return srv
}

func TestAccountSet_BlockPersonalPush(t *testing.T) {
	resetAccountSetFlags(t)
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("test-org"))

	var requests []*http.Request
	var bodies []map[string]any
	newAccountSetTestServer(t, &requests, &bodies)

	require.NoError(t, accountSetCmd.Flags().Set("block-personal-push", "true"))

	var out bytes.Buffer
	accountSetCmd.SetOut(&out)
	err := runAccountSet(accountSetCmd, nil)
	require.NoError(t, err)

	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPatch, requests[0].Method)
	assert.Equal(t, "/api/v1/accounts/test-org/block-member-personal-push", requests[0].URL.Path)
	assert.Equal(t, true, bodies[0]["enabled"])
	assert.Equal(t, "✓ "+msgBlockPersonalPushSet("test-org", true)+"\n", out.String())
}

func TestAccountSet_EveryoneSharing(t *testing.T) {
	resetAccountSetFlags(t)
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("test-org"))

	var requests []*http.Request
	var bodies []map[string]any
	newAccountSetTestServer(t, &requests, &bodies)

	require.NoError(t, accountSetCmd.Flags().Set("everyone-sharing", "false"))

	var out bytes.Buffer
	accountSetCmd.SetOut(&out)
	err := runAccountSet(accountSetCmd, nil)
	require.NoError(t, err)

	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPatch, requests[0].Method)
	assert.Equal(t, "/api/v1/accounts/test-org/everyone-sharing", requests[0].URL.Path)
	assert.Equal(t, false, bodies[0]["enabled"])
	assert.Equal(t, "✓ "+msgEveryoneSharingSet("test-org", false)+"\n", out.String())
}

func TestAccountSet_BothFlagsTogetherUpdateBothSettings(t *testing.T) {
	resetAccountSetFlags(t)
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("test-org"))

	var requests []*http.Request
	var bodies []map[string]any
	newAccountSetTestServer(t, &requests, &bodies)

	require.NoError(t, accountSetCmd.Flags().Set("block-personal-push", "true"))
	require.NoError(t, accountSetCmd.Flags().Set("everyone-sharing", "false"))

	var out bytes.Buffer
	accountSetCmd.SetOut(&out)
	err := runAccountSet(accountSetCmd, nil)
	require.NoError(t, err)

	require.Len(t, requests, 2)
	paths := []string{requests[0].URL.Path, requests[1].URL.Path}
	assert.ElementsMatch(t, []string{
		"/api/v1/accounts/test-org/block-member-personal-push",
		"/api/v1/accounts/test-org/everyone-sharing",
	}, paths)
	assert.Contains(t, out.String(), msgBlockPersonalPushSet("test-org", true))
	assert.Contains(t, out.String(), msgEveryoneSharingSet("test-org", false))
}

func TestAccountSet_NothingToUpdateWithoutAFlag(t *testing.T) {
	resetAccountSetFlags(t)

	err := runAccountSet(accountSetCmd, nil)

	require.Error(t, err)
	assert.Equal(t, errAccountSetNothingToUpdate().Error(), err.Error())
}
