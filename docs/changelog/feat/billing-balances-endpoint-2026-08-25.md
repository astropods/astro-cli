# Billing balances endpoint

## Summary

`feat/free-trial-modal` needs the account's real signup credit grant to
announce it, and no existing endpoint carries it. `GetBillingSpend`'s
`CustomerSpend` reads Metronome credits too, but only to sum a *remaining*
balance for the spend summary; it discards the per-credit grant, schedule,
and expiry entirely. Adds `GET /billing/balances`, returning the customer's
credits and commits as Metronome reports them, for a client that needs the
full record rather than one rolled-up number.

## Design

- `BillingProvider.Balances(ctx, customerID) (any, error)`, alongside the
  provider's other read-back methods (`UsageData`, `DailySpend`, `Invoices`).
- `metronome.Provider.Balances` lists the customer's credits and commits via
  `V1.Customers.Credits.ListAutoPaging` / `V1.Customers.Commits.ListAutoPaging`,
  with `IncludeBalance: true` on both (without it, every credit and commit
  comes back with a zero balance, which the client would show as fully
  spent — the same reason `CustomerSpend` sets it). Unlike `CustomerSpend`'s
  own credit read, this omits `CoveringDate`, so the client sees the full
  record, past and future grants included, not just what's active today.
  Returned passed through as-is, for the client to interpret (see
  `billing-balances.ts`'s `toBalanceRow`, which reads each record's
  `access_schedule.schedule_items` for the granted amount and `balance` for
  what's left). Covered by
  `TestBalances_RequestsIncludeBalanceForCreditsAndCommits`, which asserts a
  real, non-zero balance round-trips for both a credit and a commit, not
  just that the flag was sent.
- `noop.Provider.Balances` reports `ErrBillingUnavailable`, matching every
  other read on the OSS provider.
- `GET /billing/balances`, registered next to the other billing reads in
  `main.go`.

No time window: unlike `UsageData`/`DailySpend`, a customer's credits and
commits aren't bounded by a billing period, and the list itself is small
(a handful of grants at most), so there's nothing to page defensively
against the way `DailySpend` does.

**Public docs land on the consuming PR, not here.** This branch adds only
the endpoint; the only user-visible surface reading it is
`feat/free-trial-modal`'s one-time signup modal, not a page an account
owner can navigate back to. That branch adds a short "Signup credit"
section to `usage-limits.mdx` covering what a user actually sees, so the
deferral resolves there rather than staying open.

**The raw `shared.Credit`/`shared.Commit` pass-through is a deliberate
choice, confirmed rather than left silent.** The response carries every
field Metronome sends, including internal ones like `created_by` (the
name of the admin API key that granted the credit) that no account owner
has a reason to see. This is consistent with every other billing
read-back endpoint (`Invoices`, `UsageData`) already doing the same
pass-through, the route is account-scoped and auth-gated like the rest of
`accountManage` so it's never public, and the client already narrows to
the handful of fields it reads (`lib/api.ts`'s `BillingRecord`) rather
than trusting the wire shape. Scoping the server response down to just
those fields would break that consistency for one endpoint out of three
that share the pattern; kept as-is.

## Migration

None. New, additive endpoint; no existing route or response shape changes.
