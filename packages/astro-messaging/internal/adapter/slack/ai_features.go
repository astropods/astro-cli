package slack

import (
	"context"
	"fmt"
	"log"

	pb "github.com/astro/messaging/pkg/gen/astro/messaging/v1"
	"github.com/slack-go/slack"
)

// HandleAgentResponse processes agent responses and routes them to the appropriate handler
func (a *SlackAdapter) HandleAgentResponse(ctx context.Context, response *pb.AgentResponse) error {
	if response == nil {
		return fmt.Errorf("nil response")
	}

	switch payload := response.Payload.(type) {
	case *pb.AgentResponse_Status:
		return a.setSlackStatus(ctx, response.ConversationId, payload.Status)
	case *pb.AgentResponse_Content:
		return a.handleContentChunk(ctx, response.ConversationId, payload.Content)
	case *pb.AgentResponse_Prompts:
		return a.setSlackPrompts(ctx, response.ConversationId, payload.Prompts)
	case *pb.AgentResponse_ThreadMetadata:
		return a.handleThreadMetadata(ctx, payload.ThreadMetadata)
	case *pb.AgentResponse_Error:
		return a.handleError(ctx, response.ConversationId, payload.Error)
	default:
		log.Printf("[Slack] Unknown response payload type: %T", payload)
		return nil
	}
}

// setSlackStatus updates the thread status using Slack AI APIs
func (a *SlackAdapter) setSlackStatus(ctx context.Context, conversationID string, status *pb.StatusUpdate) error {
	if status == nil {
		return fmt.Errorf("nil status update")
	}

	// Parse conversationID to get channel and thread
	channelID, threadTS, err := a.parseConversationID(conversationID)
	if err != nil {
		return fmt.Errorf("failed to parse conversation ID: %w", err)
	}

	// Map proto status to Slack status message
	statusMessage := a.mapStatusToMessage(status)
	emoji := status.Emoji
	if emoji == "" {
		emoji = a.getDefaultEmojiForStatus(status.Status)
	}

	// Apply rate limiting
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Call Slack AI API to set thread status
	if err := a.aiClient.SetThreadStatus(ctx, channelID, threadTS, statusMessage, emoji); err != nil {
		return fmt.Errorf("failed to set Slack status: %w", err)
	}

	log.Printf("[Slack] Set status for %s: %s %s", conversationID, emoji, statusMessage)
	return nil
}

// setSlackPrompts sets suggested prompts using Slack AI APIs
func (a *SlackAdapter) setSlackPrompts(ctx context.Context, conversationID string, prompts *pb.SuggestedPrompts) error {
	if prompts == nil || len(prompts.Prompts) == 0 {
		return fmt.Errorf("nil or empty prompts")
	}

	// Parse conversationID to get channel and thread
	channelID, threadTS, err := a.parseConversationID(conversationID)
	if err != nil {
		return fmt.Errorf("failed to parse conversation ID: %w", err)
	}

	// Map proto prompts to Slack prompts format
	slackPrompts := make([]SuggestedPrompt, 0, len(prompts.Prompts))
	for _, prompt := range prompts.Prompts {
		slackPrompts = append(slackPrompts, SuggestedPrompt{
			Title:   prompt.Title,
			Message: prompt.Message,
		})
	}

	// Apply rate limiting
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Call Slack AI API to set suggested prompts
	if err := a.aiClient.SetSuggestedPrompts(ctx, channelID, threadTS, slackPrompts); err != nil {
		return fmt.Errorf("failed to set Slack prompts: %w", err)
	}

	log.Printf("[Slack] Set %d suggested prompts for %s", len(prompts.Prompts), conversationID)
	return nil
}

