package deploymentstore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// DeploymentEvent represents a single status transition in a deployment's history.
type DeploymentEvent struct {
	ID           int64           `json:"id"`
	DeploymentID string          `json:"deployment_id"`
	Status       string          `json:"status"`
	Message      string          `json:"message,omitempty"`
	Details      json.RawMessage `json:"details,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// GetDeploymentEvents returns the status event history for a deployment, newest first.
func (s *Store) GetDeploymentEvents(deploymentID string, limit int) ([]DeploymentEvent, error) {
	rows, err := s.db.Query(`
		SELECT id, deployment_id, status, message, details, created_at
		FROM deployment_events
		WHERE deployment_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, deploymentID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query deployment events: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var events []DeploymentEvent
	for rows.Next() {
		var e DeploymentEvent
		var msg sql.NullString
		var details []byte
		if err := rows.Scan(&e.ID, &e.DeploymentID, &e.Status, &msg, &details, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan deployment event: %w", err)
		}
		e.Message = msg.String
		if details != nil {
			e.Details = details
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deployment events: %w", err)
	}
	return events, nil
}
