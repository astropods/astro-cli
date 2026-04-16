package loki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestBuildSelector(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		pod        string
		deployment string
		container  string
		want       string
	}{
		{
			name:      "namespace only",
			namespace: "astro-abc123-0",
			want:      `{namespace="astro-abc123-0"}`,
		},
		{
			name:      "namespace and pod",
			namespace: "astro-abc123-0",
			pod:       "my-agent-xyz",
			want:      `{namespace="astro-abc123-0", pod="my-agent-xyz"}`,
		},
		{
			name:      "namespace pod and container",
			namespace: "astro-abc123-0",
			pod:       "my-agent-xyz",
			container: "agent",
			want:      `{namespace="astro-abc123-0", pod="my-agent-xyz", container="agent"}`,
		},
		{
			name:      "namespace and container without pod",
			namespace: "astro-abc123-0",
			container: "agent",
			want:      `{namespace="astro-abc123-0", container="agent"}`,
		},
		{
			name:       "deployment regex match",
			namespace:  "astro-abc123-0",
			deployment: "sasbot-collector",
			container:  "collector",
			want:       `{namespace="astro-abc123-0", pod=~"sasbot-collector-.+", container="collector"}`,
		},
		{
			name:       "pod takes precedence over deployment",
			namespace:  "astro-abc123-0",
			pod:        "my-agent-xyz",
			deployment: "my-agent",
			want:       `{namespace="astro-abc123-0", pod="my-agent-xyz"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSelector(tt.namespace, tt.pod, tt.deployment, tt.container)
			if got != tt.want {
				t.Errorf("buildSelector() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQueryLogs_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if q := r.URL.Query().Get("query"); q != `{namespace="astro-abc-0"}` {
			t.Errorf("query = %q, want {namespace=\"astro-abc-0\"}", q)
		}
		if r.URL.Query().Get("limit") != "200" {
			t.Errorf("limit = %q, want 200", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("direction") != "forward" {
			t.Errorf("direction = %q, want forward", r.URL.Query().Get("direction"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [{
					"stream": {"namespace": "astro-abc-0", "pod": "my-pod", "container": "agent"},
					"values": [
						["1000000000", "first log line"],
						["2000000000", "second log line"]
					]
				}]
			}
		}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	lines, err := c.QueryLogs(context.Background(), QueryParams{
		Namespace: "astro-abc-0",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].Line != "first log line" {
		t.Errorf("lines[0].Line = %q, want %q", lines[0].Line, "first log line")
	}
	if lines[1].Line != "second log line" {
		t.Errorf("lines[1].Line = %q, want %q", lines[1].Line, "second log line")
	}
	if lines[0].Pod != "my-pod" {
		t.Errorf("lines[0].Pod = %q, want %q", lines[0].Pod, "my-pod")
	}
	if lines[0].Container != "agent" {
		t.Errorf("lines[0].Container = %q, want %q", lines[0].Container, "agent")
	}
	if lines[0].Timestamp != time.Unix(0, 1000000000) {
		t.Errorf("lines[0].Timestamp = %v, want %v", lines[0].Timestamp, time.Unix(0, 1000000000))
	}
}

func TestQueryLogs_MultipleStreams_SortedByTimestamp(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Two pods with interleaved timestamps
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [
					{
						"stream": {"pod": "pod-a"},
						"values": [
							["1000000000", "pod-a line 1"],
							["3000000000", "pod-a line 2"]
						]
					},
					{
						"stream": {"pod": "pod-b"},
						"values": [
							["2000000000", "pod-b line 1"],
							["4000000000", "pod-b line 2"]
						]
					}
				]
			}
		}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	lines, err := c.QueryLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4", len(lines))
	}
	want := []string{"pod-a line 1", "pod-b line 1", "pod-a line 2", "pod-b line 2"}
	for i, l := range lines {
		if l.Line != want[i] {
			t.Errorf("lines[%d].Line = %q, want %q", i, l.Line, want[i])
		}
	}
}

func TestQueryLogs_NonOKStatus_ReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"message":"internal error"}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	_, err := c.QueryLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestQueryLogs_EmptyResult(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	lines, err := c.QueryLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("got %d lines, want 0", len(lines))
	}
}

func TestQueryLogs_PodAndContainerFilters(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	_, err := c.QueryLogs(context.Background(), QueryParams{
		Namespace: "astro-abc-0",
		Pod:       "my-pod",
		Container: "sidecar",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{namespace="astro-abc-0", pod="my-pod", container="sidecar"}`
	if gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

func TestQueryLogs_CustomLimit(t *testing.T) {
	var gotLimit string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLimit = r.URL.Query().Get("limit")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	_, err := c.QueryLogs(context.Background(), QueryParams{
		Namespace: "astro-abc-0",
		Limit:     500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != "500" {
		t.Errorf("limit = %q, want 500", gotLimit)
	}
}

func TestQueryLogs_TimeRange(t *testing.T) {
	var gotStart, gotEnd string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStart = r.URL.Query().Get("start")
		gotEnd = r.URL.Query().Get("end")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"streams","result":[]}}`)) //nolint:errcheck
	}))
	defer ts.Close()

	start := time.Unix(1000, 0)
	end := time.Unix(2000, 0)

	c := New(ts.URL)
	_, err := c.QueryLogs(context.Background(), QueryParams{
		Namespace: "astro-abc-0",
		Start:     start,
		End:       end,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotStart != "1000000000000" {
		t.Errorf("start = %q, want 1000000000000", gotStart)
	}
	if gotEnd != "2000000000000" {
		t.Errorf("end = %q, want 2000000000000", gotEnd)
	}
}

func TestQueryLogs_LevelFromStreamLabel(t *testing.T) {
	// Backward-compat: pipelines that still emit level as a stream label.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [
					{
						"stream": {"pod": "my-pod", "level": "error"},
						"values": [["1000000000", "something went wrong"]]
					},
					{
						"stream": {"pod": "my-pod", "level": "info"},
						"values": [["2000000000", "all good"]]
					},
					{
						"stream": {"pod": "my-pod"},
						"values": [["3000000000", "no level label"]]
					}
				]
			}
		}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	lines, err := c.QueryLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[0].Level != "error" {
		t.Errorf("lines[0].Level = %q, want \"error\"", lines[0].Level)
	}
	if lines[1].Level != "info" {
		t.Errorf("lines[1].Level = %q, want \"info\"", lines[1].Level)
	}
	if lines[2].Level != "" {
		t.Errorf("lines[2].Level = %q, want \"\" (absent label)", lines[2].Level)
	}
}

func TestQueryLogs_LevelFromStructuredMetadata(t *testing.T) {
	// Pipelines using stage.structured_metadata emit level as a per-entry
	// metadata field; the stream itself has no level label.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [{
					"stream": {"pod": "my-pod"},
					"values": [
						["1000000000", "something went wrong", {"level": "error"}],
						["2000000000", "all good",             {"level": "info"}],
						["3000000000", "no metadata"]
					]
				}]
			}
		}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	lines, err := c.QueryLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[0].Level != "error" {
		t.Errorf("lines[0].Level = %q, want \"error\"", lines[0].Level)
	}
	if lines[1].Level != "info" {
		t.Errorf("lines[1].Level = %q, want \"info\"", lines[1].Level)
	}
	if lines[2].Level != "" {
		t.Errorf("lines[2].Level = %q, want \"\" (no metadata, no stream label)", lines[2].Level)
	}
}

func TestQueryLogs_StructuredMetadataTakesPrecedenceOverStreamLabel(t *testing.T) {
	// Per-entry metadata is more accurate than the coarse stream-level label.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [{
					"stream": {"pod": "my-pod", "level": "info"},
					"values": [
						["1000000000", "oh no", {"level": "error"}]
					]
				}]
			}
		}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	lines, err := c.QueryLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if lines[0].Level != "error" {
		t.Errorf("lines[0].Level = %q, want \"error\" (metadata wins over stream label)", lines[0].Level)
	}
}

func TestQueryLogs_LevelFromDetectedLevel(t *testing.T) {
	// Loki 3.x auto-detects level and exposes it as "detected_level" stream label.
	// Used as final fallback when neither metadata nor explicit stream label is set.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [{
					"stream": {"pod": "my-pod", "detected_level": "error"},
					"values": [
						["1000000000", "something went wrong"],
						["2000000000", "all good"]
					]
				}]
			}
		}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	lines, err := c.QueryLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].Level != "error" {
		t.Errorf("lines[0].Level = %q, want \"error\" (from detected_level)", lines[0].Level)
	}
}

func TestQueryLogs_DetectedLevelUnknown_DiscardedAsEmpty(t *testing.T) {
	// Loki sets detected_level to "unknown" when it can't parse a level from
	// the log line. We discard it so the API returns an empty level instead.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [{
					"stream": {"pod": "my-pod", "detected_level": "unknown"},
					"values": [
						["1000000000", "Starting Sasbot..."],
						["2000000000", "Sasbot is ready and listening for messages"]
					]
				}]
			}
		}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	lines, err := c.QueryLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].Level != "" {
		t.Errorf("lines[0].Level = %q, want \"\" (unknown should be discarded)", lines[0].Level)
	}
	if lines[1].Level != "" {
		t.Errorf("lines[1].Level = %q, want \"\" (unknown should be discarded)", lines[1].Level)
	}
}

func TestQueryLogs_ExplicitLevelTakesPrecedenceOverDetectedLevel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [{
					"stream": {"pod": "my-pod", "level": "warn", "detected_level": "info"},
					"values": [["1000000000", "ambiguous"]]
				}]
			}
		}`)) //nolint:errcheck
	}))
	defer ts.Close()

	c := New(ts.URL)
	lines, err := c.QueryLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lines[0].Level != "warn" {
		t.Errorf("lines[0].Level = %q, want \"warn\" (explicit level wins over detected_level)", lines[0].Level)
	}
}

