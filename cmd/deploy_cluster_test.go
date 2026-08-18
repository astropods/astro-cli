package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDeployCluster(t *testing.T) {
	tests := []struct {
		name         string
		flag         string
		body         string
		wantCluster  string
		wantRequests int
	}{
		{
			name:        "explicit flag skips the account lookup",
			flag:        "eu-west-1-managed",
			wantCluster: "eu-west-1-managed",
		},
		{
			name:         "single allowed cluster defers to the account default",
			body:         `{"allowed_clusters":[{"cluster_id":"eu-west-1-managed","region":"eu-west-1","is_default":true}]}`,
			wantCluster:  "",
			wantRequests: 1,
		},
		{
			name:         "no allowed clusters defers to the account default",
			body:         `{}`,
			wantCluster:  "",
			wantRequests: 1,
		},
		{
			// Non-interactive runs (CI) must not block on a prompt.
			name:         "multiple clusters without a tty defers to the account default",
			body:         `{"allowed_clusters":[{"cluster_id":"eu","is_default":true},{"cluster_id":"us"}]}`,
			wantCluster:  "",
			wantRequests: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var requests int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				assert.Equal(t, "/api/v1/accounts/myorg", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			t.Cleanup(func() { blueprintServerURLOverride = "" })
			blueprintServerURLOverride = srv.URL

			cmd := &cobra.Command{}
			cmd.Flags().String("cluster", tc.flag, "")

			got, err := resolveDeployCluster(cmd, AccountToken{Account: "myorg", Token: "tok"}, false)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCluster, got)
			assert.Equal(t, tc.wantRequests, requests)
		})
	}
}

// A failed lookup must not block the deploy: an empty cluster asks the server
// for the account default, which is the same thing the CLI would have picked.
func TestResolveDeployCluster_LookupFailureFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Cleanup(func() { blueprintServerURLOverride = "" })
	blueprintServerURLOverride = srv.URL

	cmd := &cobra.Command{}
	cmd.Flags().String("cluster", "", "")

	got, err := resolveDeployCluster(cmd, AccountToken{Account: "myorg", Token: "tok"}, false)
	require.NoError(t, err)
	assert.Empty(t, got)
}
