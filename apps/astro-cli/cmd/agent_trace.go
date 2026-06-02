package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro/apps/astro-cli/internal/theme"
)

// Parity with the web UI:
//   - default: list recent traces (Agents → Monitor)
//   - --trace-id: single trace Overview
//   - --summary: agents-page activity snapshot (deployment-summaries)

var agentTraceCmd = &cobra.Command{
	Use:   "trace",
	Short: "List traces, show trace detail, or print agents-page activity summary",
	Args:  agentTargetArgs,
	RunE:  runAgentTrace,
}

func init() {
	agentCmd.AddCommand(agentTraceCmd)
	registerAgentTargetFlags(agentTraceCmd) // adds --name, --id
	agentTraceCmd.Flags().StringP("trace-id", "t", "", "Trace ID to show detail (Overview)")
	agentTraceCmd.Flags().Bool("json", false, "Print raw JSON output")
	agentTraceCmd.Flags().Int("limit", 50, "Number of traces to list")
	agentTraceCmd.Flags().Int("offset", 0, "Pagination offset for list")
	agentTraceCmd.Flags().String("start", "", "List window start (RFC3339)")
	agentTraceCmd.Flags().String("end", "", "List window end (RFC3339)")
	agentTraceCmd.Flags().Bool("summary", false, "Show agents-page activity summary (30-day series). Ignores --limit, --offset, --start, and --end")
}

type traceEntry struct {
	TraceID     string          `json:"trace_id"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	LatencyMS   float64         `json:"latency_ms"`
	TotalTokens int             `json:"total_tokens,omitempty"`
	TotalCost   float64         `json:"total_cost,omitempty"`
	Input       json.RawMessage `json:"input,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
	Timestamp   string          `json:"timestamp"`
	UserID      string          `json:"user_id,omitempty"`
}

