package adapter

import (
	"context"

	pb "github.com/astro/messaging/pkg/gen/astro/messaging/v1"
	"github.com/astro/messaging/internal/store"
)

// GRPCAdapter defines the interface for platform adapters in the gRPC architecture
type GRPCAdapter interface {
	// Lifecycle methods
	Initialize(ctx context.Context, config Config) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Capabilities
	Capabilities() AdapterCapabilities
	GetPlatformName() string
	IsHealthy(ctx context.Context) bool

	// Message handling
	// SetMessageHandler sets the handler for incoming messages from the platform
	SetMessageHandler(handler GRPCMessageHandler)

	// Response handling (agent → platform)
	HandleAgentResponse(ctx context.Context, response *pb.AgentResponse) error

	// Thread history (for hydration)
	HydrateThread(ctx context.Context, conversationID string, store *store.ThreadHistoryStore) error
}

// GRPCMessageHandler is called when a message is received from the platform
// It should forward the message to the gRPC server which sends it to the agent
type GRPCMessageHandler func(ctx context.Context, msg *pb.Message) error

// AIFeaturesAdapter extends GRPCAdapter with AI-specific features
type AIFeaturesAdapter interface {
	GRPCAdapter

	// AI Features
	SetStatus(ctx context.Context, conversationID string, status *pb.StatusUpdate) error
	SetSuggestedPrompts(ctx context.Context, conversationID string, prompts *pb.SuggestedPrompts) error
	UpdateThreadMetadata(ctx context.Context, metadata *pb.ThreadMetadata) error
}

// StreamingAdapter extends GRPCAdapter with streaming support
type StreamingAdapter interface {
	GRPCAdapter

	// Streaming
	StreamContent(ctx context.Context, conversationID string, chunks []*pb.ContentChunk) error
}

// FeedbackAdapter extends GRPCAdapter with feedback handling
type FeedbackAdapter interface {
	GRPCAdapter

	// Feedback handling (reactions, edits, deletes)
	// These are automatically captured by the adapter and sent to the agent
	// This interface exists for documentation purposes
}
