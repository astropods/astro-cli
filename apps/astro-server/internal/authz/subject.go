package authz

// Subject is the caller being authorized.
type Subject struct {
	UserID       string // WorkOS user id (sub)
	MembershipID string // WorkOS organization membership id (om_*); empty until PR 2
	OrgID        string // JWT-scoped WorkOS org id from session (switch-org); not the resource account's org
}
