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

func crashLoopingRuntime() map[string]any {
	return map[string]any{
		"ready":    0,
		"replicas": 1,
		"workloads": []any{
			map[string]any{
				"name":     "my-agent-agent",
				"phase":    "Running",
				"pod_name": "my-agent-agent-abc-xyz",
				"containers": []any{
					map[string]any{
						"name": "app", "state": "Waiting", "ready": false, "restart_count": 12,
						"message": "The container keeps crashing on startup",
					},
					map[string]any{"name": "messaging", "state": "Running", "ready": true, "restart_count": 0},
				},
			},
		},
	}
}

func runtimeTestHandler(list, detail, status, runtime, alerts any, failing map[string]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for suffix, body := range map[string]any{"/status": status, "/runtime": map[string]any{"runtime": runtime}, "/alerts": alerts} {
			if !strings.HasSuffix(r.URL.Path, suffix) {
				continue
			}
			if failing[suffix] {
				jsonHandler(http.StatusInternalServerError, map[string]any{"error": "unavailable"})(w, r)
				return
			}
			jsonHandler(http.StatusOK, body)(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/deployments/dep-abc-123") {
			jsonHandler(http.StatusOK, detail)(w, r)
			return
		}
		jsonHandler(http.StatusOK, list)(w, r)
	})
}

func TestAgentGetSurfacesRuntimeState(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{
			"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent",
			"build_id": "abc12345", "namespace": "astro-testaccount", "status": "failed",
			"created_at": "2026-01-01T10:00:00Z",
		}},
		"count": 1,
	}
	detail := map[string]any{
		"deployment": map[string]any{
			"id":     "dep-abc-123",
			"status": "failed",
			"workloads": []any{map[string]any{
				"name": "my-agent-agent", "component": "agent", "pod_name": "my-agent-agent-abc-xyz",
			}},
		},
	}
	status := map[string]any{
		"value": "error", "reason": "failed",
		"details":       "Deployment failed: partial failure",
		"error_message": "partial failure",
		"failed_on": []any{map[string]any{
			"workload": "my-agent-agent", "component": "agent", "phase": "failed",
			"message": "The container keeps crashing on startup",
		}},
	}
	activeSince := "2026-09-03T22:05:00Z"
	alerts := map[string]any{"alerts": []any{
		map[string]any{
			"name": "crash_loop", "title": "Crash loop", "severity": "critical", "state": "firing",
			"description":  "The agent crashes every time it starts, so it can't serve requests.",
			"active_since": activeSince,
		},
		map[string]any{"name": "oom", "title": "Out of memory", "severity": "critical", "state": "ok"},
	}}

	cases := []struct {
		name       string
		failing    map[string]bool
		wantOut    []string
		wantAbsent []string
	}{
		{
			name: "names the failure reason and the workload holding it back",
			wantOut: []string{
				"Deployment failed: partial failure",
				msgWorkloadIssueLine("my-agent-agent", "agent", "failed", "The container keeps crashing on startup"),
			},
		},
		{
			name:    "reports container state and restart count",
			wantOut: []string{msgContainerStateLine("app", "Waiting", 12, "The container keeps crashing on startup")},
			wantAbsent: []string{
				msgContainerStateLine("messaging", "Running", 0, ""),
			},
		},
		{
			name:       "lists firing alerts only",
			wantOut:    []string{msgAlertLine("critical", "Crash loop", "firing", activeSince)},
			wantAbsent: []string{"Out of memory"},
		},
		{
			name:       "renders the record when the runtime reads fail",
			failing:    map[string]bool{"/status": true, "/runtime": true, "/alerts": true},
			wantOut:    []string{"my-agent", "astro-testaccount", "failed"},
			wantAbsent: []string{"Crash loop", "Reason:"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupAgentTest(t, runtimeTestHandler(list, detail, status, crashLoopingRuntime(), alerts, tc.failing))

			buf := &bytes.Buffer{}
			agentGetCmd.SetOut(buf)
			agentGetCmd.SetContext(context.Background())
			setAgentTargetName(t, agentGetCmd, "my-agent")

			require.NoError(t, runAgentGet(agentGetCmd, nil))
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
			for _, absent := range tc.wantAbsent {
				assert.NotContains(t, buf.String(), absent)
			}
		})
	}
}

func TestAgentLogsWarnsWhenContainerRestarts(t *testing.T) {
	list := map[string]any{
		"deployments": []any{map[string]any{
			"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent",
			"build_id": "abc12345", "status": "active", "created_at": "2026-01-01T10:00:00Z",
		}},
		"count": 1,
	}
	detail := map[string]any{
		"deployment": map[string]any{
			"id": "dep-abc-123",
			"workloads": []any{map[string]any{
				"name": "my-agent-agent", "component": "agent", "pod_name": "my-agent-agent-abc-xyz",
				"containers": []any{map[string]any{"name": "app"}, map[string]any{"name": "messaging"}},
			}},
		},
	}
	logsBody := `[{"timestamp":"2026-01-01T10:00:00Z","level":"","message":"agent started"}]`

	cases := []struct {
		name       string
		runtime    any
		wantOut    []string
		wantAbsent []string
	}{
		{
			name:    "crash looping container",
			runtime: crashLoopingRuntime(),
			wantOut: []string{
				msgContainerRestartWarning("app", "Waiting", 12, "The container keeps crashing on startup"),
				"agent started",
			},
		},
		{
			name: "healthy container",
			runtime: map[string]any{"workloads": []any{map[string]any{
				"name": "my-agent-agent",
				"containers": []any{
					map[string]any{"name": "app", "state": "Running", "ready": true, "restart_count": 0},
				},
			}}},
			wantOut:    []string{"agent started"},
			wantAbsent: []string{"restart"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/logs") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(logsBody))
					return
				}
				runtimeTestHandler(list, detail, nil, tc.runtime, nil, nil).ServeHTTP(w, r)
			})
			setupAgentTest(t, handler)

			buf := &bytes.Buffer{}
			agentLogsCmd.SetOut(buf)
			agentLogsCmd.SetContext(context.Background())
			setAgentTargetName(t, agentLogsCmd, "my-agent")

			require.NoError(t, runAgentLogs(agentLogsCmd, nil))
			for _, want := range tc.wantOut {
				assert.Contains(t, buf.String(), want)
			}
			for _, absent := range tc.wantAbsent {
				assert.NotContains(t, buf.String(), absent)
			}
		})
	}
}

func TestRuntimeContainerHealthy(t *testing.T) {
	cases := []struct {
		name      string
		container runtimeContainer
		want      bool
	}{
		{name: "running and ready", container: runtimeContainer{State: "Running", Ready: true}, want: true},
		{name: "restarted at least once", container: runtimeContainer{State: "Running", Ready: true, RestartCount: 1}},
		{name: "waiting to start", container: runtimeContainer{State: "Waiting"}},
		{name: "running but not ready", container: runtimeContainer{State: "Running"}},
		{name: "state unreported", container: runtimeContainer{Ready: true}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.container.healthy())
		})
	}
}
