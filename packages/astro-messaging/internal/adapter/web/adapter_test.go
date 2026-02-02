package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "github.com/astro/messaging/pkg/gen/astro/messaging/v1"
	"github.com/astro/messaging/internal/store"
)

func TestConnectionManager(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)

	// Test Add
	conn := &SSEConnection{
		ID:             "conn-1",
		ConversationID: "conv-1",
		EventChan:      make(chan SSEEvent, 10),
		Done:           make(chan struct{}),
		CreatedAt:      time.Now(),
	}
	cm.Add(conn)

	if cm.GetConnectionCount("conv-1") != 1 {
		t.Errorf("expected 1 connection, got %d", cm.GetConnectionCount("conv-1"))
	}

	if cm.GetTotalConnections() != 1 {
		t.Errorf("expected 1 total connection, got %d", cm.GetTotalConnections())
	}

	// Test Broadcast
	event := SSEEvent{Event: "test", Data: `{"foo":"bar"}`}
	cm.Broadcast("conv-1", event)

	select {
	case received := <-conn.EventChan:
		if received.Event != "test" {
			t.Errorf("expected event type 'test', got %s", received.Event)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for broadcast event")
	}

	// Test Remove
	cm.Remove("conv-1", "conn-1")

	if cm.GetConnectionCount("conv-1") != 0 {
		t.Errorf("expected 0 connections after remove, got %d", cm.GetConnectionCount("conv-1"))
	}
}

func TestConnectionManager_MultipleConnections(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)

	// Add multiple connections to same conversation and keep refs
	conns := make([]*SSEConnection, 3)
	for i := 0; i < 3; i++ {
		conns[i] = &SSEConnection{
			ID:             "conn-" + string(rune('1'+i)),
			ConversationID: "conv-1",
			EventChan:      make(chan SSEEvent, 10),
			Done:           make(chan struct{}),
			CreatedAt:      time.Now(),
		}
		cm.Add(conns[i])
	}

	if cm.GetConnectionCount("conv-1") != 3 {
		t.Errorf("expected 3 connections, got %d", cm.GetConnectionCount("conv-1"))
	}

	// Broadcast should reach all connections
	cm.Broadcast("conv-1", SSEEvent{Event: "test", Data: "{}"})

	// Verify all connections received the event
	for i, conn := range conns {
		select {
		case event := <-conn.EventChan:
			if event.Event != "test" {
				t.Errorf("connection %d: expected event 'test', got %s", i, event.Event)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("connection %d: timeout waiting for event", i)
		}
	}
}

func TestConnectionManager_BroadcastNonExistent(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)

	// Broadcast to non-existent conversation should not panic
	cm.Broadcast("nonexistent", SSEEvent{Event: "test", Data: "{}"})
}

func TestConnectionManager_CloseAll(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)

	conn1 := &SSEConnection{
		ID:             "conn-1",
		ConversationID: "conv-1",
		EventChan:      make(chan SSEEvent, 10),
		Done:           make(chan struct{}),
	}
	conn2 := &SSEConnection{
		ID:             "conn-2",
		ConversationID: "conv-2",
		EventChan:      make(chan SSEEvent, 10),
		Done:           make(chan struct{}),
	}
	cm.Add(conn1)
	cm.Add(conn2)

	if cm.GetTotalConnections() != 2 {
		t.Errorf("expected 2 connections, got %d", cm.GetTotalConnections())
	}

	cm.CloseAll()

	if cm.GetTotalConnections() != 0 {
		t.Errorf("expected 0 connections after CloseAll, got %d", cm.GetTotalConnections())
	}

	// Verify Done channels are closed
	select {
	case <-conn1.Done:
		// Expected
	default:
		t.Error("conn1.Done should be closed")
	}
	select {
	case <-conn2.Done:
		// Expected
	default:
		t.Error("conn2.Done should be closed")
	}
}

