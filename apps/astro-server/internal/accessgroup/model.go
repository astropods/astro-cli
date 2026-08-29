package accessgroup

import (
	"encoding/json"
	"time"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusArchiving Status = "archiving"
	StatusArchived  Status = "archived"
	StatusRestoring Status = "restoring"
)

type ManagementSource string

const (
	ManagementSourceAstro     ManagementSource = "astro"
	ManagementSourceDirectory ManagementSource = "directory"
)

type SyncStatus string

const (
	SyncPending SyncStatus = "pending"
	SyncSynced  SyncStatus = "synced"
	SyncError   SyncStatus = "error"
)

type MembershipRole string

const (
	MembershipRoleMember MembershipRole = "member"
	MembershipRoleAdmin  MembershipRole = "admin"
)

type Group struct {
	ID                     string
	AccountID              string
	WorkOSGroupID          string
	Name                   string
	Description            string
	Status                 Status
	ManagementSource       ManagementSource
	CreatedByUserID        string
	ArchivedByUserID       string
	ArchivedAt             *time.Time
	ClassificationMetadata json.RawMessage
	SyncStatus             SyncStatus
	SyncError              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type Summary struct {
	Group
	MemberCount    int
	PreviewUserIDs []string
}

type Membership struct {
	GroupID         string
	AccountID       string
	UserID          string
	Role            MembershipRole
	AddedByUserID   string
	RemovedByUserID string
	AddedAt         time.Time
	RemovedAt       *time.Time
	SyncStatus      SyncStatus
	SyncError       string
	UpdatedAt       time.Time
}

type CreateParams struct {
	AccountID              string
	Name                   string
	Description            string
	CreatedByUserID        string
	ClassificationMetadata json.RawMessage
}

type ListFilter struct {
	Statuses []Status
	Search   string
	Limit    int
	Offset   int
}
