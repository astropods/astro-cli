package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Client queries Loki for logs using the HTTP query range API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Loki client. baseURL should be the base URL of the Loki gateway,
// e.g. "http://loki-gateway.monitoring.svc.cluster.local" or "http://<nlb-dns>:3100".
func New(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// QueryParams defines parameters for a log query.
type QueryParams struct {
	Namespace   string
	Pod         string // optional — exact pod name (used by K8s fallback)
	Workload    string // optional — k8s workload name (Deployment, StatefulSet, etc.); matches all pods with this prefix
	Container   string // optional
	Limit       int64  // default 200
	Start       time.Time
	End         time.Time
	Direction   string // "forward" (oldest first) or "backward"; default "forward"
	LevelFilter string // optional — appends `| level = "<value>"` LogQL pipeline filter (e.g. "error")
}

// LogLine is a single log entry returned from Loki.
type LogLine struct {
	Timestamp time.Time
	Pod       string
	Container string
	Level     string
	Line      string
}

// QueryLogs fetches logs from Loki matching the given params.
func (c *Client) QueryLogs(ctx context.Context, p QueryParams) ([]LogLine, error) {
	if p.Limit <= 0 {
		p.Limit = 200
	}
	end := p.End
	if end.IsZero() {
		end = time.Now()
	}
	start := p.Start
	if start.IsZero() {
		start = end.Add(-1 * time.Hour)
	}
	direction := p.Direction
	if direction == "" {
		direction = "forward"
	}

	params := url.Values{}
	query := buildSelector(p.Namespace, p.Pod, p.Workload, p.Container)
	if p.LevelFilter != "" {
		query += ` | level = "` + p.LevelFilter + `"`
	}
	params.Set("query", query)
	params.Set("limit", strconv.FormatInt(p.Limit, 10))
	params.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	params.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	params.Set("direction", direction)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/loki/api/v1/query_range?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build loki request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loki query: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki returned status %d", resp.StatusCode)
	}

	var result queryRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode loki response: %w", err)
	}

	var lines []LogLine
	for _, stream := range result.Data.Result {
		pod := stream.Stream["pod"]
		container := stream.Stream["container"]
		streamLevel := stream.Stream["level"]            // explicit stream label (legacy pipelines)
		detectedLevel := stream.Stream["detected_level"] // Loki 3.x auto-detection
		for _, entry := range stream.Values {
			tsNano, err := strconv.ParseInt(entry.Timestamp, 10, 64)
			if err != nil {
				continue
			}
			// Prefer: structured metadata > explicit stream label > Loki auto-detected level.
			// Loki sets detected_level to "unknown" when it can't parse a level
			// from the log line — discard it so consumers see an empty string
			// rather than a misleading value.
			level := entry.Metadata["level"]
			if level == "" {
				level = streamLevel
			}
			if level == "" && detectedLevel != "unknown" {
				level = detectedLevel
			}
			lines = append(lines, LogLine{
				Timestamp: time.Unix(0, tsNano),
				Pod:       pod,
				Container: container,
				Level:     level,
				Line:      entry.Line,
			})
		}
	}

	// Loki returns each stream sorted, but streams may be interleaved across pods.
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Timestamp.Before(lines[j].Timestamp)
	})

	return lines, nil
}

