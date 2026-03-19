// Package metricsstore manages agent-level metrics (lifetime message counts) in PostgreSQL.
package metricsstore

import (
	"database/sql"
	"fmt"
)

// Store provides read access to agent metric counters.
type Store struct {
	db *sql.DB
}

// New creates a new metrics store.
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// BulkMessageCounts returns lifetime message totals for all agents belonging to an account.
func (s *Store) BulkMessageCounts(accountID string) (map[string]int64, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(
		`SELECT agent_name, lifetime_total FROM agent_message_counts WHERE account_id = $1`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("bulk message counts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	counts := make(map[string]int64)
	for rows.Next() {
		var name string
		var total int64
		if err := rows.Scan(&name, &total); err != nil {
			return nil, fmt.Errorf("scan message count: %w", err)
		}
		counts[name] = total
	}
	return counts, rows.Err()
}

// MessageCount returns the lifetime message total for a single agent.
func (s *Store) MessageCount(accountID, agentName string) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	var total int64
	err := s.db.QueryRow(
		`SELECT lifetime_total FROM agent_message_counts WHERE account_id = $1 AND agent_name = $2`,
		accountID, agentName,
	).Scan(&total)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("message count: %w", err)
	}
	return total, nil
}
