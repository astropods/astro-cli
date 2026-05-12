# Hide VaultPicker "+ New" for users without write permission

## Summary

Members of an organization (role `member`) could open the VaultPicker on a deploy form, see existing variables, and also see a "+ New" affordance — but the server rejects member-initiated writes to organization variables, so clicking it and submitting always 403'd. The fix hides "+ New" (and the empty-state "New variable" button) for any user whose role on the target account isn't `admin` or `owner`. Members can still browse and select existing entries; only the affordance that the server would reject is suppressed.

This stacks on top of the prior session-scope fix so the two gates compose: "+ New" renders only when the session is scoped to the target organization and the user has permission to write there.

## Design

VaultPicker reads `accounts` from `useAuth()`, looks up the target account by `accountName`, and derives a `canCreate` predicate that mirrors the server's `variable:write` gate:

- Personal accounts always allow create.
- Organization accounts require `role === 'admin'` or `role === 'owner'`.
- Unknown accounts (lookup miss) fall through to true so the existing fallback behavior is preserved and the server stays the source of truth.

Both create affordances render only when `scopeReady && canCreate`. The empty-state copy also softens for readers who can't create — instead of "Set and manage reusable credentials...", it reads "No variables have been added for this account yet." so the description doesn't dangle a call-to-action the user can't take.

The role predicate mirrors the one already used by `OrgSettingsLayout` to gate the secrets/variables and audit-log sidebar items, keeping the FE-side definition of "writer" consistent across the app.

## Migration

None.
