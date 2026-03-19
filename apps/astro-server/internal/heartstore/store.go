// Package heartstore manages agent heart (like) records in PostgreSQL.
package heartstore

import (
	"context"
	"database/sql"
	"fmt"
)

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Toggle atomically adds or removes a heart and returns the new state + count.
func (s *Store) Toggle(ctx context.Context, accountID, agentName, userID string) (hearted bool, count int, err error) {
	err = s.db.QueryRowContext(ctx, `
		WITH toggled AS (
			DELETE FROM agent_hearts
			WHERE account_id = $1 AND agent_name = $2 AND user_id = $3
			RETURNING true
		),
		inserted AS (
			INSERT INTO agent_hearts (account_id, agent_name, user_id)
			SELECT $1, $2, $3
			WHERE NOT EXISTS (SELECT 1 FROM toggled)
			RETURNING true
		),
		prev_count AS (
			SELECT COUNT(*) AS n FROM agent_hearts WHERE account_id = $1 AND agent_name = $2
		)
		SELECT
			EXISTS (SELECT 1 FROM inserted),
			CASE
				WHEN EXISTS (SELECT 1 FROM inserted) THEN (SELECT n FROM prev_count) + 1
				WHEN EXISTS (SELECT 1 FROM toggled)  THEN (SELECT n FROM prev_count) - 1
				ELSE (SELECT n FROM prev_count)
			END`,
		accountID, agentName, userID,
	).Scan(&hearted, &count)
	if err != nil {
		return false, 0, fmt.Errorf("toggle heart: %w", err)
	}
	return hearted, count, nil
}

// AgentHeartInfo holds count + whether the caller has hearted.
type AgentHeartInfo struct {
	Count   int  `json:"count"`
	Hearted bool `json:"hearted"`
}

// Info returns heart count and whether the given user has hearted, in a single query.
// If userID is empty, hearted will be false.
func (s *Store) Info(ctx context.Context, accountID, agentName, userID string) (*AgentHeartInfo, error) {
	info := &AgentHeartInfo{}
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM agent_hearts WHERE account_id = $1 AND agent_name = $2),
			EXISTS(SELECT 1 FROM agent_hearts WHERE account_id = $1 AND agent_name = $2 AND user_id = $3)`,
		accountID, agentName, userID,
	).Scan(&info.Count, &info.Hearted)
	if err != nil {
		return nil, fmt.Errorf("heart info: %w", err)
	}
	return info, nil
}

// BulkCount returns heart counts for all agents belonging to an account.
func (s *Store) BulkCount(ctx context.Context, accountID string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT agent_name, COUNT(*) FROM agent_hearts
		WHERE account_id = $1
		GROUP BY agent_name`,
		accountID,
	)
	if err != nil {
		return nil, fmt.Errorf("bulk count hearts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	counts := make(map[string]int)
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("scan heart count: %w", err)
		}
		counts[name] = count
	}
	return counts, rows.Err()
}
