package account

import "time"

// Account represents a named account that scopes all resources
type Account struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Type                 string     `json:"type"`
	WorkOSOrganizationID string     `json:"workos_org_id,omitempty"`
	DeletedAt            *time.Time `json:"deleted_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	AvatarVersion        int        `json:"avatar_version"`
}

// AccountMember represents a user's membership in an account
type AccountMember struct {
	AccountID          string    `json:"account_id"`
	UserID             string    `json:"user_id"`
	WorkOSMembershipID string    `json:"workos_membership_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// AccountWithRole is returned when listing accounts for a user
type AccountWithRole struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Type                 string    `json:"type"`
	WorkOSOrganizationID string    `json:"workos_org_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	AvatarVersion        int       `json:"avatar_version"`
}
