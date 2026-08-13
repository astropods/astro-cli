# Scope notification links to the account they concern

## Summary

A billing notification for an organization linked to `/settings/billing`, the
personal settings page. The manager who received it followed the link, saw a
healthy personal account, and the organization stayed suspended. Organization
settings live at `/settings/org/<slug>/<section>`.

Every billing payload builder hardcoded the personal path, and the dunning sweep
emits with no account name at all, so it could not have built the right one.

## Design

The rewrite happens once, in `Deliverer.finalizePayload`, next to the existing
step that makes a relative `ctaUrl` absolute. A relative `/settings/<section>`
link on an organization's event becomes `/settings/org/<name>/<section>`.

Resolving at delivery rather than at build time is what makes the dunning sweep
work. The sweep has only account IDs, and the Deliverer looks the account up by
ID, so every emitter gets the correct link without carrying the account type.

`accountLookup` gains one narrow method, `AccountScope`, returning the account's
name and type. `AccountStore.AccountScope` reads two columns rather than reusing
`GetByID`, which joins the profile and organization tables to answer a question
about neither.

Three cases pass through unchanged: a personal account, an account that does not
resolve, and any path that is not `/settings/`. The unresolvable case matches the
client's `accountSettingsPath`, which also falls back to the personal path when
the account is unknown, so the two sides agree on the safe default.

## Migration

None. The fix applies to every notification sent after it deploys.

## Note

Also reflows comments in `metronome.LinkStripeCustomer` to the writing-style
limits in `CLAUDE.md`. No behavior change; folded in here rather than opened as a
separate pull request for one paragraph.
