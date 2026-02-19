package grpc

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	pb "github.com/astro/messaging/pkg/gen/astro/messaging/v1"
	"github.com/astro/messaging/internal/adapter"
	"github.com/astro/messaging/internal/store"
	"github.com/astro/messaging/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server implements the AgentMessaging gRPC service
type Server struct {
	pb.UnimplementedAgentMessagingServer

	// Adapters for different platforms
	adapters map[string]adapter.Adapter
	mu       sync.RWMutex

	// Thread history store
	threadStore *store.ThreadHistoryStore

	// Agent config store
	agentConfigStore *store.AgentConfigStore

	// Conversation metadata cache
	conversationCache store.ConversationStore

	// Active streams (for bidirectional communication)
	streams     map[string]*conversationStream
	streamsMu   sync.RWMutex

	// gRPC server instance
	grpcServer *grpc.Server
	listenAddr string
}

// conversationStream holds bidirectional stream state
type conversationStream struct {
	stream         pb.AgentMessaging_ProcessConversationServer
	conversationID string
	platformAdapter adapter.Adapter
	cancel         context.CancelFunc
}

// NewServer creates a new gRPC server
func NewServer(listenAddr string, threadStore *store.ThreadHistoryStore, convStore store.ConversationStore, agentConfigStore *store.AgentConfigStore) *Server {
	return &Server{
		adapters:          make(map[string]adapter.Adapter),
		threadStore:       threadStore,
		agentConfigStore:  agentConfigStore,
		conversationCache: convStore,
		streams:           make(map[string]*conversationStream),
		listenAddr:        listenAddr,
	}
}

// RegisterAdapter registers a platform adapter
func (s *Server) RegisterAdapter(name string, adpt adapter.Adapter) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.adapters[name] = adpt
	log.Printf("[gRPC] Registered adapter: %s", name)
}

