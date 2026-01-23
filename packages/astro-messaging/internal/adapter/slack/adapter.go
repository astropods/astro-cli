package slack

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	pb "github.com/astro/messaging/pkg/gen/astro/messaging/v1"
	"github.com/astro/messaging/internal/adapter"
	"github.com/astro/messaging/internal/store"
	"github.com/astro/messaging/pkg/types"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SlackAdapter implements both Adapter and GRPCAdapter interfaces for Slack
type SlackAdapter struct {
	client       *slack.Client
	socketClient *socketmode.Client
	config       adapter.Config
	handler      adapter.MessageHandler  // Old HTTP handler
	rateLimiter  *RateLimiter
	stopChan     chan struct{}

	// gRPC additions
	grpcHandler adapter.GRPCMessageHandler // Handler for gRPC message forwarding
	threadStore *store.ThreadHistoryStore
	aiClient    *SlackAIClient // Client for Slack AI APIs
}

// New creates a new Slack adapter
func New() *SlackAdapter {
	return &SlackAdapter{
		stopChan: make(chan struct{}),
	}
}

// Initialize sets up the Slack adapter with configuration
func (a *SlackAdapter) Initialize(ctx context.Context, config adapter.Config) error {
	a.config = config

	// Initialize Slack client
	a.client = slack.New(
		config.BotToken,
		slack.OptionAppLevelToken(config.AppToken),
	)

	// Initialize socket mode client if enabled
	if config.SocketMode {
		a.socketClient = socketmode.New(
			a.client,
			socketmode.OptionDebug(false),
		)
	}

	// Initialize rate limiter
	a.rateLimiter = NewRateLimiter(
		config.RateLimit.RequestsPerSecond,
		config.RateLimit.BurstSize,
	)

	// Initialize AI client for Slack AI features
	a.aiClient = NewSlackAIClient(config.BotToken)

	log.Printf("[Slack] Adapter initialized (Socket Mode: %v)", config.SocketMode)
	return nil
}

// Start begins listening for Slack events
func (a *SlackAdapter) Start(ctx context.Context) error {
	if a.config.SocketMode {
		return a.startSocketMode(ctx)
	}
	return fmt.Errorf("webhook mode not implemented, use Socket Mode")
}

// startSocketMode starts the socket mode event listener
func (a *SlackAdapter) startSocketMode(ctx context.Context) error {
	log.Println("[Slack] Starting Socket Mode connection...")

	// Start socket mode client in background (this initializes the Events channel)
	go func() {
		if err := a.socketClient.RunContext(ctx); err != nil {
			log.Printf("[Slack] Socket mode client error: %v", err)
		}
	}()

	// Listen for events from the now-initialized channel
	for {
		select {
		case <-ctx.Done():
			log.Println("[Slack] Context cancelled, stopping event listener")
			return ctx.Err()
		case <-a.stopChan:
			log.Println("[Slack] Stopping event listener")
			return nil
		case evt := <-a.socketClient.Events:
			a.handleSocketEvent(ctx, evt)
		}
	}
}

// handleSocketEvent processes incoming socket mode events
func (a *SlackAdapter) handleSocketEvent(ctx context.Context, evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeConnecting:
		log.Println("[Slack] Connecting to Slack...")

	case socketmode.EventTypeConnectionError:
		log.Printf("[Slack] Connection error: %v", evt.Data)

	case socketmode.EventTypeConnected:
		log.Println("[Slack] Connected to Slack via Socket Mode")

	case socketmode.EventTypeEventsAPI:
		// Acknowledge the event
		a.socketClient.Ack(*evt.Request)

		// Handle the inner event
		eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			log.Printf("[Slack] Could not type cast event to EventsAPIEvent")
			return
		}

		a.handleInnerEvent(ctx, eventsAPIEvent.InnerEvent)

	case socketmode.EventTypeInteractive:
		// Acknowledge interactive events (buttons, modals, etc.)
		a.socketClient.Ack(*evt.Request)

		// Handle block actions (feedback buttons, etc.)
		callback, ok := evt.Data.(slack.InteractionCallback)
		if ok && callback.Type == slack.InteractionTypeBlockActions {
			a.handleBlockActions(ctx, &callback)
		} else {
			log.Println("[Slack] Interactive event received (not yet handled)")
		}

	case socketmode.EventTypeSlashCommand:
		// Acknowledge slash commands
		a.socketClient.Ack(*evt.Request)
		log.Println("[Slack] Slash command received (not yet handled)")

	case socketmode.EventTypeHello:
		// Hello event is just a connection acknowledgment, no action needed
		// Connection is already logged in EventTypeConnected

	default:
		// Only log truly unknown event types at debug level
		if evt.Type != "" {
			log.Printf("[Slack] Unhandled event type: %s", evt.Type)
		}
	}
}

