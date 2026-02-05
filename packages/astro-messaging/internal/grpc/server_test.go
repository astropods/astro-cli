package grpc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pb "github.com/astro/messaging/pkg/gen/astro/messaging/v1"
	"github.com/astro/messaging/internal/adapter"
	"github.com/astro/messaging/internal/store"
	"github.com/astro/messaging/pkg/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockAdapter implements adapter.Adapter for testing
type mockAdapter struct {
	platform     string
	sentMessages []*types.SendMessageRequest
	mu           sync.Mutex
	healthy      bool
	sendErr      error
}

func newMockAdapter(platform string) *mockAdapter {
	return &mockAdapter{
		platform:     platform,
		sentMessages: make([]*types.SendMessageRequest, 0),
		healthy:      true,
	}
}

func (m *mockAdapter) Initialize(ctx context.Context, config adapter.Config) error { return nil }
func (m *mockAdapter) Start(ctx context.Context) error                            { return nil }
func (m *mockAdapter) Stop(ctx context.Context) error                             { return nil }
func (m *mockAdapter) OnMessage(handler adapter.MessageHandler)                   {}
func (m *mockAdapter) UpdateMessage(ctx context.Context, messageID string, content string) error {
	return nil
}
func (m *mockAdapter) GetPlatformName() string        { return m.platform }
func (m *mockAdapter) IsHealthy(ctx context.Context) bool { return m.healthy }

func (m *mockAdapter) SendMessage(ctx context.Context, req *types.SendMessageRequest) (*types.SendMessageResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	m.sentMessages = append(m.sentMessages, req)
	return &types.SendMessageResult{
		Success:   true,
		MessageID: "sent-msg-123",
	}, nil
}

func (m *mockAdapter) getLastSentMessage() *types.SendMessageRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sentMessages) == 0 {
		return nil
	}
	return m.sentMessages[len(m.sentMessages)-1]
}

// --- Tests for HandleIncomingMessageFromAdapter ---

func TestHandleIncomingMessageFromAdapter_PlatformContextPreserved(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	// Register a mock adapter
	mock := newMockAdapter("web")
	server.RegisterAdapter("web", mock)

	// Create a mock stream to capture the forwarded message
	var capturedResponse *pb.AgentResponse
	var captureMu sync.Mutex

	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			captureMu.Lock()
			capturedResponse = resp
			captureMu.Unlock()
			return nil
		},
	}

	// Register a fake agent stream
	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	// Simulate incoming message from web adapter
	msg := &types.UnifiedMessage{
		ID:                "msg-001",
		PlatformMessageID: "plat-msg-001",
		Platform:          "web",
		Content:           "Hello agent",
		UserID:            "user-123",
		UserName:          "testuser",
		ChannelID:         "conv-abc",
		ThreadID:          "thread-xyz",
		ConversationID:    "conv-abc",
		Timestamp:         time.Now(),
	}

	_, err := server.HandleIncomingMessageFromAdapter(context.Background(), msg)
	if err != nil {
		t.Fatalf("HandleIncomingMessageFromAdapter failed: %v", err)
	}

	captureMu.Lock()
	resp := capturedResponse
	captureMu.Unlock()

	if resp == nil {
		t.Fatal("expected response to be forwarded to agent stream")
	}

	// Verify the AgentResponse wraps the message correctly
	incoming := resp.GetIncomingMessage()
	if incoming == nil {
		t.Fatal("expected IncomingMessage payload in AgentResponse")
	}

	// Verify PlatformContext was populated
	pc := incoming.PlatformContext
	if pc == nil {
		t.Fatal("expected PlatformContext to be non-nil")
	}

	if pc.MessageId != "plat-msg-001" {
		t.Errorf("PlatformContext.MessageId: expected 'plat-msg-001', got %q", pc.MessageId)
	}
	if pc.ChannelId != "conv-abc" {
		t.Errorf("PlatformContext.ChannelId: expected 'conv-abc', got %q", pc.ChannelId)
	}
	if pc.ThreadId != "thread-xyz" {
		t.Errorf("PlatformContext.ThreadId: expected 'thread-xyz', got %q", pc.ThreadId)
	}
}

