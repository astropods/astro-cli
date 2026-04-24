package cmd

import (
	"encoding/json"
	"fmt"
	"io"
)

type logEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

func (e *logEntry) parts() (ts, level, msg string) {
	level = e.Level
	if level == "" {
		level = "INFO"
	}
	return e.Timestamp, level, e.Message
}

// printLogs decodes a JSON log response and prints each entry as a formatted line.
func printLogs(w io.Writer, body io.Reader) error {
	var entries []logEntry
	if err := json.NewDecoder(body).Decode(&entries); err != nil {
		return fmt.Errorf("failed to decode logs: %w", err)
	}
	if len(entries) == 0 {
		fmt.Fprintln(w, "No logs found.") //nolint:errcheck,gosec
		return nil
	}
	for _, e := range entries {
		ts, level, msg := e.parts()
		if ts != "" {
			fmt.Fprintf(w, "%s %s %s\n", ts, level, msg) //nolint:errcheck,gosec
		} else {
			fmt.Fprintf(w, "%s %s\n", level, msg) //nolint:errcheck,gosec
		}
	}
	return nil
}
