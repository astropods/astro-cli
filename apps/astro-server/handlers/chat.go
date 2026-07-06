// Deployment chat HTTP handlers — platform API for conversation history.
// Any authenticated client (web, CLI, etc.) uses these routes; messaging proxy is separate.
//
// Storage is split by sensitivity: conversation *metadata* (the sidebar — list,
// title, recency, soft-delete) lives in astro Postgres (chatstore), keyed by the
// opaque WorkOS user id with no message bodies. Message *content* is written by the
// messaging proxy (send + SSE) and hydrated on read from Langfuse traces and/or
// Postgres. See docs/04-guides/deployment-chat.md.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/chatstore"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// chatDefaultSessionTraces is the default Langfuse trace fetch for the latest
// page of a conversation (≈ one message per trace, two roles per trace).
const chatDefaultSessionTraces = 50

// chatMaxSessionTraces caps Langfuse fetches when loading older history.
const chatMaxSessionTraces = 500

const (
	chatDefaultConversationLimit = 100
	chatMaxTitleRunes            = 200
	chatMaxGetConversationLimit  = 1000
)

type ChatConversationSummaryResponse struct {
	ConversationID     string    `json:"conversation_id"`
	Title              string    `json:"title"`
	UpdatedAt          time.Time `json:"updated_at"`
	AssistantStreaming bool      `json:"assistant_streaming,omitempty"`
}

type ChatMessageResponse struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ListChatConversationsResponse struct {
	Conversations []ChatConversationSummaryResponse `json:"conversations"`
}

type GetChatConversationResponse struct {
	ConversationID string                `json:"conversation_id"`
	Title          string                `json:"title"`
	UpdatedAt      time.Time             `json:"updated_at"`
	Messages       []ChatMessageResponse `json:"messages"`
	// AssistantStreaming is true while the messaging proxy is persisting an
	// assistant reply — the server-authoritative "turn in flight" signal.
	AssistantStreaming bool `json:"assistant_streaming"`
	HasMore            bool `json:"has_more,omitempty"`
	OldestSeq          int  `json:"oldest_seq,omitempty"`
}

type UpsertChatConversationInput struct {
	Title string `json:"title"`
}

type chatConversationPage struct {
	Limit     int
	BeforeSeq int
}

func parseConversationID(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return "", false
	}
	return id.String(), true
}

type chatInvalidError struct{ msg string }

func (e chatInvalidError) Error() string { return e.msg }

func errInvalid(msg string) error { return chatInvalidError{msg: msg} }

func parseConversationPage(c *gin.Context) (chatConversationPage, error) {
	page := chatConversationPage{}
	rawLimit := strings.TrimSpace(c.Query("limit"))
	if rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 1 || limit > chatMaxGetConversationLimit {
			return page, errInvalid("invalid limit")
		}
		page.Limit = limit
	}
	rawBefore := strings.TrimSpace(c.Query("before_seq"))
	if rawBefore != "" {
		beforeSeq, err := strconv.Atoi(rawBefore)
		if err != nil || beforeSeq < 1 {
			return page, errInvalid("invalid before_seq")
		}
		page.BeforeSeq = beforeSeq
	}
	return page, nil
}

func chatInvalidFromErr(err error) (chatInvalidError, bool) {
	var inv chatInvalidError
	return inv, errors.As(err, &inv)
}

// ListDeploymentChatConversations handles GET /api/v1/deployments/:id/chat/conversations.
// Returns the authenticated user's active conversations for this deployment,
// most-recent first. Served entirely from metadata (no Langfuse call).
func ListDeploymentChatConversations(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	chatStore *chatstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		convs, err := chatStore.ListByUser(dep.ID, user.ID)
		if err != nil {
			log.Error("Failed to list chat conversations", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list conversations"})
			return
		}

		out := make([]ChatConversationSummaryResponse, 0, len(convs))
		for _, conv := range convs {
			out = append(out, ChatConversationSummaryResponse{
				ConversationID:     conv.ConversationID,
				Title:              conv.Title,
				UpdatedAt:          conv.UpdatedAt,
				AssistantStreaming: chatstore.AssistantStreamActiveFrom(conv.AssistantStreamActiveAt),
			})
		}
		c.JSON(http.StatusOK, ListChatConversationsResponse{Conversations: out})
	}
}