func TestHandleIncomingMessageFromAdapter_EmptyThreadID(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	var capturedResponse *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			capturedResponse = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	msg := &types.UnifiedMessage{
		ID:                "msg-002",
		PlatformMessageID: "plat-msg-002",
		Platform:          "web",
		Content:           "Hello",
		ChannelID:         "conv-abc",
		ThreadID:          "", // No thread
		ConversationID:    "conv-abc",
		Timestamp:         time.Now(),
	}

	_, err := server.HandleIncomingMessageFromAdapter(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	incoming := capturedResponse.GetIncomingMessage()
	if incoming == nil {
		t.Fatal("expected IncomingMessage payload")
	}

	pc := incoming.PlatformContext
	if pc == nil {
		t.Fatal("PlatformContext should be non-nil even without ThreadId")
	}

	if pc.ThreadId != "" {
		t.Errorf("expected empty ThreadId, got %q", pc.ThreadId)
	}
	if pc.ChannelId != "conv-abc" {
		t.Errorf("expected ChannelId 'conv-abc', got %q", pc.ChannelId)
	}
	if pc.MessageId != "plat-msg-002" {
		t.Errorf("expected MessageId 'plat-msg-002', got %q", pc.MessageId)
	}
}

func TestHandleIncomingMessageFromAdapter_UserFieldsPreserved(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	var capturedResponse *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			capturedResponse = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	msg := &types.UnifiedMessage{
		ID:             "msg-003",
		Platform:       "slack",
		Content:        "Hello from Slack",
		UserID:         "U12345",
		UserName:       "slackuser",
		ChannelID:      "C99999",
		ConversationID: "C99999",
		Timestamp:      time.Now(),
	}

	_, err := server.HandleIncomingMessageFromAdapter(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	incoming := capturedResponse.GetIncomingMessage()
	if incoming.User == nil {
		t.Fatal("expected User to be non-nil")
	}
	if incoming.User.Id != "U12345" {
		t.Errorf("User.Id: expected 'U12345', got %q", incoming.User.Id)
	}
	if incoming.User.Username != "slackuser" {
		t.Errorf("User.Username: expected 'slackuser', got %q", incoming.User.Username)
	}
}

// --- Tests for routeAgentMessage ---

func TestRouteAgentMessage_FullPlatformContext(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	mock := newMockAdapter("web")
	server.RegisterAdapter("web", mock)

	msg := &pb.Message{
		Id:             "resp-001",
		Platform:       "web",
		ConversationId: "conv-abc",
		Content:        "Agent response",
		PlatformContext: &pb.PlatformContext{
			MessageId: "msg-001",
			ChannelId: "conv-abc",
			ThreadId:  "thread-xyz",
		},
	}

	err := server.routeAgentMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("routeAgentMessage failed: %v", err)
	}

	sent := mock.getLastSentMessage()
	if sent == nil {
		t.Fatal("expected adapter to receive a message")
	}

	if sent.Platform != "web" {
		t.Errorf("Platform: expected 'web', got %q", sent.Platform)
	}
	if sent.ChannelID != "conv-abc" {
		t.Errorf("ChannelID: expected 'conv-abc', got %q", sent.ChannelID)
	}
	if sent.ThreadID != "thread-xyz" {
		t.Errorf("ThreadID: expected 'thread-xyz', got %q", sent.ThreadID)
	}
	if sent.Content != "Agent response" {
		t.Errorf("Content: expected 'Agent response', got %q", sent.Content)
	}
}

func TestRouteAgentMessage_ThreadIdFallsBackToMessageId(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	mock := newMockAdapter("web")
	server.RegisterAdapter("web", mock)

	// ThreadId is empty, should fall back to MessageId
	msg := &pb.Message{
		Id:             "resp-002",
		Platform:       "web",
		ConversationId: "conv-abc",
		Content:        "Response without thread",
		PlatformContext: &pb.PlatformContext{
			MessageId: "original-msg-id",
			ChannelId: "conv-abc",
			ThreadId:  "", // Empty
		},
	}

	err := server.routeAgentMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("routeAgentMessage failed: %v", err)
	}

	sent := mock.getLastSentMessage()
	if sent == nil {
		t.Fatal("expected adapter to receive a message")
	}

	// ThreadID should fall back to MessageId
	if sent.ThreadID != "original-msg-id" {
		t.Errorf("ThreadID: expected fallback to 'original-msg-id', got %q", sent.ThreadID)
	}
}