type tracesListResponse struct {
	Traces []traceEntry `json:"traces"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Offset int          `json:"offset"`
}

type deploymentSummaryEntry struct {
	TotalTraces   int    `json:"total_traces"`
	LastTraceAt   string `json:"last_trace_at"`
	RequestSeries []int  `json:"request_series,omitempty"`
	TokenSeries   []int  `json:"token_series,omitempty"`
}

type deploymentSummariesResponse struct {
	Summaries map[string]deploymentSummaryEntry `json:"summaries"`
}

type agentTraceSummaryJSON struct {
	DeploymentID string                  `json:"deployment_id"`
	DisplayName  string                  `json:"display_name,omitempty"`
	Summary      *deploymentSummaryEntry `json:"summary"`
}

type traceObservation struct {
	ID            string          `json:"id"`
	ParentID      string          `json:"parent_id,omitempty"`
	Type          string          `json:"type"`
	Name          string          `json:"name"`
	StartTime     string          `json:"start_time"`
	EndTime       string          `json:"end_time,omitempty"`
	LatencyMS     float64         `json:"latency_ms"`
	Level         string          `json:"level,omitempty"`
	StatusMessage string          `json:"status_message,omitempty"`
	Input         json.RawMessage `json:"input,omitempty"`
	Output        json.RawMessage `json:"output,omitempty"`
	Model         string          `json:"model,omitempty"`
	Cost          float64         `json:"cost,omitempty"`
}

type traceScore struct {
	Name    string  `json:"name"`
	Value   float64 `json:"value"`
	Comment string  `json:"comment,omitempty"`
}

type traceDetail struct {
	TraceID   string          `json:"trace_id"`
	Name      string          `json:"name"`
	Timestamp string          `json:"timestamp"`
	LatencyMS float64         `json:"latency_ms"`
	TotalCost float64         `json:"total_cost"`
	Input     json.RawMessage `json:"input,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	UserID    string          `json:"user_id,omitempty"`
	Tags      []string        `json:"tags,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

type traceDetailResponse struct {
	Trace        traceDetail        `json:"trace"`
	Observations []traceObservation `json:"observations"`
	Scores       []traceScore       `json:"scores"`
}

func runAgentTrace(cmd *cobra.Command, args []string) error {
	traceID, _ := cmd.Flags().GetString("trace-id")
	summary, _ := cmd.Flags().GetBool("summary")
	if summary && traceID != "" {
		return errAgentTraceSummaryWithTraceID()
	}

	if !summary {
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")
		if err := validateTracePagination(limit, offset); err != nil {
			return err
		}
		start, _ := cmd.Flags().GetString("start")
		end, _ := cmd.Flags().GetString("end")
		if err := validateTraceTimeWindow(start, end); err != nil {
			return err
		}
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}

	dep, err := resolveAgentTarget(cmd, at, verbose)
	if err != nil {
		return err
	}

	label := deploymentLabel(dep)
	if summary {
		return runAgentTraceSummary(cmd, label, dep.ID, at, verbose)
	}
	if traceID != "" {
		return runAgentTraceDetail(cmd, label, dep.ID, traceID, at, verbose)
	}
	return runAgentTraceList(cmd, label, dep.ID, at, verbose)
}

func validateTracePagination(limit, offset int) error {
	if limit <= 0 {
		return errPositiveIntFlag("limit")
	}
	if offset < 0 {
		return errNonNegativeIntFlag("offset")
	}
	return nil
}

func validateTraceTimeWindow(start, end string) error {
	var startTime, endTime time.Time
	if start != "" {
		t, err := time.Parse(time.RFC3339, start)
		if err != nil {
			return errRFC3339TimeFlag("start", start)
		}
		startTime = t
	}
	if end != "" {
		t, err := time.Parse(time.RFC3339, end)
		if err != nil {
			return errRFC3339TimeFlag("end", end)
		}
		endTime = t
	}
	if !startTime.IsZero() && !endTime.IsZero() && !startTime.Before(endTime) {
		return errTraceStartAfterEnd()
	}
	return nil
}

func runAgentTraceList(cmd *cobra.Command, label, id string, at AccountToken, verbose bool) error {
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")

	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("offset", fmt.Sprintf("%d", offset))
	if v, _ := cmd.Flags().GetString("start"); v != "" {
		q.Set("start_time", v)
	}
	if v, _ := cmd.Flags().GetString("end"); v != "" {
		q.Set("end_time", v)
	}

	u := fmt.Sprintf("%s/api/v1/deployments/%s/observability/traces?%s", agentBaseURL(), url.PathEscape(id), q.Encode())
	var result tracesListResponse
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result)
	if status == http.StatusNotFound {
		if flagString(cmd, "id") != "" {
			return errAgentDeploymentNotFoundForID(id)
		}
		return errAgentDeploymentNotFound(label)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}

	if len(result.Traces) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoTracesForAgent(label), colorReset) //nolint:errcheck,gosec
		return nil
	}

	dim := color.New(color.Faint)
	cyan := color.New(theme.PrimaryFatihAttr)

	const traceIDWidth = 32
	const nameWidth = 24
	const latWidth = 9
	dim.Fprintf(w, "%-*s  %-*s  %-*s  %-*s  %s\n", //nolint:errcheck,gosec
		tableTimeWidth, "Time",
		traceIDWidth, "Trace ID",
		nameWidth, "Name",
		latWidth, "Latency",
		"Cost")

	for _, t := range result.Traces {
		ts := truncate(t.Timestamp, tableTimeWidth)
		tid := truncate(t.TraceID, traceIDWidth)
		nm := truncate(t.Name, nameWidth)
		lat := fmt.Sprintf("%.0fms", t.LatencyMS)
		cost := ""
		if t.TotalCost > 0 {
			cost = fmt.Sprintf("$%.4f", t.TotalCost)
		}
		dim.Fprintf(w, "%-*s  ", tableTimeWidth, ts)                           //nolint:errcheck,gosec
		cyan.Fprintf(w, "%-*s  ", traceIDWidth, tid)                           //nolint:errcheck,gosec
		fmt.Fprintf(w, "%-*s  %-*s  %s\n", nameWidth, nm, latWidth, lat, cost) //nolint:errcheck,gosec
	}

	if result.Total > offset+len(result.Traces) {
		dim.Fprintf(w, "\nShowing %d–%d of %d. Page with --offset %d.\n", //nolint:errcheck,gosec
			offset+1, offset+len(result.Traces), result.Total, offset+limit)
	}
	return nil
}

func runAgentTraceSummary(cmd *cobra.Command, label, depID string, at AccountToken, verbose bool) error {
	u := fmt.Sprintf("%s/api/v1/accounts/%s/observability/deployment-summaries",
		agentBaseURL(), url.PathEscape(at.Account))
	var result deploymentSummariesResponse
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result); err != nil {
		return err
	}

	entry, ok := result.Summaries[depID]
	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		out := agentTraceSummaryJSON{DeploymentID: depID, DisplayName: label}
		if ok {
			e := entry
			out.Summary = &e
		}
		return writeJSON(w, out)
	}

	dim := color.New(color.Faint)
	accent := color.New(theme.PrimaryFatihAttr)

	if !ok {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoObsSummaryForAgent(label), colorReset) //nolint:errcheck,gosec
		return nil
	}

	accent.Fprintf(w, "%s\n", label)                           //nolint:errcheck,gosec
	fmt.Fprintf(w, "  ID:            %s\n", depID)             //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Total traces:  %d\n", entry.TotalTraces) //nolint:errcheck,gosec
	if entry.LastTraceAt != "" {
		rel := formatObsLastActive(entry.LastTraceAt)
		fmt.Fprintf(w, "  Last active:   %s (%s)\n", rel, entry.LastTraceAt) //nolint:errcheck,gosec
	}
	if len(entry.RequestSeries) > 0 {
		printObsSeriesBlock(w, "Requests", entry.RequestSeries)
	}
	if len(entry.TokenSeries) > 0 {
		printObsSeriesBlock(w, "Tokens", entry.TokenSeries)
	}
	dim.Fprintln(w, "\nActivity summary (matches /agents sparkline). Updates about every 10m.") //nolint:errcheck,gosec
	return nil
}

func parseObsTimestamp(ts string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", ts)
}

func formatObsLastActive(ts string) string {
	t, err := parseObsTimestamp(ts)
	if err != nil {
		return ts
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d / time.Minute)
		if m <= 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d / time.Hour)
		if h <= 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d / (24 * time.Hour))
		if days <= 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// obsSparkBlocks mirrors the agents-page sparkline: one glyph per day, oldest → newest.
var obsSparkBlocks = []rune("▁▂▃▄▅▆▇█")

func printObsSeriesBlock(w io.Writer, label string, series []int) {
	const pad = "  "
	const labelWidth = 9
	const seriesSuffix = " (30d):  "
	spark := formatObsSparkline(series)
	stats := formatObsSeriesStats(series)
	fmt.Fprintf(w, "%s%-*s%s%s\n", pad, labelWidth, label, seriesSuffix, spark)               //nolint:errcheck,gosec
	fmt.Fprintf(w, "%s%s%s\n", pad, strings.Repeat(" ", labelWidth+len(seriesSuffix)), stats) //nolint:errcheck,gosec
}

func formatObsSparkline(series []int) string {
	if len(series) == 0 {
		return ""
	}
	max := 0
	for _, n := range series {
		if n > max {
			max = n
		}
	}
	var b strings.Builder
	b.Grow(len(series))
	if max == 0 {
		for range series {
			b.WriteRune(obsSparkBlocks[0])
		}
		return b.String()
	}
	for _, n := range series {
		level := 0
		if n > 0 {
			level = (n * (len(obsSparkBlocks) - 1)) / max
			if level >= len(obsSparkBlocks) {
				level = len(obsSparkBlocks) - 1
			}
		}
		b.WriteRune(obsSparkBlocks[level])
	}
	return b.String()
}

func formatObsSeriesStats(series []int) string {
	total, peak, active := 0, 0, 0
	for _, n := range series {
		total += n
		if n > peak {
			peak = n
		}
		if n > 0 {
			active++
		}
	}
	if total == 0 {
		return "no activity in window"
	}
	dayWord := "days"
	if active == 1 {
		dayWord = "day"
	}
	return fmt.Sprintf("%s total · %s peak/day · %s active %s",
		formatObsCount(total), formatObsCount(peak), strconv.Itoa(active), dayWord)
}

func formatObsCount(n int) string {
	const (
		kilo = 1000
		mega = 1000 * kilo
	)
	switch {
	case n >= mega:
		if n%mega == 0 {
			return fmt.Sprintf("%dM", n/mega)
		}
		return fmt.Sprintf("%.1fM", float64(n)/float64(mega))
	case n >= kilo:
		if n%kilo == 0 {
			return fmt.Sprintf("%dk", n/kilo)
		}
		return fmt.Sprintf("%.1fk", float64(n)/float64(kilo))
	default:
		return strconv.Itoa(n)
	}
}

func runAgentTraceDetail(cmd *cobra.Command, label, id, traceID string, at AccountToken, verbose bool) error {
	u := fmt.Sprintf("%s/api/v1/deployments/%s/observability/traces/%s",
		agentBaseURL(), url.PathEscape(id), url.PathEscape(traceID))
	var result traceDetailResponse
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &result)
	if status == http.StatusNotFound {
		return errAgentTraceNotFound(traceID, label)
	}
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, result)
	}

	dim := color.New(color.Faint)
	accent := color.New(theme.PrimaryFatihAttr)

	t := result.Trace
	accent.Fprintf(w, "%s\n", t.TraceID)                  //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Name:       %s\n", t.Name)          //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Time:       %s\n", t.Timestamp)     //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Latency:    %.0fms\n", t.LatencyMS) //nolint:errcheck,gosec
	if t.TotalCost > 0 {
		fmt.Fprintf(w, "  Cost:       $%.4f\n", t.TotalCost) //nolint:errcheck,gosec
	}
	if t.SessionID != "" {
		fmt.Fprintf(w, "  Session:    %s\n", t.SessionID) //nolint:errcheck,gosec
	}
	if t.UserID != "" {
		fmt.Fprintf(w, "  User:       %s\n", t.UserID) //nolint:errcheck,gosec
	}
	if len(t.Tags) > 0 {
		fmt.Fprintf(w, "  Tags:       %s\n", strings.Join(t.Tags, ", ")) //nolint:errcheck,gosec
	}

	if len(t.Metadata) > 0 {
		dim.Fprintln(w, "\nMetadata:")                //nolint:errcheck,gosec
		fmt.Fprintln(w, indentJSON(t.Metadata, "  ")) //nolint:errcheck,gosec
	}

	if len(t.Input) > 0 {
		dim.Fprintln(w, "\nInput:")                //nolint:errcheck,gosec
		fmt.Fprintln(w, indentJSON(t.Input, "  ")) //nolint:errcheck,gosec
	}
	if len(t.Output) > 0 {
		dim.Fprintln(w, "\nOutput:")                //nolint:errcheck,gosec
		fmt.Fprintln(w, indentJSON(t.Output, "  ")) //nolint:errcheck,gosec
	}

	if len(result.Observations) > 0 {
		dim.Fprintf(w, "\nObservations (%d):\n", len(result.Observations)) //nolint:errcheck,gosec
		obs := append([]traceObservation(nil), result.Observations...)
		sort.SliceStable(obs, func(i, j int) bool { return obs[i].StartTime < obs[j].StartTime })
		for _, o := range obs {
			obsLabel := o.Name
			if o.Type != "" {
				obsLabel = fmt.Sprintf("[%s] %s", o.Type, o.Name)
			}
			line := fmt.Sprintf("  %s  %.0fms", obsLabel, o.LatencyMS)
			if o.Model != "" {
				line += "  " + o.Model
			}
			if o.Level != "" && o.Level != "DEFAULT" {
				line += "  " + o.Level
			}
			fmt.Fprintln(w, line) //nolint:errcheck,gosec
			if o.StatusMessage != "" {
				dim.Fprintf(w, "    %s\n", o.StatusMessage) //nolint:errcheck,gosec
			}
			if len(o.Input) > 0 {
				dim.Fprintln(w, "    Input:")                //nolint:errcheck,gosec
				fmt.Fprintln(w, indentJSON(o.Input, "    ")) //nolint:errcheck,gosec
			}
			if len(o.Output) > 0 {
				dim.Fprintln(w, "    Output:")                //nolint:errcheck,gosec
				fmt.Fprintln(w, indentJSON(o.Output, "    ")) //nolint:errcheck,gosec
			}
		}
	}

	if len(result.Scores) > 0 {
		dim.Fprintf(w, "\nScores (%d):\n", len(result.Scores)) //nolint:errcheck,gosec
		for _, s := range result.Scores {
			line := fmt.Sprintf("  %s: %.4f", s.Name, s.Value)
			if s.Comment != "" {
				line += "  — " + s.Comment
			}
			fmt.Fprintln(w, line) //nolint:errcheck,gosec
		}
	}
	return nil
}
