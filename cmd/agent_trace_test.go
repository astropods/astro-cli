package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setAgentTraceID(t *testing.T, traceID string) {
	t.Helper()
	require.NoError(t, agentTraceCmd.Flags().Set("trace-id", traceID))
	t.Cleanup(func() { _ = agentTraceCmd.Flags().Set("trace-id", "") })
}

func TestAgentTraceList(t *testing.T) {
	dep := map[string]any{
		"id":           "dep-abc-123",
		"name":         "coach",
		"display_name": "coach",
		"build_id":     "abc12345",
		"namespace":    "astro-testaccount",
		"status":       "active",
		"created_at":   "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}

	tracesPayload := map[string]any{
		"traces": []any{
			map[string]any{
				"trace_id":   "2dc10d31ac3ebec7dd9d27263dd7531d",
				"name":       "coach.chat",
				"status":     "ok",
				"latency_ms": 12450.0,
				"total_cost": 0.0234,
				"input":      "hi",
				"output":     "hello",
				"timestamp":  "2026-05-28T11:49:35Z",
			},
			map[string]any{
				"trace_id":   "9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c",
				"name":       "coach.chat",
				"status":     "ok",
				"latency_ms": 8200.0,
				"total_cost": 0.0,
				"input":      "what is yoda",
				"output":     "Yoda is …",
				"timestamp":  "2026-05-28T11:30:10Z",
			},
		},
		"total":  2,
		"limit":  50,
		"offset": 0,
	}

	cases := []struct {
		name       string
		body       any
		statusCode int
		jsonOutput bool
		wantErr    bool
		wantOut    string
	}{
		{name: "shows trace id", body: tracesPayload, statusCode: http.StatusOK, wantOut: "2dc10d31ac3ebec7dd9d27263dd7531d"},
		{name: "shows latency", body: tracesPayload, statusCode: http.StatusOK, wantOut: "12450ms"},
		{name: "shows cost when nonzero", body: tracesPayload, statusCode: http.StatusOK, wantOut: "$0.0234"},
		{name: "shows timestamp", body: tracesPayload, statusCode: http.StatusOK, wantOut: "2026-05-28"},
		{name: "object input output", body: map[string]any{
			"traces": []any{
				map[string]any{
					"trace_id":   "abc123",
					"name":       "agent.run",
					"status":     "ok",
					"latency_ms": 100.0,
					"input":      map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}},
					"output":     map[string]any{"text": "hello"},
					"timestamp":  "2026-06-01T12:00:00Z",
				},
			},
			"total": 1, "limit": 50, "offset": 0,
		}, statusCode: http.StatusOK, wantOut: "abc123"},
		{name: "empty", body: map[string]any{"traces": []any{}, "total": 0, "limit": 50, "offset": 0}, statusCode: http.StatusOK, wantOut: msgNoTracesForAgent("coach")},
		{name: "json output", body: tracesPayload, statusCode: http.StatusOK, jsonOutput: true, wantOut: `"traces"`},
		{name: "server error", body: map[string]any{"error": "boom"}, statusCode: http.StatusInternalServerError, wantErr: true},
		{name: "pagination hint", body: map[string]any{
			"traces": tracesPayload["traces"],
			"total":  100,
			"limit":  50,
			"offset": 0,
		}, statusCode: http.StatusOK, wantOut: "Showing 1–2 of 100. Page with --offset 50."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/observability/traces"):
					jsonHandler(tc.statusCode, tc.body)(w, r)
				default:
					jsonHandler(http.StatusOK, listPayload)(w, r)
				}
			})
			setupAgentTest(t, handler)
			setAgentTargetName(t, agentTraceCmd, "coach")
			if tc.jsonOutput {
				require.NoError(t, agentTraceCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { _ = agentTraceCmd.Flags().Set("json", "false") })
			}
			buf := &bytes.Buffer{}
			agentTraceCmd.SetOut(buf)
			agentTraceCmd.SetContext(context.Background())

			err := runAgentTrace(agentTraceCmd, nil)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
		})
	}
}