// Start starts the gRPC server
func (s *Server) Start(ctx context.Context) error {
	lis, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.grpcServer = grpc.NewServer(
		grpc.MaxConcurrentStreams(100),
		grpc.MaxRecvMsgSize(4 * 1024 * 1024), // 4MB
		grpc.MaxSendMsgSize(4 * 1024 * 1024), // 4MB
	)

	pb.RegisterAgentMessagingServer(s.grpcServer, s)

	log.Printf("[gRPC] Server listening on %s", s.listenAddr)

	// Start server in goroutine
	go func() {
		if err := s.grpcServer.Serve(lis); err != nil {
			log.Printf("[gRPC] Server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Println("[gRPC] Shutting down server...")
	s.grpcServer.GracefulStop()

	return nil
}

// Stop stops the gRPC server
func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}

// ProcessConversation handles bidirectional streaming
func (s *Server) ProcessConversation(stream pb.AgentMessaging_ProcessConversationServer) error {
	ctx := stream.Context()
	streamID := fmt.Sprintf("stream-%p", stream)

	log.Printf("[gRPC] New bidirectional stream: %s", streamID)

	// Wait for initial registration message from agent
	req, err := stream.Recv()
	if err != nil {
		log.Printf("[gRPC] Stream closed before registration: %v", err)
		return err
	}

	// Extract conversation ID from first message (agent should send a registration)
	var conversationID string
	switch payload := req.Request.(type) {
	case *pb.ConversationRequest_Message:
		conversationID = payload.Message.ConversationId
	case *pb.ConversationRequest_AgentConfig:
		// Agent sent config as first message; store it and wait for registration
		if s.agentConfigStore != nil {
			s.agentConfigStore.Set(payload.AgentConfig)
			log.Printf("[gRPC] Stored agent config from stream")
		}
		conversationID = "agent-stream"
	default:
		// For now, use a generic ID if no message provided
		conversationID = "agent-stream"
	}

	// Create stream context with cancellation
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Register the stream
	s.streamsMu.Lock()
	s.streams[conversationID] = &conversationStream{
		stream:         stream,
		conversationID: conversationID,
		cancel:         cancel,
	}
	s.streamsMu.Unlock()

	log.Printf("[gRPC] Registered agent stream for conversation: %s", conversationID)

	// Clean up on exit
	defer func() {
		s.streamsMu.Lock()
		delete(s.streams, conversationID)
		s.streamsMu.Unlock()
		log.Printf("[gRPC] Unregistered agent stream: %s", conversationID)
	}()

	// Handle incoming requests from agent
	for {
		req, err := stream.Recv()
		if err != nil {
			log.Printf("[gRPC] Stream closed: %v", err)
			return err
		}

		switch payload := req.Request.(type) {
		case *pb.ConversationRequest_Message:
			// Agent sending a response message
			msg := payload.Message
			log.Printf("[gRPC] Agent response for conversation: %s", msg.ConversationId)

			// Route to appropriate adapter
			if err := s.routeAgentMessage(streamCtx, msg); err != nil {
				log.Printf("[gRPC] Error routing agent message: %v", err)
			}

		case *pb.ConversationRequest_Feedback:
			// Agent acknowledging feedback
			log.Printf("[gRPC] Agent feedback: %s", payload.Feedback.ConversationId)

		case *pb.ConversationRequest_AgentConfig:
			// Agent sending/updating its config
			if s.agentConfigStore != nil {
				s.agentConfigStore.Set(payload.AgentConfig)
				log.Printf("[gRPC] Stored agent config from stream")
			}

		case *pb.ConversationRequest_AgentResponse:
			// Agent sending a typed response (ContentChunk, StatusUpdate, etc.)
			response := payload.AgentResponse
			log.Printf("[gRPC] Agent response for conversation: %s", response.ConversationId)
			if err := s.routeAgentResponse(streamCtx, response); err != nil {
				log.Printf("[gRPC] Error routing agent response: %v", err)
			}

		default:
			log.Printf("[gRPC] Unknown request type in stream")
		}

		// Check if context is cancelled
		select {
		case <-streamCtx.Done():
			return streamCtx.Err()
		default:
		}
	}
}

// ProcessMessage handles server-side streaming (message → stream of responses)
func (s *Server) ProcessMessage(req *pb.Message, stream pb.AgentMessaging_ProcessMessageServer) error {
	log.Printf("[gRPC] ProcessMessage: %s from %s", req.Id, req.Platform)

	// This would typically forward the message to an agent
	// For now, we'll just acknowledge
	// In a full implementation, this would:
	// 1. Route to appropriate agent
	// 2. Stream agent responses back to caller

	return nil
}

// GetThreadHistory returns thread history from store
func (s *Server) GetThreadHistory(ctx context.Context, req *pb.ThreadHistoryRequest) (*pb.ThreadHistoryResponse, error) {
	log.Printf("[gRPC] GetThreadHistory: %s (max: %d)", req.ConversationId, req.MaxMessages)

	// Check if we need to hydrate from platform
	if s.threadStore.IsStale(req.ConversationId, s.getRefreshInterval()) {
		log.Printf("[gRPC] Thread history stale, hydrating from platform...")

		// Find the right adapter based on conversation metadata
		if err := s.hydrateThreadHistory(ctx, req.ConversationId); err != nil {
			log.Printf("[gRPC] Failed to hydrate: %v", err)
			// Continue with cached data if available
		}
	}

	// Get history from store
	maxMessages := int(req.MaxMessages)
	if maxMessages == 0 {
		maxMessages = 50 // Default
	}

	includeDeleted := req.IncludeDeleted
	history := s.threadStore.GetHistory(req.ConversationId, maxMessages, includeDeleted)

	log.Printf("[gRPC] Returning %d messages for conversation %s", len(history.Messages), req.ConversationId)

	return history, nil
}

// GetConversationMetadata returns conversation metadata
func (s *Server) GetConversationMetadata(ctx context.Context, req *pb.ConversationMetadataRequest) (*pb.ConversationMetadataResponse, error) {
	var conversationID string

	switch id := req.Identifier.(type) {
	case *pb.ConversationMetadataRequest_ConversationId:
		conversationID = id.ConversationId

	case *pb.ConversationMetadataRequest_PlatformId:
		// Build conversation ID from platform identifiers
		conversationID = buildConversationID(id.PlatformId.Platform, id.PlatformId.ChannelId, id.PlatformId.ThreadId)

	default:
		return &pb.ConversationMetadataResponse{Found: false}, nil
	}

	// Get from cache
	conv, err := s.conversationCache.Get(ctx, conversationID)
	if err != nil {
		return &pb.ConversationMetadataResponse{
			ConversationId: conversationID,
			Found:          false,
		}, nil
	}

	return &pb.ConversationMetadataResponse{
		ConversationId:  conversationID,
		Platform:        conv.Platform,
		ChannelId:       conv.ChannelID,
		ThreadId:        conv.ThreadID,
		LastMessageTime: timestamppb.New(conv.LastMessageAt),
		MessageCount:    int32(conv.MessageCount),
		Found:           true,
	}, nil
}

// HealthCheck returns server health status
func (s *Server) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	// Check adapter health
	s.mu.RLock()
	defer s.mu.RUnlock()

	allHealthy := true
	for name, adpt := range s.adapters {
		if !adpt.IsHealthy(ctx) {
			log.Printf("[gRPC] Adapter unhealthy: %s", name)
			allHealthy = false
		}
	}

	status := pb.HealthCheckResponse_HEALTHY
	if !allHealthy {
		status = pb.HealthCheckResponse_DEGRADED
	}

	return &pb.HealthCheckResponse{
		Status:  status,
		Version: "1.0.0",
	}, nil
}

// HandleIncomingMessage is called by adapters when a message arrives from a platform
// This routes the message to the agent via gRPC stream
func (s *Server) HandleIncomingMessage(ctx context.Context, msg *pb.Message) error {
	log.Printf("[gRPC] Incoming platform message: %s from %s", msg.Id, msg.Platform)

	// Update conversation cache
	if err := s.updateConversationCache(ctx, msg); err != nil {
		log.Printf("[gRPC] Warning: failed to update conversation cache: %v", err)
	}

	// Find active agent stream
	// For now, use a single agent stream (multi-agent routing can be added later)
	s.streamsMu.RLock()
	var stream *conversationStream
	for _, s := range s.streams {
		stream = s
		break
	}
	s.streamsMu.RUnlock()

	if stream == nil {
		return fmt.Errorf("no active agent stream available")
	}

	// Send message to agent via stream wrapped in AgentResponse with incoming_message payload
	response := &pb.AgentResponse{
		ConversationId: msg.ConversationId,
		ResponseId:     msg.Id,
		Payload: &pb.AgentResponse_IncomingMessage{
			IncomingMessage: msg,
		},
	}

	if err := stream.stream.Send(response); err != nil {
		return fmt.Errorf("failed to send to agent: %w", err)
	}

	log.Printf("[gRPC] Message forwarded to agent via stream")
	return nil
}

// routeAgentMessage routes agent responses back to the appropriate platform adapter
func (s *Server) routeAgentMessage(ctx context.Context, msg *pb.Message) error {
	log.Printf("[gRPC] Routing agent message to platform: %s", msg.Platform)
	log.Printf("[gRPC] Message content: %s", msg.Content)
	log.Printf("[gRPC] PlatformContext: %+v", msg.PlatformContext)

	// Get the adapter for this platform
	s.mu.RLock()
	adpt, exists := s.adapters[msg.Platform]
	s.mu.RUnlock()

	if !exists {
		log.Printf("[gRPC] ERROR: No adapter found for platform: %s", msg.Platform)
		return fmt.Errorf("no adapter for platform: %s", msg.Platform)
	}

	// Check if PlatformContext is nil
	if msg.PlatformContext == nil {
		log.Printf("[gRPC] ERROR: PlatformContext is nil")
		return fmt.Errorf("platform context is nil")
	}

	// Convert protobuf message to internal types
	// Use ThreadId if present, otherwise use MessageId (for non-threaded messages)
	// This ensures we can clear the loading state on the correct message
	threadTS := msg.PlatformContext.ThreadId
	if threadTS == "" {
		threadTS = msg.PlatformContext.MessageId
	}

	req := &types.SendMessageRequest{
		Platform:  msg.Platform,
		ChannelID: msg.PlatformContext.ChannelId,
		ThreadID:  threadTS,
		Content:   msg.Content,
	}

	log.Printf("[gRPC] Sending message via adapter: channel=%s, thread=%s", req.ChannelID, req.ThreadID)

	// Send via adapter
	result, err := adpt.SendMessage(ctx, req)
	if err != nil {
		log.Printf("[gRPC] Failed to send agent message to %s: %v", msg.Platform, err)
		return fmt.Errorf("failed to send to platform: %w", err)
	}

	log.Printf("[gRPC] ✅ Agent message sent to %s: message_id=%s", msg.Platform, result.MessageID)
	return nil
}

// routeAgentResponse routes typed AgentResponse payloads (ContentChunk, StatusUpdate, etc.)
// back to the appropriate platform adapter via HandleAgentResponse
func (s *Server) routeAgentResponse(ctx context.Context, response *pb.AgentResponse) error {
	conversationID := response.ConversationId

	// Try to find the platform from conversation cache
	conv, err := s.conversationCache.Get(ctx, conversationID)
	if err == nil {
		// Found in cache — route to the specific adapter
		s.mu.RLock()
		adpt, exists := s.adapters[conv.Platform]
		s.mu.RUnlock()

		if exists {
			if grpcAdapter, ok := adpt.(adapter.GRPCAdapter); ok {
				return grpcAdapter.HandleAgentResponse(ctx, response)
			}
		}
	}

	// Conversation not in cache — broadcast to all adapters
	// Each adapter handles unknown conversations gracefully
	log.Printf("[gRPC] Conversation %s not in cache, broadcasting to all adapters", conversationID)
	s.mu.RLock()
	defer s.mu.RUnlock()

	for name, adpt := range s.adapters {
		if grpcAdapter, ok := adpt.(adapter.GRPCAdapter); ok {
			if err := grpcAdapter.HandleAgentResponse(ctx, response); err != nil {
				log.Printf("[gRPC] Error routing agent response to %s: %v", name, err)
			}
		}
	}

	return nil
}

// updateConversationCache updates the conversation metadata cache
func (s *Server) updateConversationCache(ctx context.Context, msg *pb.Message) error {
	conversationID := msg.ConversationId

	// Try to get existing conversation
	conv, err := s.conversationCache.Get(ctx, conversationID)
	if err != nil {
		// Create new conversation entry so routeAgentResponse can look up the platform
		var channelID, threadID string
		if msg.PlatformContext != nil {
			channelID = msg.PlatformContext.ChannelId
			threadID = msg.PlatformContext.ThreadId
		}

		now := time.Now()
		return s.conversationCache.Create(ctx, &types.ConversationContext{
			ConversationID: conversationID,
			Platform:       msg.Platform,
			ChannelID:      channelID,
			ThreadID:       threadID,
			UserID:         msg.User.GetId(),
			CreatedAt:      now,
			LastMessageAt:  now,
			MessageCount:   1,
		})
	}

	// Update metadata
	conv.LastMessageAt = time.Now()
	conv.MessageCount++

	return s.conversationCache.Update(ctx, conv)
}

// SendToAgent sends a message to an agent stream
func (s *Server) SendToAgent(conversationID string, response *pb.AgentResponse) error {
	s.streamsMu.RLock()
	stream, exists := s.streams[conversationID]
	s.streamsMu.RUnlock()

	if !exists {
		return fmt.Errorf("no active stream for conversation: %s", conversationID)
	}

	return stream.stream.Send(response)
}

// hydrateThreadHistory fetches thread history from the appropriate adapter
func (s *Server) hydrateThreadHistory(ctx context.Context, conversationID string) error {
	// Get conversation metadata to find the platform
	conv, err := s.conversationCache.Get(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("conversation not found: %w", err)
	}

	// Get the adapter for this platform
	s.mu.RLock()
	adpt, exists := s.adapters[conv.Platform]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no adapter for platform: %s", conv.Platform)
	}

	// Check if adapter supports thread hydration
	hydrator, ok := adpt.(ThreadHydrator)
	if !ok {
		return fmt.Errorf("adapter does not support thread hydration: %s", conv.Platform)
	}

	// Hydrate from platform
	return hydrator.HydrateThread(ctx, conversationID, s.threadStore)
}

