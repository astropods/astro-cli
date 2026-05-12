# VaultPicker scope-aware "+ New"

## Summary

An admin or owner of an organization could not create a new variable for that organization from a deploy form when their active session was still scoped to a different organization. Clicking "+ New" in the VaultPicker fired the create-variable mutation against the target account, the server saw a session scoped to the wrong org, and returned 403 — even though the user genuinely had permission on the target org. This shipped surprising failures any time a user moved between orgs and immediately deployed.

The fix detects this mismatch on mount, re-scopes the session to the target organization in the background, and hides "+ New" until the switch resolves. The picker itself stays fully usable for browsing and selecting existing entries throughout, so only the action that would have 403'd is gated.

## Design

VaultPicker now derives `targetOrgId` from `useAuth()` — the WorkOS organization id of the form's target account, or `null` when no switch is required (personal account, missing/unknown account, or session already scoped). When `targetOrgId` is set, an effect calls `switchOrg(targetOrgId)`; both create affordances (the header "+ New" and the empty-state "New variable" button) render only when `scopeReady` (i.e. `targetOrgId === null`). The picker itself stays fully functional for browsing and selecting existing entries throughout.

This mirrors the eager `switchOrg` pattern already used by `OrgSettingsLayout`. To avoid firing N parallel `switchOrg` calls when a deploy form renders N VaultPicker instances for the same target org (WorkOS refresh-token rotation cannot tolerate concurrent calls), VaultPicker maintains a small module-level `Map<targetOrgId, Promise>` that coalesces concurrent switches: the first instance starts the switch and registers the promise; subsequent instances see the inflight entry and skip. The map entry is cleared in the promise's `finally`, so subsequent target orgs are handled independently. Failures are silent — "+ New" stays hidden rather than blocking the rest of the picker.

## Migration

None.