// handleInnerEvent processes the actual event data
func (a *SlackAdapter) handleInnerEvent(ctx context.Context, innerEvent slackevents.EventsAPIInnerEvent) {
	switch ev := innerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		a.handleMessage(ctx, ev)

	case *slackevents.AppMentionEvent:
		a.handleAppMention(ctx, ev)

	default:
		log.Printf("[Slack] Unhandled inner event type: %s", innerEvent.Type)
	}
}

// handleMessage processes message events
func (a *SlackAdapter) handleMessage(ctx context.Context, ev *slackevents.MessageEvent) {
	// Filter out bot messages
	if ev.BotID != "" {
		return
	}

	// Filter out message subtypes we don't want to process
	if ev.SubType != "" && ev.SubType != "thread_broadcast" {
		return
	}

	// Only process message events in DMs (channel starts with 'D')
	// In public/private channels, we'll rely on app_mention events to avoid duplicates
	if ev.Channel != "" && ev.Channel[0] != 'D' {
		log.Printf("[Slack] Ignoring message in channel %s (will handle via app_mention)", ev.Channel)
		return
	}

	log.Printf("[Slack] Message received: channel=%s, user=%s, text=%s", ev.Channel, ev.User, ev.Text)

	// Translate to unified message
	unifiedMsg := TranslateMessageEvent(ev)

	// Call handler if registered
	if a.handler != nil {
		resp, err := a.handler(ctx, unifiedMsg)
		if err != nil {
			log.Printf("[Slack] Error handling message: %v", err)
			// Send error message to user
			a.sendErrorMessage(ctx, ev.Channel, ev.ThreadTimeStamp, err)
			return
		}

		// Send response if provided
		if resp != nil && resp.Content != "" {
			a.sendResponse(ctx, unifiedMsg, resp)
		}
	}
}

// handleBlockActions processes block action events (button clicks, etc.)
func (a *SlackAdapter) handleBlockActions(ctx context.Context, callback *slack.InteractionCallback) {
	log.Printf("[Slack] Block action received: type=%s, actions=%d", callback.Type, len(callback.ActionCallback.BlockActions))

	for _, action := range callback.ActionCallback.BlockActions {
		log.Printf("[Slack] Action: id=%s, value=%s", action.ActionID, action.Value)

		// Handle feedback button clicks
		if action.ActionID == "feedback_buttons" {
			feedbackType := action.Value // "positive_feedback" or "negative_feedback"
			log.Printf("[Slack] Feedback received: %s from user %s on message %s",
				feedbackType, callback.User.ID, callback.Message.Timestamp)

			// Log feedback for observability
			// In a real implementation, you might want to:
			// 1. Store feedback in a database
			// 2. Send to analytics service
			// 3. Update model training data
			// 4. Send notification to admin dashboard

			// Use Slack emoji names (not emoji characters)
			emojiName := "thumbsup"
			if feedbackType == "negative_feedback" {
				emojiName = "thumbsdown"
			}

			// Remove feedback buttons from the message first
			// Extract the message text and keep only the section block (remove context_actions)
			if len(callback.Message.Blocks.BlockSet) > 0 {
				// Keep only the section blocks (remove feedback buttons)
				updatedBlocks := []slack.Block{}
				for _, block := range callback.Message.Blocks.BlockSet {
					// Keep all blocks except context_actions (which contains feedback buttons)
					if block.BlockType() != "context_actions" {
						updatedBlocks = append(updatedBlocks, block)
					}
				}

				// Update the message with blocks without feedback buttons
				_, _, _, err := a.client.UpdateMessage(
					callback.Channel.ID,
					callback.Message.Timestamp,
					slack.MsgOptionBlocks(updatedBlocks...),
					slack.MsgOptionText(callback.Message.Text, false),
				)

				if err != nil {
					log.Printf("[Slack] Failed to remove feedback buttons: %v", err)
				} else {
					log.Printf("[Slack] ✓ Feedback buttons removed from message")
				}
			}

			// Then acknowledge the feedback visually by adding a reaction
			err := a.client.AddReaction(emojiName, slack.ItemRef{
				Channel:   callback.Channel.ID,
				Timestamp: callback.Message.Timestamp,
			})

			if err != nil {
				log.Printf("[Slack] Failed to add reaction: %v", err)
			} else {
				log.Printf("[Slack] ✓ Feedback acknowledged with :%s: reaction", emojiName)
			}
		}
	}
}

