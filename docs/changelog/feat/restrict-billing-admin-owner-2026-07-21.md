# Restrict billing to org admins and owners

## Summary

Billing was reachable by any organization member: the Billing nav item, the
billing route, and every billing API (usage, invoices, balances, and payment-
method save/delete) were gated on account *membership* only. That meant a plain
member could view an org's financial data and add or remove the saved card.
Billing is now restricted to org admins and owners.

## Design

The authorization seam already exists: account-scoped routes are grouped by an
`org:manage` WorkOS permission (admin/owner) versus plain membership. The change
moves the `/billing/*` endpoints from the member group into the `org:manage`
group, so the server rejects members at the API layer regardless of the client.

Endpoints moved: billing usage, invoices, invoice PDF, balances, setup-intent,
and payment-method GET/POST/DELETE. `/quota-increase` was already admin/owner-only.

`/usage` (quota usage) stays on the member group deliberately — it backs the
deploy flows, not just billing, so restricting it would block members from
deploying. Personal accounts are unaffected: their sole member is the owner and
holds `org:manage` implicitly.

The client mirrors the server: the org Billing nav item is hidden for members
(alongside Secrets, API Keys, Audit Log), and the org billing page renders a 403
guard so direct navigation to the URL by a member is denied rather than showing
a broken view.

## Migration

None required. Existing admins and owners see no change. Members lose access to
the org Billing page and its APIs; personal-account billing is unchanged.
