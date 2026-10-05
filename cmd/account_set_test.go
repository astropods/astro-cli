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

// resetBlockPersonalPushFlag restores accountSetCmd's shared flag to its
// pristine, unset state, both before and after the calling test, so neither
// a leftover from an earlier test nor this test's own --block-personal-push
// leaks Changed=true into a test run after it.
func resetBlockPersonalPushFlag(t *testing.T) {
	t.Helper()
	flag := accountSetCmd.Flags().Lookup("block-personal-push")
	reset := func() {
		flag.Changed = false
		flag.Value.Set("false") //nolint:errcheck
	}
	reset()
	t.Cleanup(reset)
}

func TestAccountSet_BlockPersonalPush(t *testing.T) {
	resetBlockPersonalPushFlag(t)
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("test-org"))

	var receivedMethod, receivedPath string
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": true})
	}))
	defer srv.Close()
	accountServerURLOverride = srv.URL
	t.Cleanup(func() { accountServerURLOverride = "" })

	require.NoError(t, accountSetCmd.Flags().Set("block-personal-push", "true"))

	var out bytes.Buffer
	accountSetCmd.SetOut(&out)
	err := runAccountSet(accountSetCmd, nil)
	require.NoError(t, err)

	assert.Equal(t, http.MethodPatch, receivedMethod)
	assert.Equal(t, "/api/v1/accounts/test-org/block-member-personal-push", receivedPath)
	assert.Equal(t, true, receivedBody["enabled"])
	assert.Equal(t, "✓ "+msgBlockPersonalPushSet("test-org", true)+"\n", out.String())
}

func TestAccountSet_NothingToUpdateWithoutAFlag(t *testing.T) {
	resetBlockPersonalPushFlag(t)

	err := runAccountSet(accountSetCmd, nil)

	require.Error(t, err)
	assert.Equal(t, errAccountSetNothingToUpdate().Error(), err.Error())
}