// handleAppMention processes app mention events
func (a *SlackAdapter) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
	log.Printf("[Slack] App mentioned: channel=%s, user=%s, text=%s", ev.Channel, ev.User, ev.Text)

	// Translate to unified message
	unifiedMsg := TranslateAppMentionEvent(ev)

	log.Printf("[Slack] ✓ handleAppMention executing")

	// Set loading state using Slack AI API
	threadTS := ev.ThreadTimeStamp
	if threadTS == "" {
		threadTS = ev.TimeStamp // Use message timestamp if not in a thread
	}

	log.Printf("[Slack] Setting loading state: channel=%s, threadTS=%s", ev.Channel, threadTS)
	if err := a.aiClient.SetThreadStatus(ctx, ev.Channel, threadTS, "Assistant is thinking...", "thinking_face"); err != nil {
		log.Printf("[Slack] ERROR: Failed to set loading state: %v", err)
		// Continue anyway - this is not a critical error
	} else {
		log.Printf("[Slack] ✓ Loading state set successfully")
	}

	// Call handler if registered
	if a.handler != nil {
		resp, err := a.handler(ctx, unifiedMsg)
		if err != nil {
			log.Printf("[Slack] Error handling mention: %v", err)
			a.sendErrorMessage(ctx, ev.Channel, threadTS, err)
			// Clear loading state on error
			a.aiClient.SetThreadStatus(ctx, ev.Channel, threadTS, "", "")
			return
		}

		// Don't send the acknowledgment response - the agent will send the real response
		// The loading state will be automatically cleared when the agent sends its response
		if resp != nil && resp.Content != "" && resp.Content != "Message received and forwarded to agent" {
			a.sendResponse(ctx, unifiedMsg, resp)
		}
	}
}

// sendResponse sends the agent's response back to Slack
func (a *SlackAdapter) sendResponse(ctx context.Context, originalMsg *types.UnifiedMessage, resp *types.AgentResponse) {
	req := &types.SendMessageRequest{
		Platform:  "slack",
		ChannelID: originalMsg.ChannelID,
		ThreadID:  originalMsg.ThreadID,
		Content:   resp.Content,
	}

	// Create thread if requested and not already in a thread
	if resp.CreateThread && originalMsg.ThreadID == "" {
		req.ThreadID = originalMsg.PlatformMessageID
	}

	result, err := a.SendMessage(ctx, req)
	if err != nil {
		log.Printf("[Slack] Error sending response: %v", err)
		return
	}

	log.Printf("[Slack] Response sent: message_id=%s", result.MessageID)
}

// sendErrorMessage sends an error message to Slack
func (a *SlackAdapter) sendErrorMessage(ctx context.Context, channelID, threadTS string, err error) {
	req := &types.SendMessageRequest{
		Platform:  "slack",
		ChannelID: channelID,
		ThreadID:  threadTS,
		Content:   fmt.Sprintf("❌ Error: %s", err.Error()),
	}

	if _, sendErr := a.SendMessage(ctx, req); sendErr != nil {
		log.Printf("[Slack] Error sending error message: %v", sendErr)
	}
}

