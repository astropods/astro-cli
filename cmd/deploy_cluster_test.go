package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubInteractiveTerminal(t *testing.T, interactive bool) {
	t.Helper()
	prev := interactiveTerminal
	t.Cleanup(func() { interactiveTerminal = prev })
	interactiveTerminal = func() bool { return interactive }
}

func TestResolveDeployCluster(t *testing.T) {
	tests := []struct {
		name         string
		flag         string
		dryRun       bool
		jsonOut      bool
		interactive  bool
		body         string
		wantCluster  string
		wantRequests int
	}{
		{
			name:        "explicit flag skips the account lookup",
			flag:        "eu-west-1-managed",
			interactive: true,
			wantCluster: "eu-west-1-managed",
		},
		{
			name:         "single allowed cluster defers to the account default",
			interactive:  true,
			body:         `{"allowed_clusters":[{"cluster_id":"eu-west-1-managed","region":"eu-west-1","is_default":true}]}`,
			wantCluster:  "",
			wantRequests: 1,
		},
		{
			name:         "no allowed clusters defers to the account default",
			interactive:  true,
			body:         `{}`,
			wantCluster:  "",
			wantRequests: 1,
		},
		{
			name:        "multiple clusters without a tty defers to the account default",
			interactive: false,
			body:        `{"allowed_clusters":[{"cluster_id":"eu","is_default":true},{"cluster_id":"us"}]}`,
			wantCluster: "",
		},
		{
			name:        "dry run does not prompt",
			dryRun:      true,
			interactive: true,
			body:        `{"allowed_clusters":[{"cluster_id":"eu","is_default":true},{"cluster_id":"us"}]}`,
			wantCluster: "",
		},
		{
			name:        "json output does not prompt",
			jsonOut:     true,
			interactive: true,
			body:        `{"allowed_clusters":[{"cluster_id":"eu","is_default":true},{"cluster_id":"us"}]}`,
			wantCluster: "",
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
			stubInteractiveTerminal(t, tc.interactive)

			cmd := &cobra.Command{}
			cmd.Flags().String("cluster", tc.flag, "")
			cmd.Flags().Bool("dry-run", tc.dryRun, "")
			cmd.Flags().Bool("json", tc.jsonOut, "")

			got, err := resolveDeployCluster(cmd, AccountToken{Account: "myorg", Token: "tok"}, false)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCluster, got)
			assert.Equal(t, tc.wantRequests, requests)
		})
	}
}

func TestResolveDeployCluster_LookupFailureFallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Cleanup(func() { blueprintServerURLOverride = "" })
	blueprintServerURLOverride = srv.URL
	stubInteractiveTerminal(t, true)

	cmd := &cobra.Command{}
	cmd.Flags().String("cluster", "", "")

	got, err := resolveDeployCluster(cmd, AccountToken{Account: "myorg", Token: "tok"}, false)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestClusterRegionLabel(t *testing.T) {
	tests := []struct {
		name    string
		cluster allowedCluster
		want    string
	}{
		{
			name:    "flag and label",
			cluster: allowedCluster{ClusterID: "eu", Region: "eu-west-1", RegionLabel: "Europe (Ireland)", RegionFlag: "🇮🇪"},
			want:    "🇮🇪  Europe (Ireland)",
		},
		{
			name:    "default is called out",
			cluster: allowedCluster{ClusterID: "eu", Region: "eu-west-1", RegionLabel: "Europe (Ireland)", RegionFlag: "🇮🇪", IsDefault: true},
			want:    "🇮🇪  Europe (Ireland)  Default",
		},
		{
			name:    "missing flag falls back to a globe",
			cluster: allowedCluster{ClusterID: "eu", Region: "eu-west-1", RegionLabel: "Europe (Ireland)"},
			want:    "🌐  Europe (Ireland)",
		},
		{
			name:    "missing label falls back to the region",
			cluster: allowedCluster{ClusterID: "eu", Region: "eu-west-1", RegionFlag: "🇮🇪"},
			want:    "🇮🇪  eu-west-1",
		},
		{
			name:    "missing region falls back to the cluster ID",
			cluster: allowedCluster{ClusterID: "eu-west-1-managed"},
			want:    "🌐  eu-west-1-managed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, clusterRegionLabel(tc.cluster))
		})
	}
}

func TestClusterPromptOptions(t *testing.T) {
	tests := []struct {
		name         string
		allowed      []allowedCluster
		wantLabels   []string
		wantSelected string
	}{
		{
			name: "preselects the account default",
			allowed: []allowedCluster{
				{ClusterID: "us", RegionLabel: "US East", RegionFlag: "🇺🇸"},
				{ClusterID: "eu", RegionLabel: "Europe (Ireland)", RegionFlag: "🇮🇪", IsDefault: true},
			},
			wantLabels:   []string{"🇺🇸  US East", "🇮🇪  Europe (Ireland)  Default"},
			wantSelected: "eu",
		},
		{
			name: "falls back to the first cluster when none is default",
			allowed: []allowedCluster{
				{ClusterID: "us", RegionLabel: "US East", RegionFlag: "🇺🇸"},
				{ClusterID: "eu", RegionLabel: "Europe (Ireland)", RegionFlag: "🇮🇪"},
			},
			wantLabels:   []string{"🇺🇸  US East", "🇮🇪  Europe (Ireland)"},
			wantSelected: "us",
		},
		{
			name:         "no clusters yields no options",
			allowed:      nil,
			wantLabels:   nil,
			wantSelected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			options, selected := clusterPromptOptions(tc.allowed)
			labels := make([]string, 0, len(options))
			for _, o := range options {
				labels = append(labels, o.Key)
			}
			assert.Equal(t, tc.wantSelected, selected)
			if tc.wantLabels == nil {
				assert.Empty(t, labels)
				return
			}
			assert.Equal(t, tc.wantLabels, labels)
		})
	}
}

func TestClusterNotAvailableFromErr(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
	}{
		{
			name: "cluster placement refusal",
			body: `{"error":"cluster \"us\" is not available to this account (available: eu)","cluster_id":"us","available_clusters":["eu"]}`,
			want: errClusterNotAvailable("us", []string{"eu"}),
		},
		{
			name: "refusal without an available list",
			body: `{"cluster_id":"us"}`,
			want: errClusterNotAvailable("us", nil),
		},
		{
			name: "unrelated forbidden response",
			body: `{"error":"insufficient permissions"}`,
			want: nil,
		},
		{
			name: "non-JSON body",
			body: `<html>403</html>`,
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := clusterNotAvailableFromErr(newAPIError(http.StatusForbidden, []byte(tc.body)))
			if tc.want == nil {
				assert.NoError(t, got)
				return
			}
			require.Error(t, got)
			assert.Equal(t, tc.want.Error(), got.Error())
		})
	}
}