func TestRouteAgentMessage_NilPlatformContext(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	mock := newMockAdapter("web")
	server.RegisterAdapter("web", mock)

	msg := &pb.Message{
		Id:              "resp-003",
		Platform:        "web",
		ConversationId:  "conv-abc",
		Content:         "No platform context",
		PlatformContext: nil, // Nil
	}

	err := server.routeAgentMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for nil PlatformContext")
	}

	if mock.getLastSentMessage() != nil {
		t.Error("adapter should not receive message when PlatformContext is nil")
	}
}

func TestRouteAgentMessage_EmptyPlatformContext(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	mock := newMockAdapter("web")
	server.RegisterAdapter("web", mock)

	// PlatformContext exists but has all empty fields
	msg := &pb.Message{
		Id:             "resp-004",
		Platform:       "web",
		ConversationId: "conv-abc",
		Content:        "Empty context fields",
		PlatformContext: &pb.PlatformContext{
			MessageId: "",
			ChannelId: "",
			ThreadId:  "",
		},
	}

	err := server.routeAgentMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("routeAgentMessage should not error on empty PlatformContext fields: %v", err)
	}

	sent := mock.getLastSentMessage()
	if sent == nil {
		t.Fatal("expected adapter to receive message")
	}

	// Both should be empty since both ThreadId and MessageId are empty
	if sent.ChannelID != "" {
		t.Errorf("ChannelID: expected empty, got %q", sent.ChannelID)
	}
	if sent.ThreadID != "" {
		t.Errorf("ThreadID: expected empty (both ThreadId and MessageId empty), got %q", sent.ThreadID)
	}
}

func TestRouteAgentMessage_UnknownPlatform(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	// No adapter registered for "discord"
	msg := &pb.Message{
		Id:       "resp-005",
		Platform: "discord",
		Content:  "Hello",
		PlatformContext: &pb.PlatformContext{
			MessageId: "msg-1",
			ChannelId: "ch-1",
		},
	}

	err := server.routeAgentMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for unknown platform")
	}
}

func TestRouteAgentMessage_AdapterSendError(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	mock := newMockAdapter("web")
	mock.sendErr = fmt.Errorf("platform API unavailable")
	server.RegisterAdapter("web", mock)

	msg := &pb.Message{
		Id:       "resp-006",
		Platform: "web",
		Content:  "Test",
		PlatformContext: &pb.PlatformContext{
			MessageId: "msg-1",
			ChannelId: "ch-1",
		},
	}

	err := server.routeAgentMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when adapter fails to send")
	}
}

// --- Tests for HandleIncomingMessage ---