// ── TailLogs tests ────────────────────────────────────────────────────────────

var wsUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func newWSSrv(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/loki/api/v1/tail" {
			handler(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestTailLogs_ReceivesLogLines(t *testing.T) {
	ts := newWSSrv(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close() //nolint:errcheck

		frame := tailWSMessage{
			Streams: []struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			}{
				{
					Stream: map[string]string{"pod": "my-pod", "container": "agent"},
					Values: [][]string{
						{"1000000000", "hello world"},
						{"2000000000", "second line"},
					},
				},
			},
		}
		data, _ := json.Marshal(frame)
		conn.WriteMessage(websocket.TextMessage, data) //nolint:errcheck
		// Close normally so the goroutine exits cleanly.
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")) //nolint:errcheck
	})

	c := New(ts.URL)
	ch, err := c.TailLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("TailLogs: %v", err)
	}

	var lines []LogLine
	for ll := range ch {
		lines = append(lines, ll)
	}

	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].Line != "hello world" {
		t.Errorf("lines[0].Line = %q, want %q", lines[0].Line, "hello world")
	}
	if lines[1].Line != "second line" {
		t.Errorf("lines[1].Line = %q, want %q", lines[1].Line, "second line")
	}
	if lines[0].Pod != "my-pod" {
		t.Errorf("lines[0].Pod = %q, want %q", lines[0].Pod, "my-pod")
	}
	if lines[0].Container != "agent" {
		t.Errorf("lines[0].Container = %q, want %q", lines[0].Container, "agent")
	}
	if lines[0].Timestamp != time.Unix(0, 1000000000) {
		t.Errorf("lines[0].Timestamp = %v, want %v", lines[0].Timestamp, time.Unix(0, 1000000000))
	}
}