func TestAgentTraceDetail(t *testing.T) {
	dep := map[string]any{
		"id":           "dep-abc-123",
		"name":         "coach",
		"display_name": "coach",
		"build_id":     "abc12345",
		"namespace":    "astro-testaccount",
		"status":       "active",
		"created_at":   "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}

	detailPayload := map[string]any{
		"trace": map[string]any{
			"trace_id":   "2dc10d31ac3ebec7dd9d27263dd7531d",
			"name":       "coach.chat",
			"timestamp":  "2026-05-28T11:49:35Z",
			"latency_ms": 12450.0,
			"total_cost": 0.0234,
			"input":      "hi",
			"output":     "hello world",
			"session_id": "C0B4RBREB47",
			"user_id":    "U12345",
			"tags":       []any{"deployment:dep-abc-123", "slack"},
			"metadata":   map[string]any{"channel": "C0B4RBREB47"},
		},
		"observations": []any{
			map[string]any{
				"id":         "obs-1",
				"type":       "GENERATION",
				"name":       "anthropic.messages",
				"start_time": "2026-05-28T11:49:35Z",
				"latency_ms": 11000.0,
				"model":      "claude-sonnet-4-6",
				"input": map[string]any{
					"messages": []any{
						map[string]any{"role": "user", "content": "hi"},
					},
				},
				"output": map[string]any{"content": "hello world"},
			},
			map[string]any{
				"id":             "obs-2",
				"type":           "SPAN",
				"name":           "slack.post_card",
				"start_time":     "2026-05-28T11:49:46Z",
				"latency_ms":     250.0,
				"level":          "ERROR",
				"status_message": "invalid_blocks",
			},
		},
		"scores": []any{
			map[string]any{"name": "groundedness", "value": 0.92},
		},
	}

	cases := []struct {
		name       string
		body       any
		statusCode int
		jsonOutput bool
		wantErr    error
		wantOut    string
	}{
		{name: "shows trace id header", body: detailPayload, statusCode: http.StatusOK, wantOut: "2dc10d31ac3ebec7dd9d27263dd7531d"},
		{name: "shows latency", body: detailPayload, statusCode: http.StatusOK, wantOut: "12450ms"},
		{name: "shows session", body: detailPayload, statusCode: http.StatusOK, wantOut: "C0B4RBREB47"},
		{name: "shows tags", body: detailPayload, statusCode: http.StatusOK, wantOut: "slack"},
		{name: "shows metadata", body: detailPayload, statusCode: http.StatusOK, wantOut: "channel"},
		{name: "shows observation input", body: detailPayload, statusCode: http.StatusOK, wantOut: `"role": "user"`},
		{name: "shows observation output", body: detailPayload, statusCode: http.StatusOK, wantOut: `"content": "hello world"`},
		{name: "shows observation model", body: detailPayload, statusCode: http.StatusOK, wantOut: "claude-sonnet-4-6"},
		{name: "shows error observation message", body: detailPayload, statusCode: http.StatusOK, wantOut: "invalid_blocks"},
		{name: "shows score", body: detailPayload, statusCode: http.StatusOK, wantOut: "groundedness"},
		{name: "json output", body: detailPayload, statusCode: http.StatusOK, jsonOutput: true, wantOut: `"observations"`},
		{name: "not found", body: map[string]any{"error": "trace not found"}, statusCode: http.StatusNotFound, wantErr: errAgentTraceNotFound("2dc10d31ac3ebec7dd9d27263dd7531d", "coach")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/observability/traces/"):
					jsonHandler(tc.statusCode, tc.body)(w, r)
				default:
					jsonHandler(http.StatusOK, listPayload)(w, r)
				}
			})
			setupAgentTest(t, handler)
			setAgentTargetName(t, agentTraceCmd, "coach")
			setAgentTraceID(t, "2dc10d31ac3ebec7dd9d27263dd7531d")
			if tc.jsonOutput {
				require.NoError(t, agentTraceCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { _ = agentTraceCmd.Flags().Set("json", "false") })
			}
			buf := &bytes.Buffer{}
			agentTraceCmd.SetOut(buf)
			agentTraceCmd.SetContext(context.Background())

			err := runAgentTrace(agentTraceCmd, nil)
			if tc.wantErr != nil {
				require.EqualError(t, err, tc.wantErr.Error())
			} else {
				require.NoError(t, err)
				assert.Contains(t, buf.String(), tc.wantOut)
			}
		})
	}
}

