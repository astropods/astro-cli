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
	Assignments      []Assignment
	AssignmentCount  int
	CreatedAt        string
	SyncState        string
	LastError        string
}

type Assignment struct {
	SubjectType  string
	SubjectID    string
	SubjectLabel string
	Role         string
	Source       string
}

type Inventory struct {
	Resources []Resource
}
