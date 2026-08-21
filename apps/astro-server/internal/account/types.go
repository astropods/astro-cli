package account

import (
	"encoding/json"
	"time"
)

// Account represents a named account that scopes all resources
type Account struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	Type                 string           `json:"type"`
	WorkOSOrganizationID string           `json:"workos_org_id,omitempty"`
	DeletedAt            *time.Time       `json:"deleted_at,omitempty"`
	CreatedAt            time.Time        `json:"created_at"`
	UpdatedAt            time.Time        `json:"updated_at"`
	DisplayName          string           `json:"display_name"`
	AvatarColors         *json.RawMessage `json:"avatar_colors,omitempty"`
	AvatarUpdatedAt      *time.Time       `json:"avatar_updated_at,omitempty"`
	AccountNumber        *int             `json:"account_number,omitempty"`
	Bio                  string           `json:"bio,omitempty"`
	Location             string           `json:"location,omitempty"`
	LocalTimezone        string           `json:"local_timezone,omitempty"`
	Pronouns             string           `json:"pronouns,omitempty"`
	Website              string           `json:"website,omitempty"`
	SocialLinks          []string         `json:"social_links"`
	BlueprintOrder       []string         `json:"blueprint_order"`
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
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Type                 string     `json:"type"`
	WorkOSOrganizationID string     `json:"workos_org_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	DisplayName          string     `json:"display_name"`
	AvatarUpdatedAt      *time.Time `json:"avatar_updated_at,omitempty"`
}
