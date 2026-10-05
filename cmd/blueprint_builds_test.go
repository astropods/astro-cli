package cmd

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cobra flags outlive a single test, so each flag set here is reset on cleanup.
func runBlueprintSubcommand(t *testing.T, cmd *cobra.Command, flags map[string]string, fn func() error) (string, error) {
	t.Helper()
	for name, value := range flags {
		f := cmd.Flags().Lookup(name)
		require.NotNil(t, f, name)
		def := f.DefValue
		require.NoError(t, cmd.Flags().Set(name, value))
		t.Cleanup(func() {
			cmd.Flags().Set(name, def) //nolint:errcheck
			f.Changed = false
		})
	}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetContext(context.Background())
	err := fn()
	return buf.String(), err
}

var testBuildsPayload = map[string]any{
	"builds": []any{
		map[string]any{
			"build_id": "abc12345", "source": "github", "status": "registered", "is_latest": true,
			"started_at": "2026-10-01T12:00:00Z", "completed_at": "2026-10-01T12:02:30Z",
			"commit_sha": "0123456789abcdef", "commit_message": "Fix the parser\n\nLonger body.",
			"branch": "main", "repo_full_name": "example/agent", "pushed_by": "Ada Lovelace",
			"vulnerabilities": map[string]any{
				"status": "succeeded", "critical": 2, "high": 5, "medium": 0, "low": 1, "unknown": 0, "fixable": 3,
			},
		},
		map[string]any{
			"build_id": "def67890", "source": "cli", "status": "registered",
			"started_at":      "2026-09-30T09:00:00Z",
			"vulnerabilities": map[string]any{"status": "scanning"},
		},
		map[string]any{
			"build_id": "0f0f0f0f", "source": "github", "status": "failed", "started_at": "2026-09-29T09:00:00Z",
			"step": "build", "error": "exit status 1",
		},
	},
}

func TestBlueprintBuildsList(t *testing.T) {
	cases := []struct {
		name       string
		flags      map[string]string
		statusCode int
		body       any
		wantQuery  string
		wantErr    error
		wantOut    []string
	}{
		{
			name:       "lists builds with their vulnerability summary",
			statusCode: http.StatusOK,
			body:       testBuildsPayload,
			wantOut: []string{
				"abc12345", "registered (latest)", "2 critical, 5 high, 1 low",
				"def67890", "scanning",
				"0f0f0f0f", "failed",
			},
		},
		{
			name:       "passes --limit to the server",
			flags:      map[string]string{"limit": "5"},
			statusCode: http.StatusOK,
			body:       testBuildsPayload,
			wantQuery:  "limit=5",
			wantOut:    []string{"abc12345"},
		},
		{
			name:       "json output",
			flags:      map[string]string{"json": "true"},
			statusCode: http.StatusOK,
			body:       testBuildsPayload,
			wantOut:    []string{`"build_id": "abc12345"`, `"critical": 2`},
		},
		{
			name:       "no builds",
			statusCode: http.StatusOK,
			body:       map[string]any{"builds": []any{}},
			wantOut:    []string{msgNoBlueprintBuilds("my-agent")},
		},
		{
			name:       "hidden or missing blueprint",
			statusCode: http.StatusNotFound,
			body:       map[string]any{"error": "not found"},
			wantErr:    errBlueprintBuildsNotFound("my-agent", "testaccount"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotQuery string
			setupBlueprintTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
				jsonHandler(tc.statusCode, tc.body)(w, r)
			}))

			out, err := runBlueprintSubcommand(t, blueprintBuildsListCmd, tc.flags, func() error {
				return runBlueprintBuildsList(blueprintBuildsListCmd, []string{"my-agent"})
			})
			assert.Equal(t, "/api/v1/agents/testaccount/my-agent/builds", gotPath)
			assert.Equal(t, tc.wantQuery, gotQuery)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
				return
			}
			require.NoError(t, err)
			for _, want := range tc.wantOut {
				assert.Contains(t, out, want)
			}
		})
	}
}