func TestHandleIncomingMessage_ForwardsToStream(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	var capturedResponse *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			capturedResponse = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	msg := &pb.Message{
		Id:             "msg-100",
		Platform:       "slack",
		ConversationId: "conv-slack-123",
		Content:        "Test message",
		Timestamp:      timestamppb.Now(),
		PlatformContext: &pb.PlatformContext{
			MessageId:   "1234567890.123456",
			ChannelId:   "C123456",
			ThreadId:    "1234567890.000001",
			ChannelName: "#general",
			WorkspaceId: "T123456",
		},
		User: &pb.User{
			Id:       "U123456",
			Username: "testuser",
		},
	}

	err := server.HandleIncomingMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("HandleIncomingMessage failed: %v", err)
	}

	if capturedResponse == nil {
		t.Fatal("expected message to be forwarded via stream")
	}

	if capturedResponse.ConversationId != "conv-slack-123" {
		t.Errorf("ConversationId: expected 'conv-slack-123', got %q", capturedResponse.ConversationId)
	}

	incoming := capturedResponse.GetIncomingMessage()
	if incoming == nil {
		t.Fatal("expected IncomingMessage payload")
	}

	// Verify full PlatformContext fidelity
	pc := incoming.PlatformContext
	if pc.MessageId != "1234567890.123456" {
		t.Errorf("MessageId: expected '1234567890.123456', got %q", pc.MessageId)
	}
	if pc.ChannelId != "C123456" {
		t.Errorf("ChannelId: expected 'C123456', got %q", pc.ChannelId)
	}
	if pc.ThreadId != "1234567890.000001" {
		t.Errorf("ThreadId: expected '1234567890.000001', got %q", pc.ThreadId)
	}
	if pc.ChannelName != "#general" {
		t.Errorf("ChannelName: expected '#general', got %q", pc.ChannelName)
	}
	if pc.WorkspaceId != "T123456" {
		t.Errorf("WorkspaceId: expected 'T123456', got %q", pc.WorkspaceId)
	}
}

func TestHandleIncomingMessage_NoActiveStream(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)
	// No streams registered

	msg := &pb.Message{
		Id:       "msg-101",
		Platform: "web",
		Content:  "Hello",
	}

	err := server.HandleIncomingMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when no active stream")
	}
}

func TestHandleIncomingMessage_StreamSendError(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			return fmt.Errorf("stream broken")
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	msg := &pb.Message{
		Id:       "msg-102",
		Platform: "web",
		Content:  "Hello",
	}

	err := server.HandleIncomingMessage(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when stream send fails")
	}
}

// --- Tests for PlatformContext roundtrip ---

func TestPlatformContext_RoundtripWebMessage(t *testing.T) {
	// Simulates the full path: web adapter creates message → forwarded to agent → agent responds → routed back
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	webAdapter := newMockAdapter("web")
	server.RegisterAdapter("web", webAdapter)

	// Step 1: Capture what gets sent to the agent
	var agentReceived *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			agentReceived = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	// Step 2: Web adapter sends incoming message (simulating HandleSendMessage in web/handlers.go)
	incomingMsg := &types.UnifiedMessage{
		ID:                "msg-web-001",
		PlatformMessageID: "msg-web-001",
		Platform:          "web",
		Content:           "What is an API?",
		UserID:            "user-42",
		UserName:          "webuser",
		ChannelID:         "conv-web-123", // For web, channel = conversation ID
		ThreadID:          "",             // Web messages typically have no thread
		ConversationID:    "conv-web-123",
		Timestamp:         time.Now(),
	}

	_, err := server.HandleIncomingMessageFromAdapter(context.Background(), incomingMsg)
	if err != nil {
		t.Fatalf("step 2 failed: %v", err)
	}

	// Step 3: Extract what the agent received and verify PlatformContext
	incoming := agentReceived.GetIncomingMessage()
	if incoming == nil {
		t.Fatal("agent did not receive IncomingMessage")
	}

	receivedPC := incoming.PlatformContext
	if receivedPC == nil {
		t.Fatal("agent received nil PlatformContext")
	}

	// Step 4: Agent echoes back the same PlatformContext (this is what the TS agent does)
	agentResponse := &pb.Message{
		Id:              "agent-resp-001",
		Platform:        incoming.Platform,
		ConversationId:  incoming.ConversationId,
		Content:         "An API is an Application Programming Interface",
		PlatformContext: receivedPC, // Agent forwards the same PlatformContext
		User: &pb.User{
			Id:       "engineering-assistant",
			Username: "Engineering Assistant",
		},
	}

	// Step 5: Route agent response back to platform
	err = server.routeAgentMessage(context.Background(), agentResponse)
	if err != nil {
		t.Fatalf("step 5 (routeAgentMessage) failed: %v", err)
	}

	// Step 6: Verify the adapter received correct routing
	sent := webAdapter.getLastSentMessage()
	if sent == nil {
		t.Fatal("web adapter did not receive the response")
	}

	if sent.Platform != "web" {
		t.Errorf("Platform: expected 'web', got %q", sent.Platform)
	}
	if sent.ChannelID != "conv-web-123" {
		t.Errorf("ChannelID: expected 'conv-web-123', got %q", sent.ChannelID)
	}
	// ThreadID should fall back to MessageId since web messages have no thread
	if sent.ThreadID != "msg-web-001" {
		t.Errorf("ThreadID: expected fallback to 'msg-web-001', got %q", sent.ThreadID)
	}
	if sent.Content != "An API is an Application Programming Interface" {
		t.Errorf("Content mismatch: got %q", sent.Content)
	}
}

