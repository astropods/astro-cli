// Package chatstore persists deployment chat history for all Astro API clients.
//
// Schema: sql/astro-server/schema.sql (deployment_chat_conversations, deployment_chat_messages).
// API contract: docs/04-guides/deployment-chat.md.
package chatstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MaxMessageContentRunes is the per-message content limit for deployment chat API.
const MaxMessageContentRunes = 128_000

// StreamPersistMaxAccumBytes bounds in-memory SSE accumulation (generous UTF-8 estimate).
const StreamPersistMaxAccumBytes = MaxMessageContentRunes * 4

// ActiveAssistantStreamWindow bounds how long a stream-active marker blocks ReplaceMessages.
const ActiveAssistantStreamWindow = 15 * time.Minute

// MaxMessagesPerConversation caps ReplaceMessages payload size.
const MaxMessagesPerConversation = 1000

// MaxListConversations bounds ListConversations result size.
const MaxListConversations = 200

// MaxGetConversationLimit caps optional message pagination on GetConversation.
const MaxGetConversationLimit = MaxMessagesPerConversation

// ErrConversationNotFound is returned when the conversation is missing or not owned.
var ErrConversationNotFound = errors.New("conversation not found")

// ErrConversationIDConflict is returned when a conversation id is already owned elsewhere.
var ErrConversationIDConflict = errors.New("conversation id already in use")

// ErrActiveAssistantStream is returned when client writes are blocked during SSE persistence.
var ErrActiveAssistantStream = errors.New("assistant stream in progress")

// ErrMessageLimitReached is returned when append would exceed MaxMessagesPerConversation.
var ErrMessageLimitReached = errors.New("conversation message limit reached")

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

type ConversationSummary struct {
	ID        string
	Title     string
	UpdatedAt time.Time
}

type Message struct {
	ID      string
	Role    string
	Content string
	Seq     int
}

type Conversation struct {
	ID        string
	Title     string
	UpdatedAt time.Time
	Messages  []Message
}

// ConversationPage optional bounds for GetConversation message history.
// Limit 0 returns the full thread (legacy default). Limit > 0 returns at most
// that many messages: the tail when BeforeSeq is 0, or older rows with seq < BeforeSeq.
type ConversationPage struct {
	Limit     int
	BeforeSeq int
}

// ConversationResult is a conversation thread with optional pagination metadata.
type ConversationResult struct {
	Conversation
	HasMore   bool
	OldestSeq int
}

// ListConversations returns summaries for the given deployment and WorkOS user.
func (s *Store) ListConversations(ctx context.Context, deploymentID, userID string) ([]ConversationSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, updated_at
		FROM deployment_chat_conversations
		WHERE deployment_id = $1 AND user_id = $2
		ORDER BY updated_at DESC
		LIMIT $3`,
		deploymentID, userID, MaxListConversations,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	var out []ConversationSummary
	for rows.Next() {
		var c ConversationSummary
		if err := rows.Scan(&c.ID, &c.Title, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetConversation loads a thread when it belongs to deploymentID and userID.
func (s *Store) GetConversation(
	ctx context.Context,
	deploymentID, userID, conversationID string,
	page ConversationPage,
) (*ConversationResult, error) {
	var c Conversation
	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, updated_at
		FROM deployment_chat_conversations
		WHERE id = $1 AND deployment_id = $2 AND user_id = $3`,
		conversationID, deploymentID, userID,
	).Scan(&c.ID, &c.Title, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	messages, hasMore, err := s.loadConversationMessages(ctx, conversationID, page)
	if err != nil {
		return nil, err
	}
	c.Messages = messages

	result := &ConversationResult{Conversation: c, HasMore: hasMore}
	if len(messages) > 0 {
		result.OldestSeq = messages[0].Seq
	}
	return result, nil
}

func (s *Store) loadConversationMessages(
	ctx context.Context,
	conversationID string,
	page ConversationPage,
) ([]Message, bool, error) {
	if page.Limit <= 0 {
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, role, content, seq
			FROM deployment_chat_messages
			WHERE conversation_id = $1
			ORDER BY seq ASC`,
			conversationID,
		)
		if err != nil {
			return nil, false, err
		}
		defer rows.Close() //nolint:errcheck

		raw, err := scanMessages(rows)
		if err != nil {
			return nil, false, err
		}
		return dedupeMessages(raw), false, nil
	}

	var query string
	var args []any
	if page.BeforeSeq <= 0 {
		query = `
			SELECT id, role, content, seq FROM (
				SELECT id, role, content, seq
				FROM deployment_chat_messages
				WHERE conversation_id = $1
				ORDER BY seq DESC
				LIMIT $2
			) t ORDER BY seq ASC`
		args = []any{conversationID, page.Limit}
	} else {
		query = `
			SELECT id, role, content, seq FROM (
				SELECT id, role, content, seq
				FROM deployment_chat_messages
				WHERE conversation_id = $1 AND seq < $2
				ORDER BY seq DESC
				LIMIT $3
			) t ORDER BY seq ASC`
		args = []any{conversationID, page.BeforeSeq, page.Limit}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close() //nolint:errcheck

	raw, err := scanMessages(rows)
	if err != nil {
		return nil, false, err
	}
	messages := dedupeMessages(raw)
	if len(messages) == 0 {
		return messages, false, nil
	}

	var hasMore bool
	err = s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM deployment_chat_messages
			WHERE conversation_id = $1 AND seq < $2
		)`,
		conversationID, messages[0].Seq,
	).Scan(&hasMore)
	if err != nil {
		return nil, false, err
	}
	return messages, hasMore, nil
}

