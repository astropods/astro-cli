# Send the billing banner to the gated account's billing page

## Summary

The suspension banner reads the active account's billing status, which can be an
organization, but its call to action always navigated to `/settings/billing`.
That route is the personal account's page.

An organization owner whose agents were stopped saw "Your agents are stopped.
Add a payment method to switch to pay-as-you-go", clicked it, landed on their
personal billing page, and added a card there. The personal account un-gated.
The organization stayed suspended, still with no Stripe customer, and the banner
stayed up. The card panel on that page then showed a Visa directly above a
banner saying to add one, because the panel is scoped to the route and the
banner to the active account.

## Design

Route through `accountSettingsPath`, which already resolves a personal account to
`/settings/<section>` and an organization to `/settings/org/<slug>/<section>`.
The helper existed and four other call sites use it; the banner predates it.

The two scopes are now the same account. Everything else about the banner is
unchanged: it still reads status for the active account and still renders
nothing while active.

**The banner waits for the account list.** `activeAccount` comes from the root
loader, which resolves before `AuthProvider` fetches accounts, and
`useBillingStatus` is enabled on the account name alone. In that window
`accountSettingsPath` finds no matching account and falls back to the personal
path, reproducing the same bug. Rendering nothing for a render is cheaper than
sending an owner somewhere their card cannot help.

## Migration

None. A gated organization's banner now links to that organization's billing
page instead of the personal one.
