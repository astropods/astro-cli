package notify

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

// fakeProvider records the last trigger it received.
type fakeProvider struct {
	calls      int
	workflowID string
	recipients []Recipient
	payload    map[string]any
	txID       string
	err        error
}

func (f *fakeProvider) Trigger(_ context.Context, workflowID string, recipients []Recipient, payload map[string]any, txID string) error {
	f.calls++
	f.workflowID = workflowID
	f.recipients = recipients
	f.payload = payload
	f.txID = txID
	return f.err
}

type fakeEmails struct {
	// email -> user_id, as the mirror stores it.
	byEmail map[string]string
	err     error
}

func (f fakeEmails) EmailsForAccount(_ context.Context, _ string) (map[string]string, error) {
	return f.byEmail, f.err
}

type fakeAccounts struct {
	ownerID string
	err     error
	names   map[string]string // user_id -> display name
}

func (f fakeAccounts) GetFirstMemberUserID(string) (string, error) { return f.ownerID, f.err }

func (f fakeAccounts) DisplayNamesForUsers(userIDs []string) (map[string]string, error) {
	if f.names == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(userIDs))
	for _, id := range userIDs {
		if n, ok := f.names[id]; ok {
			out[id] = n
		}
	}
	return out, nil
}

func newDeliverer(p Provider, emails fakeEmails, accounts fakeAccounts) *Deliverer {
	return NewDeliverer(p, emails, accounts, nil, "https://app.example.com", nil)
}

func TestDeliverActorResolvesSelf(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"a@x.com": "u_actor", "b@x.com": "u_other"}}
	d := newDeliverer(prov, emails, fakeAccounts{})

	err := d.Deliver(context.Background(), Event{
		Type:        TypeSystemTest,
		AccountID:   "acct_1",
		Audience:    AudienceActor,
		ActorUserID: "u_actor",
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if prov.calls != 1 {
		t.Fatalf("expected 1 trigger, got %d", prov.calls)
	}
	if prov.workflowID != "system.test" {
		t.Fatalf("workflow = %q, want system.test", prov.workflowID)
	}
	if len(prov.recipients) != 1 || prov.recipients[0].UserID != "u_actor" || prov.recipients[0].Email != "a@x.com" {
		t.Fatalf("recipients = %+v, want single actor a@x.com", prov.recipients)
	}
}

func TestDeliverAttachesDisplayName(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"a@x.com": "u_actor"}}
	accounts := fakeAccounts{names: map[string]string{"u_actor": "Jane Doe"}}
	d := newDeliverer(prov, emails, accounts)

	err := d.Deliver(context.Background(), Event{
		Type:        TypeSystemTest,
		AccountID:   "acct_1",
		Audience:    AudienceActor,
		ActorUserID: "u_actor",
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(prov.recipients) != 1 || prov.recipients[0].Name != "Jane Doe" {
		t.Fatalf("recipients = %+v, want name Jane Doe attached", prov.recipients)
	}
}

// No audience broadcasts to the whole account; a retired one must be rejected
// rather than silently resolving to every member.
func TestDeliverRejectsRetiredBroadcastAudience(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"o@x.com": "u_owner", "m@x.com": "u_member"}}
	d := newDeliverer(prov, emails, fakeAccounts{})

	err := d.Deliver(context.Background(), Event{
		Type: TypeBuildFailed, AccountID: "acct_1", Audience: Audience("members"),
	})
	if err == nil {
		t.Fatalf("want unknown-audience error, got nil (recipients: %+v)", prov.recipients)
	}
	if prov.calls != 0 {
		t.Fatalf("provider must not be triggered for an unknown audience, got %d calls", prov.calls)
	}
}

