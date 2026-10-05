package cmd

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fastBuildLogPolls(t *testing.T) {
	t.Helper()
	prev := buildLogsPollInterval
	buildLogsPollInterval = time.Millisecond
	t.Cleanup(func() { buildLogsPollInterval = prev })
}

func sequenceHandler(bodies ...any) http.HandlerFunc {
	var mu sync.Mutex
	next := 0
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		body := bodies[min(next, len(bodies)-1)]
		next++
		mu.Unlock()
		jsonHandler(http.StatusOK, body)(w, r)
	}
}

func buildLogsBody(phase, logs string) map[string]any {
	return map[string]any{
		"phase":      phase,
		"components": []any{map[string]any{"name": "agent", "status": phase, "logs": logs}},
	}
}

func TestBlueprintBuildsLogs(t *testing.T) {
	cases := []struct {
		name         string
		args         []string
		builds       any
		logsStatus   int
		logs         any
		wantLogsPath string
		wantErr      error
		wantOut      string
	}{
		{
			name:         "prints each component's logs",
			args:         []string{"my-agent", "abc12345"},
			logs:         buildLogsBody("registered", "=== build ===\nstep 1\nstep 2\n\n"),
			wantLogsPath: "/api/v1/agents/testaccount/my-agent/github/builds/abc12345/logs",
			wantOut:      "agent  registered\n=== build ===\nstep 1\nstep 2\n",
		},
		{
			name:         "defaults to the newest server-side build",
			args:         []string{"my-agent"},
			builds:       map[string]any{"builds": []any{map[string]any{"build_id": "cli00001", "source": "cli"}, map[string]any{"build_id": "srv00002", "source": "hosted"}}},
			logs:         buildLogsBody("registered", "done\n"),
			wantLogsPath: "/api/v1/agents/testaccount/my-agent/github/builds/srv00002/logs",
			wantOut:      "agent  registered\ndone\n",
		},
		{
			name:         "falls back to the flat logs of an older build",
			args:         []string{"my-agent", "abc12345"},
			logs:         map[string]any{"phase": "failed", "logs": "boom\n"},
			wantLogsPath: "/api/v1/agents/testaccount/my-agent/github/builds/abc12345/logs",
			wantOut:      "agent  failed\nboom\n",
		},
		{
			name:         "a queued build has no logs yet",
			args:         []string{"my-agent", "abc12345"},
			logs:         map[string]any{"phase": "pending", "components": []any{}},
			wantLogsPath: "/api/v1/agents/testaccount/my-agent/github/builds/abc12345/logs",
			wantOut:      msgNoBuildLogsYet("abc12345", "pending"),
		},
		{
			name:    "only CLI pushes",
			args:    []string{"my-agent"},
			builds:  map[string]any{"builds": []any{map[string]any{"build_id": "cli00001", "source": "cli"}}},
			wantErr: errBlueprintNoServerBuilds("my-agent"),
		},
		{
			name:         "a CLI push has no server logs",
			args:         []string{"my-agent", "cli00001"},
			logsStatus:   http.StatusNotFound,
			logs:         map[string]any{"error": "build not found"},
			wantLogsPath: "/api/v1/agents/testaccount/my-agent/github/builds/cli00001/logs",
			wantErr:      errBuildLogsNotFound("my-agent", "cli00001"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotLogsPath string
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/agents/testaccount/my-agent/builds", jsonHandler(http.StatusOK, tc.builds))
			mux.HandleFunc("/api/v1/agents/testaccount/my-agent/github/builds/", func(w http.ResponseWriter, r *http.Request) {
				gotLogsPath = r.URL.Path
				status := tc.logsStatus
				if status == 0 {
					status = http.StatusOK
				}
				jsonHandler(status, tc.logs)(w, r)
			})
			setupBlueprintTest(t, mux)

			out, err := runBlueprintSubcommand(t, blueprintBuildsLogsCmd, nil, func() error {
				return runBlueprintBuildsLogs(blueprintBuildsLogsCmd, tc.args)
			})
			assert.Equal(t, tc.wantLogsPath, gotLogsPath)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
				return
			}
			require.NoError(t, err)
			assert.Contains(t, out, tc.wantOut)
		})
	}
}