func TestPlatformContext_RoundtripSlackThreadedMessage(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	slackAdapter := newMockAdapter("slack")
	server.RegisterAdapter("slack", slackAdapter)

	var agentReceived *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			agentReceived = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	// Slack threaded message
	incomingMsg := &types.UnifiedMessage{
		ID:                "msg-slack-001",
		PlatformMessageID: "C123:1234567890.999999",
		Platform:          "slack",
		Content:           "Thread reply",
		UserID:            "U123456",
		UserName:          "slackuser",
		ChannelID:         "C123456",
		ThreadID:          "1234567890.000001", // Thread timestamp
		ConversationID:    "C123456-1234567890.000001",
		Timestamp:         time.Now(),
	}

	_, err := server.HandleIncomingMessageFromAdapter(context.Background(), incomingMsg)
	if err != nil {
		t.Fatalf("HandleIncomingMessageFromAdapter failed: %v", err)
	}

	incoming := agentReceived.GetIncomingMessage()
	receivedPC := incoming.PlatformContext

	// Agent sends response with same PlatformContext
	agentResponse := &pb.Message{
		Platform:        incoming.Platform,
		ConversationId:  incoming.ConversationId,
		Content:         "Here's my answer",
		PlatformContext: receivedPC,
	}

	err = server.routeAgentMessage(context.Background(), agentResponse)
	if err != nil {
		t.Fatalf("routeAgentMessage failed: %v", err)
	}

	sent := slackAdapter.getLastSentMessage()
	if sent == nil {
		t.Fatal("slack adapter did not receive the response")
	}

	// Slack threaded messages should use the thread timestamp
	if sent.ChannelID != "C123456" {
		t.Errorf("ChannelID: expected 'C123456', got %q", sent.ChannelID)
	}
	if sent.ThreadID != "1234567890.000001" {
		t.Errorf("ThreadID: expected '1234567890.000001', got %q", sent.ThreadID)
	}
}

func TestPlatformContext_PlatformDataPreserved(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	var capturedResponse *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			capturedResponse = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	// Message with platform_data map
	msg := &pb.Message{
		Id:             "msg-200",
		Platform:       "slack",
		ConversationId: "conv-200",
		Content:        "Test",
		PlatformContext: &pb.PlatformContext{
			MessageId:   "1234567890.123456",
			ChannelId:   "C123456",
			WorkspaceId: "T999",
			PlatformData: map[string]string{
				"team_id":     "T999",
				"bot_id":      "B123",
				"app_id":      "A456",
				"custom_field": "custom_value",
			},
		},
		User: &pb.User{
			Id:       "U123",
			Username: "test",
		},
	}

	err := server.HandleIncomingMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("HandleIncomingMessage failed: %v", err)
	}

	incoming := capturedResponse.GetIncomingMessage()
	pc := incoming.PlatformContext

	if len(pc.PlatformData) != 4 {
		t.Errorf("expected 4 platform_data entries, got %d", len(pc.PlatformData))
	}
	if pc.PlatformData["team_id"] != "T999" {
		t.Errorf("platform_data[team_id]: expected 'T999', got %q", pc.PlatformData["team_id"])
	}
	if pc.PlatformData["custom_field"] != "custom_value" {
		t.Errorf("platform_data[custom_field]: expected 'custom_value', got %q", pc.PlatformData["custom_field"])
	}
}

