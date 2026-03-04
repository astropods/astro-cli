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

// ListOpts provides pagination options for list operations
type ListOpts struct {
	Limit  int
	After  string
	Before string
}