// SendMessage sends a message to Slack
func (a *SlackAdapter) SendMessage(ctx context.Context, req *types.SendMessageRequest) (*types.SendMessageResult, error) {
	// Wait for rate limiter
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter context cancelled: %w", err)
	}

	log.Printf("[Slack] Sending message with feedback buttons: channel=%s, thread=%s", req.ChannelID, req.ThreadID)

	// Send message with feedback buttons using raw API
	timestamp, err := a.sendMessageWithFeedback(ctx, req.ChannelID, req.Content, req.ThreadID)

	if err != nil {
		log.Printf("[Slack] ERROR sending message with feedback: %v", err)
		return &types.SendMessageResult{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	log.Printf("[Slack] ✓ Message with feedback sent successfully: timestamp=%s", timestamp)

	// Clear loading state after successfully sending the message
	// Use the thread timestamp if provided, otherwise use the message timestamp
	threadTS := req.ThreadID
	if threadTS == "" {
		threadTS = timestamp
	}
	log.Printf("[Slack] Clearing loading state: channel=%s, threadTS=%s", req.ChannelID, threadTS)
	if clearErr := a.aiClient.SetThreadStatus(ctx, req.ChannelID, threadTS, "", ""); clearErr != nil {
		log.Printf("[Slack] ERROR: Failed to clear loading state: %v", clearErr)
		// Don't fail the message send just because we couldn't clear the loading state
	} else {
		log.Printf("[Slack] ✓ Loading state cleared successfully")
	}

	return &types.SendMessageResult{
		Success:   true,
		MessageID: FormatMessageID(req.ChannelID, timestamp),
		Timestamp: timestamp,
	}, nil
}

// sendMessageWithFeedback sends a message with feedback buttons
func (a *SlackAdapter) sendMessageWithFeedback(ctx context.Context, channelID, content, threadID string) (string, error) {
	// Post message with feedback buttons using AI client
	timestamp, err := a.aiClient.PostMessageWithFeedback(ctx, channelID, content, threadID)
	if err != nil {
		return "", err
	}

	// Clear loading state after successfully sending the message
	threadTS := threadID
	if threadTS == "" {
		threadTS = timestamp
	}
	log.Printf("[Slack] Clearing loading state: channel=%s, threadTS=%s", channelID, threadTS)
	if clearErr := a.aiClient.SetThreadStatus(ctx, channelID, threadTS, "", ""); clearErr != nil {
		log.Printf("[Slack] ERROR: Failed to clear loading state: %v", clearErr)
	} else {
		log.Printf("[Slack] ✓ Loading state cleared successfully")
	}

	return timestamp, nil
}

// UpdateMessage updates an existing Slack message (for streaming)
func (a *SlackAdapter) UpdateMessage(ctx context.Context, messageID string, content string) error {
	// Wait for rate limiter
	if err := a.rateLimiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter context cancelled: %w", err)
	}

	// Parse message ID
	channelID, timestamp := ParseMessageID(messageID)
	if channelID == "" || timestamp == "" {
		return fmt.Errorf("invalid message ID format: %s", messageID)
	}

	// Update message
	_, _, _, err := a.client.UpdateMessageContext(
		ctx,
		channelID,
		timestamp,
		slack.MsgOptionText(content, false),
	)

	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}

	return nil
}

// OnMessage registers a handler for incoming messages
func (a *SlackAdapter) OnMessage(handler adapter.MessageHandler) {
	a.handler = handler
}

// GetPlatformName returns the platform identifier
func (a *SlackAdapter) GetPlatformName() string {
	return "slack"
}

// IsHealthy checks if the adapter is connected and healthy
func (a *SlackAdapter) IsHealthy(ctx context.Context) bool {
	if a.client == nil {
		return false
	}

	// Test authentication
	_, err := a.client.AuthTestContext(ctx)
	return err == nil
}

// Stop gracefully shuts down the adapter
func (a *SlackAdapter) Stop(ctx context.Context) error {
	log.Println("[Slack] Stopping adapter...")

	// Signal stop to event listener
	close(a.stopChan)

	// No explicit cleanup needed for socket mode
	// The context cancellation will handle disconnection

	log.Println("[Slack] Adapter stopped")
	return nil
}

// ============================================================================
// gRPC Adapter Implementation
// ============================================================================

// SetMessageHandler sets the handler for forwarding messages to gRPC
func (a *SlackAdapter) SetMessageHandler(handler adapter.GRPCMessageHandler) {
	a.grpcHandler = handler
}

// Capabilities returns the adapter's capabilities
func (a *SlackAdapter) Capabilities() adapter.AdapterCapabilities {
	// Default to false for AI features (can be configured later)
	return adapter.SlackCapabilities(false)
}

// SetThreadStore sets the thread history store
func (a *SlackAdapter) SetThreadStore(store *store.ThreadHistoryStore) {
	a.threadStore = store
}

