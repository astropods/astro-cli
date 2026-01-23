package adapter

import (
	"context"

	"github.com/astro/messaging/pkg/types"
)

// Adapter is the interface that all platform adapters must implement
type Adapter interface {
	// Initialize sets up the adapter with configuration
	Initialize(ctx context.Context, config Config) error

	// Start begins listening for platform events
	Start(ctx context.Context) error

	// Stop gracefully shuts down the adapter
	Stop(ctx context.Context) error

	// OnMessage registers a handler for incoming messages
	OnMessage(handler MessageHandler)

	// SendMessage sends a message to the platform
	SendMessage(ctx context.Context, req *types.SendMessageRequest) (*types.SendMessageResult, error)

	// UpdateMessage updates an existing message (for streaming)
	UpdateMessage(ctx context.Context, messageID string, content string) error

	// GetPlatformName returns the platform identifier
	GetPlatformName() string

	// IsHealthy checks if the adapter is connected and healthy
	IsHealthy(ctx context.Context) bool
}

// MessageHandler is called when a message is received from the platform
type MessageHandler func(ctx context.Context, msg *types.UnifiedMessage) (*types.AgentResponse, error)

// Config holds adapter configuration
type Config struct {
	BotToken   string
	AppToken   string // For Slack Socket Mode
	SocketMode bool
	WebhookURL string
	AutoThread bool
	RateLimit  RateLimitConfig
}

// RateLimitConfig configures rate limiting
type RateLimitConfig struct {
	RequestsPerSecond float64
	BurstSize         int
}

// PlatformCapabilities describes what features a platform supports
type PlatformCapabilities struct {
	SupportsThreads        bool
	SupportsStreaming      bool
	SupportsAttachments    bool
	SupportsEphemeral      bool
	SupportsMessageUpdates bool
	SupportsRichFormatting bool
	MaxMessageLength       int
}
