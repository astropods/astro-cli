package accessgroup

import "time"

type Status string

const (
	StatusActive    Status = "active"
	StatusArchiving Status = "archiving"
	StatusArchived  Status = "archived"
	StatusRestoring Status = "restoring"
)

type Group struct {
	ID               string
	AccountID        string
	WorkOSGroupID    string
	Name             string
	Description      string
	Status           Status
	CreatedByUserID  string
	ArchivedByUserID string
	ArchivedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CreateParams struct {
	AccountID       string
	Name            string
	Description     string
	CreatedByUserID string
}

type ListFilter struct {
	Statuses []Status
	Search   string
	Limit    int
	Offset   int
}
