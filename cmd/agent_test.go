package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAgentTest(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	agentServerURLOverride = srv.URL
	t.Cleanup(func() { agentServerURLOverride = "" })
}

func TestAgentGet(t *testing.T) {
	dep := map[string]any{
		"id":           "dep-abc-123",
		"name":         "my-agent",
		"display_name": "my-agent",
		"build_id":     "abc12345",
		"namespace":    "astro-testaccount",
		"status":       "active",
		"created_at":   "2026-01-01T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}
	detailPayload := map[string]any{
		"deployment": map[string]any{
			"id": "dep-abc-123",
			"workloads": []any{
				map[string]any{
					"name":      "my-agent-agent",
					"component": "agent",
					"pod_name":  "my-agent-agent-abc-xyz",
					"containers": []any{
						map[string]any{"name": "app"},
						map[string]any{"name": "messaging"},
					},
				},
			},
		},
	}

	cases := []struct {
		name       string
		listBody   any
		jsonOutput bool
		wantErr    bool
		wantOut    string
	}{
		{name: "shows agent name", listBody: listPayload, wantOut: "my-agent"},
		{name: "shows status", listBody: listPayload, wantOut: "active"},
		{name: "shows build id", listBody: listPayload, wantOut: "abc12345"},
		{name: "shows deployment id", listBody: listPayload, wantOut: "dep-abc-123"},
		{name: "shows namespace", listBody: listPayload, wantOut: "astro-testaccount"},
		{name: "shows component name", listBody: listPayload, wantOut: "agent"},
		{name: "json output", listBody: listPayload, jsonOutput: true, wantOut: `"namespace"`},
		{name: "works when paused", listBody: map[string]any{
			"deployments": []any{map[string]any{"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent", "build_id": "abc12345", "namespace": "astro-testaccount", "status": "scaled_down", "created_at": "2026-01-01T10:00:00Z"}},
			"count":       1,
		}, wantOut: "scaled_down"},
		{name: "not found", listBody: map[string]any{"deployments": []any{}, "count": 0}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/deployments/dep-abc-123") {
					jsonHandler(http.StatusOK, detailPayload)(w, r)
				} else {
					jsonHandler(http.StatusOK, tc.listBody)(w, r)
				}
			})
			setupAgentTest(t, handler)
			if tc.jsonOutput {
				require.NoError(t, agentGetCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { agentGetCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}
			buf := &bytes.Buffer{}
			agentGetCmd.SetOut(buf)
			agentGetCmd.SetContext(context.Background())

			err := runAgentGet(agentGetCmd, []string{"my-agent"})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
		})
	}
}

