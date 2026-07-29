package notify

// Payload property keys pushed to Novu. The backend pushes structured DATA
// only — the message wording (subject, body) lives in the Novu workflow
// templates, which compose the message from these properties. ctaUrl is a
// relative app path the Deliverer absolutizes against the app base URL.
const (
	PayloadCTAURL  = "ctaUrl"
	PayloadAccount = "account"
	PayloadAgent   = "agent"
	PayloadReason  = "reason"
	PayloadRole    = "role"
	PayloadAction  = "action" // e.g. added|role_changed|removed, created|revoked
	PayloadKeyKind = "keyKind"
	PayloadKeyName = "keyName"
)

// payloadProps is the per-type payload contract: the property keys each
// notification pushes. It is the single source of truth for the builders below
// and the contract each Novu workflow's payload schema must match, so the
// workflow templates know exactly which variables are available.
var payloadProps = map[Type][]string{
	TypeSystemTest: {PayloadAccount},

	TypeBuildFailed: {PayloadAgent, PayloadReason, PayloadCTAURL},

	TypeBillingPaymentFailed:  {PayloadAccount, PayloadCTAURL},
	TypeBillingActionRequired: {PayloadAccount, PayloadCTAURL},
	TypeBillingSpendThreshold: {PayloadAccount, PayloadCTAURL},
	TypeBillingRecovered:      {PayloadAccount},
	TypeBillingSuspended:      {PayloadAccount, PayloadCTAURL},

	TypeTeamMemberChanged:    {PayloadAccount, PayloadRole, PayloadAction, PayloadCTAURL},
	TypeOwnershipTransferred: {PayloadAccount, PayloadAgent, PayloadCTAURL},

	TypeSecurityKeyChanged: {PayloadKeyKind, PayloadKeyName, PayloadAction, PayloadCTAURL},

	TypeObservationWarning:  {PayloadAgent, PayloadReason, PayloadCTAURL},
	TypeObservationCritical: {PayloadAgent, PayloadReason, PayloadCTAURL},
}

// PayloadProperties returns the payload property keys a notification type
// pushes, for building the Novu workflow's payload schema.
func PayloadProperties(t Type) []string { return payloadProps[t] }

// --- Deployments ---

// BuildFailed builds the build.failed event for an account's members.
func BuildFailed(accountID, agentName, buildID, reason string) Event {
	return Event{
		Type:      TypeBuildFailed,
		AccountID: accountID,
		Audience:  AudienceMembers,
		EntityID:  buildID,
		Payload: map[string]any{
			PayloadAgent:  agentName,
			PayloadReason: reason,
			PayloadCTAURL: "/agents",
		},
	}
}

// --- Billing (manager-addressed) ---

func billingEvent(t Type, accountID string, payload map[string]any) Event {
	return Event{Type: t, AccountID: accountID, Audience: AudienceManagers, Payload: payload}
}

// BillingPaymentFailed builds the billing.payment_failed event.
func BillingPaymentFailed(accountID, accountName string) Event {
	return billingEvent(TypeBillingPaymentFailed, accountID, map[string]any{
		PayloadAccount: accountName, PayloadCTAURL: "/settings/billing",
	})
}

// BillingActionRequired builds the billing.action_required event. hostedInvoiceURL
// is Stripe's 3DS link (absolute) used as the CTA.
func BillingActionRequired(accountID, accountName, hostedInvoiceURL string) Event {
	return billingEvent(TypeBillingActionRequired, accountID, map[string]any{
		PayloadAccount: accountName, PayloadCTAURL: hostedInvoiceURL,
	})
}

// BillingSpendThreshold builds the billing.spend_threshold event.
func BillingSpendThreshold(accountID, accountName string) Event {
	return billingEvent(TypeBillingSpendThreshold, accountID, map[string]any{
		PayloadAccount: accountName, PayloadCTAURL: "/settings/billing",
	})
}

// BillingRecovered builds the billing.recovered event (no CTA).
func BillingRecovered(accountID, accountName string) Event {
	return billingEvent(TypeBillingRecovered, accountID, map[string]any{PayloadAccount: accountName})
}