func scanMessages(rows *sql.Rows) ([]Message, error) {
	var raw []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.Seq); err != nil {
			return nil, err
		}
		raw = append(raw, m)
	}
	return raw, rows.Err()
}

// dedupeMessages collapses corrupt persistence artifacts: duplicate seq rows and
// consecutive assistant-only runs (keep the longest content per run).
func dedupeMessages(msgs []Message) []Message {
	if len(msgs) == 0 {
		return msgs
	}

	bySeq := make(map[int]Message, len(msgs))
	seqs := make([]int, 0, len(msgs))
	for _, m := range msgs {
		if existing, seen := bySeq[m.Seq]; !seen || len(m.Content) > len(existing.Content) {
			bySeq[m.Seq] = m
			if !seen {
				seqs = append(seqs, m.Seq)
			}
		}
	}

	sort.Ints(seqs)
	ordered := make([]Message, 0, len(seqs))
	for _, seq := range seqs {
		ordered = append(ordered, bySeq[seq])
	}
	return collapseConsecutiveAssistants(ordered)
}

func collapseConsecutiveAssistants(msgs []Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "assistant" && len(out) > 0 && out[len(out)-1].Role == "assistant" {
			prev := &out[len(out)-1]
			if len(m.Content) > len(prev.Content) {
				*prev = m
			}
			continue
		}
		out = append(out, m)
	}
	return out
}

func conversationAdvisoryLockID(conversationID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(conversationID))
	return int64(h.Sum64()) //nolint:gosec // advisory lock key, not security-sensitive
}

func streamWriteBlocked(streamActiveAt sql.NullTime) bool {
	return streamActiveAt.Valid && time.Since(streamActiveAt.Time) < ActiveAssistantStreamWindow
}

