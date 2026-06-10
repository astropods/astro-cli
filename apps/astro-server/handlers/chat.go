// Deployment chat HTTP handlers — platform API for conversation history.
// Any authenticated client (web, CLI, etc.) uses these routes; messaging proxy is separate.
//
// TODO: Back deployment chat history with Langfuse traces (tagged by conversation_id)
// instead of astro-server Postgres. See docs/04-guides/deployment-chat.md.
package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	chatMaxTitleRunes           = 200
	chatMaxMessageContentRunes  = 128_000
	chatMaxMessagesPerThread    = 1000
	chatMaxGetConversationLimit = chatMaxMessagesPerThread
)

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
	if utf8.RuneCountInString(m.Content) > chatMaxMessageContentRunes {
		return errInvalid("message content too long")
	}
	return nil
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
func ListDeploymentChatConversations(
	_ *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if _, err := resolveDeployment(c, deployStore, accountStore); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		_ = user
		c.JSON(http.StatusOK, ListChatConversationsResponse{Conversations: []ChatConversationSummaryResponse{}})
	}
}

// GetDeploymentChatConversation handles GET /api/v1/deployments/:id/chat/conversations/:conversationId.
func GetDeploymentChatConversation(
	_ *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if _, err := resolveDeployment(c, deployStore, accountStore); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}

		convID, ok := parseConversationID(c.Param("conversationId"))
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}

		if _, err := parseConversationPage(c); err != nil {
			if inv, ok := chatInvalidFromErr(err); ok {
				c.JSON(http.StatusBadRequest, gin.H{"error": inv.Error()})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pagination"})
			return
		}

		_ = user
		c.JSON(http.StatusOK, GetChatConversationResponse{
			ConversationID:     convID,
			Title:              "New conversation",
			UpdatedAt:          time.Now().UTC(),
			Messages:           []ChatMessageResponse{},
			AssistantStreaming: false,
		})
	}
}

// UpsertDeploymentChatConversation handles PUT /api/v1/deployments/:id/chat/conversations/:conversationId.
func UpsertDeploymentChatConversation(
	_ *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if _, err := resolveDeployment(c, deployStore, accountStore); err != nil {
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

		_ = user
		c.JSON(http.StatusOK, gin.H{"conversation_id": convID, "title": title})
	}
}

// ReplaceDeploymentChatMessages handles PUT /api/v1/deployments/:id/chat/conversations/:conversationId/messages.
func ReplaceDeploymentChatMessages(
	_ *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if _, err := resolveDeployment(c, deployStore, accountStore); err != nil {
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

		if len(input.Messages) > chatMaxMessagesPerThread {
			c.JSON(http.StatusBadRequest, gin.H{"error": "too many messages"})
			return
		}

		for _, m := range input.Messages {
			if err := validateChatMessage(m); err != nil {
				if inv, ok := chatInvalidFromErr(err); ok {
					c.JSON(http.StatusBadRequest, gin.H{"error": inv.Error()})
					return
				}
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid message"})
				return
			}
		}

		_ = user
		_ = convID
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// AppendDeploymentChatMessage handles POST /api/v1/deployments/:id/chat/conversations/:conversationId/messages.
func AppendDeploymentChatMessage(
	_ *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		if _, err := resolveDeployment(c, deployStore, accountStore); err != nil {
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

		_ = user
		_ = convID
		c.Status(http.StatusCreated)
	}
}