func TestAgentList(t *testing.T) {
	payload := map[string]any{
		"deployments": []any{
			map[string]any{
				"id":         "dep-1",
				"name":       "my-agent",
				"build_id":   "abc12345",
				"status":     "active",
				"created_at": "2026-01-01T10:00:00Z",
			},
			map[string]any{
				"id":         "dep-2",
				"name":       "other-agent",
				"build_id":   "def67890",
				"status":     "stopped",
				"created_at": "2026-02-01T08:00:00Z",
			},
		},
		"count": 2,
	}

	cases := []struct {
		name       string
		statusCode int
		body       any
		jsonOutput bool
		wantErr    bool
		wantOut    string
	}{
		{
			name:       "shows agent name",
			statusCode: http.StatusOK,
			body:       payload,
			wantOut:    "my-agent",
		},
		{
			name:       "shows build id",
			statusCode: http.StatusOK,
			body:       payload,
			wantOut:    "abc12345",
		},
		{
			name:       "shows status",
			statusCode: http.StatusOK,
			body:       payload,
			wantOut:    "active",
		},
		{
			name:       "shows deployed date",
			statusCode: http.StatusOK,
			body:       payload,
			wantOut:    "2026-01-01",
		},
		{
			name:       "empty account",
			statusCode: http.StatusOK,
			body:       map[string]any{"deployments": []any{}, "count": 0},
			wantOut:    "No deployments",
		},
		{
			name:       "json output",
			statusCode: http.StatusOK,
			body:       payload,
			jsonOutput: true,
			wantOut:    `"count"`,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			body:       map[string]any{"error": "internal error"},
			wantErr:    true,
		},
		{
			name:       "matches on display_name",
			statusCode: http.StatusOK,
			body: map[string]any{
				"deployments": []any{
					map[string]any{
						"id":           "dep-3",
						"name":         "weather-poet",
						"display_name": "my-weather-agent",
						"build_id":     "ghi11111",
						"status":       "active",
						"created_at":   "2026-03-01T12:00:00Z",
					},
				},
				"count": 1,
			},
			wantOut: "my-weather-agent",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupAgentTest(t, jsonHandler(tc.statusCode, tc.body))
			if tc.jsonOutput {
				require.NoError(t, agentListCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { agentListCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}
			buf := &bytes.Buffer{}
			agentListCmd.SetOut(buf)
			agentListCmd.SetContext(context.Background())

			err := runAgentList(agentListCmd, nil)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
		})
	}
}

func TestAgentPauseResume(t *testing.T) {
	cases := []struct {
		name         string
		action       string // "pause" or "resume"
		useID        bool   // pass --id flag, skip lookup
		emptyList    bool   // simulate agent not found in list
		actionStatus int
		actionBody   any
		wantErr      bool
		wantOut      string
	}{
		{name: "pause success", action: "pause", actionStatus: http.StatusOK, wantOut: "paused"},
		{name: "pause with --id skips lookup", action: "pause", useID: true, actionStatus: http.StatusOK, wantOut: "paused"},
		{name: "pause agent not found", action: "pause", emptyList: true, actionStatus: http.StatusOK, wantErr: true},
		{name: "pause not found", action: "pause", actionStatus: http.StatusNotFound, actionBody: map[string]any{"error": "not found"}, wantErr: true},
		{name: "resume success", action: "resume", actionStatus: http.StatusOK, wantOut: "resumed"},
		{name: "resume with --id skips lookup", action: "resume", useID: true, actionStatus: http.StatusOK, wantOut: "resumed"},
		{name: "resume not found", action: "resume", actionStatus: http.StatusNotFound, actionBody: map[string]any{"error": "not found"}, wantErr: true},
	}

	listPayload := map[string]any{
		"deployments": []any{
			map[string]any{"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent", "build_id": "abc12345", "status": "scaled_down", "created_at": "2026-01-01T10:00:00Z"},
		},
		"count": 1,
	}
	emptyPayload := map[string]any{"deployments": []any{}, "count": 0}
	detailPayload := map[string]any{
		"deployment": map[string]any{
			"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent",
			"build_id": "abc12345", "status": "scaled_down", "created_at": "2026-01-01T10:00:00Z",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookupCalled := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if strings.Contains(r.URL.Path, "/deployments/dep-abc-123") {
						jsonHandler(http.StatusOK, detailPayload)(w, r)
						return
					}
					lookupCalled = true
					if tc.emptyList {
						jsonHandler(http.StatusOK, emptyPayload)(w, r)
					} else {
						jsonHandler(http.StatusOK, listPayload)(w, r)
					}
				} else {
					jsonHandler(tc.actionStatus, tc.actionBody)(w, r)
				}
			})
			setupAgentTest(t, handler)

			var (
				buf bytes.Buffer
				err error
			)
			switch tc.action {
			case "pause":
				if tc.useID {
					require.NoError(t, agentPauseCmd.Flags().Set("id", "dep-abc-123"))
					t.Cleanup(func() { agentPauseCmd.Flags().Set("id", "") }) //nolint:errcheck
				}
				agentPauseCmd.SetOut(&buf)
				agentPauseCmd.SetContext(context.Background())
				err = runAgentPause(agentPauseCmd, []string{"my-agent"})
			case "resume":
				if tc.useID {
					require.NoError(t, agentResumeCmd.Flags().Set("id", "dep-abc-123"))
					t.Cleanup(func() { agentResumeCmd.Flags().Set("id", "") }) //nolint:errcheck
				}
				agentResumeCmd.SetOut(&buf)
				agentResumeCmd.SetContext(context.Background())
				err = runAgentResume(agentResumeCmd, []string{"my-agent"})
			}

			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
			if tc.useID {
				assert.False(t, lookupCalled, "lookup should be skipped when --id is provided")
			}
		})
	}
}