func conversationIDInUse(ctx context.Context, tx *sql.Tx, conversationID string) (bool, error) {
	var one int
	err := tx.QueryRowContext(ctx, `
		SELECT 1 FROM deployment_chat_conversations WHERE id = $1`,
		conversationID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// UpsertConversation creates or updates conversation metadata (any API client may call).
func (s *Store) UpsertConversation(ctx context.Context, accountID, deploymentID, userID, conversationID, title string) error {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO deployment_chat_conversations (id, deployment_id, account_id, user_id, title, updated_at, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			updated_at = NOW()
		WHERE deployment_chat_conversations.deployment_id = $2
		  AND deployment_chat_conversations.user_id = $4`,
		conversationID, deploymentID, accountID, userID, title,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConversationIDConflict
	}
	return nil
}

// AppendMessage adds one message to an existing conversation.
func (s *Store) AppendMessage(ctx context.Context, deploymentID, userID, conversationID string, msg Message) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, conversationAdvisoryLockID(conversationID)); err != nil {
		return err
	}

	var ownerDeployment string
	var streamActiveAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT deployment_id, assistant_stream_active_at
		FROM deployment_chat_conversations
		WHERE id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&ownerDeployment, &streamActiveAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConversationNotFound
	}
	if err != nil {
		return err
	}
	if ownerDeployment != deploymentID {
		return ErrConversationNotFound
	}
	if streamWriteBlocked(streamActiveAt) {
		return ErrActiveAssistantStream
	}

	var nextSeq int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(seq), 0) + 1 FROM deployment_chat_messages WHERE conversation_id = $1`,
		conversationID,
	).Scan(&nextSeq)
	if err != nil {
		return err
	}
	if nextSeq > MaxMessagesPerConversation {
		return ErrMessageLimitReached
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO deployment_chat_messages (id, conversation_id, role, content, seq)
		VALUES ($1, $2, $3, $4, $5)`,
		msg.ID, conversationID, msg.Role, msg.Content, nextSeq,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE deployment_chat_conversations SET updated_at = NOW() WHERE id = $1`,
		conversationID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// AppendUserMessage creates the conversation row when missing, then appends a user
// message in one locked transaction (avoids loading full thread history on send).
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

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, conversationAdvisoryLockID(conversationID)); err != nil {
		return err
	}

	var ownerDeployment string
	var streamActiveAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT deployment_id, assistant_stream_active_at
		FROM deployment_chat_conversations
		WHERE id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&ownerDeployment, &streamActiveAt)
	if errors.Is(err, sql.ErrNoRows) {
		inUse, ierr := conversationIDInUse(ctx, tx, conversationID)
		if ierr != nil {
			return ierr
		}
		if inUse {
			return ErrConversationIDConflict
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO deployment_chat_conversations (id, deployment_id, account_id, user_id, title, updated_at, created_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`,
			conversationID, deploymentID, accountID, userID, title,
		)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		if ownerDeployment != deploymentID {
			return ErrConversationNotFound
		}
		if streamWriteBlocked(streamActiveAt) {
			return ErrActiveAssistantStream
		}
	}

	var nextSeq int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(seq), 0) + 1 FROM deployment_chat_messages WHERE conversation_id = $1`,
		conversationID,
	).Scan(&nextSeq)
	if err != nil {
		return err
	}
	if nextSeq > MaxMessagesPerConversation {
		return ErrMessageLimitReached
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO deployment_chat_messages (id, conversation_id, role, content, seq)
		VALUES ($1, $2, $3, $4, $5)`,
		msg.ID, conversationID, msg.Role, msg.Content, nextSeq,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE deployment_chat_conversations SET updated_at = NOW() WHERE id = $1`,
		conversationID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// UpsertAssistantProgress inserts or updates the in-flight assistant message for a
// turn under a per-conversation advisory lock so concurrent SSE consumers cannot
// append duplicate rows.
func (s *Store) UpsertAssistantProgress(ctx context.Context, deploymentID, userID, conversationID, content string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, conversationAdvisoryLockID(conversationID)); err != nil {
		return "", err
	}

	var ownerDeployment string
	err = tx.QueryRowContext(ctx, `
		SELECT deployment_id FROM deployment_chat_conversations
		WHERE id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&ownerDeployment)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrConversationNotFound
	}
	if err != nil {
		return "", err
	}
	if ownerDeployment != deploymentID {
		return "", ErrConversationNotFound
	}

	var lastID, lastRole string
	var lastSeq int
	err = tx.QueryRowContext(ctx, `
		SELECT id, role, COALESCE(seq, 0)
		FROM deployment_chat_messages
		WHERE conversation_id = $1
		ORDER BY seq DESC
		LIMIT 1`,
		conversationID,
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
			UPDATE deployment_chat_conversations SET updated_at = NOW() WHERE id = $1`,
			conversationID,
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
		INSERT INTO deployment_chat_messages (id, conversation_id, role, content, seq)
		VALUES ($1, $2, 'assistant', $3, $4)`,
		messageID, conversationID, content, nextSeq,
	); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE deployment_chat_conversations SET updated_at = NOW() WHERE id = $1`,
		conversationID,
	); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return messageID, nil
}

// SetAssistantStreamActive marks whether the messaging proxy is consuming an assistant SSE
// stream for this conversation. A non-null timestamp blocks ReplaceMessages until the
// stream ends or ActiveAssistantStreamWindow elapses (crash safety).
func (s *Store) SetAssistantStreamActive(ctx context.Context, deploymentID, userID, conversationID string, active bool) error {
	var activeAt any
	if active {
		activeAt = time.Now()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE deployment_chat_conversations
		SET assistant_stream_active_at = $1
		WHERE id = $2 AND deployment_id = $3 AND user_id = $4`,
		activeAt, conversationID, deploymentID, userID,
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

// ReplaceMessages overwrites the full message list (e.g. after an assistant turn completes).
func (s *Store) ReplaceMessages(ctx context.Context, deploymentID, userID, conversationID string, messages []Message) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, conversationAdvisoryLockID(conversationID)); err != nil {
		return err
	}

	var ownerDeployment string
	var streamActiveAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT deployment_id, assistant_stream_active_at
		FROM deployment_chat_conversations
		WHERE id = $1 AND user_id = $2`,
		conversationID, userID,
	).Scan(&ownerDeployment, &streamActiveAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrConversationNotFound
	}
	if err != nil {
		return err
	}
	if ownerDeployment != deploymentID {
		return ErrConversationNotFound
	}
	if streamActiveAt.Valid && time.Since(streamActiveAt.Time) < ActiveAssistantStreamWindow {
		return ErrActiveAssistantStream
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM deployment_chat_messages WHERE conversation_id = $1`, conversationID); err != nil {
		return err
	}

	if len(messages) > 0 {
		if err := insertMessagesBatch(ctx, tx, conversationID, messages); err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE deployment_chat_conversations SET updated_at = NOW() WHERE id = $1`,
		conversationID,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func insertMessagesBatch(ctx context.Context, tx *sql.Tx, conversationID string, messages []Message) error {
	const batchSize = 100
	for start := 0; start < len(messages); start += batchSize {
		end := start + batchSize
		if end > len(messages) {
			end = len(messages)
		}
		chunk := messages[start:end]
		var sb strings.Builder
		sb.WriteString(`INSERT INTO deployment_chat_messages (id, conversation_id, role, content, seq) VALUES `)
		args := make([]any, 0, len(chunk)*5)
		for i, msg := range chunk {
			if i > 0 {
				sb.WriteString(", ")
			}
			base := i*5 + 1
			fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d)", base, base+1, base+2, base+3, base+4)
			args = append(args, msg.ID, conversationID, msg.Role, msg.Content, start+i+1)
		}
		if _, err := tx.ExecContext(ctx, sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}