// --- Tests for protobuf serialization edge cases ---

func TestProtobufMessage_ConversationIdPreserved(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	var capturedResponse *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			capturedResponse = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	msg := &pb.Message{
		Id:             "msg-300",
		Platform:       "web",
		ConversationId: "conv-uuid-with-dashes-123",
		Content:        "Test conversation ID preservation",
		PlatformContext: &pb.PlatformContext{
			MessageId: "msg-300",
			ChannelId: "conv-uuid-with-dashes-123",
		},
	}

	err := server.HandleIncomingMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedResponse.ConversationId != "conv-uuid-with-dashes-123" {
		t.Errorf("ConversationId: expected 'conv-uuid-with-dashes-123', got %q", capturedResponse.ConversationId)
	}

	incoming := capturedResponse.GetIncomingMessage()
	if incoming.ConversationId != "conv-uuid-with-dashes-123" {
		t.Errorf("IncomingMessage.ConversationId: expected 'conv-uuid-with-dashes-123', got %q", incoming.ConversationId)
	}
}

func TestProtobufMessage_TimestampPreserved(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	var capturedResponse *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			capturedResponse = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	now := time.Now().UTC().Truncate(time.Second)
	msg := &pb.Message{
		Id:             "msg-301",
		Platform:       "web",
		ConversationId: "conv-301",
		Content:        "Test timestamp",
		Timestamp:      timestamppb.New(now),
		PlatformContext: &pb.PlatformContext{
			MessageId: "msg-301",
			ChannelId: "conv-301",
		},
	}

	err := server.HandleIncomingMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	incoming := capturedResponse.GetIncomingMessage()
	if incoming.Timestamp == nil {
		t.Fatal("expected Timestamp to be preserved")
	}

	receivedTime := incoming.Timestamp.AsTime().UTC().Truncate(time.Second)
	if !receivedTime.Equal(now) {
		t.Errorf("Timestamp: expected %v, got %v", now, receivedTime)
	}
}

func TestProtobufMessage_AttachmentsPreserved(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	var capturedResponse *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			capturedResponse = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["agent-stream"] = &conversationStream{
		stream:         mockStream,
		conversationID: "agent-stream",
	}
	server.streamsMu.Unlock()

	incomingMsg := &types.UnifiedMessage{
		ID:             "msg-400",
		Platform:       "slack",
		Content:        "Check this file",
		ChannelID:      "C123",
		ConversationID: "C123",
		Timestamp:      time.Now(),
		Attachments: []types.Attachment{
			{
				Type:     "IMAGE",
				URL:      "https://example.com/image.png",
				Name:     "screenshot.png",
				MimeType: "image/png",
				Size:     1024,
			},
		},
	}

	_, err := server.HandleIncomingMessageFromAdapter(context.Background(), incomingMsg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	incoming := capturedResponse.GetIncomingMessage()
	if len(incoming.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(incoming.Attachments))
	}

	att := incoming.Attachments[0]
	if att.Url != "https://example.com/image.png" {
		t.Errorf("Attachment.Url: expected 'https://example.com/image.png', got %q", att.Url)
	}
	if att.Filename != "screenshot.png" {
		t.Errorf("Attachment.Filename: expected 'screenshot.png', got %q", att.Filename)
	}
	if att.MimeType != "image/png" {
		t.Errorf("Attachment.MimeType: expected 'image/png', got %q", att.MimeType)
	}
	if att.SizeBytes != 1024 {
		t.Errorf("Attachment.SizeBytes: expected 1024, got %d", att.SizeBytes)
	}
}

// --- Tests for buildConversationID ---