func TestAgentDelete(t *testing.T) {
	listPayload := map[string]any{
		"deployments": []any{
			map[string]any{"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent", "build_id": "abc12345", "status": "active", "created_at": "2026-01-01T10:00:00Z"},
		},
		"count": 1,
	}

	cases := []struct {
		name         string
		confirm      string
		listStatus   int
		listBody     any
		deleteStatus int
		deleteBody   any
		wantErr      bool
		wantOut      string
	}{
		{name: "success", confirm: "my-agent", listStatus: http.StatusOK, listBody: listPayload, deleteStatus: http.StatusOK, wantOut: "deleted"},
		{name: "agent not found", confirm: "my-agent", listStatus: http.StatusOK, listBody: map[string]any{"deployments": []any{}, "count": 0}, deleteStatus: http.StatusOK, wantErr: true},
		{name: "delete returns not found", confirm: "my-agent", listStatus: http.StatusOK, listBody: listPayload, deleteStatus: http.StatusNotFound, wantErr: true},
		{name: "wrong confirm value errors immediately", confirm: "wrong-name", listStatus: http.StatusOK, listBody: listPayload, deleteStatus: http.StatusOK, wantOut: "Confirmation does not match"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					jsonHandler(tc.listStatus, tc.listBody)(w, r)
				} else {
					jsonHandler(tc.deleteStatus, tc.deleteBody)(w, r)
				}
			})
			setupAgentTest(t, handler)
			require.NoError(t, agentDeleteCmd.Flags().Set("confirm", tc.confirm))
			t.Cleanup(func() { agentDeleteCmd.Flags().Set("confirm", "") }) //nolint:errcheck

			buf := &bytes.Buffer{}
			agentDeleteCmd.SetOut(buf)
			agentDeleteCmd.SetContext(context.Background())

			err := runAgentDelete(agentDeleteCmd, []string{"my-agent"})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
		})
	}
}

func TestAgentHistory(t *testing.T) {
	listPayload := map[string]any{
		"deployments": []any{
			map[string]any{"id": "dep-1", "name": "my-agent", "display_name": "my-agent", "build_id": "abc12345", "status": "active", "created_at": "2026-01-01T10:00:00Z"},
		},
		"count": 1,
	}
	payload := map[string]any{
		"deployments": []any{
			map[string]any{
				"id":          "dep-3",
				"agent_name":  "my-agent",
				"revision":    3,
				"build_id":    "abc12345",
				"status":      "active",
				"deployed_at": "2026-03-01T12:00:00Z",
			},
			map[string]any{
				"id":          "dep-2",
				"agent_name":  "my-agent",
				"revision":    2,
				"build_id":    "def67890",
				"status":      "stopped",
				"deployed_at": "2026-02-01T08:00:00Z",
			},
		},
		"count": 2,
	}

	cases := []struct {
		name       string
		statusCode int
		body       any
		jsonOutput bool
		wantErr    bool
		wantOut    string
	}{
		{name: "shows build id", statusCode: http.StatusOK, body: payload, wantOut: "abc12345"},
		{name: "shows revision", statusCode: http.StatusOK, body: payload, wantOut: "3"},
		{name: "shows status", statusCode: http.StatusOK, body: payload, wantOut: "active"},
		{name: "shows deployed date", statusCode: http.StatusOK, body: payload, wantOut: "2026-03-01"},
		{name: "empty history", statusCode: http.StatusOK, body: map[string]any{"deployments": []any{}, "count": 0}, wantOut: "No deployment history"},
		{name: "json output", statusCode: http.StatusOK, body: payload, jsonOutput: true, wantOut: `"revision"`},
		{name: "server error", statusCode: http.StatusInternalServerError, body: map[string]any{"error": "internal error"}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/v1/deployments") && r.URL.Query().Get("account") != "" {
					jsonHandler(http.StatusOK, listPayload)(w, r)
					return
				}
				jsonHandler(tc.statusCode, tc.body)(w, r)
			})
			setupAgentTest(t, handler)
			if tc.jsonOutput {
				require.NoError(t, agentHistoryCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { agentHistoryCmd.Flags().Set("json", "false") }) //nolint:errcheck
			}
			buf := &bytes.Buffer{}
			agentHistoryCmd.SetOut(buf)
			agentHistoryCmd.SetContext(context.Background())

			err := runAgentHistory(agentHistoryCmd, []string{"my-agent"})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
		})
	}
}