func (s *Server) getRefreshInterval() time.Duration {
	return 5 * time.Minute
}

// ThreadHydrator interface for adapters that support thread hydration
type ThreadHydrator interface {
	HydrateThread(ctx context.Context, conversationID string, store *store.ThreadHistoryStore) error
}

// Helper function to build conversation ID
func buildConversationID(platform, channelID, threadID string) string {
	if threadID != "" {
		return fmt.Sprintf("%s-%s-%s", platform, channelID, threadID)
	}
	return fmt.Sprintf("%s-%s", platform, channelID)
}

// HandleIncomingMessageFromAdapter is an adapter-compatible message handler
// It converts types.UnifiedMessage to protobuf and routes to agent via gRPC
func (s *Server) HandleIncomingMessageFromAdapter(ctx context.Context, msg *types.UnifiedMessage) (*types.AgentResponse, error) {
	log.Printf("[gRPC] Converting adapter message to protobuf: %s", msg.ID)

	// Convert internal type to protobuf
	pbMsg := &pb.Message{
		Id:             msg.ID,
		Timestamp:      timestamppb.New(msg.Timestamp),
		Platform:       msg.Platform,
		ConversationId: msg.ConversationID,
		Content:        msg.Content,
		PlatformContext: &pb.PlatformContext{
			MessageId: msg.PlatformMessageID,
			ChannelId: msg.ChannelID,
			ThreadId:  msg.ThreadID,
		},
		User: &pb.User{
			Id:       msg.UserID,
			Username: msg.UserName,
		},
	}

	// Convert attachments
	for _, att := range msg.Attachments {
		pbMsg.Attachments = append(pbMsg.Attachments, &pb.Attachment{
			Type:     pb.Attachment_Type(pb.Attachment_Type_value[att.Type]),
			Url:      att.URL,
			Filename: att.Name,
			MimeType: att.MimeType,
			SizeBytes: att.Size,
		})
	}

	// Send to agent via gRPC
	if err := s.HandleIncomingMessage(ctx, pbMsg); err != nil {
		return nil, err
	}

	// Return empty response - don't send any acknowledgment message
	// The agent will send the actual response via the stream
	// The Slack adapter will show a loading state while waiting
	return &types.AgentResponse{
		Content: "",
	}, nil
}
