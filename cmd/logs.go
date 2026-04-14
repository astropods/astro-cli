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

// printLogs decodes a JSON log response and prints each entry as a formatted line.
func printLogs(body io.Reader) error {
	var entries []logEntry
	if err := json.NewDecoder(body).Decode(&entries); err != nil {
		return fmt.Errorf("failed to decode logs: %w", err)
	}
	if len(entries) == 0 {
		fmt.Println("No logs found.")
		return nil
	}
	for _, e := range entries {
		level := e.Level
		if level == "" {
			level = "INFO"
		}
		if e.Timestamp != "" {
			fmt.Printf("%s %s %s\n", e.Timestamp, level, e.Message)
		} else {
			fmt.Printf("%s %s\n", level, e.Message)
		}
	}
	return nil
}
