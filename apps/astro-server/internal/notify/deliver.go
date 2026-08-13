package notify

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// accountTypeOrganization is the accounts.type value for an organization.
const accountTypeOrganization = "organization"

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
	// AccountScope returns the account's name and type ("personal" or
	// "organization").
	AccountScope(accountID string) (name, accountType string, err error)
}

// managerLookup resolves an account's org managers (org:manage — owner + admin)
// to WorkOS user ids by querying WorkOS. Optional: a nil lookup (or an empty
// result, e.g. a personal account with no org) falls back to the owner.
type managerLookup interface {
	ManagerUserIDs(ctx context.Context, accountID string) ([]string, error)
}

// watcherLookup resolves the members subscribed to a deployment's alerts.
// *watcher.Store satisfies it. Optional: a nil lookup makes AudienceWatchers
// behave as AudienceManagers.
type watcherLookup interface {
	ActiveUserIDs(ctx context.Context, deploymentID string) ([]string, error)
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
	watchers   watcherLookup
	appBaseURL string // trimmed of trailing slash; used to absolutize relative ctaUrl
	log        *logger.Logger
}

// WithWatchers attaches the watcher lookup that backs AudienceWatchers. It is a
// setter rather than a constructor parameter because it is optional and the
// constructor already carries every required collaborator.
func (d *Deliverer) WithWatchers(w watcherLookup) *Deliverer {
	d.watchers = w
	return d
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

	return d.provider.Trigger(ctx, ev.workflowID(), recipients, d.finalizePayload(ev), ev.transactionID())
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
// outside the app, and with the RFC 3339 timestamp every workflow receives.
// Absolute or empty ctaUrl values pass through unchanged.
func (d *Deliverer) finalizePayload(ev Event) map[string]any {
	out := make(map[string]any, len(ev.Payload)+1)
	maps.Copy(out, ev.Payload)
	if url, ok := out[PayloadCTAURL].(string); ok {
		url = d.accountScopedCTA(url, ev.AccountID)
		if d.appBaseURL != "" && strings.HasPrefix(url, "/") {
			url = d.appBaseURL + url
		}
		out[PayloadCTAURL] = url
	}
	// An event that reached delivery unstamped (built outside an emit seam) is
	// still better served by the delivery time than by an empty field.
	out[PayloadTimestamp] = ev.Stamped(time.Now()).OccurredAt.Format(time.RFC3339)
	return out
}

// accountScopedCTA rewrites a relative "/settings/<section>" link to the
// organization-scoped route. An organization's settings live there, so the
// personal path sends its manager to a page that reports nothing wrong. A
// personal account, an unresolvable account, and any other path pass through.
// That fallback matches the client's accountSettingsPath.
func (d *Deliverer) accountScopedCTA(url, accountID string) string {
	section, ok := strings.CutPrefix(url, "/settings/")
	if !ok || accountID == "" || d.accounts == nil || strings.HasPrefix(section, "org/") {
		return url
	}
	name, accountType, err := d.accounts.AccountScope(accountID)
	if err != nil || accountType != accountTypeOrganization || name == "" {
		return url
	}
	return "/settings/org/" + name + "/" + section
}

// resolveRecipients maps the audience policy to concrete recipients. Every
// audience resolves to a named user or to the account's managers; nothing
// addresses the full member list.
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
	case AudienceWatchers:
		return d.resolveWatchers(ctx, ev, recip)
	default:
		return nil, fmt.Errorf("unknown audience %q", ev.Audience)
	}
}

// resolveWatchers returns the members subscribed to the event's deployment. It
// falls back to managers when there is no watcher lookup (unconfigured), no
// deployment scope on the event, nobody watching yet, or the lookup fails — an
// alert with no watchers is a routing gap, not a reason to stay silent.
func (d *Deliverer) resolveWatchers(ctx context.Context, ev Event, recip func(string) []Recipient) ([]Recipient, error) {
	if d.watchers == nil || ev.DeploymentID == "" {
		return d.resolveManagers(ctx, ev, recip)
	}
	ids, err := d.watchers.ActiveUserIDs(ctx, ev.DeploymentID)
	if err != nil && d.log != nil {
		d.log.Warn("notify: watcher lookup failed, falling back to managers",
			"error", err, "type", ev.Type, "deployment_id", ev.DeploymentID)
	}
	if len(ids) == 0 {
		return d.resolveManagers(ctx, ev, recip)
	}
	out := make([]Recipient, 0, len(ids))
	for _, uid := range ids {
		out = append(out, recip(uid)...)
	}
	// Every watcher resolving to an unmirrored email would otherwise drop the
	// alert entirely; managers are the same backstop as having no watchers.
	if len(out) == 0 {
		return d.resolveManagers(ctx, ev, recip)
	}
	return out, nil
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