// HydrateThread fetches thread history from Slack API
func (a *SlackAdapter) HydrateThread(ctx context.Context, conversationID string, threadStore *store.ThreadHistoryStore) error {
	parts := strings.Split(conversationID, "-")
	if len(parts) < 2 {
		return fmt.Errorf("invalid conversation ID format: %s", conversationID)
	}

	channelID := parts[1]
	var threadTS string
	if len(parts) == 3 {
		threadTS = parts[2]
	}

	log.Printf("[Slack] Hydrating thread: channel=%s, thread=%s", channelID, threadTS)

	var messages []slack.Message

	if threadTS != "" {
		msgs, _, _, err := a.client.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Limit:     50,
		})
		if err != nil {
			return fmt.Errorf("failed to fetch thread: %w", err)
		}
		messages = msgs
	} else {
		history, err := a.client.GetConversationHistoryContext(ctx, &slack.GetConversationHistoryParameters{
			ChannelID: channelID,
			Limit:     50,
		})
		if err != nil {
			return fmt.Errorf("failed to fetch history: %w", err)
		}
		messages = history.Messages
	}

	for _, msg := range messages {
		if msg.Type != "message" || msg.SubType == "bot_message" {
			continue
		}

		threadMsg := &pb.ThreadMessage{
			MessageId: msg.Timestamp,
			User: &pb.User{
				Id:       msg.User,
				Username: msg.Username,
			},
			Content:   msg.Text,
			Timestamp: timestamppb.New(parseSlackTimestamp(msg.Timestamp)),
			WasEdited: msg.Edited != nil,
			PlatformData: map[string]string{
				"team":    msg.Team,
				"subtype": msg.SubType,
			},
		}

		if msg.Edited != nil {
			threadMsg.EditedAt = timestamppb.New(parseSlackTimestamp(msg.Edited.Timestamp))
		}

		threadStore.AddMessage(conversationID, threadMsg)
	}

	log.Printf("[Slack] Hydrated %d messages for %s", len(messages), conversationID)
	return nil
}

// HandleAgentResponse processes responses from the agent (placeholder for now)
// ============================================================================
// Helper Functions
// ============================================================================

func parseSlackTimestamp(ts string) time.Time {
	parts := strings.Split(ts, ".")
	if len(parts) == 0 {
		return time.Now()
	}
	var seconds int64
	fmt.Sscanf(parts[0], "%d", &seconds)
	return time.Unix(seconds, 0)
}

func FormatMessageID(channelID, timestamp string) string {
	return fmt.Sprintf("%s:%s", channelID, timestamp)
}

func ParseMessageID(messageID string) (string, string) {
	parts := strings.Split(messageID, ":")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func TranslateMessageEvent(ev *slackevents.MessageEvent) *types.UnifiedMessage {
	conversationID := ev.Channel
	if ev.ThreadTimeStamp != "" {
		conversationID = fmt.Sprintf("%s-%s", ev.Channel, ev.ThreadTimeStamp)
	}

	return &types.UnifiedMessage{
		ID:                uuid.NewString(),
		PlatformMessageID: ev.TimeStamp,
		Platform:          "slack",
		Content:           ev.Text,
		UserID:            ev.User,
		ChannelID:         ev.Channel,
		ThreadID:          ev.ThreadTimeStamp,
		ConversationID:    conversationID,
		Timestamp:         parseSlackTimestamp(ev.TimeStamp),
		Metadata: map[string]interface{}{
			"event_ts": ev.EventTimeStamp,
			"subtype":  ev.SubType,
		},
	}
}

func TranslateAppMentionEvent(ev *slackevents.AppMentionEvent) *types.UnifiedMessage {
	conversationID := ev.Channel
	if ev.ThreadTimeStamp != "" {
		conversationID = fmt.Sprintf("%s-%s", ev.Channel, ev.ThreadTimeStamp)
	}

	text := stripMentions(ev.Text)

	return &types.UnifiedMessage{
		ID:                uuid.NewString(),
		PlatformMessageID: ev.TimeStamp,
		Platform:          "slack",
		Content:           text,
		UserID:            ev.User,
		ChannelID:         ev.Channel,
		ThreadID:          ev.ThreadTimeStamp,
		ConversationID:    conversationID,
		Timestamp:         parseSlackTimestamp(ev.TimeStamp),
		Metadata: map[string]interface{}{
			"event_ts": ev.EventTimeStamp,
		},
	}
}

func stripMentions(text string) string {
	re := regexp.MustCompile(`<@[A-Z0-9]+>`)
	text = re.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}
