package chatstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/google/uuid"
)

// MaxMessageContentRunes is the per-message content limit for deployment chat API.
const MaxMessageContentRunes = 128_000

// StreamPersistMaxAccumBytes bounds in-memory SSE accumulation (generous UTF-8 estimate).
const StreamPersistMaxAccumBytes = MaxMessageContentRunes * 4

// MaxMessagesPerConversation caps messages per thread.
const MaxMessagesPerConversation = 1000

// ActiveAssistantStreamWindow bounds how long a stream-active marker blocks sends.
const ActiveAssistantStreamWindow = 15 * time.Minute

// ErrConversationNotFound is returned when the conversation is missing or not owned.
var ErrConversationNotFound = errors.New("conversation not found")

// ErrConversationIDConflict is returned when a conversation id is owned elsewhere.
var ErrConversationIDConflict = errors.New("conversation id already in use")

// ErrActiveAssistantStream is returned when sends are blocked during SSE persistence.
var ErrActiveAssistantStream = errors.New("assistant stream in progress")

// ErrMessageLimitReached is returned when append would exceed MaxMessagesPerConversation.
var ErrMessageLimitReached = errors.New("conversation message limit reached")

// Message is one chat turn row.
type Message struct {
	ID      string
	Role    string
	Content string
	Seq     int
}

func conversationAdvisoryLockID(deploymentID, conversationID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(deploymentID + ":" + conversationID))
	return int64(h.Sum64()) //nolint:gosec // advisory lock key, not security-sensitive
}

func streamWriteBlocked(activeAt sql.NullTime) bool {
	if !activeAt.Valid {
		return false
	}
	return time.Since(activeAt.Time) < ActiveAssistantStreamWindow
}

// AssistantStreamActiveFrom reports whether a stored stream marker is still active.
func AssistantStreamActiveFrom(activeAt sql.NullTime) bool {
	return streamWriteBlocked(activeAt)
}