// buildSelector constructs a LogQL stream selector from the given labels.
// When workload is set (and pod is empty), it uses a regex match to capture
// all pods belonging to that k8s workload (pod=~"<workload>-.+").
func buildSelector(namespace, pod, workload, container string) string {
	parts := []string{`namespace="` + namespace + `"`}
	if pod != "" {
		parts = append(parts, `pod="`+pod+`"`)
	} else if workload != "" {
		parts = append(parts, `pod=~"`+workload+`-.+"`)
	}
	if container != "" {
		parts = append(parts, `container="`+container+`"`)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// logEntryValue is a single Loki value tuple: [timestamp, line] or
// [timestamp, line, metadata]. The third element is present when the Alloy
// pipeline emits structured metadata (e.g. stage.structured_metadata).
type logEntryValue struct {
	Timestamp string
	Line      string
	Metadata  map[string]string
}

func (v *logEntryValue) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) < 2 {
		return fmt.Errorf("loki value entry: expected at least 2 elements, got %d", len(raw))
	}
	if err := json.Unmarshal(raw[0], &v.Timestamp); err != nil {
		return fmt.Errorf("loki value entry: parse timestamp: %w", err)
	}
	if err := json.Unmarshal(raw[1], &v.Line); err != nil {
		return fmt.Errorf("loki value entry: parse line: %w", err)
	}
	if len(raw) >= 3 {
		if err := json.Unmarshal(raw[2], &v.Metadata); err != nil {
			return fmt.Errorf("loki value entry: parse metadata: %w", err)
		}
	}
	return nil
}

// queryRangeResponse is the Loki /loki/api/v1/query_range response envelope.
type queryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values []logEntryValue   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// tailWSMessage is a single WebSocket frame from Loki's /loki/api/v1/tail endpoint.
type tailWSMessage struct {
	Streams []struct {
		Stream map[string]string `json:"stream"`
		Values [][]string        `json:"values"` // [unix_nano_str, log_line]
	} `json:"streams"`
}

const tailReadDeadline = 90 * time.Second

var wsSchemeReplacer = strings.NewReplacer("https://", "wss://", "http://", "ws://")

// TailLogs connects to Loki's WebSocket tail endpoint and streams new log lines
// into the returned channel. The channel is closed when ctx is cancelled or the
// connection drops. An error is returned only if the initial dial fails.
func (c *Client) TailLogs(ctx context.Context, p QueryParams) (<-chan LogLine, error) {
	wsBase := wsSchemeReplacer.Replace(c.baseURL)

	start := p.Start
	if start.IsZero() {
		start = time.Now()
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 200
	}

	// The tail endpoint does not automatically promote structured metadata
	// (like detected_level) into stream labels. Use "| keep detected_level"
	// to surface it so we can extract log levels from tailed entries.
	query := buildSelector(p.Namespace, p.Pod, p.Workload, p.Container) + " | keep detected_level"

	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", strconv.FormatInt(limit, 10))
	params.Set("start", strconv.FormatInt(start.UnixNano(), 10))

	wsURL := wsBase + "/loki/api/v1/tail?" + params.Encode()

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("loki tail dial: %w", err)
	}

	ch := make(chan LogLine, 64)

	go func() {
		defer close(ch)
		defer func() { _ = conn.Close() }()

		// When the context is cancelled, close the connection so ReadMessage
		// unblocks promptly. Using a separate goroutine that exits when the
		// reader returns (via done channel) avoids leaking it.
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-done:
			}
		}()

		var frame tailWSMessage
		for {
			_ = conn.SetReadDeadline(time.Now().Add(tailReadDeadline))
			_, msg, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}

			frame.Streams = frame.Streams[:0]
			if err := json.Unmarshal(msg, &frame); err != nil {
				continue
			}

			for _, stream := range frame.Streams {
				pod := stream.Stream["pod"]
				container := stream.Stream["container"]
				// Same level cascade as QueryLogs: explicit label > detected_level.
				level := stream.Stream["level"]
				if level == "" {
					if dl := stream.Stream["detected_level"]; dl != "unknown" {
						level = dl
					}
				}
				for _, entry := range stream.Values {
					if len(entry) < 2 {
						continue
					}
					tsNano, parseErr := strconv.ParseInt(entry[0], 10, 64)
					if parseErr != nil {
						continue
					}
					select {
					case ch <- LogLine{
						Timestamp: time.Unix(0, tsNano),
						Pod:       pod,
						Container: container,
						Level:     level,
						Line:      entry[1],
					}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch, nil
}
