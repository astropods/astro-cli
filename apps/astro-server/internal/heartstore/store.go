// Package heartstore manages agent heart (like) records in PostgreSQL.
package heartstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Heart struct {
	AccountID string    `json:"account_id"`
	AgentName string    `json:"agent_name"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Heart adds a heart. Uses ON CONFLICT to make it idempotent.
// Returns true if a new row was inserted, false if already hearted.
func (s *Store) Heart(ctx context.Context, accountID, agentName, userID string) (bool, error) {
	var created bool
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO agent_hearts (account_id, agent_name, user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (account_id, agent_name, user_id) DO NOTHING
		RETURNING true`,
		accountID, agentName, userID,
	).Scan(&created)
	if err == sql.ErrNoRows {
		return false, nil // already hearted
	}
	if err != nil {
		return false, fmt.Errorf("heart agent: %w", err)
	}
	return true, nil
}

// Unheart removes a heart. Returns true if a row was deleted.
func (s *Store) Unheart(ctx context.Context, accountID, agentName, userID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM agent_hearts
		WHERE account_id = $1 AND agent_name = $2 AND user_id = $3`,
		accountID, agentName, userID,
	)
	if err != nil {
		return false, fmt.Errorf("unheart agent: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("unheart rows affected: %w", err)
	}
	return n > 0, nil
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
		)
		SELECT
			EXISTS (SELECT 1 FROM inserted),
			(SELECT COUNT(*) FROM agent_hearts WHERE account_id = $1 AND agent_name = $2)`,
		accountID, agentName, userID,
	).Scan(&hearted, &count)
	if err != nil {
		return false, 0, fmt.Errorf("toggle heart: %w", err)
	}
	return hearted, count, nil
}

// Count returns the total heart count for an agent.
func (s *Store) Count(ctx context.Context, accountID, agentName string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_hearts
		WHERE account_id = $1 AND agent_name = $2`,
		accountID, agentName,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count hearts: %w", err)
	}
	return count, nil
}

// IsHearted checks whether a specific user has hearted an agent.
func (s *Store) IsHearted(ctx context.Context, accountID, agentName, userID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM agent_hearts
			WHERE account_id = $1 AND agent_name = $2 AND user_id = $3
		)`,
		accountID, agentName, userID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check heart: %w", err)
	}
	return exists, nil
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

// BulkIsHearted returns which agents the user has hearted within an account.
func (s *Store) BulkIsHearted(ctx context.Context, accountID, userID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT agent_name FROM agent_hearts
		WHERE account_id = $1 AND user_id = $2`,
		accountID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("bulk is hearted: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	hearted := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan hearted agent: %w", err)
		}
		hearted[name] = true
	}
	return hearted, rows.Err()
}