// ListMessages returns the full ordered thread for one conversation.
func (s *Store) ListMessages(deploymentID, conversationID string) ([]Message, error) {
	rows, err := s.db.Query(`
		SELECT id::text, role, content, seq
		FROM deployment_chat_messages
		WHERE deployment_id = $1 AND conversation_id = $2
		ORDER BY seq ASC`,
		deploymentID, conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("chatstore list messages: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	out := make([]Message, 0, 32)
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.Seq); err != nil {
			return nil, fmt.Errorf("chatstore list messages scan: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chatstore list messages rows: %w", err)
	}
	return out, nil
}

// AssistantStreamActive reports whether the messaging proxy is persisting a reply.
func (s *Store) AssistantStreamActive(deploymentID, conversationID string) (bool, error) {
	var activeAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT assistant_stream_active_at
		FROM deployment_chat_conversations
		WHERE deployment_id = $1 AND conversation_id = $2 AND archived_at IS NULL`,
		deploymentID, conversationID,
	).Scan(&activeAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("chatstore assistant stream active: %w", err)
	}
	return streamWriteBlocked(activeAt), nil
}

// AppendUserMessage creates the conversation metadata row when missing, then appends
// the user message in one locked transaction. Used by the messaging proxy on send.
func (s *Store) AppendUserMessage(
	ctx context.Context,
	accountID, deploymentID, userID, conversationID, title string,
	msg Message,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, conversationAdvisoryLockID(deploymentID, conversationID)); err != nil {
		return err
	}

	var streamActiveAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT assistant_stream_active_at
		FROM deployment_chat_conversations
		WHERE deployment_id = $1 AND conversation_id = $2 AND user_id = $3 AND archived_at IS NULL`,
		deploymentID, conversationID, userID,
	).Scan(&streamActiveAt)
	if errors.Is(err, sql.ErrNoRows) {
		var existingUser string
		ownerErr := tx.QueryRowContext(ctx, `
			SELECT user_id FROM deployment_chat_conversations
			WHERE deployment_id = $1 AND conversation_id = $2 AND archived_at IS NULL`,
			deploymentID, conversationID,
		).Scan(&existingUser)
		if ownerErr == nil && existingUser != userID {
			return ErrConversationIDConflict
		}
		if ownerErr != nil && !errors.Is(ownerErr, sql.ErrNoRows) {
			return ownerErr
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO deployment_chat_conversations
				(deployment_id, conversation_id, account_id, user_id, title)
			VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5, ''), ''))`,
			deploymentID, conversationID, accountID, userID, title,
		)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if streamWriteBlocked(streamActiveAt) {
		return ErrActiveAssistantStream
	}

	var nextSeq int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(seq), 0) + 1
		FROM deployment_chat_messages
		WHERE deployment_id = $1 AND conversation_id = $2`,
		deploymentID, conversationID,
	).Scan(&nextSeq)
	if err != nil {
		return err
	}
	if nextSeq > MaxMessagesPerConversation {
		return ErrMessageLimitReached
	}

	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO deployment_chat_messages (id, deployment_id, conversation_id, role, content, seq)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		msg.ID, deploymentID, conversationID, msg.Role, msg.Content, nextSeq,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE deployment_chat_conversations
		SET updated_at = now(), archived_at = NULL
		WHERE deployment_id = $1 AND conversation_id = $2 AND user_id = $3`,
		deploymentID, conversationID, userID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpsertAssistantProgress inserts or updates the in-flight assistant message for a
// turn under a per-conversation advisory lock.
func (s *Store) UpsertAssistantProgress(ctx context.Context, deploymentID, userID, conversationID, content string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, conversationAdvisoryLockID(deploymentID, conversationID)); err != nil {
		return "", err
	}

	var ownerUser string
	err = tx.QueryRowContext(ctx, `
		SELECT user_id FROM deployment_chat_conversations
		WHERE deployment_id = $1 AND conversation_id = $2 AND archived_at IS NULL`,
		deploymentID, conversationID,
	).Scan(&ownerUser)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrConversationNotFound
	}
	if err != nil {
		return "", err
	}
	if ownerUser != userID {
		return "", ErrConversationNotFound
	}

	var lastID, lastRole string
	var lastSeq int
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, role, COALESCE(seq, 0)
		FROM deployment_chat_messages
		WHERE deployment_id = $1 AND conversation_id = $2
		ORDER BY seq DESC
		LIMIT 1`,
		deploymentID, conversationID,
	).Scan(&lastID, &lastRole, &lastSeq)
	if errors.Is(err, sql.ErrNoRows) {
		lastSeq = 0
	} else if err != nil {
		return "", err
	}

	if lastRole == "assistant" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE deployment_chat_messages SET content = $1 WHERE id = $2`,
			content, lastID,
		); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE deployment_chat_conversations SET updated_at = now()
			WHERE deployment_id = $1 AND conversation_id = $2`,
			deploymentID, conversationID,
		); err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return lastID, nil
	}

	messageID := uuid.NewString()
	nextSeq := lastSeq + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO deployment_chat_messages (id, deployment_id, conversation_id, role, content, seq)
		VALUES ($1, $2, $3, 'assistant', $4, $5)`,
		messageID, deploymentID, conversationID, content, nextSeq,
	); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE deployment_chat_conversations SET updated_at = now()
		WHERE deployment_id = $1 AND conversation_id = $2`,
		deploymentID, conversationID,
	); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return messageID, nil
}

// SetAssistantStreamActive marks whether the messaging proxy is consuming an assistant SSE stream.
func (s *Store) SetAssistantStreamActive(ctx context.Context, deploymentID, userID, conversationID string, active bool) error {
	var activeAt any
	if active {
		activeAt = time.Now()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE deployment_chat_conversations
		SET assistant_stream_active_at = $1
		WHERE deployment_id = $2 AND conversation_id = $3 AND user_id = $4 AND archived_at IS NULL`,
		activeAt, deploymentID, conversationID, userID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConversationNotFound
	}
	return nil
}