func TestSSEEventFormat(t *testing.T) {
	tests := []struct {
		name     string
		event    SSEEvent
		expected string
	}{
		{
			name:     "simple event",
			event:    SSEEvent{Event: "chunk", Data: `{"content":"hello"}`},
			expected: "event: chunk\ndata: {\"content\":\"hello\"}\n\n",
		},
		{
			name:     "event with ID",
			event:    SSEEvent{Event: "chunk", ID: "123", Data: `{}`},
			expected: "id: 123\nevent: chunk\ndata: {}\n\n",
		},
		{
			name:     "event with retry",
			event:    SSEEvent{Event: "error", Data: `{}`, Retry: 5000},
			expected: "event: error\nretry: 5000\ndata: {}\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.event.Format()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestNewChunkEvent(t *testing.T) {
	tests := []struct {
		name          string
		chunkType     pb.ContentChunk_ChunkType
		expectedType  string
	}{
		{"start", pb.ContentChunk_START, "start"},
		{"delta", pb.ContentChunk_DELTA, "delta"},
		{"end", pb.ContentChunk_END, "end"},
		{"replace", pb.ContentChunk_REPLACE, "replace"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk := &pb.ContentChunk{
				Type:              tt.chunkType,
				Content:           "Hello world",
				PlatformMessageId: "msg-123",
			}

			event := NewChunkEvent(chunk, "resp-1")

			if event.Event != EventChunk {
				t.Errorf("expected event type %s, got %s", EventChunk, event.Event)
			}

			var data ChunkEventData
			if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
				t.Fatalf("failed to unmarshal event data: %v", err)
			}

			if data.Content != "Hello world" {
				t.Errorf("expected content 'Hello world', got %s", data.Content)
			}
			if data.ChunkType != tt.expectedType {
				t.Errorf("expected chunk type %q, got %q", tt.expectedType, data.ChunkType)
			}
			if data.ResponseID != "resp-1" {
				t.Errorf("expected response ID 'resp-1', got %s", data.ResponseID)
			}
		})
	}
}

func TestNewConnectedEvent(t *testing.T) {
	event := NewConnectedEvent("conv-123", "conn-456")

	if event.Event != EventConnected {
		t.Errorf("expected event type %s, got %s", EventConnected, event.Event)
	}

	var data ConnectedEventData
	if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
		t.Fatalf("failed to unmarshal event data: %v", err)
	}

	if data.ConversationID != "conv-123" {
		t.Errorf("expected conversation ID 'conv-123', got %s", data.ConversationID)
	}
	if data.ConnectionID != "conn-456" {
		t.Errorf("expected connection ID 'conn-456', got %s", data.ConnectionID)
	}
}

func TestNewFinishEvent(t *testing.T) {
	event := NewFinishEvent("resp-123")

	if event.Event != EventFinish {
		t.Errorf("expected event type %s, got %s", EventFinish, event.Event)
	}

	var data FinishEventData
	if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
		t.Fatalf("failed to unmarshal event data: %v", err)
	}

	if data.ResponseID != "resp-123" {
		t.Errorf("expected response ID 'resp-123', got %s", data.ResponseID)
	}
}

func TestNewPromptsEvent(t *testing.T) {
	prompts := &pb.SuggestedPrompts{
		Prompts: []*pb.SuggestedPrompts_Prompt{
			{Id: "p1", Title: "Help", Message: "How can I help?", Description: "Get assistance"},
			{Id: "p2", Title: "Status", Message: "What is my status?"},
		},
	}

	event := NewPromptsEvent(prompts)

	if event.Event != EventPrompts {
		t.Errorf("expected event type %s, got %s", EventPrompts, event.Event)
	}

	var data PromptsEventData
	if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
		t.Fatalf("failed to unmarshal event data: %v", err)
	}

	if len(data.Prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(data.Prompts))
	}
	if data.Prompts[0].Title != "Help" {
		t.Errorf("expected first prompt title 'Help', got %s", data.Prompts[0].Title)
	}
}

func TestNewErrorEventFromMessage(t *testing.T) {
	event := NewErrorEventFromMessage("INTERNAL_ERROR", "Something went wrong", true)

	if event.Event != EventError {
		t.Errorf("expected event type %s, got %s", EventError, event.Event)
	}

	var data ErrorEventData
	if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
		t.Fatalf("failed to unmarshal event data: %v", err)
	}

	if data.Code != "INTERNAL_ERROR" {
		t.Errorf("expected code 'INTERNAL_ERROR', got %s", data.Code)
	}
	if data.Message != "Something went wrong" {
		t.Errorf("expected message 'Something went wrong', got %s", data.Message)
	}
	if !data.Retryable {
		t.Error("expected retryable to be true")
	}
}