// GetDeploymentChatConversation handles GET /api/v1/deployments/:id/chat/conversations/:conversationId.
// Verifies the conversation belongs to the authenticated user (metadata row),
// then hydrates the message thread from Langfuse traces keyed by session id.
func GetDeploymentChatConversation(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	chatStore *chatstore.Store,
	langfuseStore *langfuse.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		convID, ok := parseConversationID(c.Param("conversationId"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}

		page, err := parseConversationPage(c)
		if err != nil {
			if inv, ok := chatInvalidFromErr(err); ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": inv.Error()})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pagination"})
			return
		}

		// Ownership: only the user who owns the metadata row may read it. This is
		// the authoritative per-user scope on top of account membership.
		conv, err := chatStore.Get(dep.ID, convID)
		if err != nil {
			log.Error("Failed to load chat conversation", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load conversation"})
			return
		}
		if conv == nil || conv.UserID != user.ID {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}

		thread := hydrateConversationMessages(
			c.Request.Context(), log, cfg, langfuseStore, chatStore, dep, user.ID, convID, page,
		)
		messages, hasMore, oldestSeq := paginateConversationMessages(thread.messages, page, thread.truncated)

		assistantStreaming := false
		if active, err := chatStore.AssistantStreamActive(dep.ID, convID); err != nil {
			log.Warn("Failed to check assistant stream state",
				"deployment_id", dep.ID, "conversation_id", convID, "error", err)
		} else {
			assistantStreaming = active
		}

		c.JSON(http.StatusOK, GetChatConversationResponse{
			ConversationID:     convID,
			Title:              conv.Title,
			UpdatedAt:          conv.UpdatedAt,
			Messages:           messages,
			AssistantStreaming: assistantStreaming,
			HasMore:            hasMore,
			OldestSeq:          oldestSeq,
		})
	}
}

type hydratedThread struct {
	messages  []ChatMessageResponse
	truncated bool
}

// hydrateConversationMessages reconstructs the chat thread. Langfuse traces are
// preferred when present; otherwise messages are read from the messaging-proxy
// persistence layer in Postgres (primary in dev when OTEL export is off).
func hydrateConversationMessages(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	chatStore *chatstore.Store,
	dep *deploymentstore.Deployment,
	userID, conversationID string,
	page chatConversationPage,
) hydratedThread {
	langfuseMsgs, langfuseTruncated := hydrateFromLangfuse(
		ctx, log, cfg, langfuseStore, dep, userID, conversationID, page,
	)
	postgresMsgs := hydrateFromChatStore(log, chatStore, dep.ID, conversationID)
	selected := selectLongerHydratedThread(langfuseMsgs, postgresMsgs)
	truncated := langfuseTruncated && len(langfuseMsgs) >= len(postgresMsgs)
	return hydratedThread{messages: selected, truncated: truncated}
}

// selectLongerHydratedThread picks the more complete thread when Langfuse traces
// lag behind messaging-proxy Postgres persistence (common during live turns).
func selectLongerHydratedThread(langfuseMsgs, postgresMsgs []ChatMessageResponse) []ChatMessageResponse {
	if len(langfuseMsgs) == 0 {
		return postgresMsgs
	}
	if len(postgresMsgs) > len(langfuseMsgs) {
		return postgresMsgs
	}
	return langfuseMsgs
}