func TestBuildConversationID(t *testing.T) {
	tests := []struct {
		name      string
		platform  string
		channelID string
		threadID  string
		expected  string
	}{
		{
			name:      "with thread",
			platform:  "slack",
			channelID: "C123456",
			threadID:  "1234567890.123456",
			expected:  "slack-C123456-1234567890.123456",
		},
		{
			name:      "without thread",
			platform:  "slack",
			channelID: "C123456",
			threadID:  "",
			expected:  "slack-C123456",
		},
		{
			name:      "web platform",
			platform:  "web",
			channelID: "conv-uuid-123",
			threadID:  "",
			expected:  "web-conv-uuid-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildConversationID(tt.platform, tt.channelID, tt.threadID)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// --- Tests for HealthCheck ---

func TestHealthCheck_AllHealthy(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	mock1 := newMockAdapter("web")
	mock2 := newMockAdapter("slack")
	server.RegisterAdapter("web", mock1)
	server.RegisterAdapter("slack", mock2)

	resp, err := server.HealthCheck(context.Background(), &pb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if resp.Status != pb.HealthCheckResponse_HEALTHY {
		t.Errorf("expected HEALTHY, got %v", resp.Status)
	}
}

func TestHealthCheck_Degraded(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	healthyAdapter := newMockAdapter("web")
	unhealthyAdapter := newMockAdapter("slack")
	unhealthyAdapter.healthy = false

	server.RegisterAdapter("web", healthyAdapter)
	server.RegisterAdapter("slack", unhealthyAdapter)

	resp, err := server.HealthCheck(context.Background(), &pb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if resp.Status != pb.HealthCheckResponse_DEGRADED {
		t.Errorf("expected DEGRADED, got %v", resp.Status)
	}
}

// --- Tests for RegisterAdapter ---

func TestRegisterAdapter(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	mock := newMockAdapter("web")
	server.RegisterAdapter("web", mock)

	server.mu.RLock()
	adpt, exists := server.adapters["web"]
	server.mu.RUnlock()

	if !exists {
		t.Fatal("expected adapter to be registered")
	}
	if adpt.GetPlatformName() != "web" {
		t.Errorf("expected platform 'web', got %q", adpt.GetPlatformName())
	}
}

// --- Tests for SendToAgent ---

func TestSendToAgent_Success(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	var capturedResponse *pb.AgentResponse
	mockStream := &captureStream{
		sendFunc: func(resp *pb.AgentResponse) error {
			capturedResponse = resp
			return nil
		},
	}

	server.streamsMu.Lock()
	server.streams["conv-123"] = &conversationStream{
		stream:         mockStream,
		conversationID: "conv-123",
	}
	server.streamsMu.Unlock()

	resp := &pb.AgentResponse{
		ConversationId: "conv-123",
		Payload: &pb.AgentResponse_Content{
			Content: &pb.ContentChunk{
				Type:    pb.ContentChunk_DELTA,
				Content: "test content",
			},
		},
	}

	err := server.SendToAgent("conv-123", resp)
	if err != nil {
		t.Fatalf("SendToAgent failed: %v", err)
	}

	if capturedResponse == nil {
		t.Fatal("expected response to be sent")
	}
}

func TestSendToAgent_NoStream(t *testing.T) {
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	convStore := store.NewMemoryStore()
	server := NewServer(":0", threadStore, convStore)

	resp := &pb.AgentResponse{
		ConversationId: "nonexistent",
	}

	err := server.SendToAgent("nonexistent", resp)
	if err == nil {
		t.Fatal("expected error for nonexistent stream")
	}
}

// --- captureStream mock ---

// captureStream implements pb.AgentMessaging_ProcessConversationServer for testing
type captureStream struct {
	pb.AgentMessaging_ProcessConversationServer
	sendFunc func(*pb.AgentResponse) error
}

func (s *captureStream) Send(resp *pb.AgentResponse) error {
	if s.sendFunc != nil {
		return s.sendFunc(resp)
	}
	return nil
}

func (s *captureStream) Context() context.Context {
	return context.Background()
}