func TestAgentTraceListNotFound(t *testing.T) {
	dep := map[string]any{
		"id": "dep-abc-123", "name": "coach", "display_name": "coach",
		"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}
	fullPayload := map[string]any{
		"deployment": map[string]any{
			"id": "dep-direct-999", "name": "coach", "display_name": "coach",
			"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
		},
	}

	cases := []struct {
		name    string
		useID   bool
		wantErr error
	}{
		{name: "by name", wantErr: errAgentDeploymentNotFound("coach")},
		{name: "by id", useID: true, wantErr: errAgentDeploymentNotFoundForID("dep-direct-999")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/observability/traces"):
					jsonHandler(http.StatusNotFound, map[string]any{"error": "deployment not found"})(w, r)
				case strings.Contains(r.URL.Path, "/deployments/dep-direct-999"):
					jsonHandler(http.StatusOK, fullPayload)(w, r)
				default:
					jsonHandler(http.StatusOK, listPayload)(w, r)
				}
			})
			setupAgentTest(t, handler)
			if tc.useID {
				setAgentTargetID(t, agentTraceCmd, "dep-direct-999")
			} else {
				setAgentTargetName(t, agentTraceCmd, "coach")
			}

			agentTraceCmd.SetContext(context.Background())
			err := runAgentTrace(agentTraceCmd, nil)
			require.EqualError(t, err, tc.wantErr.Error())
		})
	}
}

func TestAgentTraceListFlagValidationWithTraceID(t *testing.T) {
	setAgentTargetName(t, agentTraceCmd, "coach")
	setAgentTraceID(t, "trace-abc")
	require.NoError(t, agentTraceCmd.Flags().Set("limit", "-1"))
	t.Cleanup(func() { _ = agentTraceCmd.Flags().Set("limit", "50") })

	agentTraceCmd.SetContext(context.Background())
	err := runAgentTrace(agentTraceCmd, nil)
	require.EqualError(t, err, errPositiveIntFlag("limit").Error())
}

func TestAgentTraceUsesDeploymentIDFlag(t *testing.T) {
	var listHit bool
	var tracesPath string
	fullPayload := map[string]any{
		"deployment": map[string]any{
			"id": "dep-direct-999", "name": "coach", "display_name": "coach",
			"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
		},
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/observability/traces"):
			tracesPath = r.URL.Path
			jsonHandler(http.StatusOK, map[string]any{
				"traces": []any{}, "total": 0, "limit": 50, "offset": 0,
			})(w, r)
		case strings.Contains(r.URL.Path, "/deployments/dep-direct-999"):
			jsonHandler(http.StatusOK, fullPayload)(w, r)
		default:
			listHit = true
			jsonHandler(http.StatusOK, map[string]any{"deployments": []any{}, "count": 0})(w, r)
		}
	})
	setupAgentTest(t, handler)

	setAgentTargetID(t, agentTraceCmd, "dep-direct-999")

	buf := &bytes.Buffer{}
	agentTraceCmd.SetOut(buf)
	agentTraceCmd.SetContext(context.Background())

	err := runAgentTrace(agentTraceCmd, nil)
	require.NoError(t, err)
	assert.False(t, listHit, "should skip deployment name lookup when --id is provided")
	assert.Contains(t, tracesPath, "dep-direct-999")
}

