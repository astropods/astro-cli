package notify

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// memberEmails is the member-email mirror lookup the Deliverer needs.
// *memberemails.Store satisfies it.
type memberEmails interface {
	EmailsForAccount(ctx context.Context, accountID string) (map[string]string, error)
}

// accountLookup is the account-store slice the Deliverer needs.
// *account.AccountStore satisfies it.
type accountLookup interface {
	GetFirstMemberUserID(accountID string) (string, error)
	// DisplayNamesForUsers returns user_id → display name for the given users;
	// users without a resolvable name are omitted.
	DisplayNamesForUsers(userIDs []string) (map[string]string, error)
}

// managerLookup resolves an account's org managers (org:manage — owner + admin)
// to WorkOS user ids by querying WorkOS. Optional: a nil lookup (or an empty
// result, e.g. a personal account with no org) falls back to the owner.
type managerLookup interface {
	ManagerUserIDs(ctx context.Context, accountID string) ([]string, error)
}

// Deliverer resolves an event's audience to recipients and hands off to the
// Provider. It reads recipient emails from the member-email mirror and
// owner/membership from the account store. Per-channel preferences are owned by
// Novu (the workflow default + the subscriber's per-workflow overrides) and
// enforced by Novu at delivery, so the Deliverer does not gate channels.
type Deliverer struct {
	provider   Provider
	emails     memberEmails
	accounts   accountLookup
	managers   managerLookup
	appBaseURL string // trimmed of trailing slash; used to absolutize relative ctaUrl
	log        *logger.Logger
}

// NewDeliverer builds a Deliverer. provider must be non-nil (use the no-op
// provider when Novu is unconfigured). managers may be nil (manager audiences
// fall back to the owner). appBaseURL is the public app origin (FrontendURL);
// relative ctaUrl payload values are prefixed with it so email links are absolute.
func NewDeliverer(provider Provider, emails memberEmails, accounts accountLookup, managers managerLookup, appBaseURL string, log *logger.Logger) *Deliverer {
	return &Deliverer{provider: provider, emails: emails, accounts: accounts, managers: managers, appBaseURL: strings.TrimRight(appBaseURL, "/"), log: log}
}

// Deliver resolves recipients and triggers the workflow. Novu applies each
// subscriber's channel preferences at delivery. It returns an error only for a
// genuine delivery failure (so River retries); no resolvable recipients is
// logged and treated as done.
func (d *Deliverer) Deliver(ctx context.Context, ev Event) error {
	recipients, err := d.resolveRecipients(ctx, ev)
	if err != nil {
		return fmt.Errorf("notify: resolve recipients for %s: %w", ev.Type, err)
	}
	if len(recipients) == 0 {
		if d.log != nil {
			d.log.Warn("notify: no resolvable recipients, dropping",
				"type", ev.Type, "account_id", ev.AccountID, "audience", ev.Audience)
		}
		return nil
	}

	d.attachNames(recipients)

	return d.provider.Trigger(ctx, ev.workflowID(), recipients, d.finalizePayload(ev.Payload), ev.transactionID())
}

// attachNames fills each recipient's display name for the subscriber greeting,
// in place. Best-effort: a lookup failure logs and leaves names empty rather
// than failing the send — the alert matters more than the greeting.
func (d *Deliverer) attachNames(recipients []Recipient) {
	if d.accounts == nil {
		return
	}
	ids := make([]string, 0, len(recipients))
	for _, r := range recipients {
		ids = append(ids, r.UserID)
	}
	names, err := d.accounts.DisplayNamesForUsers(ids)
	if err != nil {
		if d.log != nil {
			d.log.Warn("notify: display name lookup failed, sending without names", "error", err)
		}
		return
	}
	for i := range recipients {
		recipients[i].Name = names[recipients[i].UserID]
	}
}

