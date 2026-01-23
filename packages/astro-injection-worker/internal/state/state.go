package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State tracks the injection progress for incremental syncs
type State struct {
	Mode                  string    `json:"mode"`                     // "backfill" or "incremental"
	BackfillComplete      bool      `json:"backfill_complete"`
	CurrentCursor         string    `json:"current_cursor"`           // Cursor for pagination
	LastSyncTimestamp     string    `json:"last_sync_timestamp"`
	TotalIssuesProcessed  int       `json:"total_issues_processed"`
	RateLimitReset        string    `json:"rate_limit_reset,omitempty"`
	ExpectedGithubCount   int       `json:"expected_github_count,omitempty"`
	ActualQdrantCount     int       `json:"actual_qdrant_count,omitempty"`
	LastCountCheck        time.Time `json:"last_count_check,omitempty"`
}

const defaultStateFile = "/app/state/sync_state.json"

// Load loads state from persistent storage
func Load() (*State, error) {
	stateFile := os.Getenv("STATE_FILE")
	if stateFile == "" {
		stateFile = defaultStateFile
	}

	// Return default state if file doesn't exist
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		return &State{
			Mode:             "backfill",
			BackfillComplete: false,
			CurrentCursor:    "",
		}, nil
	}

	// Read and parse state file
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	return &state, nil
}

// Save saves state to persistent storage
func (s *State) Save() error {
	stateFile := os.Getenv("STATE_FILE")
	if stateFile == "" {
		stateFile = defaultStateFile
	}

	// Ensure directory exists
	dir := filepath.Dir(stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Marshal state
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to file
	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// IsUnderRateLimit checks if we're still under GitHub rate limit
func (s *State) IsUnderRateLimit() (bool, time.Duration) {
	if s.RateLimitReset == "" {
		return false, 0
	}

	resetTime, err := time.Parse(time.RFC3339, s.RateLimitReset)
	if err != nil {
		return false, 0
	}

	now := time.Now().UTC()
	if now.Before(resetTime) {
		return true, resetTime.Sub(now)
	}

	return false, 0
}

// ClearRateLimit clears the rate limit flag
func (s *State) ClearRateLimit() {
	s.RateLimitReset = ""
}

// MarkBackfillComplete marks backfill as complete and switches to incremental mode
func (s *State) MarkBackfillComplete() {
	s.BackfillComplete = true
	s.Mode = "incremental"
	s.CurrentCursor = ""
	s.LastSyncTimestamp = time.Now().UTC().Format(time.RFC3339)
}

// Reset resets the state for a fresh backfill
func (s *State) Reset() {
	s.Mode = "backfill"
	s.BackfillComplete = false
	s.CurrentCursor = ""
	s.TotalIssuesProcessed = 0
	s.LastSyncTimestamp = ""
	s.RateLimitReset = ""
}
