package auditlog

import (
	"encoding/json"
	"time"
)

// ActorType distinguishes who performed the action.
type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorAdmin  ActorType = "admin"
	ActorSystem ActorType = "system"
)

// Entry is an audit log row returned by queries.
type Entry struct {
	ID           int64           `json:"id"`
	AccountID    string          `json:"account_id"`
	ActorID      string          `json:"actor_id"`
	ActorType    ActorType       `json:"actor_type"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	ResourceName string          `json:"resource_name,omitempty"`
	Description  string          `json:"description,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	IPAddress    string          `json:"ip_address,omitempty"`
	UserAgent    string          `json:"user_agent,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Event is the input for creating an audit log entry.
type Event struct {
	AccountID    string
	ActorID      string
	ActorType    ActorType
	Action       string
	ResourceType string
	ResourceID   string
	ResourceName string
	Description  string
	Metadata     any // marshaled to JSONB on insert
	IPAddress    string
	UserAgent    string
}

// QueryParams controls audit log listing.
type QueryParams struct {
	AccountID    string
	ActorID      string
	ResourceType string
	ResourceID   string
	Action       string
	Before       *time.Time // cursor: created_at < Before
	Limit        int        // default 50, max 200
}