// BillingSuspended builds the billing.dunning_suspended event. accountName may
// be empty (the dunning sweep works off ids); the template handles that.
// EntityID is the account so it dedupes per suspension.
func BillingSuspended(accountID, accountName string) Event {
	ev := billingEvent(TypeBillingSuspended, accountID, map[string]any{
		PayloadAccount: accountName, PayloadCTAURL: "/settings/billing",
	})
	ev.EntityID = accountID
	return ev
}

// --- Team / account ---

func teamEvent(accountID, subjectUserID string, payload map[string]any) Event {
	return Event{
		Type:          TypeTeamMemberChanged,
		AccountID:     accountID,
		Audience:      AudienceSubject,
		SubjectUserID: subjectUserID,
		Payload:       payload,
	}
}

// TeamMemberAdded builds the team.member_changed event for an added member.
func TeamMemberAdded(accountID, accountName, subjectUserID, role string) Event {
	return teamEvent(accountID, subjectUserID, map[string]any{
		PayloadAccount: accountName, PayloadRole: role, PayloadAction: "added", PayloadCTAURL: "/",
	})
}

// TeamRoleChanged builds the team.member_changed event for a role change.
func TeamRoleChanged(accountID, accountName, subjectUserID, role string) Event {
	return teamEvent(accountID, subjectUserID, map[string]any{
		PayloadAccount: accountName, PayloadRole: role, PayloadAction: "role_changed", PayloadCTAURL: "/",
	})
}

// TeamMemberRemoved builds the team.member_changed event for a removal (no CTA).
func TeamMemberRemoved(accountID, accountName, subjectUserID string) Event {
	return teamEvent(accountID, subjectUserID, map[string]any{
		PayloadAccount: accountName, PayloadAction: "removed",
	})
}

// AgentTransferred builds the account.ownership_transferred event for the owner
// of the account that received the agent.
func AgentTransferred(targetAccountID, targetAccountName, agentName string) Event {
	return Event{
		Type:      TypeOwnershipTransferred,
		AccountID: targetAccountID,
		Audience:  AudienceManagers,
		EntityID:  agentName,
		Payload: map[string]any{
			PayloadAccount: targetAccountName, PayloadAgent: agentName, PayloadCTAURL: "/agents",
		},
	}
}

// --- Security (manager-addressed) ---

func securityKeyEvent(accountID string, payload map[string]any) Event {
	return Event{Type: TypeSecurityKeyChanged, AccountID: accountID, Audience: AudienceManagers, Payload: payload}
}

// SecurityKeyCreated builds the security.key_changed event for a created token.
func SecurityKeyCreated(accountID, keyKind, keyName string) Event {
	return securityKeyEvent(accountID, map[string]any{
		PayloadKeyKind: keyKind, PayloadKeyName: keyName, PayloadAction: "created", PayloadCTAURL: "/settings/api-keys",
	})
}

// SecurityKeyRevoked builds the security.key_changed event for a revoked token.
// keyName may be empty (revoke by id).
func SecurityKeyRevoked(accountID, keyKind, keyName string) Event {
	return securityKeyEvent(accountID, map[string]any{
		PayloadKeyKind: keyKind, PayloadKeyName: keyName, PayloadAction: "revoked", PayloadCTAURL: "/settings/api-keys",
	})
}

// --- Observation (member-addressed) ---

// Observation builds an observation alert for an account's members. t is the
// severity workflow (TypeObservationWarning or TypeObservationCritical); reason is
// the human condition title (e.g. "Out of memory") the shared template renders,
// since both severities cover many conditions. The observation evaluator sets a
// per-episode DedupeKey so a re-fire after a resolve isn't collapsed with the
// prior episode at Novu.
func Observation(t Type, accountID, agentName, deploymentID, reason string) Event {
	return Event{
		Type:      t,
		AccountID: accountID,
		Audience:  AudienceMembers,
		EntityID:  deploymentID,
		Payload:   map[string]any{PayloadAgent: agentName, PayloadReason: reason, PayloadCTAURL: "/agents"},
	}
}