func TestNewStatusEvent(t *testing.T) {
	status := &pb.StatusUpdate{
		Status:        pb.StatusUpdate_THINKING,
		CustomMessage: "Processing your request",
		Emoji:         ":thinking:",
	}

	event := NewStatusEvent(status)

	if event.Event != EventStatus {
		t.Errorf("expected event type %s, got %s", EventStatus, event.Event)
	}

	var data StatusEventData
	if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
		t.Fatalf("failed to unmarshal event data: %v", err)
	}

	if data.Message != "Processing your request" {
		t.Errorf("expected message 'Processing your request', got %s", data.Message)
	}
}

func TestNewErrorEvent(t *testing.T) {
	err := &pb.ErrorResponse{
		Code:      pb.ErrorResponse_RATE_LIMIT,
		Message:   "Too many requests",
		Details:   "Try again in 30 seconds",
		Retryable: true,
	}

	event := NewErrorEvent(err)

	if event.Event != EventError {
		t.Errorf("expected event type %s, got %s", EventError, event.Event)
	}

	var data ErrorEventData
	if jsonErr := json.Unmarshal([]byte(event.Data), &data); jsonErr != nil {
		t.Fatalf("failed to unmarshal event data: %v", jsonErr)
	}

	if data.Message != "Too many requests" {
		t.Errorf("expected message 'Too many requests', got %s", data.Message)
	}
	if !data.Retryable {
		t.Error("expected retryable to be true")
	}
}

func TestHandlers_CreateConversation(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &NoopSessionManager{}
	handlers := NewHandlers(cm, sm, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/conversations", nil)
	w := httptest.NewRecorder()

	handlers.HandleCreateConversation(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, w.Code)
	}

	var resp CreateConversationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.ConversationID == "" {
		t.Error("expected non-empty conversation ID")
	}
	// Conversation ID should be a UUID (no prefix)
	if len(resp.ConversationID) != 36 {
		t.Errorf("expected conversation ID to be a UUID (36 chars), got %s", resp.ConversationID)
	}
}

func TestHandlers_SendMessage(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &NoopSessionManager{}
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	handlers := NewHandlers(cm, sm, threadStore)

	var receivedMsg *pb.Message
	handlers.SetMessageHandler(func(ctx context.Context, msg *pb.Message) error {
		receivedMsg = msg
		return nil
	})

	body := `{"content":"Hello, world!"}`
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/conv-123/messages", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "conv-123")
	w := httptest.NewRecorder()

	handlers.HandleSendMessage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp SendMessageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.MessageID == "" {
		t.Error("expected non-empty message ID")
	}

	if receivedMsg == nil {
		t.Fatal("expected message handler to be called")
	}
	if receivedMsg.Content != "Hello, world!" {
		t.Errorf("expected content 'Hello, world!', got %s", receivedMsg.Content)
	}
	if receivedMsg.ConversationId != "conv-123" {
		t.Errorf("expected conversation ID 'conv-123', got %s", receivedMsg.ConversationId)
	}
}

