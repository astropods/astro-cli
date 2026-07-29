// Package notify is the notifications seam: a typed Event, an audience policy,
// and a Deliverer that resolves recipients and hands off to a Provider (Novu on
// the hosted path, a no-op when unconfigured). Sources emit events; the River
// NotifyWorker calls Deliver. This package must not import riverqueue — the
// worker imports it, not the reverse.
package notify

// Type is a stable `<domain>.<event>` identifier that maps 1:1 to a Novu
// workflow id. Adding a Type means authoring the matching workflow in Novu.
type Type string

const (
	// TypeSystemTest is the user-triggered "Send test" from the settings page.
	// It is not in the catalog (not user-configurable) and always delivers.
	TypeSystemTest Type = "system.test"

	TypeBuildFailed Type = "build.failed"

	TypeBillingPaymentFailed  Type = "billing.payment_failed"
	TypeBillingActionRequired Type = "billing.action_required"
	TypeBillingSpendThreshold Type = "billing.spend_threshold"
	TypeBillingSuspended      Type = "billing.dunning_suspended"
	TypeBillingRecovered      Type = "billing.recovered"

	TypeTeamMemberChanged    Type = "team.member_changed"
	TypeOwnershipTransferred Type = "account.ownership_transferred"

	TypeSecurityKeyChanged Type = "security.key_changed"

	// All observation conditions collapse to two workflows by severity: a
	// degraded-but-running agent triggers Warning, a failing agent triggers
	// Critical. The specific condition (crash loop, OOM, …) rides in the payload
	// `reason`, so the two templates render any condition. Keeping two workflows
	// means two preference toggles, not one per condition.
	TypeObservationWarning  Type = "observation.warning"
	TypeObservationCritical Type = "observation.critical"
)

// Audience is a recipient policy resolved at delivery, not at emit.
type Audience string

const (
	AudienceActor    Audience = "actor"    // the triggering user
	AudienceOwner    Audience = "owner"    // account owner
	AudienceManagers Audience = "managers" // org managers (org:manage — owner + admin)
	AudienceAdmins   Audience = "admins"   // org admins
	AudienceMembers  Audience = "members"  // all account members
	AudienceSubject  Audience = "subject"  // the user the event is about
)

// Event is one alert to deliver. Recipients are derived from Audience +
// AccountID at delivery; the emit site only declares intent and payload.
type Event struct {
	Type          Type
	AccountID     string
	Audience      Audience
	ActorUserID   string         // set for AudienceActor and to exclude self
	SubjectUserID string         // set for AudienceSubject
	EntityID      string         // deployment/build/invoice id, for dedupe + copy
	Payload       map[string]any // workflow template variables
	DedupeKey     string         // idempotency key; defaults from Type+EntityID
	// WorkflowID overrides the Novu workflow this triggers. Normally the
	// workflow id equals the Type; this override exists for local dev, where the
	// authored workflow may be named differently (e.g. NOVU_TEST_WORKFLOW_ID).
	WorkflowID string
}

// transactionID is the Novu idempotency key. An explicit DedupeKey wins;
// otherwise it derives from Type and EntityID.
func (e Event) transactionID() string {
	if e.DedupeKey != "" {
		return e.DedupeKey
	}
	if e.EntityID != "" {
		return string(e.Type) + ":" + e.EntityID
	}
	return ""
}

// workflowID is the Novu workflow identifier this event triggers: the explicit
// override if set, otherwise the Type.
func (e Event) workflowID() string {
	if e.WorkflowID != "" {
		return e.WorkflowID
	}
	return string(e.Type)
}