func TestTailLogs_DialFailure(t *testing.T) {
	c := New("http://127.0.0.1:1") // nothing listening on port 1
	_, err := c.TailLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
}

func TestTailLogs_DetectedLevel(t *testing.T) {
	ts := newWSSrv(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify the query includes "| keep detected_level".
		q := r.URL.Query().Get("query")
		if !strings.Contains(q, "| keep detected_level") {
			t.Errorf("query %q missing '| keep detected_level'", q)
		}

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close() //nolint:errcheck

		// Simulate what Loki returns when "| keep detected_level" is in the query:
		// each level gets its own stream with detected_level in the label set.
		frame := map[string]any{
			"streams": []map[string]any{
				{
					"stream": map[string]string{"pod": "my-pod", "container": "agent", "detected_level": "error"},
					"values": [][]string{{"1000000000", "something broke"}},
				},
				{
					"stream": map[string]string{"pod": "my-pod", "container": "agent", "detected_level": "info"},
					"values": [][]string{{"2000000000", "all good"}},
				},
				{
					"stream": map[string]string{"pod": "my-pod", "container": "agent", "detected_level": "unknown"},
					"values": [][]string{{"3000000000", "plain line"}},
				},
			},
		}
		data, _ := json.Marshal(frame)
		conn.WriteMessage(websocket.TextMessage, data)                                                            //nolint:errcheck
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")) //nolint:errcheck
	})

	c := New(ts.URL)
	ch, err := c.TailLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("TailLogs: %v", err)
	}

	var lines []LogLine
	for ll := range ch {
		lines = append(lines, ll)
	}

	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	if lines[0].Level != "error" {
		t.Errorf("lines[0].Level = %q, want \"error\"", lines[0].Level)
	}
	if lines[1].Level != "info" {
		t.Errorf("lines[1].Level = %q, want \"info\"", lines[1].Level)
	}
	// "unknown" should be discarded to empty string.
	if lines[2].Level != "" {
		t.Errorf("lines[2].Level = %q, want \"\" (unknown discarded)", lines[2].Level)
	}
}

