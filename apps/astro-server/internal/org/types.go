package org

// Organization represents a WorkOS organization linked to an Astro account
type Organization struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ExternalID string `json:"external_id,omitempty"`
}

// Membership represents a WorkOS organization membership
type Membership struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	OrganizationID string `json:"organization_id"`
	RoleSlug       string `json:"role_slug"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// Invitation represents a WorkOS organization invitation
type Invitation struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	State          string `json:"state"`
	OrganizationID string `json:"organization_id,omitempty"`
	InviterUserID  string `json:"inviter_user_id,omitempty"`
	ExpiresAt      string `json:"expires_at"`
	CreatedAt      string `json:"created_at"`
}

// Role represents a WorkOS organization role
type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
}

// InviteRequest describes a single invitation to send (by email or account name).
type InviteRequest struct {
	Kind     string // "email" or "account"
	Value    string // email address or account name
	RoleSlug string
}

// InviteResult is the outcome of a single invitation attempt.
type InviteResult struct {
	Value      string      `json:"value"`
	Kind       string      `json:"kind"`
	Email      string      `json:"email,omitempty"`
	Success    bool        `json:"success"`
	Error      string      `json:"error,omitempty"`
	Invitation *Invitation `json:"invitation,omitempty"`
}

// ListOpts provides pagination options for list operations
type ListOpts struct {
	Limit  int
	After  string
	Before string
}
