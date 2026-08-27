# Close four real test gaps in frontend billing

## Summary

A coverage sweep scoped to frontend billing, looking for gaps that would
actually catch a regression, not ones that would pad a percentage. Found
four: an authorization function with no test despite a documented past
bug, the actual card-confirmation flow's error branches, a destructive
dialog with no test file at all, and an access-control gate blocking
direct URL navigation to org billing.

## Design

**`canManageAccountBilling`/`canManageBilling` (`billing-copy.ts`).**
Zero tests existed. The function's own comment says it exists because a
role-only check wrongly denied a personal account's own owner (the
session's org role is null outside an org context). Added tests for the
personal-account bypass, the org-role fallback, and that an unresolved
personal-account name doesn't accidentally match every account.

**`PaymentMethod.tsx`'s `handleSubmit`.** Every existing test mocked
`useStripe`/`useElements` to always return `null`, so the confirmation
flow's actual logic never ran regardless of what the test clicked. Made
both controllable per test and added the four real branches: the
`!elements` guard the button's own `disabled` state doesn't account for,
Stripe's own decline message, a SetupIntent that reports something other
than `succeeded`, and the server-side save failing after Stripe already
succeeded. Added a fifth test for the happy path (toast, mutate, dialog
close), since that was equally untested before.

**`RemovePaymentMethodDialog.tsx`.** No test file existed. Added coverage
for the destructive-checkbox gate, the mutation firing on confirm, the
dialog closing on success, the error message (mutation's own vs. the
fallback), reset-on-cancel clearing stale error state, and the inline
"Update card instead" escape hatch closing this dialog before opening the
next one (not after, which would double-render both momentarily).

**`OrgBillingSettings.tsx`.** The `isOrgAdmin` gate blocking direct URL
access to org billing for non-admins had no test. This is a real
access-control boundary, not just UI polish: the nav item is hidden for
members, but the route itself is still reachable by typing the URL.

Every new test was verified against a real regression, not just written
and left to pass: each assertion was checked by temporarily breaking the
code it covers (inverting a condition, swapping a call order) and
confirming the test actually fails, then reverting.

## Migration

Test-only. No behavior change.
