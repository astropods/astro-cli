package types

import "time"

// UnifiedMessage represents a platform-agnostic incoming message
type UnifiedMessage struct {
	ID                string                 `json:"id"`
	PlatformMessageID string                 `json:"platform_message_id"`
	Platform          string                 `json:"platform"`
	Content           string                 `json:"content"`
	UserID            string                 `json:"user_id"`
	UserName          string                 `json:"user_name,omitempty"`
	ChannelID         string                 `json:"channel_id"`
	ThreadID          string                 `json:"thread_id,omitempty"`
	ConversationID    string                 `json:"conversation_id"`
	Timestamp         time.Time              `json:"timestamp"`
	Attachments       []Attachment           `json:"attachments,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// AgentResponse represents a response from the agent to the messaging service
type AgentResponse struct {
	Content      string       `json:"content"`
	Attachments  []Attachment `json:"attachments,omitempty"`
	CreateThread bool         `json:"create_thread,omitempty"`
	Ephemeral    bool         `json:"ephemeral,omitempty"`
	Stream       bool         `json:"stream,omitempty"`
	StreamURL    string       `json:"stream_url,omitempty"`
}

// SendMessageRequest represents a request to send a message to a platform
type SendMessageRequest struct {
	Platform    string                 `json:"platform"`
	ChannelID   string                 `json:"channel_id"`
	ThreadID    string                 `json:"thread_id,omitempty"`
	Content     string                 `json:"content"`
	Attachments []Attachment           `json:"attachments,omitempty"`
	Ephemeral   bool                   `json:"ephemeral,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// UpdateMessageRequest represents a request to update an existing message
type UpdateMessageRequest struct {
	Platform string `json:"platform"`
	Content  string `json:"content"`
}

// Attachment represents a file or media attachment
type Attachment struct {
	Type     string `json:"type"` // file, image, video, audio, link
	URL      string `json:"url"`
	Name     string `json:"name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

// ConversationContext stores metadata about a conversation
type ConversationContext struct {
	ConversationID string                 `json:"conversation_id"`
	Platform       string                 `json:"platform"`
	ChannelID      string                 `json:"channel_id"`
	ThreadID       string                 `json:"thread_id,omitempty"`
	UserID         string                 `json:"user_id"`
	CreatedAt      time.Time              `json:"created_at"`
	LastMessageAt  time.Time              `json:"last_message_at"`
	MessageCount   int                    `json:"message_count"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SendMessageResult contains the result of sending a message
type SendMessageResult struct {
	Success   bool   `json:"success"`
	MessageID string `json:"message_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Error     string `json:"error,omitempty"`
}

// HealthStatus represents the health status of the messaging service
type HealthStatus struct {
	Status   string            `json:"status"`
	Adapters map[string]string `json:"adapters"`
	Uptime   int64             `json:"uptime"`
}
