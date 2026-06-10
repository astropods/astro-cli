// Deployment chat HTTP handlers — platform API for durable conversation history.
// Any authenticated client (web, CLI, etc.) uses these routes; messaging proxy is separate.
// See docs/04-guides/deployment-chat.md.
package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/chatstore"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const chatMaxTitleRunes = 200

func chatConversationNotFound(err error) bool {
	return errors.Is(err, chatstore.ErrConversationNotFound)
}

func chatConversationIDConflict(err error) bool {
	return errors.Is(err, chatstore.ErrConversationIDConflict)
}

func chatActiveAssistantStream(err error) bool {
	return errors.Is(err, chatstore.ErrActiveAssistantStream)
}

func chatMessageLimitReached(err error) bool {
	return errors.Is(err, chatstore.ErrMessageLimitReached)
}

type ChatConversationSummaryResponse struct {
	ConversationID string    `json:"conversation_id"`
	Title          string    `json:"title"`
	UpdatedAt      time.Time `json:"updated_at"`
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

type ReplaceChatMessagesInput struct {
	Messages []ChatMessageResponse `json:"messages"`
}

// AppendChatMessageInput shares the wire shape of ChatMessageResponse.
type AppendChatMessageInput ChatMessageResponse

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

func parseMessageID(raw string) (string, bool) {
	return parseConversationID(raw)
}

func validateChatMessage(m ChatMessageResponse) error {
	if _, ok := parseMessageID(m.ID); !ok {
		return errInvalid("invalid message id")
	}
	role := strings.TrimSpace(m.Role)
	if role != "user" && role != "assistant" {
		return errInvalid("role must be user or assistant")
	}
	if strings.TrimSpace(m.Content) == "" {
		return errInvalid("message content is required")
	}
	if utf8.RuneCountInString(m.Content) > chatstore.MaxMessageContentRunes {
		return errInvalid("message content too long")
	}
	return nil
}

func normalizeChatMessage(m ChatMessageResponse) chatstore.Message {
	id, _ := parseMessageID(m.ID)
	return chatstore.Message{
		ID:      id,
		Role:    strings.TrimSpace(m.Role),
		Content: m.Content,
	}
}

type chatInvalidError struct{ msg string }

func (e chatInvalidError) Error() string { return e.msg }

func errInvalid(msg string) error { return chatInvalidError{msg: msg} }

func parseConversationPage(c *gin.Context) (chatstore.ConversationPage, error) {
	page := chatstore.ConversationPage{}
	rawLimit := strings.TrimSpace(c.Query("limit"))
	if rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 1 || limit > chatstore.MaxGetConversationLimit {
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

		rows, err := chatStore.ListConversations(c.Request.Context(), dep.ID, user.ID)
		if err != nil {
			log.Error("list chat conversations", "deployment", dep.ID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list conversations"})
			return
		}

		out := make([]ChatConversationSummaryResponse, 0, len(rows))
		for _, row := range rows {
			out = append(out, ChatConversationSummaryResponse{
				ConversationID: row.ID,
				Title:          row.Title,
				UpdatedAt:      row.UpdatedAt,
			})
		}
		c.JSON(http.StatusOK, ListChatConversationsResponse{Conversations: out})
	}
}

// GetDeploymentChatConversation handles GET /api/v1/deployments/:id/chat/conversations/:conversationId.
func GetDeploymentChatConversation(
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

		page, err := parseConversationPage(c)
		if err != nil {
			if inv, ok := chatInvalidFromErr(err); ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": inv.Error()})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pagination"})
			return
		}

		conv, err := chatStore.GetConversation(c.Request.Context(), dep.ID, user.ID, convID, page)
		if err != nil {
			log.Error("get chat conversation", "deployment", dep.ID, "conversation", convID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load conversation"})
			return
		}
		if conv == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}

		messages := make([]ChatMessageResponse, 0, len(conv.Messages))
		for _, m := range conv.Messages {
			messages = append(messages, ChatMessageResponse{ID: m.ID, Role: m.Role, Content: m.Content})
		}
		resp := GetChatConversationResponse{
			ConversationID:     conv.ID,
			Title:              conv.Title,
			UpdatedAt:          conv.UpdatedAt,
			Messages:           messages,
			AssistantStreaming: conv.AssistantStreaming,
		}
		if page.Limit > 0 {
			resp.HasMore = conv.HasMore
			resp.OldestSeq = conv.OldestSeq
		}
		c.JSON(http.StatusOK, resp)
	}
}

