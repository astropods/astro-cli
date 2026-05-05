package auth

import "time"

// User represents an authenticated user
type User struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	OrganizationID string `json:"organization_id,omitempty"`
}

// Session represents an authenticated user session
type Session struct {
	ID             string    `json:"session_id"`
	UserID         string    `json:"user_id"`
	OrganizationID string    `json:"organization_id,omitempty"`
	Role           string    `json:"role,omitempty"`
	Permissions    []string  `json:"permissions,omitempty"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// ErrorResponse represents an authentication error
type ErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated user
	UserContextKey ContextKey = "user"
	// SessionContextKey is the context key for the session
	SessionContextKey ContextKey = "session"
	// RegistryClaimsContextKey is the context key for verified registry-scope
	// token claims, when authentication used the registry-token path.
	// Absent for WorkOS-bearer requests.
	RegistryClaimsContextKey ContextKey = "registry_claims"
)