func TestAgentTracePaginationValidation(t *testing.T) {
	cases := []struct {
		name   string
		limit  int
		offset int
		want   error
	}{
		{name: "zero limit", limit: 0, offset: 0, want: errPositiveIntFlag("limit")},
		{name: "negative limit", limit: -1, offset: 0, want: errPositiveIntFlag("limit")},
		{name: "negative offset", limit: 50, offset: -5, want: errNonNegativeIntFlag("offset")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, agentTraceCmd.Flags().Set("limit", fmt.Sprintf("%d", tc.limit)))
			require.NoError(t, agentTraceCmd.Flags().Set("offset", fmt.Sprintf("%d", tc.offset)))
			t.Cleanup(func() {
				_ = agentTraceCmd.Flags().Set("limit", "50")
				_ = agentTraceCmd.Flags().Set("offset", "0")
			})

			setAgentTargetName(t, agentTraceCmd, "coach")
			agentTraceCmd.SetContext(context.Background())
			err := runAgentTrace(agentTraceCmd, nil)
			require.EqualError(t, err, tc.want.Error())
		})
	}
}

func TestAgentTraceTimeWindowValidation(t *testing.T) {
	cases := []struct {
		name  string
		start string
		end   string
		want  error
	}{
		{name: "invalid start", start: "yesterday", want: errRFC3339TimeFlag("start", "yesterday")},
		{name: "invalid end", end: "tomorrow", want: errRFC3339TimeFlag("end", "tomorrow")},
		{name: "start after end", start: "2026-05-30T00:00:00Z", end: "2026-05-01T00:00:00Z", want: errTraceStartAfterEnd()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, agentTraceCmd.Flags().Set("start", tc.start))
			require.NoError(t, agentTraceCmd.Flags().Set("end", tc.end))
			t.Cleanup(func() {
				_ = agentTraceCmd.Flags().Set("start", "")
				_ = agentTraceCmd.Flags().Set("end", "")
			})

			setAgentTargetName(t, agentTraceCmd, "coach")
			agentTraceCmd.SetContext(context.Background())
			err := runAgentTrace(agentTraceCmd, nil)
			require.EqualError(t, err, tc.want.Error())
		})
	}
}

func TestAgentTraceTimeWindowValidationWithTraceID(t *testing.T) {
	setAgentTargetName(t, agentTraceCmd, "coach")
	setAgentTraceID(t, "trace-abc")
	require.NoError(t, agentTraceCmd.Flags().Set("start", "not-a-date"))
	t.Cleanup(func() { _ = agentTraceCmd.Flags().Set("start", "") })

	agentTraceCmd.SetContext(context.Background())
	err := runAgentTrace(agentTraceCmd, nil)
	require.EqualError(t, err, errRFC3339TimeFlag("start", "not-a-date").Error())
}

func TestAgentTraceListQueryParams(t *testing.T) {
	dep := map[string]any{
		"id": "dep-abc-123", "name": "coach", "display_name": "coach",
		"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}
	tracesPayload := map[string]any{"traces": []any{}, "total": 0, "limit": 10, "offset": 20}

	var rawQuery string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/observability/traces"):
			rawQuery = r.URL.RawQuery
			jsonHandler(http.StatusOK, tracesPayload)(w, r)
		default:
			jsonHandler(http.StatusOK, listPayload)(w, r)
		}
	})
	setupAgentTest(t, handler)
	setAgentTargetName(t, agentTraceCmd, "coach")
	require.NoError(t, agentTraceCmd.Flags().Set("limit", "10"))
	require.NoError(t, agentTraceCmd.Flags().Set("offset", "20"))
	require.NoError(t, agentTraceCmd.Flags().Set("start", "2026-05-28T00:00:00Z"))
	require.NoError(t, agentTraceCmd.Flags().Set("end", "2026-05-28T23:59:59Z"))
	t.Cleanup(func() {
		_ = agentTraceCmd.Flags().Set("limit", "50")
		_ = agentTraceCmd.Flags().Set("offset", "0")
		_ = agentTraceCmd.Flags().Set("start", "")
		_ = agentTraceCmd.Flags().Set("end", "")
	})

	buf := &bytes.Buffer{}
	agentTraceCmd.SetOut(buf)
	agentTraceCmd.SetContext(context.Background())

	require.NoError(t, runAgentTrace(agentTraceCmd, nil))
	assert.Contains(t, rawQuery, "limit=10")
	assert.Contains(t, rawQuery, "offset=20")
	assert.Contains(t, rawQuery, "start_time=2026-05-28T00%3A00%3A00Z")
	assert.Contains(t, rawQuery, "end_time=2026-05-28T23%3A59%3A59Z")
}

