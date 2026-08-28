package authorizationadmin

import (
	"errors"
	"time"
)

var (
	ErrNotConfigured       = errors.New("authorization administration is not configured")
	ErrOperationNotFound   = errors.New("authorization administration operation not found")
	ErrOperationInProgress = errors.New("authorization operation already in progress")
	ErrAccountNotFound     = errors.New("authorization reset account not found")
	ErrAccountNotLinked    = errors.New("authorization reset account has no WorkOS organization")
)

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

type Operation struct {
	ID             string
	Kind           string
	AccountID      string
	DryRun         bool
	Status         string
	ConfirmedCount *int
	TargetCount    int
	ProcessedCount int
	SucceededCount int
	FailedCount    int
	AttemptCount   int
	LastError      string
	CreatedAt      time.Time
}

type ReportEntry struct {
	ResourceID string `json:"resource_id"`
	Type       string `json:"type"`
	ExternalID string `json:"external_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}