func paginateConversationMessages(
	all []ChatMessageResponse,
	page chatConversationPage,
	sourceTruncated bool,
) (messages []ChatMessageResponse, hasMore bool, oldestSeq int) {
	limit := page.Limit
	if limit == 0 {
		limit = chatDefaultConversationLimit
	}
	n := len(all)
	if n == 0 {
		return nil, false, 0
	}

	if page.BeforeSeq > 0 {
		end := page.BeforeSeq - 1
		if end < 1 {
			return nil, false, 0
		}
		start := end - limit + 1
		if start < 1 {
			start = 1
		}
		return all[start-1 : end], start > 1, start
	}

	if n <= limit {
		return all, sourceTruncated, 1
	}
	start := n - limit
	return all[start:], true, start + 1
}

func langfuseTraceLimit(page chatConversationPage) int {
	if page.BeforeSeq > 0 {
		return chatMaxSessionTraces
	}
	msgLimit := page.Limit
	if msgLimit == 0 {
		msgLimit = chatDefaultConversationLimit
	}
	traces := (msgLimit + 1) / 2
	if traces < chatDefaultSessionTraces {
		traces = chatDefaultSessionTraces
	}
	if traces > chatMaxSessionTraces {
		traces = chatMaxSessionTraces
	}
	return traces
}

func langfuseHydrationCacheKey(deploymentID, userID, conversationID string) string {
	return deploymentID + ":" + userID + ":" + conversationID
}

func hydrateFromLangfuse(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	dep *deploymentstore.Deployment,
	userID, conversationID string,
	page chatConversationPage,
) ([]ChatMessageResponse, bool) {
	cacheKey := langfuseHydrationCacheKey(dep.ID, userID, conversationID)
	if page.BeforeSeq == 0 {
		if cached, truncated, ok := chatLangfuseHydrationCache.get(cacheKey); ok {
			return cached, truncated
		}
	}

	creds, err := langfuseStore.Get(dep.AccountID)
	if err != nil || creds == nil {
		log.Debug("Langfuse not configured; skipping trace hydration",
			"deployment_id", dep.ID, "error", err)
		return nil, false
	}

	traceLimit := langfuseTraceLimit(page)
	orderBy := "timestamp.desc"
	if page.BeforeSeq > 0 {
		orderBy = "timestamp.asc"
	}

	client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)
	resp, err := client.GetSessionTraces(ctx, dep.ID, userID, conversationID, traceLimit, orderBy)
	if err != nil {
		log.Warn("Failed to hydrate chat history from Langfuse",
			"deployment_id", dep.ID, "conversation_id", conversationID, "error", err)
		return nil, false
	}

	traces := resp.Data
	if orderBy == "timestamp.desc" {
		sort.SliceStable(traces, func(i, j int) bool {
			return traces[i].CreatedAt < traces[j].CreatedAt
		})
	}

	messages := tracesToChatMessages(traces)
	truncated := resp.Meta.TotalItems > len(traces) || len(traces) >= traceLimit

	if page.BeforeSeq == 0 {
		chatLangfuseHydrationCache.set(cacheKey, messages, truncated)
	}
	return messages, truncated
}

func tracesToChatMessages(traces []langfuse.Trace) []ChatMessageResponse {
	messages := make([]ChatMessageResponse, 0, len(traces)*2)
	for _, t := range traces {
		if userText := traceContentText(t.Input); userText != "" {
			messages = append(messages, ChatMessageResponse{
				ID:      t.ID + "-u",
				Role:    "user",
				Content: userText,
			})
		}
		if assistantText := traceContentText(t.Output); assistantText != "" {
			messages = append(messages, ChatMessageResponse{
				ID:      t.ID + "-a",
				Role:    "assistant",
				Content: assistantText,
			})
		}
	}
	return messages
}

func hydrateFromChatStore(
	log *logger.Logger,
	chatStore *chatstore.Store,
	deploymentID, conversationID string,
) []ChatMessageResponse {
	stored, err := chatStore.ListMessages(deploymentID, conversationID)
	if err != nil {
		log.Warn("Failed to hydrate chat history from Postgres",
			"deployment_id", deploymentID, "conversation_id", conversationID, "error", err)
		return []ChatMessageResponse{}
	}
	out := make([]ChatMessageResponse, 0, len(stored))
	for _, m := range stored {
		out = append(out, ChatMessageResponse{
			ID:      m.ID,
			Role:    m.Role,
			Content: m.Content,
		})
	}
	return out
}