func TestAgentTraceRejectsPositionalArgs(t *testing.T) {
	require.EqualError(t, agentTargetArgs(agentTraceCmd, []string{"coach"}), errAgentUnexpectedArgument("coach").Error())
}

func TestAgentTraceSummary(t *testing.T) {
	dep := map[string]any{
		"id": "dep-abc-123", "name": "coach", "display_name": "coach",
		"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}
	summariesPayload := map[string]any{
		"summaries": map[string]any{
			"dep-abc-123": map[string]any{
				"total_traces":   42,
				"last_trace_at":  "2026-06-01T10:00:00Z",
				"request_series": []any{0, 1, 2},
				"token_series":   []any{10, 20, 30},
			},
		},
	}

	cases := []struct {
		name       string
		body       any
		jsonOutput bool
		wantOut    string
	}{
		{name: "shows totals", body: summariesPayload, wantOut: "Total traces:  42"},
		{name: "shows sparkline and stats", body: summariesPayload, wantOut: "3 total"},
		{name: "json output", body: summariesPayload, jsonOutput: true, wantOut: "dep-abc-123"},
		{name: "json missing entry", body: map[string]any{"summaries": map[string]any{}}, jsonOutput: true, wantOut: `"summary": null`},
		{name: "missing entry", body: map[string]any{"summaries": map[string]any{}}, wantOut: msgNoObsSummaryForAgent("coach")},
		{name: "stats aligned under series", body: summariesPayload, wantOut: "Requests  (30d):"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/agents/usage"):
					jsonHandler(http.StatusOK, tc.body)(w, r)
				default:
					jsonHandler(http.StatusOK, listPayload)(w, r)
				}
			})
			setupAgentTest(t, handler)
			setAgentTargetName(t, agentTraceCmd, "coach")
			require.NoError(t, agentTraceCmd.Flags().Set("summary", "true"))
			t.Cleanup(func() { _ = agentTraceCmd.Flags().Set("summary", "false") })
			if tc.jsonOutput {
				require.NoError(t, agentTraceCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { _ = agentTraceCmd.Flags().Set("json", "false") })
			}
			buf := &bytes.Buffer{}
			agentTraceCmd.SetOut(buf)
			agentTraceCmd.SetContext(context.Background())

			require.NoError(t, runAgentTrace(agentTraceCmd, nil))
			assert.Contains(t, buf.String(), tc.wantOut)
		})
	}
}

// The server serves the summary at /agents/usage. The old
// /observability/deployment-summaries path survives only as a deprecated
// alias for already-released binaries, so a regression here would leave this
// CLI on a path scheduled for removal.
func TestAgentTraceSummaryCallsTheAgentUsageEndpoint(t *testing.T) {
	dep := map[string]any{
		"id": "dep-abc-123", "name": "coach", "display_name": "coach",
		"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
	}
	var requested []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/agents/usage"):
			jsonHandler(http.StatusOK, map[string]any{"summaries": map[string]any{
				"dep-abc-123": map[string]any{"total_traces": 3},
			}})(w, r)
		default:
			jsonHandler(http.StatusOK, map[string]any{"deployments": []any{dep}, "count": 1})(w, r)
		}
	})
	setupAgentTest(t, handler)
	setAgentTargetName(t, agentTraceCmd, "coach")
	require.NoError(t, agentTraceCmd.Flags().Set("summary", "true"))
	t.Cleanup(func() { _ = agentTraceCmd.Flags().Set("summary", "false") })

	buf := &bytes.Buffer{}
	agentTraceCmd.SetOut(buf)
	agentTraceCmd.SetContext(context.Background())
	require.NoError(t, runAgentTrace(agentTraceCmd, nil))

	assert.Contains(t, requested, "/api/v1/accounts/testaccount/agents/usage")
	for _, path := range requested {
		assert.NotContains(t, path, "deployment-summaries",
			"the deprecated alias must not be called")
	}
	assert.Contains(t, buf.String(), "Total traces:  3")
}

