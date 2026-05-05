# Connect device registration ignores JWT org claim

## Summary

`ast connect` was failing on `fleet.astropod.ai` with `registration rejected: registration failed`. The Connect handler was reading `org_id` from the caller's JWT claim and using it as the `account_id` on the new `connected_devices` row. When the claim referenced an org that wasn't present in the local `accounts` table, the FK constraint rejected the INSERT and the user saw a generic failure message — the real DB error was only logged server-side.

The deeper issue: a developer machine doesn't really belong to an org. Scoping device records by org-from-JWT made registration brittle without buying any isolation that the rest of the system actually uses (sessions are keyed by `device_id` alone; `ListAll` returns devices across all accounts).

## Design

Drop the JWT org-resolution branch in `connectgrpc.Server.Connect`. Always resolve the user's personal account and use that as the device's `account_id`. The schema, `ListAll`, the admin gRPC `ConnectedDevice` shape, and the queen TUI's "Account" column are unchanged — the `account_id` slot is still populated, it's just always the personal account.

The `OrgIDFromContext` helper and the `org_id` context key were removed since no callers remain. The JWT validator still parses the org claim; it's simply no longer threaded into request context.

If a user has no personal account, registration now returns `PermissionDenied "no personal account found — run 'ast login' and create an account first"` rather than the previous generic failure.

## Migration

None. No schema changes; no client-facing protocol changes. Existing device rows are unaffected.
