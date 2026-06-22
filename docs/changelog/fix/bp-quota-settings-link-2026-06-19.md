# Quota-limit messages link to the correct Settings → Usage page

## Summary

When an account hits a usage quota, the UI tells the user to request a quota
increase "from Settings → Usage" but left that as plain text — users had to find
the page themselves, with no guarantee of landing on the correct account's usage
page. This affects two flows:

- **Deploy** — the "Compute limit reached" panel on the deploy form.
- **Blueprint create** — the "Agents limit reached" message shown after
  connecting a repo / registering a new agent.

## Design

Both surfaces now offer two real actions on a quota limit: a **"request a quota
increase"** dialog and a navigation **link to Settings → Usage**, scoped to the
account in context (the deploy target / the account the blueprint is being
created into), not the viewer's personal account.

Scoping is centralized in a shared `accountSettingsPath(accounts, name, section)`
helper: personal and organization accounts have distinct routes
(`/settings/usage` vs. `/settings/org/:slug/usage`); it picks the org-scoped path
when the account `type` is not `personal`.

The blueprint-create limit originates server-side: the entitlement middleware
returns **HTTP 402** with `error: "Limit reached"` (distinct from
`"Feature not available"` for plan gaps). Detection keys off `status === 402`
plus that error category — no change to the shared error type. The quota-increase
dialog targets the agents feature and lazily loads usage (only once a limit is
hit) to populate the request meter.

## Migration

None. Behavior-only changes to the deploy compute-limit panel and the blueprint
create quota message.