// handleContentChunk sends content to Slack
func (a *SlackAdapter) handleContentChunk(ctx context.Context, conversationID string, content *pb.ContentChunk) error {
	if content == nil {
		return fmt.Errorf("nil content chunk")
	}

	// Parse conversationID to get channel and thread
	channelID, threadTS, err := a.parseConversationID(conversationID)
	if err != nil {
		return fmt.Errorf("failed to parse conversation ID: %w", err)
	}

	// Apply rate limiting
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	// For Slack, we don't support streaming (due to 3-second rate limit)
	// We only send complete messages
	// Only process END or REPLACE chunks, ignore START and DELTA
	if content.Type != pb.ContentChunk_END && content.Type != pb.ContentChunk_REPLACE {
		log.Printf("[Slack] Ignoring non-final content chunk (streaming not supported)")
		return nil
	}

	// Send message to Slack
	_, _, err = a.client.PostMessageContext(ctx, channelID, slack.MsgOptionText(content.Content, false), slack.MsgOptionTS(threadTS))
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	log.Printf("[Slack] Sent content to %s (%d chars)", conversationID, len(content.Content))
	return nil
}

// handleThreadMetadata handles thread metadata updates
func (a *SlackAdapter) handleThreadMetadata(ctx context.Context, metadata *pb.ThreadMetadata) error {
	if metadata == nil {
		return fmt.Errorf("nil thread metadata")
	}

	// Store thread metadata in local store if needed
	log.Printf("[Slack] Thread metadata update: %s (title: %s)", metadata.ThreadId, metadata.Title)

	// For Slack, thread metadata is mostly informational
	// We could update channel topic or similar, but for now we just log it
	return nil
}

// handleError handles error responses from the agent
func (a *SlackAdapter) handleError(ctx context.Context, conversationID string, errorResponse *pb.ErrorResponse) error {
	if errorResponse == nil {
		return fmt.Errorf("nil error response")
	}

	// Parse conversationID to get channel and thread
	channelID, threadTS, err := a.parseConversationID(conversationID)
	if err != nil {
		return fmt.Errorf("failed to parse conversation ID: %w", err)
	}

	// Apply rate limiting
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limit wait failed: %w", err)
	}

	// Send error message to user
	errorMessage := fmt.Sprintf(":warning: Error: %s", errorResponse.Message)
	if errorResponse.Code != pb.ErrorResponse_ERROR_CODE_UNSPECIFIED {
		errorMessage += fmt.Sprintf(" (code: %s)", errorResponse.Code.String())
	}

	_, _, err = a.client.PostMessageContext(ctx, channelID, slack.MsgOptionText(errorMessage, false), slack.MsgOptionTS(threadTS))
	if err != nil {
		return fmt.Errorf("failed to send error message: %w", err)
	}

	log.Printf("[Slack] Sent error message to %s: %s", conversationID, errorResponse.Message)
	return nil
}

// Helper functions

// parseConversationID parses a conversation ID into channel ID and thread timestamp
func (a *SlackAdapter) parseConversationID(conversationID string) (channelID string, threadTS string, err error) {
	channelID, threadTS = ParseMessageID(conversationID)
	if channelID == "" {
		return "", "", fmt.Errorf("invalid conversation ID format: %s", conversationID)
	}
	return channelID, threadTS, nil
}

// mapStatusToMessage converts proto status to human-readable message
func (a *SlackAdapter) mapStatusToMessage(status *pb.StatusUpdate) string {
	if status.CustomMessage != "" {
		return status.CustomMessage
	}

	switch status.Status {
	case pb.StatusUpdate_THINKING:
		return "Thinking..."
	case pb.StatusUpdate_SEARCHING:
		return "Searching..."
	case pb.StatusUpdate_GENERATING:
		return "Generating response..."
	case pb.StatusUpdate_PROCESSING:
		return "Processing..."
	case pb.StatusUpdate_ANALYZING:
		return "Analyzing..."
	case pb.StatusUpdate_CUSTOM:
		return status.CustomMessage
	default:
		return "Working..."
	}
}

// getDefaultEmojiForStatus returns a default emoji for a status
func (a *SlackAdapter) getDefaultEmojiForStatus(status pb.StatusUpdate_Status) string {
	switch status {
	case pb.StatusUpdate_THINKING:
		return ":thought_balloon:"
	case pb.StatusUpdate_SEARCHING:
		return ":mag:"
	case pb.StatusUpdate_GENERATING:
		return ":pencil2:"
	case pb.StatusUpdate_PROCESSING:
		return ":gear:"
	case pb.StatusUpdate_ANALYZING:
		return ":bar_chart:"
	default:
		return ":robot_face:"
	}
}