func TestBlueprintBuildsGet(t *testing.T) {
	scans := map[string]any{
		"build_id": "abc12345",
		"summary":  map[string]any{"status": "succeeded", "critical": 1, "high": 0, "medium": 1, "low": 0, "unknown": 0, "fixable": 1},
		"components": []any{
			map[string]any{
				"component": "agent", "status": "succeeded",
				"image_digest": "sha256:0123456789abcdef0123456789abcdef",
				"scanned_at":   "2026-10-01T12:05:00Z",
				"counts":       map[string]any{"critical": 1, "medium": 1, "fixable": 1},
				"findings": []any{
					map[string]any{"id": "CVE-2026-0001", "severity": "CRITICAL", "package": "openssl", "installed_version": "3.0.1", "fixed_version": "3.0.2", "title": "Buffer overflow"},
					map[string]any{"id": "CVE-2026-0002", "severity": "MEDIUM", "package": "zlib", "installed_version": "1.2.11"},
				},
			},
			map[string]any{"component": "worker", "status": "succeeded", "counts": map[string]any{}, "findings": []any{}},
		},
	}
	noScans := map[string]any{"build_id": "0f0f0f0f", "summary": nil, "components": []any{}}

	cases := []struct {
		name          string
		args          []string
		flags         map[string]string
		builds        any
		scans         any
		wantScansPath string
		wantErr       error
		wantOut       []string
	}{
		{
			name:          "shows the latest published build without a build ID",
			args:          []string{"my-agent"},
			builds:        testBuildsPayload,
			scans:         scans,
			wantScansPath: "/api/v1/agents/testaccount/my-agent/builds/abc12345/vulnerabilities",
			wantOut: []string{
				"Build abc12345  registered (latest)",
				"Source:", "github",
				"Completed:", "2026-10-01T12:02:30Z (2m30s)",
				"Commit:", "0123456 Fix the parser",
				"Branch:", "main",
				"Repository:", "example/agent",
				"Pushed by:", "Ada Lovelace",
				"Vulnerabilities  1 critical, 1 medium (1 fixable)",
				"agent", "sha256:0123456789ab", "scanned 2026-10-01T12:05:00Z",
				"CRITICAL", "CVE-2026-0001", "openssl", "3.0.1", "3.0.2", "Buffer overflow",
				"CVE-2026-0002", "zlib",
				"worker  none found",
			},
		},
		{
			name:          "shows the step and error of a failed build",
			args:          []string{"my-agent", "0f0f0f0f"},
			builds:        testBuildsPayload,
			scans:         noScans,
			wantScansPath: "/api/v1/agents/testaccount/my-agent/builds/0f0f0f0f/vulnerabilities",
			wantOut:       []string{"Build 0f0f0f0f  failed", "Step:", "build", "Error:", "exit status 1", "Vulnerabilities  not scanned"},
		},
		{
			name:          "json output",
			args:          []string{"my-agent", "abc12345"},
			flags:         map[string]string{"json": "true"},
			builds:        testBuildsPayload,
			scans:         scans,
			wantScansPath: "/api/v1/agents/testaccount/my-agent/builds/abc12345/vulnerabilities",
			wantOut:       []string{`"build": {`, `"commit_sha": "0123456789abcdef"`, `"vulnerabilities": {`, `"id": "CVE-2026-0001"`},
		},
		{
			name:    "build not in the history",
			args:    []string{"my-agent", "ffffffff"},
			builds:  testBuildsPayload,
			wantErr: errBuildNotFound("my-agent", "ffffffff", maxBuildListLimit),
		},
		{
			name:    "no published build",
			args:    []string{"my-agent"},
			builds:  map[string]any{"builds": []any{}},
			wantErr: errBlueprintNoPublishedBuild("my-agent"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotQuery, gotScansPath string
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/agents/testaccount/my-agent/builds", func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.RawQuery
				jsonHandler(http.StatusOK, tc.builds)(w, r)
			})
			mux.HandleFunc("/api/v1/agents/testaccount/my-agent/builds/", func(w http.ResponseWriter, r *http.Request) {
				gotScansPath = r.URL.Path
				jsonHandler(http.StatusOK, tc.scans)(w, r)
			})
			setupBlueprintTest(t, mux)

			out, err := runBlueprintSubcommand(t, blueprintBuildsGetCmd, tc.flags, func() error {
				return runBlueprintBuildsGet(blueprintBuildsGetCmd, tc.args)
			})
			assert.Equal(t, "limit=200", gotQuery)
			assert.Equal(t, tc.wantScansPath, gotScansPath)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
				return
			}
			require.NoError(t, err)
			for _, want := range tc.wantOut {
				assert.Contains(t, out, want)
			}
		})
	}
}

func TestBlueprintBuildArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "name only", args: []string{"my-agent"}},
		{name: "name and build", args: []string{"my-agent", "abc12345"}},
		{name: "no args", args: nil, wantErr: true},
		{name: "too many args", args: []string{"my-agent", "a", "b"}, wantErr: true},
		{name: "invalid name", args: []string{"My Agent"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := blueprintBuildArgs(nil, tc.args)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVulnerabilitySummary(t *testing.T) {
	cases := []struct {
		name   string
		status string
		counts vulnerabilityCounts
		want   string
	}{
		{name: "not scanned", status: "skipped", want: "not scanned"},
		{name: "pending", status: "pending", want: "scanning"},
		{name: "scanning hides partial counts", status: "scanning", counts: vulnerabilityCounts{High: 3}, want: "scanning"},
		{name: "failed", status: "failed", want: "scan failed"},
		{name: "clean", status: "succeeded", want: "none found"},
		{
			name:   "lists non-zero severities worst first",
			status: "succeeded",
			counts: vulnerabilityCounts{Critical: 1, Medium: 4, Unknown: 2, Fixable: 3},
			want:   "1 critical, 4 medium, 2 unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, vulnerabilitySummary(tc.status, tc.counts))
		})
	}
}

func TestShortImageDigest(t *testing.T) {
	assert.Equal(t, "sha256:0123456789ab", shortImageDigest("sha256:0123456789abcdef"))
	assert.Equal(t, "sha256:abc", shortImageDigest("sha256:abc"))
}
