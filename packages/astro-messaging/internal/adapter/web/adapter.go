package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	pb "github.com/astro/messaging/pkg/gen/astro/messaging/v1"
	"github.com/astro/messaging/internal/adapter"
	"github.com/astro/messaging/internal/store"
)

// WebAdapter implements GRPCAdapter and StreamingAdapter for web browser clients
type WebAdapter struct {
	config         adapter.Config
	grpcHandler    adapter.GRPCMessageHandler
	connManager    *ConnectionManager
	sessionManager SessionManager
	threadStore    *store.ThreadHistoryStore
	server         *http.Server
	handlers       *Handlers

	// Configuration
	listenAddr        string
	heartbeatInterval time.Duration
}

// WebAdapterOption configures the WebAdapter
type WebAdapterOption func(*WebAdapter)

// WithListenAddr sets the listen address for the HTTP server
func WithListenAddr(addr string) WebAdapterOption {
	return func(a *WebAdapter) {
		a.listenAddr = addr
	}
}

// WithSessionManager sets the session manager
func WithSessionManager(sm SessionManager) WebAdapterOption {
	return func(a *WebAdapter) {
		a.sessionManager = sm
	}
}

// WithHeartbeatInterval sets the SSE heartbeat interval
func WithHeartbeatInterval(d time.Duration) WebAdapterOption {
	return func(a *WebAdapter) {
		a.heartbeatInterval = d
	}
}

// New creates a new WebAdapter
func New(opts ...WebAdapterOption) *WebAdapter {
	a := &WebAdapter{
		listenAddr:        ":8080",
		heartbeatInterval: 30 * time.Second,
		sessionManager:    &NoopSessionManager{},
	}

	for _, opt := range opts {
		opt(a)
	}

	return a
}

// Initialize sets up the web adapter with configuration
func (a *WebAdapter) Initialize(ctx context.Context, config adapter.Config) error {
	a.config = config

	// Initialize connection manager
	a.connManager = NewConnectionManager(a.heartbeatInterval)

	// Initialize handlers
	a.handlers = NewHandlers(a.connManager, a.sessionManager, a.threadStore)

	log.Printf("[Web] Adapter initialized (listen: %s)", a.listenAddr)
	return nil
}

// Start begins the HTTP server and SSE connections
func (a *WebAdapter) Start(ctx context.Context) error {
	// Start connection manager heartbeat
	a.connManager.Start(ctx)

	// Set up HTTP routes
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("POST /api/conversations", a.handlers.HandleCreateConversation)
	mux.HandleFunc("POST /api/conversations/{id}/messages", a.handlers.HandleSendMessage)
	mux.HandleFunc("GET /api/conversations/{id}/stream", a.handlers.HandleStream)
	mux.HandleFunc("GET /api/conversations/{id}/history", a.handlers.HandleHistory)
	mux.HandleFunc("GET /health", a.handlers.HandleHealth)

	// Create server
	a.server = &http.Server{
		Addr:         a.listenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // No timeout for SSE
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("[Web] Starting HTTP server on %s", a.listenAddr)

	// Start server in goroutine
	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Web] HTTP server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	return a.Stop(context.Background())
}

// Stop gracefully shuts down the adapter
func (a *WebAdapter) Stop(ctx context.Context) error {
	log.Println("[Web] Stopping adapter...")

	// Close all SSE connections
	if a.connManager != nil {
		a.connManager.CloseAll()
		a.connManager.Stop()
	}

	// Shutdown HTTP server
	if a.server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			log.Printf("[Web] Error shutting down server: %v", err)
			return err
		}
	}

	log.Println("[Web] Adapter stopped")
	return nil
}

// Capabilities returns the adapter's capabilities
func (a *WebAdapter) Capabilities() adapter.AdapterCapabilities {
	return adapter.WebCapabilities()
}

// GetPlatformName returns the platform identifier
func (a *WebAdapter) GetPlatformName() string {
	return "web"
}

// IsHealthy checks if the adapter is connected and healthy
func (a *WebAdapter) IsHealthy(ctx context.Context) bool {
	return a.server != nil && a.connManager != nil
}

// SetMessageHandler sets the handler for incoming messages from the web client
func (a *WebAdapter) SetMessageHandler(handler adapter.GRPCMessageHandler) {
	a.grpcHandler = handler
	// Note: handlers.grpcHandler is set during Initialize via NewHandlers
	// This method is called after Initialize, so handlers is guaranteed to be non-nil
	if a.handlers != nil {
		a.handlers.SetMessageHandler(func(ctx context.Context, msg *pb.Message) error {
			return handler(ctx, msg)
		})
	}
}

// HandleAgentResponse processes responses from the agent and sends to SSE clients
func (a *WebAdapter) HandleAgentResponse(ctx context.Context, response *pb.AgentResponse) error {
	conversationID := response.ConversationId
	if conversationID == "" {
		return fmt.Errorf("missing conversation ID in response")
	}

	// Convert response to SSE events based on payload type
	switch payload := response.Payload.(type) {
	case *pb.AgentResponse_Content:
		// Content chunk
		event := NewChunkEvent(payload.Content, response.ResponseId)
		a.connManager.Broadcast(conversationID, event)

		// Send finish event on END chunk
		if payload.Content.Type == pb.ContentChunk_END {
			finishEvent := NewFinishEvent(response.ResponseId)
			a.connManager.Broadcast(conversationID, finishEvent)
		}

		// Store message content for thread history
		if a.threadStore != nil && payload.Content.Type == pb.ContentChunk_END {
			a.threadStore.AddMessage(conversationID, &pb.ThreadMessage{
				MessageId: response.ResponseId,
				User: &pb.User{
					Id:       "agent",
					Username: "Agent",
				},
				Content: payload.Content.Content,
			})
		}

	case *pb.AgentResponse_Status:
		// Status update
		event := NewStatusEvent(payload.Status)
		a.connManager.Broadcast(conversationID, event)

	case *pb.AgentResponse_Prompts:
		// Suggested prompts
		event := NewPromptsEvent(payload.Prompts)
		a.connManager.Broadcast(conversationID, event)

	case *pb.AgentResponse_Error:
		// Error response
		event := NewErrorEvent(payload.Error)
		a.connManager.Broadcast(conversationID, event)

	case *pb.AgentResponse_ThreadMetadata:
		// Thread metadata - could emit a custom event if needed
		log.Printf("[Web] Thread metadata received: %+v", payload.ThreadMetadata)

	default:
		log.Printf("[Web] Unhandled response payload type: %T", response.Payload)
	}

	return nil
}

// HydrateThread fetches thread history (web adapter maintains its own history)
func (a *WebAdapter) HydrateThread(ctx context.Context, conversationID string, threadStore *store.ThreadHistoryStore) error {
	// Web adapter doesn't need external hydration - history is maintained locally
	// This is a no-op since we store messages as they're sent
	return nil
}

// StreamContent implements StreamingAdapter for streaming content chunks
func (a *WebAdapter) StreamContent(ctx context.Context, conversationID string, chunks []*pb.ContentChunk) error {
	for _, chunk := range chunks {
		event := NewChunkEvent(chunk, "")
		a.connManager.Broadcast(conversationID, event)

		// Send finish on END
		if chunk.Type == pb.ContentChunk_END {
			finishEvent := NewFinishEvent("")
			a.connManager.Broadcast(conversationID, finishEvent)
		}
	}
	return nil
}

// SetThreadStore sets the thread history store
func (a *WebAdapter) SetThreadStore(store *store.ThreadHistoryStore) {
	a.threadStore = store
	if a.handlers != nil {
		a.handlers.threadStore = store
	}
}
