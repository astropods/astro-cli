package auth

import (
	"time"

	"github.com/workos/workos-go/v4/pkg/usermanagement"
)

// User represents an authenticated user
type User struct {
	ID                string            `json:"id"`
	Email             string            `json:"email"`
	FirstName         string            `json:"first_name,omitempty"`
	LastName          string            `json:"last_name,omitempty"`
	EmailVerified     bool              `json:"email_verified"`
	ProfilePictureURL string            `json:"profile_picture_url,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
}

// Session represents an authenticated user session
type Session struct {
	ID             string    `json:"session_id"`
	UserID         string    `json:"user_id"`
	OrganizationID string    `json:"organization_id,omitempty"`
	Role           string    `json:"role,omitempty"`
	Permissions    []string  `json:"permissions,omitempty"`
	AccessToken    string    `json:"access_token"`
	RefreshToken   string    `json:"refresh_token"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}

// SessionData contains encrypted session data stored in cookie
type SessionData struct {
	Session *Session `json:"session"`
	User    *User    `json:"user"`
}

// AuthAccountResponse represents an account in the auth response
type AuthAccountResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Role string `json:"role"`
}

// AuthResponse is returned to the client after successful authentication
type AuthResponse struct {
	User         *User                 `json:"user"`
	SessionID    string                `json:"session_id"`
	Organization string                `json:"organization_id,omitempty"`
	Role         string                `json:"role,omitempty"`
	ExpiresAt    string                `json:"expires_at"`
	Accounts     []AuthAccountResponse `json:"accounts"`
}

// ErrorResponse represents an authentication error
type ErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
	Code        string `json:"code,omitempty"`
}

// UserFromWorkOS converts a WorkOS user to our User type
func UserFromWorkOS(wosUser usermanagement.User) *User {
	return &User{
		ID:                wosUser.ID,
		Email:             wosUser.Email,
		FirstName:         wosUser.FirstName,
		LastName:          wosUser.LastName,
		EmailVerified:     wosUser.EmailVerified,
		ProfilePictureURL: wosUser.ProfilePictureURL,
		Metadata:          wosUser.Metadata,
		CreatedAt:         wosUser.CreatedAt,
		UpdatedAt:         wosUser.UpdatedAt,
	}
}

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated user
	UserContextKey ContextKey = "user"
	// SessionContextKey is the context key for the session
	SessionContextKey ContextKey = "session"
	// AccountContextKey is the context key for the resolved account
	AccountContextKey ContextKey = "account"
)