func TestTailLogs_ExplicitLevelTakesPrecedenceOverDetectedLevel(t *testing.T) {
	ts := newWSSrv(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close() //nolint:errcheck

		frame := map[string]any{
			"streams": []map[string]any{
				{
					"stream": map[string]string{"pod": "my-pod", "level": "warn", "detected_level": "info"},
					"values": [][]string{{"1000000000", "ambiguous"}},
				},
			},
		}
		data, _ := json.Marshal(frame)
		conn.WriteMessage(websocket.TextMessage, data)                                                            //nolint:errcheck
		conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")) //nolint:errcheck
	})

	c := New(ts.URL)
	ch, err := c.TailLogs(context.Background(), QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("TailLogs: %v", err)
	}

	var lines []LogLine
	for ll := range ch {
		lines = append(lines, ll)
	}

	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if lines[0].Level != "warn" {
		t.Errorf("lines[0].Level = %q, want \"warn\" (explicit level wins)", lines[0].Level)
	}
}

func TestTailLogs_ContextCancellation(t *testing.T) {
	started := make(chan struct{})
	ts := newWSSrv(t, func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close() //nolint:errcheck
		close(started)
		// Block until client disconnects.
		conn.ReadMessage() //nolint:errcheck
	})

	ctx, cancel := context.WithCancel(context.Background())

	c := New(ts.URL)
	ch, err := c.TailLogs(ctx, QueryParams{Namespace: "astro-abc-0"})
	if err != nil {
		t.Fatalf("TailLogs: %v", err)
	}

	<-started
	cancel()

	// Channel should close promptly after cancellation.
	select {
	case _, open := <-ch:
		if open {
			t.Error("expected channel to be closed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel did not close after context cancellation")
	}
}
