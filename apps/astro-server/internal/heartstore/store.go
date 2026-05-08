// Package heartstore manages agent heart (like) records in PostgreSQL.
package heartstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
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

// HeartedAgent is a blueprint the given user has hearted, with its current heart count.
type HeartedAgent struct {
	Account      string           `json:"account"`
	Name         string           `json:"name"`
	Visibility   string           `json:"visibility"`
	AvatarColors *json.RawMessage `json:"avatar_colors,omitempty"`
	HeartCount   int              `json:"heart_count"`
	DeployCount  int64            `json:"deploy_count"`
	HeartedAt    time.Time        `json:"hearted_at"`
	Description  string           `json:"description,omitempty"`
}

// ListHearted returns blueprints hearted by the given user, ordered by hearted_at desc.
// cursor is an RFC3339 hearted_at timestamp for pagination; pass "" for the first page.
func (s *Store) ListHearted(ctx context.Context, userID string, pageSize int, cursor string) ([]HeartedAgent, string, error) {
	var rows *sql.Rows
	var err error

	const heartedQuery = `
		SELECT owner.name, a.name, a.visibility, a.avatar_colors,
		       (SELECT COUNT(*) FROM agent_hearts h2 WHERE h2.account_id = a.account_id AND h2.agent_name = a.name),
		       (SELECT COUNT(*) FROM deployments d WHERE d.account_id = a.account_id AND d.agent_name = a.name),
		       ah.created_at,
		       COALESCE(v.agent_card_json::jsonb ->> 'description', '')
		FROM agent_hearts ah
		JOIN agents a ON a.account_id = ah.account_id AND a.name = ah.agent_name
		JOIN accounts owner ON owner.id = a.account_id
		LEFT JOIN LATERAL (
			SELECT agent_card_json FROM agent_versions
			WHERE account_id = a.account_id AND name = a.name
			ORDER BY published_at DESC LIMIT 1
		) v ON true
		WHERE ah.user_id = $1 AND a.archived_at IS NULL AND a.visibility = 'public'`

	if cursor == "" {
		rows, err = s.db.QueryContext(ctx, heartedQuery+`
			ORDER BY ah.created_at DESC
			LIMIT $2
		`, userID, pageSize+1)
	} else {
		rows, err = s.db.QueryContext(ctx, heartedQuery+`
			AND ah.created_at < $2
			ORDER BY ah.created_at DESC
			LIMIT $3
		`, userID, cursor, pageSize+1)
	}
	if err != nil {
		return nil, "", fmt.Errorf("list hearted: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []HeartedAgent
	for rows.Next() {
		var item HeartedAgent
		var avatarColors []byte
		if err := rows.Scan(&item.Account, &item.Name, &item.Visibility, &avatarColors, &item.HeartCount, &item.DeployCount, &item.HeartedAt, &item.Description); err != nil {
			return nil, "", fmt.Errorf("scan hearted agent: %w", err)
		}
		if avatarColors != nil {
			raw := json.RawMessage(avatarColors)
			item.AvatarColors = &raw
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate hearted agents: %w", err)
	}

	var nextCursor string
	if len(items) > pageSize {
		items = items[:pageSize]
		nextCursor = items[len(items)-1].HeartedAt.UTC().Format(time.RFC3339Nano)
	}

	return items, nextCursor, nil
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