// finalizePayload returns a copy of the event payload with a relative ctaUrl
// (leading "/") made absolute against appBaseURL, so email links resolve
// outside the app. Absolute or empty ctaUrl values pass through unchanged.
func (d *Deliverer) finalizePayload(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	if url, ok := out[PayloadCTAURL].(string); ok && d.appBaseURL != "" && strings.HasPrefix(url, "/") {
		out[PayloadCTAURL] = d.appBaseURL + url
	}
	return out
}

// resolveRecipients maps the audience policy to concrete recipients, excluding
// the actor from broadcast audiences so a user is not alerted about their own
// action.
func (d *Deliverer) resolveRecipients(ctx context.Context, ev Event) ([]Recipient, error) {
	emailByUser, err := d.emailByUser(ctx, ev.AccountID)
	if err != nil {
		return nil, err
	}

	recip := func(userID string) []Recipient {
		email := emailByUser[userID]
		if email == "" {
			if d.log != nil {
				d.log.Warn("notify: no mirrored email for user, skipping recipient",
					"type", ev.Type, "user_id", userID)
			}
			return nil
		}
		return []Recipient{{UserID: userID, Email: email}}
	}

	switch ev.Audience {
	case AudienceActor:
		return recip(ev.ActorUserID), nil
	case AudienceSubject:
		return recip(ev.SubjectUserID), nil
	case AudienceOwner:
		ownerID, err := d.accounts.GetFirstMemberUserID(ev.AccountID)
		if err != nil {
			return nil, fmt.Errorf("owner user id: %w", err)
		}
		return recip(ownerID), nil
	case AudienceManagers:
		return d.resolveManagers(ctx, ev, recip)
	case AudienceMembers, AudienceAdmins:
		// TODO(notify): AudienceAdmins should filter to org admin/owner roles via
		// the org client; until the team PR wires that, it broadcasts to all
		// members like AudienceMembers.
		return d.allMembers(ev, emailByUser), nil
	default:
		return nil, fmt.Errorf("unknown audience %q", ev.Audience)
	}
}

// resolveManagers returns the account's org managers (owner + admin), resolved
// via WorkOS. It falls back to the account owner when there is no manager lookup
// (unconfigured), no org (personal account → empty result), or the lookup fails
// — so a critical alert (billing/security) always reaches at least the owner.
func (d *Deliverer) resolveManagers(ctx context.Context, ev Event, recip func(string) []Recipient) ([]Recipient, error) {
	var ids []string
	if d.managers != nil {
		got, err := d.managers.ManagerUserIDs(ctx, ev.AccountID)
		if err != nil && d.log != nil {
			d.log.Warn("notify: manager lookup failed, falling back to owner",
				"error", err, "type", ev.Type, "account_id", ev.AccountID)
		}
		ids = got
	}
	if len(ids) == 0 {
		ownerID, err := d.accounts.GetFirstMemberUserID(ev.AccountID)
		if err != nil {
			return nil, fmt.Errorf("owner user id: %w", err)
		}
		return recip(ownerID), nil
	}
	out := make([]Recipient, 0, len(ids))
	for _, uid := range ids {
		out = append(out, recip(uid)...)
	}
	return out, nil
}

// allMembers returns every member with a mirrored email, minus the actor.
func (d *Deliverer) allMembers(ev Event, emailByUser map[string]string) []Recipient {
	out := make([]Recipient, 0, len(emailByUser))
	for userID, email := range emailByUser {
		if userID == ev.ActorUserID {
			continue
		}
		out = append(out, Recipient{UserID: userID, Email: email})
	}
	return out
}

// emailByUser inverts the mirror's email→user_id map to user_id→email.
func (d *Deliverer) emailByUser(ctx context.Context, accountID string) (map[string]string, error) {
	emailToUser, err := d.emails.EmailsForAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(emailToUser))
	for email, userID := range emailToUser {
		// Keep the first email seen per user; the mirror stores one per source.
		if _, ok := out[userID]; !ok {
			out[userID] = email
		}
	}
	return out, nil
}