func TestTailBuildLogs(t *testing.T) {
	cases := []struct {
		name      string
		snapshots []buildLogsResponse
		wantOut   string
		wantErr   error
	}{
		{
			name: "prints each line once as sections grow",
			snapshots: []buildLogsResponse{
				{Phase: "pending"},
				{Phase: "building", Components: []buildLogComponent{{Name: "agent", Status: "building",
					Logs: "=== clone ===\ncloned\n\n=== build ===\nstep 1\n\n=== push ===\n(no output)\n"}}},
				{Phase: "building", Components: []buildLogComponent{{Name: "agent", Status: "building",
					Logs: "=== clone ===\ncloned\n\n=== build ===\nstep 1\nstep 2\n\n=== push ===\n(no output)\n"}}},
				{Phase: "registered", Components: []buildLogComponent{{Name: "agent", Status: "registered",
					Logs: "=== clone ===\ncloned\n\n=== build ===\nstep 1\nstep 2\n\n=== push ===\npushed\n\n"}}},
			},
			wantOut: "agent  building\n=== clone ===\ncloned\n=== build ===\nstep 1\nstep 2\n=== push ===\npushed\n" +
				msgBuildFinished("abc12345", "registered") + "\n",
		},
		{
			name: "follows a container past the server's tail window",
			snapshots: []buildLogsResponse{
				{Phase: "building", Components: []buildLogComponent{{Name: "agent", Status: "building",
					Logs: "=== build ===\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\n"}}},
				{Phase: "registered", Components: []buildLogComponent{{Name: "agent", Status: "registered",
					Logs: "=== build ===\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n"}}},
			},
			wantOut: "agent  building\n=== build ===\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n" +
				msgBuildFinished("abc12345", "registered") + "\n",
		},
		{
			name: "a failed build is an error",
			snapshots: []buildLogsResponse{
				{Phase: "failed", Components: []buildLogComponent{{Name: "agent", Status: "failed", Logs: "=== build ===\nboom\n"}}},
			},
			wantOut: "agent  failed\n=== build ===\nboom\n",
			wantErr: errBuildFailed("abc12345"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastBuildLogPolls(t)
			next := 0
			fetch := func() (buildLogsResponse, error) {
				resp := tc.snapshots[min(next, len(tc.snapshots)-1)]
				next++
				return resp, nil
			}
			buf := &bytes.Buffer{}
			err := tailBuildLogs(context.Background(), buf, "abc12345", fetch)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantOut, buf.String())
		})
	}
}

func TestUnseenLines(t *testing.T) {
	cases := []struct {
		name      string
		prev, cur []string
		want      []string
	}{
		{name: "nothing printed yet", cur: []string{"a"}, want: []string{"a"}},
		{name: "appended lines", prev: []string{"a", "b"}, cur: []string{"a", "b", "c"}, want: []string{"c"}},
		{name: "no change", prev: []string{"a", "b"}, cur: []string{"a", "b"}, want: []string{}},
		{
			name: "window slid past the start",
			prev: []string{"1", "2", "3", "4", "5", "6", "7", "8"},
			cur:  []string{"4", "5", "6", "7", "8", "9"},
			want: []string{"9"},
		},
		{
			name: "an overlap too short to trust prints everything",
			prev: []string{"1", "2", "3", "4", "5", "6", "7", "8"},
			cur:  []string{"7", "8", "9"},
			want: []string{"7", "8", "9"},
		},
		{name: "no overlap prints everything", prev: []string{"a"}, cur: []string{"x", "y"}, want: []string{"x", "y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, unseenLines(tc.prev, tc.cur))
		})
	}
}

