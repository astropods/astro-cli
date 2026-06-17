// Package chatstore persists deployment chat conversation *metadata* — the
// durable sidebar (list, title, recency, soft-delete) for the web chat UI.
//
// It intentionally stores no message bodies and no user PII: rows are keyed by
// (deployment_id, conversation_id) and scoped by the opaque WorkOS user id.
// Message content lives in Langfuse traces keyed by session_id = conversation_id
// and is hydrated separately at read time.
package chatstore

import (
	"database/sql"
	"fmt"
	"time"
)

// Conversation is one row of conversation metadata.
type Conversation struct {
	DeploymentID   string
	ConversationID string
	AccountID      string
	UserID         string
	Title          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Store manages the deployment_chat_conversations table.
type Store struct {
	db *sql.DB
}

// NewStore creates a new chat conversation metadata store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Upsert creates the conversation row if absent or, when it already exists,
// bumps updated_at for recency. The title is only written when a non-empty
// title is supplied, so a recency "touch" (empty title) never clobbers a
// user-set or auto-derived title. Inserts with an empty title fall back to the
// column default (”).
func (s *Store) Upsert(deploymentID, conversationID, accountID, userID, title string) error {
	_, err := s.db.Exec(`
		INSERT INTO deployment_chat_conversations
			(deployment_id, conversation_id, account_id, user_id, title)
		VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5, ''), ''))
		ON CONFLICT (deployment_id, conversation_id) DO UPDATE SET
			title       = COALESCE(NULLIF($5, ''), deployment_chat_conversations.title),
			updated_at  = now(),
			archived_at = NULL
		WHERE deployment_chat_conversations.user_id = $4
	`, deploymentID, conversationID, accountID, userID, title)
	if err != nil {
		return fmt.Errorf("chatstore upsert: %w", err)
	}
	return nil
}

// Touch bumps updated_at without changing the title (recency ordering on send).
func (s *Store) Touch(deploymentID, conversationID, userID string) error {
	_, err := s.db.Exec(`
		UPDATE deployment_chat_conversations
		SET updated_at = now()
		WHERE deployment_id = $1 AND conversation_id = $2 AND user_id = $3
		  AND archived_at IS NULL
	`, deploymentID, conversationID, userID)
	if err != nil {
		return fmt.Errorf("chatstore touch: %w", err)
	}
	return nil
}

// ListByUser returns the active conversations for one (deployment, user),
// most-recently-updated first.
func (s *Store) ListByUser(deploymentID, userID string) ([]Conversation, error) {
	rows, err := s.db.Query(`
		SELECT deployment_id, conversation_id, account_id, user_id, title, created_at, updated_at
		FROM deployment_chat_conversations
		WHERE deployment_id = $1 AND user_id = $2 AND archived_at IS NULL
		ORDER BY updated_at DESC
	`, deploymentID, userID)
	if err != nil {
		return nil, fmt.Errorf("chatstore list: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]Conversation, 0, 16)
	for rows.Next() {
		var conv Conversation
		if err := rows.Scan(
			&conv.DeploymentID, &conv.ConversationID, &conv.AccountID,
			&conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("chatstore list scan: %w", err)
		}
		out = append(out, conv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chatstore list rows: %w", err)
	}
	return out, nil
}

// Get returns one active conversation, or nil if it does not exist (or is
// archived). Used to verify ownership before hydrating messages from Langfuse.
func (s *Store) Get(deploymentID, conversationID string) (*Conversation, error) {
	row := s.db.QueryRow(`
		SELECT deployment_id, conversation_id, account_id, user_id, title, created_at, updated_at
		FROM deployment_chat_conversations
		WHERE deployment_id = $1 AND conversation_id = $2 AND archived_at IS NULL
	`, deploymentID, conversationID)

	var conv Conversation
	err := row.Scan(
		&conv.DeploymentID, &conv.ConversationID, &conv.AccountID,
		&conv.UserID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chatstore get: %w", err)
	}
	return &conv, nil
}

// SoftDelete marks a conversation archived for the owning user. The Langfuse
// traces are left intact (no per-session delete in our integration); they are
// purged with the account. Returns true when a row was archived.
func (s *Store) SoftDelete(deploymentID, conversationID, userID string) (bool, error) {
	res, err := s.db.Exec(`
		UPDATE deployment_chat_conversations
		SET archived_at = now()
		WHERE deployment_id = $1 AND conversation_id = $2 AND user_id = $3
		  AND archived_at IS NULL
	`, deploymentID, conversationID, userID)
	if err != nil {
		return false, fmt.Errorf("chatstore soft delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("chatstore soft delete rows: %w", err)
	}
	return n > 0, nil
}