// traceContentText best-effort extracts a display string from a Langfuse trace
// input/output value, which may be a plain string, a message object, or a list
// of message objects depending on the agent framework.
func traceContentText(v any) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(val)
	case map[string]any:
		// A stopped/cancelled turn records a control marker as the trace output
		// (e.g. {"status":"aborted","reason":"abort"}) rather than real content.
		// Treat it as empty so it is dropped from the Langfuse thread and the
		// persisted partial (chatstore) wins on hydration, instead of surfacing
		// the raw marker as the assistant message.
		if isAbortMarker(val) {
			return ""
		}
		for _, key := range []string{"content", "text", "message", "output", "value", "response"} {
			if s, ok := val[key].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		// Some frameworks nest the last message under "messages".
		if msgs, ok := val["messages"].([]any); ok {
			if s := lastMessageText(msgs); s != "" {
				return s
			}
		}
		return jsonFallback(val)
	case []any:
		if s := lastMessageText(val); s != "" {
			return s
		}
		return jsonFallback(val)
	default:
		return jsonFallback(val)
	}
}

// lastMessageText pulls the content of the last string-bearing entry from a
// slice of message-like values (objects with a "content"/"text" field, or bare
// strings).
func lastMessageText(items []any) string {
	for i := len(items) - 1; i >= 0; i-- {
		switch item := items[i].(type) {
		case string:
			if s := strings.TrimSpace(item); s != "" {
				return s
			}
		case map[string]any:
			for _, key := range []string{"content", "text"} {
				if s, ok := item[key].(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

// isAbortMarker reports whether a trace value is a generation-abort control
// object (recorded when a turn is stopped) rather than real message content.
//
// It matches only the exact marker shape — BOTH status=="aborted" AND
// reason=="abort" — so a legitimate structured message that merely carries a
// "status" or "reason" field on its own isn't mistaken for a marker and
// dropped from hydration.
func isAbortMarker(m map[string]any) bool {
	status, _ := m["status"].(string)
	reason, _ := m["reason"].(string)
	return strings.EqualFold(status, "aborted") && strings.EqualFold(reason, "abort")
}

func jsonFallback(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if s == "null" || s == `""` || s == "{}" || s == "[]" {
		return ""
	}
	return s
}

// UpsertDeploymentChatConversation handles PUT /api/v1/deployments/:id/chat/conversations/:conversationId.
// Creates the conversation row (on first send / "New conversation"), renames it
// (non-empty title), or just bumps recency (empty title = touch). The opaque
// WorkOS user id is the only identity persisted.
func UpsertDeploymentChatConversation(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	chatStore *chatstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		convID, ok := parseConversationID(c.Param("conversationId"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}

		var input UpsertChatConversationInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		title := strings.TrimSpace(input.Title)
		if utf8.RuneCountInString(title) > chatMaxTitleRunes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title too long"})
			return
		}

		if err := chatStore.Upsert(dep.ID, convID, dep.AccountID, user.ID, title); err != nil {
			log.Error("Failed to upsert chat conversation", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save conversation"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"conversation_id": convID, "title": title})
	}
}

// DeleteDeploymentChatConversation handles DELETE /api/v1/deployments/:id/chat/conversations/:conversationId.
// Soft-deletes the conversation for the owning user. Langfuse traces are left
// intact (purged with the account); the thread simply disappears from the list
// and subsequent reads 404.
func DeleteDeploymentChatConversation(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	chatStore *chatstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		dep, err := resolveDeployment(c, deployStore, accountStore)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		convID, ok := parseConversationID(c.Param("conversationId"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}

		deleted, err := chatStore.SoftDelete(dep.ID, convID, user.ID)
		if err != nil {
			log.Error("Failed to delete chat conversation", "error", err, "deployment_id", dep.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete conversation"})
			return
		}
		if !deleted {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}

		c.Status(http.StatusNoContent)
	}
}