func TestBlueprintBuildsRebuild(t *testing.T) {
	started := map[string]any{"build_id": "new00001", "commit_sha": "0123456789abcdef", "commit_message": "Fix the parser\n\nBody."}
	idle := map[string]any{"builds": []any{map[string]any{"build_id": "old00001", "source": "github", "status": "registered"}}}
	running := map[string]any{"builds": []any{
		map[string]any{"build_id": "cli00001", "source": "cli", "status": "registered"},
		map[string]any{"build_id": "run00001", "source": "github", "status": "building"},
	}}

	cases := []struct {
		name          string
		flags         map[string]string
		buildsStatus  int
		builds        any
		rebuildStatus int
		rebuild       any
		logs          []any
		wantRebuild   bool
		wantErr       error
		wantOut       []string
	}{
		{
			name:          "starts a rebuild and says how to follow it",
			builds:        idle,
			rebuildStatus: http.StatusAccepted,
			rebuild:       started,
			wantRebuild:   true,
			wantOut: []string{
				msgRebuildStarted("new00001", "0123456789abcdef", "Fix the parser\n\nBody."),
				msgTailBuildLogs("my-agent", "new00001"),
			},
		},
		{
			name:    "refuses while a build is running",
			builds:  running,
			wantErr: errRebuildCancelsRunningBuild("run00001", "building"),
		},
		{
			name:          "--yes rebuilds over a running build",
			flags:         map[string]string{"yes": "true"},
			builds:        running,
			rebuildStatus: http.StatusAccepted,
			rebuild:       started,
			wantRebuild:   true,
			wantOut:       []string{"Rebuild started: build new00001 from commit 0123456 Fix the parser"},
		},
		{
			name:          "rebuilds when the build history is hidden",
			buildsStatus:  http.StatusNotFound,
			builds:        map[string]any{"error": "not found"},
			rebuildStatus: http.StatusAccepted,
			rebuild:       started,
			wantRebuild:   true,
			wantOut:       []string{"Rebuild started"},
		},
		{
			name:          "--tail streams the new build",
			flags:         map[string]string{"tail": "true"},
			builds:        idle,
			rebuildStatus: http.StatusAccepted,
			rebuild:       started,
			logs: []any{
				buildLogsBody("building", "=== build ===\nstep 1\n"),
				buildLogsBody("registered", "=== build ===\nstep 1\nstep 2\n"),
			},
			wantRebuild: true,
			wantOut:     []string{"=== build ===\nstep 1\nstep 2\n" + msgBuildFinished("new00001", "registered")},
		},
		{
			name:          "no connected source",
			builds:        idle,
			rebuildStatus: http.StatusNotFound,
			rebuild:       map[string]any{"error": "no GitHub connection for this agent"},
			wantRebuild:   true,
			wantErr:       errRebuildUnavailable("my-agent", "testaccount"),
		},
		{
			name:          "GitHub account not connected",
			builds:        idle,
			rebuildStatus: http.StatusUnprocessableEntity,
			rebuild:       map[string]any{"error": "github_not_connected"},
			wantRebuild:   true,
			wantErr:       errRebuildGitHubNotConnected(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastBuildLogPolls(t)
			var gotRebuild bool
			mux := http.NewServeMux()
			mux.HandleFunc("/api/v1/agents/testaccount/my-agent/builds", func(w http.ResponseWriter, r *http.Request) {
				status := tc.buildsStatus
				if status == 0 {
					status = http.StatusOK
				}
				jsonHandler(status, tc.builds)(w, r)
			})
			mux.HandleFunc("/api/v1/agents/testaccount/my-agent/github/rebuild", func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				gotRebuild = true
				jsonHandler(tc.rebuildStatus, tc.rebuild)(w, r)
			})
			if len(tc.logs) > 0 {
				mux.HandleFunc("/api/v1/agents/testaccount/my-agent/github/builds/new00001/logs", sequenceHandler(tc.logs...))
			}
			setupBlueprintTest(t, mux)

			out, err := runBlueprintSubcommand(t, blueprintBuildsRebuildCmd, tc.flags, func() error {
				return runBlueprintBuildsRebuild(blueprintBuildsRebuildCmd, []string{"my-agent"})
			})
			assert.Equal(t, tc.wantRebuild, gotRebuild)
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