func TestAgentTraceSummarySkipsListFlagValidation(t *testing.T) {
	dep := map[string]any{
		"id": "dep-abc-123", "name": "coach", "display_name": "coach",
		"build_id": "abc12345", "namespace": "astro-testaccount", "status": "active", "created_at": "2026-05-28T10:00:00Z",
	}
	listPayload := map[string]any{"deployments": []any{dep}, "count": 1}
	summariesPayload := map[string]any{
		"summaries": map[string]any{
			"dep-abc-123": map[string]any{"total_traces": 1, "last_trace_at": "2026-06-01T10:00:00Z"},
		},
	}
	var tracesCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/observability/traces"):
			tracesCalled = true
			jsonHandler(http.StatusOK, map[string]any{"traces": []any{}, "total": 0})(w, r)
		case strings.HasSuffix(r.URL.Path, "/agents/usage"):
			jsonHandler(http.StatusOK, summariesPayload)(w, r)
		default:
			jsonHandler(http.StatusOK, listPayload)(w, r)
		}
	})
	setupAgentTest(t, handler)
	setAgentTargetName(t, agentTraceCmd, "coach")
	require.NoError(t, agentTraceCmd.Flags().Set("summary", "true"))
	require.NoError(t, agentTraceCmd.Flags().Set("limit", "0"))
	t.Cleanup(func() {
		_ = agentTraceCmd.Flags().Set("summary", "false")
		_ = agentTraceCmd.Flags().Set("limit", "50")
	})

	agentTraceCmd.SetContext(context.Background())
	require.NoError(t, runAgentTrace(agentTraceCmd, nil))
	assert.False(t, tracesCalled, "summary mode must not call traces endpoint")
}

func TestFormatObsLastActive(t *testing.T) {
	t.Parallel()
	_, err := parseObsTimestamp(time.Now().Add(-time.Hour).Format(time.RFC3339))
	require.NoError(t, err)
	_, err = parseObsTimestamp(time.Now().Add(-time.Hour).Format(time.RFC3339Nano))
	require.NoError(t, err)

	// The server sends UTC midnight, so the day is the only honest unit. An
	// agent that ran minutes ago must not read as most of a day stale.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	cases := []struct {
		name string
		day  time.Time
		want string
	}{
		{name: "active today", day: today, want: "today (" + today.Format(time.DateOnly) + ")"},
		{
			name: "active yesterday",
			day:  today.AddDate(0, 0, -1),
			want: "yesterday (" + today.AddDate(0, 0, -1).Format(time.DateOnly) + ")",
		},
		{
			name: "active earlier",
			day:  today.AddDate(0, 0, -5),
			want: "5 days ago (" + today.AddDate(0, 0, -5).Format(time.DateOnly) + ")",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, formatObsLastActive(tc.day.Format(time.RFC3339)))
		})
	}

	t.Run("an unparseable value passes through", func(t *testing.T) {
		assert.Equal(t, "not-a-date", formatObsLastActive("not-a-date"))
	})

	// A timestamp with a time of day still resolves to its day rather than
	// leaking an hour count the server cannot actually support.
	t.Run("a mid-day timestamp still reports the day", func(t *testing.T) {
		got := formatObsLastActive(today.Add(9 * time.Hour).Format(time.RFC3339Nano))
		assert.Equal(t, "today ("+today.Format(time.DateOnly)+")", got)
		assert.NotContains(t, got, "hour")
	})
}

func TestAgentTraceSummaryRejectsTraceID(t *testing.T) {
	setAgentTargetName(t, agentTraceCmd, "coach")
	setAgentTraceID(t, "trace-abc")
	require.NoError(t, agentTraceCmd.Flags().Set("summary", "true"))
	t.Cleanup(func() { _ = agentTraceCmd.Flags().Set("summary", "false") })

	agentTraceCmd.SetContext(context.Background())
	err := runAgentTrace(agentTraceCmd, nil)
	require.EqualError(t, err, errAgentTraceSummaryWithTraceID().Error())
}