// UpsertDeploymentChatConversation handles PUT /api/v1/deployments/:id/chat/conversations/:conversationId.
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
		if title == "" {
			title = "New conversation"
		}
		if utf8.RuneCountInString(title) > chatMaxTitleRunes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title too long"})
			return
		}

		if err := chatStore.UpsertConversation(c.Request.Context(), dep.AccountID, dep.ID, user.ID, convID, title, dep.AgentName); err != nil {
			if chatConversationIDConflict(err) {
				c.JSON(http.StatusConflict, gin.H{"error": chatstore.ErrConversationIDConflict.Error()})
				return
			}
			log.Error("upsert chat conversation", "deployment", dep.ID, "conversation", convID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save conversation"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"conversation_id": convID, "title": title})
	}
}

// ReplaceDeploymentChatMessages handles PUT /api/v1/deployments/:id/chat/conversations/:conversationId/messages.
func ReplaceDeploymentChatMessages(
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

		var input ReplaceChatMessagesInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		if len(input.Messages) > chatstore.MaxMessagesPerConversation {
			c.JSON(http.StatusBadRequest, gin.H{"error": "too many messages"})
			return
		}

		msgs := make([]chatstore.Message, 0, len(input.Messages))
		for _, m := range input.Messages {
			if err := validateChatMessage(m); err != nil {
				if inv, ok := chatInvalidFromErr(err); ok {
					c.JSON(http.StatusBadRequest, gin.H{"error": inv.Error()})
					return
				}
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message"})
				return
			}
			msgs = append(msgs, normalizeChatMessage(m))
		}

		if err := chatStore.ReplaceMessages(c.Request.Context(), dep.ID, user.ID, convID, msgs); err != nil {
			if chatConversationNotFound(err) {
				c.JSON(http.StatusNotFound, gin.H{"error": chatstore.ErrConversationNotFound.Error()})
				return
			}
			if chatActiveAssistantStream(err) {
				c.JSON(http.StatusConflict, gin.H{"error": chatstore.ErrActiveAssistantStream.Error()})
				return
			}
			log.Error("replace chat messages", "deployment", dep.ID, "conversation", convID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save messages"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// AppendDeploymentChatMessage handles POST /api/v1/deployments/:id/chat/conversations/:conversationId/messages.
func AppendDeploymentChatMessage(
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

		var input AppendChatMessageInput
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		msg := ChatMessageResponse(input)
		if err := validateChatMessage(msg); err != nil {
			if inv, ok := chatInvalidFromErr(err); ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": inv.Error()})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message"})
			return
		}

		if err := chatStore.AppendMessage(c.Request.Context(), dep.ID, user.ID, convID, normalizeChatMessage(msg)); err != nil {
			if chatConversationNotFound(err) {
				c.JSON(http.StatusNotFound, gin.H{"error": chatstore.ErrConversationNotFound.Error()})
				return
			}
			if chatActiveAssistantStream(err) {
				c.JSON(http.StatusConflict, gin.H{"error": chatstore.ErrActiveAssistantStream.Error()})
				return
			}
			if chatMessageLimitReached(err) {
				c.JSON(http.StatusBadRequest, gin.H{"error": chatstore.ErrMessageLimitReached.Error()})
				return
			}
			log.Error("append chat message", "deployment", dep.ID, "conversation", convID, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save message"})
			return
		}

		c.Status(http.StatusCreated)
	}
}