func TestDeliverOwnerUsesFirstMember(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"owner@x.com": "u_owner", "m@x.com": "u_m"}}
	d := newDeliverer(prov, emails, fakeAccounts{ownerID: "u_owner"})

	if err := d.Deliver(context.Background(), Event{
		Type:      TypeBuildFailed,
		AccountID: "acct_1",
		Audience:  AudienceOwner,
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(prov.recipients) != 1 || prov.recipients[0].UserID != "u_owner" {
		t.Fatalf("owner recipients = %+v, want single u_owner", prov.recipients)
	}
}

func TestDeliverRendersOccurredAtAsRFC3339(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"a@x.com": "u_actor"}}
	d := newDeliverer(prov, emails, fakeAccounts{})

	occurred := time.Date(2026, 8, 4, 21, 37, 2, 0, time.UTC)
	if err := d.Deliver(context.Background(), Event{
		Type:        TypeSystemTest,
		AccountID:   "acct_1",
		Audience:    AudienceActor,
		ActorUserID: "u_actor",
		OccurredAt:  occurred,
		Payload:     map[string]any{PayloadAccount: "acme"},
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got := prov.payload[PayloadTimestamp]; got != "2026-08-04T21:37:02Z" {
		t.Fatalf("timestamp = %v, want 2026-08-04T21:37:02Z", got)
	}
	if got := prov.payload[PayloadAccount]; got != "acme" {
		t.Fatalf("existing payload props must survive, account = %v", got)
	}
}

// A nil payload and an unstamped event both still carry a timestamp, so no
// workflow template renders an empty variable.
func TestDeliverAlwaysSendsTimestamp(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"a@x.com": "u_actor"}}
	d := newDeliverer(prov, emails, fakeAccounts{})

	if err := d.Deliver(context.Background(), Event{
		Type:        TypeSystemTest,
		AccountID:   "acct_1",
		Audience:    AudienceActor,
		ActorUserID: "u_actor",
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	got, _ := prov.payload[PayloadTimestamp].(string)
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Fatalf("timestamp %q is not RFC3339: %v", got, err)
	}
}

// Stamping happens at emit, so a value a source already set is preserved.
func TestStampedPreservesExplicitOccurredAt(t *testing.T) {
	occurred := time.Date(2026, 8, 4, 21, 37, 2, 0, time.UTC)
	if got := (Event{OccurredAt: occurred}).Stamped(time.Now()).OccurredAt; !got.Equal(occurred) {
		t.Fatalf("OccurredAt = %v, want preserved %v", got, occurred)
	}
	if (Event{}).Stamped(time.Now()).OccurredAt.IsZero() {
		t.Fatal("unset OccurredAt should be stamped")
	}
}

func TestPayloadPropertiesIncludeTimestamp(t *testing.T) {
	for _, typ := range []Type{TypeSystemTest, TypeBuildFailed, TypeObservationCritical, TypeBillingPaymentFailed} {
		props := PayloadProperties(typ)
		if !slices.Contains(props, PayloadTimestamp) {
			t.Fatalf("%s properties %v missing %s", typ, props, PayloadTimestamp)
		}
	}
	if got := PayloadProperties(Type("nope.unknown")); got != nil {
		t.Fatalf("unknown type properties = %v, want nil", got)
	}
}

