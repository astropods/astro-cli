// Package notify is the notifications seam: a typed Event, an audience policy,
// and a Deliverer that resolves recipients and hands off to a Provider (Novu on
// the hosted path, a no-op when unconfigured). Sources emit events; the River
// NotifyWorker calls Deliver. This package must not import riverqueue — the
// worker imports it, not the reverse.
package notify

import "time"

// Type is a stable `<domain>.<event>` identifier that maps 1:1 to a Novu
// workflow id. Adding a Type means authoring the matching workflow in Novu.
type Type string

const (
	// TypeSystemTest is the user-triggered "Send test" from the settings page.
	// It is not in the catalog (not user-configurable) and always delivers.
	TypeSystemTest Type = "system.test"

	// TypeAccountWelcome greets the creator of a newly created account (personal
	// or organization). Addressed to the actor, deduped per account.
	TypeAccountWelcome Type = "account.welcome"

	TypeBuildFailed Type = "build.failed"

	TypeBillingPaymentFailed  Type = "billing.payment_failed"
	TypeBillingActionRequired Type = "billing.action_required"
	TypeBillingSpendThreshold Type = "billing.spend_threshold"
	TypeBillingSuspended      Type = "billing.dunning_suspended"
	TypeBillingRecovered      Type = "billing.recovered"

	TypeTeamMemberChanged    Type = "team.member_changed"
	TypeOwnershipTransferred Type = "account.ownership_transferred"

	TypeSecurityKeyChanged Type = "security.key_changed"

	// All observation conditions collapse to three workflows by severity: a
	// healthy agent wasting resources (over-provisioned) triggers Info, a
	// degraded-but-running agent triggers Warning, a failing agent triggers
	// Critical. The specific condition (crash loop, OOM, …) rides in the payload
	// `reason`, so the three templates render any condition. Keeping three
	// workflows means three preference toggles, not one per condition.
	TypeObservationInfo     Type = "observation.info"
	TypeObservationWarning  Type = "observation.warning"
	TypeObservationCritical Type = "observation.critical"
)

// Audience is a recipient policy resolved at delivery, not at emit.
type Audience string

const (
	AudienceActor    Audience = "actor"    // the triggering user
	AudienceOwner    Audience = "owner"    // account owner
	AudienceManagers Audience = "managers" // org managers (org:manage — owner + admin)
	AudienceSubject  Audience = "subject"  // the user the event is about
	// AudienceWatchers is the members subscribed to Event.DeploymentID by having
	// acted on it. Falls back to managers when a deployment has no watchers, so
	// an agent nobody has touched (or one deployed only by automation) still
	// alerts someone.
	AudienceWatchers Audience = "watchers"
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
	// DeploymentID scopes AudienceWatchers. Kept separate from EntityID, which
	// is a dedupe key that is not always a deployment (build.failed carries a
	// build id), so routing never has to guess what EntityID currently means.
	DeploymentID string
	DedupeKey    string // idempotency key; defaults from Type+EntityID
	// OccurredAt is when the event happened. Stamped at the emit seam, not at
	// delivery: a job that exhausts its backoff can trigger long after the
	// incident, and the template must show the incident time.
	OccurredAt time.Time
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

// Stamped returns the event with OccurredAt set to now (UTC) when a source has
// not set it explicitly. Emit seams call this so the time is captured once, at
// emit, and rides the queued job through any retries.
func (e Event) Stamped(now time.Time) Event {
	if e.OccurredAt.IsZero() {
		e.OccurredAt = now.UTC()
	}
	return e
}

// workflowID is the Novu workflow identifier this event triggers: the explicit
// override if set, otherwise the Type.
func (e Event) workflowID() string {
	if e.WorkflowID != "" {
		return e.WorkflowID
	}
	return string(e.Type)
}