func TestHandlers_SendMessage_Unauthorized(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &HeaderSessionManager{UserIDHeader: "X-User-ID"} // Requires header
	handlers := NewHandlers(cm, sm, nil)

	body := `{"content":"Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/conv-123/messages", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "conv-123")
	w := httptest.NewRecorder()

	handlers.HandleSendMessage(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestHandlers_SendMessage_EmptyContent(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &NoopSessionManager{}
	handlers := NewHandlers(cm, sm, nil)

	body := `{"content":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/conv-123/messages", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "conv-123")
	w := httptest.NewRecorder()

	handlers.HandleSendMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandlers_SendMessage_InvalidJSON(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &NoopSessionManager{}
	handlers := NewHandlers(cm, sm, nil)

	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/conv-123/messages", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "conv-123")
	w := httptest.NewRecorder()

	handlers.HandleSendMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandlers_SendMessage_NoHandler(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &NoopSessionManager{}
	handlers := NewHandlers(cm, sm, nil)
	// Note: no SetMessageHandler called

	body := `{"content":"Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/conv-123/messages", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "conv-123")
	w := httptest.NewRecorder()

	handlers.HandleSendMessage(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d: %s", http.StatusServiceUnavailable, w.Code, w.Body.String())
	}
}

func TestHandlers_SendMessage_HandlerError(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &NoopSessionManager{}
	handlers := NewHandlers(cm, sm, nil)

	handlers.SetMessageHandler(func(ctx context.Context, msg *pb.Message) error {
		return context.DeadlineExceeded // Simulate an error
	})

	body := `{"content":"Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/conv-123/messages", bytes.NewReader([]byte(body)))
	req.SetPathValue("id", "conv-123")
	w := httptest.NewRecorder()

	handlers.HandleSendMessage(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d: %s", http.StatusInternalServerError, w.Code, w.Body.String())
	}
}

func TestHandlers_SendMessage_MissingConversationID(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &NoopSessionManager{}
	handlers := NewHandlers(cm, sm, nil)

	body := `{"content":"Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/conversations//messages", bytes.NewReader([]byte(body)))
	// Note: not setting path value
	w := httptest.NewRecorder()

	handlers.HandleSendMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestHandlers_History(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &NoopSessionManager{}
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	handlers := NewHandlers(cm, sm, threadStore)

	// Add some messages
	threadStore.AddMessage("conv-123", &pb.ThreadMessage{
		MessageId: "msg-1",
		Content:   "First message",
		User:      &pb.User{Id: "user-1", Username: "Alice"},
	})
	threadStore.AddMessage("conv-123", &pb.ThreadMessage{
		MessageId: "msg-2",
		Content:   "Second message",
		User:      &pb.User{Id: "user-2", Username: "Bob"},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/conversations/conv-123/history", nil)
	req.SetPathValue("id", "conv-123")
	w := httptest.NewRecorder()

	handlers.HandleHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	messages, ok := resp["messages"].([]any)
	if !ok {
		t.Fatal("expected messages array in response")
	}
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
}

func TestHandlers_Health(t *testing.T) {
	cm := NewConnectionManager(30 * time.Second)
	sm := &NoopSessionManager{}
	handlers := NewHandlers(cm, sm, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handlers.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", resp["status"])
	}
}

func TestWebAdapter_Capabilities(t *testing.T) {
	adapter := New()
	caps := adapter.Capabilities()

	if !caps.SupportsStreaming {
		t.Error("expected SupportsStreaming to be true")
	}
	if !caps.SupportsStatusUpdates {
		t.Error("expected SupportsStatusUpdates to be true")
	}
	if !caps.SupportsSuggestedPrompts {
		t.Error("expected SupportsSuggestedPrompts to be true")
	}
	if !caps.SupportsThreads {
		t.Error("expected SupportsThreads to be true")
	}
	if caps.SupportsTypingIndicator {
		t.Error("expected SupportsTypingIndicator to be false")
	}
	if caps.MaxUpdateRateHz != 0 {
		t.Errorf("expected MaxUpdateRateHz to be 0, got %f", caps.MaxUpdateRateHz)
	}
}

func TestWebAdapter_GetPlatformName(t *testing.T) {
	adapter := New()
	if adapter.GetPlatformName() != "web" {
		t.Errorf("expected platform name 'web', got %s", adapter.GetPlatformName())
	}
}

func TestWebAdapter_HandleAgentResponse_Content(t *testing.T) {
	adapter := New()
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	adapter.SetThreadStore(threadStore)

	// Initialize adapter to create connection manager
	ctx := context.Background()
	if err := adapter.Initialize(ctx, adapter.config); err != nil {
		t.Fatalf("failed to initialize adapter: %v", err)
	}

	// Create a connection to receive events
	conn := &SSEConnection{
		ID:             "test-conn",
		ConversationID: "conv-123",
		EventChan:      make(chan SSEEvent, 10),
		Done:           make(chan struct{}),
	}
	adapter.connManager.Add(conn)

	// Send a content response
	response := &pb.AgentResponse{
		ConversationId: "conv-123",
		ResponseId:     "resp-1",
		Payload: &pb.AgentResponse_Content{
			Content: &pb.ContentChunk{
				Type:    pb.ContentChunk_DELTA,
				Content: "Hello from agent",
			},
		},
	}

	if err := adapter.HandleAgentResponse(ctx, response); err != nil {
		t.Fatalf("HandleAgentResponse failed: %v", err)
	}

	// Verify event was broadcast
	select {
	case event := <-conn.EventChan:
		if event.Event != EventChunk {
			t.Errorf("expected event type %s, got %s", EventChunk, event.Event)
		}
		var data ChunkEventData
		if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
			t.Fatalf("failed to unmarshal event data: %v", err)
		}
		if data.Content != "Hello from agent" {
			t.Errorf("expected content 'Hello from agent', got %s", data.Content)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestWebAdapter_HandleAgentResponse_ContentEnd(t *testing.T) {
	adapter := New()
	threadStore := store.NewThreadHistoryStore(100, 50, time.Hour)
	adapter.SetThreadStore(threadStore)

	ctx := context.Background()
	if err := adapter.Initialize(ctx, adapter.config); err != nil {
		t.Fatalf("failed to initialize adapter: %v", err)
	}

	conn := &SSEConnection{
		ID:             "test-conn",
		ConversationID: "conv-123",
		EventChan:      make(chan SSEEvent, 10),
		Done:           make(chan struct{}),
	}
	adapter.connManager.Add(conn)

	// Send END chunk - should also send finish event
	response := &pb.AgentResponse{
		ConversationId: "conv-123",
		ResponseId:     "resp-1",
		Payload: &pb.AgentResponse_Content{
			Content: &pb.ContentChunk{
				Type:    pb.ContentChunk_END,
				Content: "Final content",
			},
		},
	}

	if err := adapter.HandleAgentResponse(ctx, response); err != nil {
		t.Fatalf("HandleAgentResponse failed: %v", err)
	}

	// Should receive chunk event
	event1 := <-conn.EventChan
	if event1.Event != EventChunk {
		t.Errorf("expected first event type %s, got %s", EventChunk, event1.Event)
	}

	// Should also receive finish event
	event2 := <-conn.EventChan
	if event2.Event != EventFinish {
		t.Errorf("expected second event type %s, got %s", EventFinish, event2.Event)
	}

	// Verify message was stored in thread history
	history := threadStore.GetHistory("conv-123", 10, false)
	if len(history.Messages) != 1 {
		t.Errorf("expected 1 message in history, got %d", len(history.Messages))
	}
}

func TestWebAdapter_HandleAgentResponse_Status(t *testing.T) {
	adapter := New()

	ctx := context.Background()
	if err := adapter.Initialize(ctx, adapter.config); err != nil {
		t.Fatalf("failed to initialize adapter: %v", err)
	}

	conn := &SSEConnection{
		ID:             "test-conn",
		ConversationID: "conv-123",
		EventChan:      make(chan SSEEvent, 10),
		Done:           make(chan struct{}),
	}
	adapter.connManager.Add(conn)

	response := &pb.AgentResponse{
		ConversationId: "conv-123",
		Payload: &pb.AgentResponse_Status{
			Status: &pb.StatusUpdate{
				Status:        pb.StatusUpdate_THINKING,
				CustomMessage: "Processing...",
			},
		},
	}

	if err := adapter.HandleAgentResponse(ctx, response); err != nil {
		t.Fatalf("HandleAgentResponse failed: %v", err)
	}

	select {
	case event := <-conn.EventChan:
		if event.Event != EventStatus {
			t.Errorf("expected event type %s, got %s", EventStatus, event.Event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestWebAdapter_HandleAgentResponse_Error(t *testing.T) {
	adapter := New()

	ctx := context.Background()
	if err := adapter.Initialize(ctx, adapter.config); err != nil {
		t.Fatalf("failed to initialize adapter: %v", err)
	}

	conn := &SSEConnection{
		ID:             "test-conn",
		ConversationID: "conv-123",
		EventChan:      make(chan SSEEvent, 10),
		Done:           make(chan struct{}),
	}
	adapter.connManager.Add(conn)

	response := &pb.AgentResponse{
		ConversationId: "conv-123",
		Payload: &pb.AgentResponse_Error{
			Error: &pb.ErrorResponse{
				Code:      pb.ErrorResponse_AGENT_ERROR,
				Message:   "Something went wrong",
				Retryable: false,
			},
		},
	}

	if err := adapter.HandleAgentResponse(ctx, response); err != nil {
		t.Fatalf("HandleAgentResponse failed: %v", err)
	}

	select {
	case event := <-conn.EventChan:
		if event.Event != EventError {
			t.Errorf("expected event type %s, got %s", EventError, event.Event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestWebAdapter_HandleAgentResponse_MissingConversationID(t *testing.T) {
	adapter := New()

	ctx := context.Background()
	if err := adapter.Initialize(ctx, adapter.config); err != nil {
		t.Fatalf("failed to initialize adapter: %v", err)
	}

	response := &pb.AgentResponse{
		ConversationId: "", // Missing
		Payload: &pb.AgentResponse_Content{
			Content: &pb.ContentChunk{
				Type:    pb.ContentChunk_DELTA,
				Content: "test",
			},
		},
	}

	err := adapter.HandleAgentResponse(ctx, response)
	if err == nil {
		t.Error("expected error for missing conversation ID")
	}
}

func TestWebAdapter_IsHealthy(t *testing.T) {
	adapter := New()

	// Before initialization - not healthy (no server, no connManager)
	if adapter.IsHealthy(context.Background()) {
		t.Error("expected IsHealthy to be false before initialization")
	}

	// After initialization - still not healthy (server is created in Start())
	ctx := context.Background()
	if err := adapter.Initialize(ctx, adapter.config); err != nil {
		t.Fatalf("failed to initialize adapter: %v", err)
	}

	// IsHealthy requires both server and connManager
	// Initialize only creates connManager, Start() creates server
	// So after Initialize alone, not yet healthy
	if adapter.IsHealthy(context.Background()) {
		t.Error("expected IsHealthy to be false after Initialize (server not started)")
	}

	// Verify connManager was created
	if adapter.connManager == nil {
		t.Error("expected connManager to be initialized")
	}
}

func TestWebAdapter_Options(t *testing.T) {
	// Test WithListenAddr
	adapter := New(WithListenAddr(":9090"))
	if adapter.listenAddr != ":9090" {
		t.Errorf("expected listen addr ':9090', got %s", adapter.listenAddr)
	}

	// Test WithHeartbeatInterval
	adapter2 := New(WithHeartbeatInterval(60 * time.Second))
	if adapter2.heartbeatInterval != 60*time.Second {
		t.Errorf("expected heartbeat interval 60s, got %v", adapter2.heartbeatInterval)
	}

	// Test WithSessionManager
	sm := &HeaderSessionManager{UserIDHeader: "X-User"}
	adapter3 := New(WithSessionManager(sm))
	if adapter3.sessionManager != sm {
		t.Error("expected custom session manager to be set")
	}
}

func TestHeaderSessionManager(t *testing.T) {
	sm := NewHeaderSessionManager("X-User-ID", "X-Username", "X-Email")

	// Test with valid headers
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-User-ID", "user-123")
	req.Header.Set("X-Username", "testuser")
	req.Header.Set("X-Email", "test@example.com")

	session, err := sm.ValidateRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.UserID != "user-123" {
		t.Errorf("expected user ID 'user-123', got %s", session.UserID)
	}
	if session.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %s", session.Username)
	}

	// Test without headers
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	session2, err := sm.ValidateRequest(context.Background(), req2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session2 != nil {
		t.Error("expected nil session when no headers present")
	}
}

func TestBearerTokenSessionManager(t *testing.T) {
	sm := NewBearerTokenSessionManager(func(ctx context.Context, token string) (*Session, error) {
		if token == "valid-token" {
			return &Session{
				UserID:   "user-from-token",
				Username: "tokenuser",
			}, nil
		}
		return nil, nil
	})

	// Test with valid token
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")

	session, err := sm.ValidateRequest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.UserID != "user-from-token" {
		t.Errorf("expected user ID 'user-from-token', got %s", session.UserID)
	}

	// Test with invalid token
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Authorization", "Bearer invalid-token")

	session2, err := sm.ValidateRequest(context.Background(), req2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session2 != nil {
		t.Error("expected nil session for invalid token")
	}

	// Test without authorization header
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	session3, err := sm.ValidateRequest(context.Background(), req3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session3 != nil {
		t.Error("expected nil session when no auth header")
	}
}