func TestAgentRestart(t *testing.T) {
	listPayload := map[string]any{
		"deployments": []any{
			map[string]any{"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent", "build_id": "abc12345", "status": "active", "created_at": "2026-01-01T10:00:00Z"},
		},
		"count": 1,
	}
	detailPayload := map[string]any{
		"deployment": map[string]any{
			"id": "dep-abc-123",
			"workloads": []any{
				map[string]any{
					"name":      "my-agent-agent",
					"component": "agent",
					"pod_name":  "my-agent-agent-abc-xyz",
					"containers": []any{
						map[string]any{"name": "app"},
						map[string]any{"name": "messaging"},
					},
				},
			},
		},
	}

	cases := []struct {
		name         string
		component    string
		useID        bool
		actionStatus int
		wantErr      bool
		wantOut      string
	}{
		{name: "restart agent component", component: "agent", actionStatus: http.StatusOK, wantOut: "restarted"},
		{name: "restart with --id skips lookup", component: "agent", useID: true, actionStatus: http.StatusOK, wantOut: "restarted"},
		{name: "unknown component returns error", component: "collector", actionStatus: http.StatusOK, wantErr: true},
		{name: "not found", component: "agent", actionStatus: http.StatusNotFound, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookupCalled := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if strings.Contains(r.URL.Path, "/deployments/dep-abc-123") {
						jsonHandler(http.StatusOK, detailPayload)(w, r)
					} else {
						lookupCalled = true
						jsonHandler(http.StatusOK, listPayload)(w, r)
					}
				} else {
					jsonHandler(tc.actionStatus, nil)(w, r)
				}
			})
			setupAgentTest(t, handler)

			require.NoError(t, agentRestartCmd.Flags().Set("component", tc.component))
			t.Cleanup(func() { agentRestartCmd.Flags().Set("component", "") }) //nolint:errcheck
			if tc.useID {
				require.NoError(t, agentRestartCmd.Flags().Set("id", "dep-abc-123"))
				t.Cleanup(func() { agentRestartCmd.Flags().Set("id", "") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			agentRestartCmd.SetOut(buf)
			agentRestartCmd.SetContext(context.Background())

			err := runAgentRestart(agentRestartCmd, []string{"my-agent"})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
			if tc.useID {
				assert.False(t, lookupCalled, "lookup should be skipped when --id is provided")
			}
		})
	}
}

