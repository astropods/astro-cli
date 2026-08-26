package authorizationadmin

import (
	"errors"
)

var ErrNotConfigured = errors.New("authorization administration is not configured")

type Resource struct {
	Type             string
	Name             string
	ExternalID       string
	WorkOSResourceID string
	AccountID        string
	AccountName      string
	DirectAdmins     []string
	AssignmentCount  int
	CreatedAt        string
	SyncState        string
	LastError        string
}

type Inventory struct {
	Resources []Resource
}
