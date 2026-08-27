# One org switcher, one org-scoped session

## Summary

Agents, Blueprints, and Knowledge each carried a multi-select account filter, so
a list could span several accounts at once. A session JWT carries exactly one
WorkOS organization, and the server now refuses account reads the session is not
scoped to, so a cross-account list is no longer a scope the session can express.

Every primitive now shows the same single org switcher Insights already had, and
selecting an account re-mints the session token for that organization.

## Design

**One scope, owned by the active account.** `ActiveAccountProvider` is the only
writer of the org scope. `setActiveAccount` now:

1. Calls `switchOrg(account.organization_id)` when the target account belongs to
   a different organization than the session claims.
2. Writes the active-account cookie only after that resolves.
3. Revalidates loaders, which then run under both the new cookie and the new
   token.

A failed re-scope leaves the scope where it was, so the UI never shows a list the
session cannot read. The switch keeps using the existing progress bar, which now
covers the token round trip as well.

**Same switcher everywhere.** `AccountScopeFilter` sits next to the page title
("Agents for ▾ acme"), matching Insights. `useOrgScope` gives each page the
active account, the setter, and the single-account scope its list query needs.

**The switch answers the click, not the network.** Re-minting the token is a
round trip, so the scope cannot commit instantly. The switcher therefore renders
the account the switch is moving to, marks itself busy, and refuses a second
switch until the first settles, which WorkOS refresh-token rotation cannot
tolerate anyway. The pending target lives in the same module-level store the
progress bar reads, and the provider drops it on unmount so a switch that loses
its tree cannot leave the app looking busy.

`POST /auth/switch-org` also stopped listing WorkOS memberships on every switch.
The listing exists so the local `account_members` row can supply a membership id
the JWT did not carry, so it now runs only when the claim is actually missing.
That takes one of two WorkOS round trips off the critical path of every org
change.

**`?account=` is a deep link, not page state.** A URL naming a membership adopts
that organization as the active scope and drops the param. Links that already
point at an account, such as the post-deploy redirect to `/agents?account=x`,
land on the right organization instead of a page-local filter. SSR resolves the
same account through `loadOrgScoped`, so the primed query matches what the client
adopts.

A switch that cannot re-scope the session says so, naming the account it failed
to reach, and the switcher returns to the account still in scope.

**Removed.** The multi-account filter (`AccountFilter`), its URL and
localStorage plumbing (`use-account-filter-param`,
`use-persistent-page-filter-path`), the page-local Insights scope hook, and the
orphaned `OrgSwitcher`. Header nav links no longer carry a persisted account
filter. The `/me/*` list endpoints keep their multi-account contract; the client
only ever asks for one account.

The `astro:default-account` localStorage key goes too. It predates the
active-account cookie, and the migration that copied it into the cookie has run
on every load since the cookie shipped, so the cookie is now the only record of
which account is active.

## Migration

None. A persisted multi-account filter is ignored, and the first list a user
opens shows their active organization.