func TestAgentLogsWorkload(t *testing.T) {
	// Verifies that the workload in the logs URL is derived from the deployment
	// detail workload name, not the user's input (which may be a display name).
	listPayload := map[string]any{
		"deployments": []any{
			map[string]any{
				"id": "dep-abc-123", "name": "weather-poet", "display_name": "ABC !#@#",
				"build_id": "abc12345", "status": "active", "created_at": "2026-01-01T10:00:00Z",
			},
		},
		"count": 1,
	}
	detailPayload := map[string]any{
		"deployment": map[string]any{
			"id": "dep-abc-123",
			"workloads": []any{
				map[string]any{"name": "weather-poet-agent", "component": "agent", "pod_name": "weather-poet-agent-abc-xyz"},
			},
		},
	}
	logsPayload := `[{"timestamp":"2026-01-01T10:00:00Z","level":"","message":"started"}]`

	var capturedWorkload string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/logs") {
			capturedWorkload = r.URL.Query().Get("workload")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, logsPayload)
		} else if strings.Contains(r.URL.Path, "/deployments/") {
			jsonHandler(http.StatusOK, detailPayload)(w, r)
		} else {
			jsonHandler(http.StatusOK, listPayload)(w, r)
		}
	})
	setupAgentTest(t, handler)
	require.NoError(t, agentLogsCmd.Flags().Set("container", "app"))
	t.Cleanup(func() { agentLogsCmd.Flags().Set("container", "") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	agentLogsCmd.SetOut(buf)
	agentLogsCmd.SetContext(context.Background())

	require.NoError(t, runAgentLogs(agentLogsCmd, []string{"ABC !#@#"}))
	assert.Equal(t, "weather-poet-agent", capturedWorkload)
}

func TestAgentLogs(t *testing.T) {
	listPayload := map[string]any{
		"deployments": []any{
			map[string]any{"id": "dep-abc-123", "name": "my-agent", "display_name": "my-agent", "build_id": "abc12345", "status": "active", "created_at": "2026-01-01T10:00:00Z"},
		},
		"count": 1,
	}
	detailPayload := map[string]any{
		"deployment": map[string]any{
			"id": "dep-abc-123",
			"workloads": []any{
				map[string]any{"name": "my-agent-agent", "component": "agent", "pod_name": "my-agent-agent-abc-xyz"},
				map[string]any{"name": "my-agent-knowledge-chat-sandbox", "component": "knowledge", "pod_name": "my-agent-knowledge-chat-sandbox-0"},
				map[string]any{"name": "my-agent-knowledge-vectors", "component": "knowledge", "pod_name": "my-agent-knowledge-vectors-0"},
			},
		},
	}
	logsPayload := `[{"timestamp":"2026-01-01T10:00:00Z","level":"","message":"agent started"}]`
	ssePayload := "data: {\"timestamp\":\"2026-01-01T10:00:00Z\",\"level\":\"\",\"message\":\"agent started\"}\n\n"

	cases := []struct {
		name         string
		container    string
		workload     string
		tail         bool
		useID        bool
		listStatus   int
		logsStatus   int
		logsBody     string
		wantErr      bool
		wantOut      string
		wantWorkload string
	}{
		{name: "app container", container: "app", listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: logsPayload, wantOut: "agent started", wantWorkload: "my-agent-agent"},
		{name: "messaging container", container: "messaging", listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: logsPayload, wantOut: "agent started", wantWorkload: "my-agent-agent"},
		{name: "non-default container forwarded to server", container: "collector", listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: logsPayload, wantOut: "agent started", wantWorkload: "my-agent-agent"},
		{name: "with --id skips lookup", container: "app", useID: true, listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: logsPayload, wantOut: "agent started", wantWorkload: "my-agent-agent"},
		{name: "--tail streams SSE", container: "app", tail: true, listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: ssePayload, wantOut: "agent started", wantWorkload: "my-agent-agent"},
		{name: "empty logs", container: "app", listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: `[]`, wantOut: "No logs found", wantWorkload: "my-agent-agent"},
		{name: "deployment not found", container: "app", listStatus: http.StatusOK, logsStatus: http.StatusNotFound, logsBody: `{}`, wantErr: true},
		{name: "--workload by knowledge entry name", container: "app", workload: "chat-sandbox", listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: logsPayload, wantOut: "agent started", wantWorkload: "my-agent-knowledge-chat-sandbox"},
		{name: "--workload by full workload name", container: "app", workload: "my-agent-knowledge-vectors", listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: logsPayload, wantOut: "agent started", wantWorkload: "my-agent-knowledge-vectors"},
		{name: "--workload ambiguous component errors", container: "app", workload: "knowledge", listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: logsPayload, wantErr: true},
		{name: "--workload no match errors", container: "app", workload: "ghost", listStatus: http.StatusOK, logsStatus: http.StatusOK, logsBody: logsPayload, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookupCalled := false
			var capturedWorkload string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/logs/stream") {
					capturedWorkload = r.URL.Query().Get("workload")
					if tc.tail {
						assert.NotEmpty(t, r.URL.Query().Get("since"), "tail stream must pass since to skip backfill")
					}
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(tc.logsStatus)
					fmt.Fprint(w, tc.logsBody)
				} else if strings.Contains(r.URL.Path, "/logs") {
					capturedWorkload = r.URL.Query().Get("workload")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.logsStatus)
					fmt.Fprint(w, tc.logsBody)
				} else if strings.Contains(r.URL.Path, "/deployments/") {
					jsonHandler(http.StatusOK, detailPayload)(w, r)
				} else {
					lookupCalled = true
					jsonHandler(tc.listStatus, listPayload)(w, r)
				}
			})
			setupAgentTest(t, handler)

			require.NoError(t, agentLogsCmd.Flags().Set("container", tc.container))
			t.Cleanup(func() { agentLogsCmd.Flags().Set("container", "") }) //nolint:errcheck
			if tc.workload != "" {
				require.NoError(t, agentLogsCmd.Flags().Set("workload", tc.workload))
				t.Cleanup(func() { agentLogsCmd.Flags().Set("workload", "") }) //nolint:errcheck
			}
			if tc.useID {
				require.NoError(t, agentLogsCmd.Flags().Set("id", "dep-abc-123"))
				t.Cleanup(func() { agentLogsCmd.Flags().Set("id", "") }) //nolint:errcheck
			}
			if tc.tail {
				require.NoError(t, agentLogsCmd.Flags().Set("tail", "true"))
				t.Cleanup(func() { agentLogsCmd.Flags().Set("tail", "false") }) //nolint:errcheck
			}

			buf := &bytes.Buffer{}
			agentLogsCmd.SetOut(buf)

			ctx := context.Background()
			if tc.tail {
				// cancel the context after a brief delay so the reconnect loop exits
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 200*time.Millisecond)
				defer cancel()
			}
			agentLogsCmd.SetContext(ctx)

			err := runAgentLogs(agentLogsCmd, []string{"my-agent"})
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
				if tc.wantWorkload != "" {
					assert.Equal(t, tc.wantWorkload, capturedWorkload, "server should receive resolved workload name")
				}
			}
			if tc.useID {
				assert.False(t, lookupCalled, "lookup should be skipped when --id is provided")
			}
		})
	}
}