func TestDeliverNoRecipientDoesNotTrigger(t *testing.T) {
	prov := &fakeProvider{}
	// Actor has no mirrored email → no resolvable recipient.
	emails := fakeEmails{byEmail: map[string]string{"other@x.com": "u_other"}}
	d := newDeliverer(prov, emails, fakeAccounts{})

	if err := d.Deliver(context.Background(), Event{
		Type:        TypeSystemTest,
		AccountID:   "acct_1",
		Audience:    AudienceActor,
		ActorUserID: "u_actor",
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if prov.calls != 0 {
		t.Fatalf("expected no trigger when no recipient resolves, got %d", prov.calls)
	}
}

func TestDeliverTransactionIDFromDedupeKey(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"a@x.com": "u_actor"}}
	d := newDeliverer(prov, emails, fakeAccounts{})

	if err := d.Deliver(context.Background(), Event{
		Type:        TypeSystemTest,
		AccountID:   "acct_1",
		Audience:    AudienceActor,
		ActorUserID: "u_actor",
		DedupeKey:   "dk-123",
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if prov.txID != "dk-123" {
		t.Fatalf("transactionID = %q, want dk-123", prov.txID)
	}
}

func TestDeliverAbsolutizesRelativeCTA(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"a@x.com": "u_actor"}}
	d := newDeliverer(prov, emails, fakeAccounts{}) // appBaseURL = https://app.example.com

	if err := d.Deliver(context.Background(), Event{
		Type:        TypeSystemTest,
		AccountID:   "acct_1",
		Audience:    AudienceActor,
		ActorUserID: "u_actor",
		Payload:     map[string]any{PayloadCTAURL: "/agents"},
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got := prov.payload[PayloadCTAURL]; got != "https://app.example.com/agents" {
		t.Fatalf("ctaUrl = %v, want absolute https://app.example.com/agents", got)
	}
}

func TestDeliverLeavesAbsoluteCTA(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"a@x.com": "u_actor"}}
	d := newDeliverer(prov, emails, fakeAccounts{})

	if err := d.Deliver(context.Background(), Event{
		Type:        TypeSystemTest,
		AccountID:   "acct_1",
		Audience:    AudienceActor,
		ActorUserID: "u_actor",
		Payload:     map[string]any{PayloadCTAURL: "https://pay.stripe.com/x"},
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got := prov.payload[PayloadCTAURL]; got != "https://pay.stripe.com/x" {
		t.Fatalf("absolute ctaUrl should pass through, got %v", got)
	}
}

func TestBuildFailedPayload(t *testing.T) {
	ev := BuildFailed("acct_1", "acme", "my-agent", "build_9", "exit code 1")
	if ev.Type != TypeBuildFailed || ev.Audience != AudienceManagers || ev.EntityID != "build_9" {
		t.Fatalf("unexpected event envelope: %+v", ev)
	}
	// Data only — no prose. Wording lives in the Novu template.
	if ev.Payload[PayloadAgent] != "my-agent" || ev.Payload[PayloadReason] != "exit code 1" {
		t.Fatalf("payload data = %+v, want agent/reason", ev.Payload)
	}
	if ev.Payload[PayloadCTAURL] != "/acme/my-agent" {
		t.Fatalf("ctaUrl = %v, want /acme/my-agent", ev.Payload[PayloadCTAURL])
	}
	if _, hasSubject := ev.Payload["subject"]; hasSubject {
		t.Fatalf("payload must not carry prose (subject), got %+v", ev.Payload)
	}
}

func TestAccountWelcomePayload(t *testing.T) {
	ev := AccountWelcome("acct_1", "acme", "u_creator")
	if ev.Type != TypeAccountWelcome || ev.Audience != AudienceActor {
		t.Fatalf("unexpected envelope: %+v", ev)
	}
	if ev.ActorUserID != "u_creator" || ev.EntityID != "acct_1" {
		t.Fatalf("want actor=u_creator, entity=acct_1 (dedupe per account), got %+v", ev)
	}
	if ev.Payload[PayloadAccount] != "acme" {
		t.Fatalf("payload account = %v, want acme", ev.Payload[PayloadAccount])
	}
}

func TestObservationPayloadDeepLink(t *testing.T) {
	ev := Observation(TypeObservationCritical, "acct_1", "acme", "my-agent", "dep_9", "Out of memory", "A container was killed for exceeding its memory limit.")
	if got := ev.Payload[PayloadCTAURL]; got != "/acme/agents/dep_9/deployments" {
		t.Fatalf("ctaUrl = %v, want /acme/agents/dep_9/deployments", got)
	}
}

func TestObservationPayloadFallsBackWithoutHandle(t *testing.T) {
	ev := Observation(TypeObservationCritical, "acct_1", "", "my-agent", "dep_9", "Out of memory", "A container was killed for exceeding its memory limit.")
	if got := ev.Payload[PayloadCTAURL]; got != "/agents" {
		t.Fatalf("ctaUrl = %v, want /agents fallback when handle unknown", got)
	}
}

type fakeManagers struct {
	ids []string
	err error
}

func (f fakeManagers) ManagerUserIDs(context.Context, string) ([]string, error) { return f.ids, f.err }

func TestDeliverManagersResolvesOwnerAndAdmins(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"o@x.com": "u_owner", "a@x.com": "u_admin", "m@x.com": "u_member"}}
	d := NewDeliverer(prov, emails, fakeAccounts{}, fakeManagers{ids: []string{"u_owner", "u_admin"}}, "", nil)

	if err := d.Deliver(context.Background(), Event{
		Type: TypeBillingPaymentFailed, AccountID: "acct_1", Audience: AudienceManagers,
	}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(prov.recipients) != 2 {
		t.Fatalf("want 2 manager recipients (owner+admin), got %d: %+v", len(prov.recipients), prov.recipients)
	}
	for _, r := range prov.recipients {
		if r.UserID == "u_member" {
			t.Fatalf("non-manager member must not be a recipient")
		}
	}
}

func TestDeliverManagersFallsBackToOwner(t *testing.T) {
	emails := fakeEmails{byEmail: map[string]string{"o@x.com": "u_owner", "a@x.com": "u_admin"}}

	// nil lookup (unconfigured) and empty result (personal account) both fall back.
	for name, mgr := range map[string]managerLookup{"nil": nil, "empty": fakeManagers{}} {
		prov := &fakeProvider{}
		d := NewDeliverer(prov, emails, fakeAccounts{ownerID: "u_owner"}, mgr, "", nil)
		if err := d.Deliver(context.Background(), Event{
			Type: TypeSecurityKeyChanged, AccountID: "acct_1", Audience: AudienceManagers,
		}); err != nil {
			t.Fatalf("[%s] Deliver: %v", name, err)
		}
		if len(prov.recipients) != 1 || prov.recipients[0].UserID != "u_owner" {
			t.Fatalf("[%s] want owner fallback, got %+v", name, prov.recipients)
		}
	}
}

type fakeWatchers struct {
	ids []string
	err error
}

func (f fakeWatchers) ActiveUserIDs(context.Context, string) ([]string, error) { return f.ids, f.err }

func watcherEvent() Event {
	return Event{
		Type: TypeObservationCritical, AccountID: "acct_1",
		Audience: AudienceWatchers, EntityID: "dep_9", DeploymentID: "dep_9",
	}
}

func TestDeliverWatchersResolvesSubscribers(t *testing.T) {
	prov := &fakeProvider{}
	emails := fakeEmails{byEmail: map[string]string{"w@x.com": "u_watcher", "o@x.com": "u_owner", "a@x.com": "u_admin"}}
	d := NewDeliverer(prov, emails, fakeAccounts{}, fakeManagers{ids: []string{"u_owner", "u_admin"}}, "", nil).
		WithWatchers(fakeWatchers{ids: []string{"u_watcher"}})

	if err := d.Deliver(context.Background(), watcherEvent()); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(prov.recipients) != 1 || prov.recipients[0].UserID != "u_watcher" {
		t.Fatalf("want only the watcher, got %+v", prov.recipients)
	}
}

// Every path that yields no usable watcher must land on managers rather than
// dropping the alert.
func TestDeliverWatchersFallsBackToManagers(t *testing.T) {
	emails := fakeEmails{byEmail: map[string]string{"o@x.com": "u_owner", "a@x.com": "u_admin"}}
	mgrs := fakeManagers{ids: []string{"u_owner", "u_admin"}}

	cases := map[string]struct {
		lookup watcherLookup
		event  Event
	}{
		"no lookup wired":  {nil, watcherEvent()},
		"nobody watching":  {fakeWatchers{}, watcherEvent()},
		"lookup failed":    {fakeWatchers{err: errors.New("boom")}, watcherEvent()},
		"no deployment id": {fakeWatchers{ids: []string{"u_watcher"}}, Event{Type: TypeObservationCritical, AccountID: "acct_1", Audience: AudienceWatchers}},
		// A watcher with no mirrored email resolves to zero recipients; managers
		// are the same backstop as having no watchers at all.
		"watcher unreachable": {fakeWatchers{ids: []string{"u_ghost"}}, watcherEvent()},
	}

	for name, tc := range cases {
		prov := &fakeProvider{}
		d := NewDeliverer(prov, emails, fakeAccounts{ownerID: "u_owner"}, mgrs, "", nil).WithWatchers(tc.lookup)

		if err := d.Deliver(context.Background(), tc.event); err != nil {
			t.Fatalf("[%s] Deliver: %v", name, err)
		}
		if len(prov.recipients) != 2 {
			t.Fatalf("[%s] want 2 manager recipients, got %+v", name, prov.recipients)
		}
	}
}

func TestObservationAddressesWatchers(t *testing.T) {
	ev := Observation(TypeObservationCritical, "acct_1", "acme", "my-agent", "dep_9", "Out of memory", "details")
	if ev.Audience != AudienceWatchers {
		t.Fatalf("audience = %s, want %s", ev.Audience, AudienceWatchers)
	}
	if ev.DeploymentID != "dep_9" {
		t.Fatalf("DeploymentID = %q, want dep_9 so watchers can resolve", ev.DeploymentID)
	}
}

func TestDeliverPropagatesProviderError(t *testing.T) {
	prov := &fakeProvider{err: errors.New("boom")}
	emails := fakeEmails{byEmail: map[string]string{"a@x.com": "u_actor"}}
	d := newDeliverer(prov, emails, fakeAccounts{})

	err := d.Deliver(context.Background(), Event{
		Type:        TypeSystemTest,
		AccountID:   "acct_1",
		Audience:    AudienceActor,
		ActorUserID: "u_actor",
	})
	if err == nil {
		t.Fatal("expected provider error to propagate so River retries")
	}
}
