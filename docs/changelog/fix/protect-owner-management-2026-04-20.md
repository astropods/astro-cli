## Summary

Close a permission gap where org admins could remove or demote owners through the members page, and fix a related auth-provider regression that was surfacing as a spurious "Failed to check authentication" banner for logged-out users.

The membership hierarchy is now strictly owner > admin > member: admins retain full control over members and other admins, but only owners can modify or remove other owners. The last-owner guard (which prevents demoting or removing the only owner) remains intact on top of this check.

## Design

**Backend.** The authorization check lives inside the org sync layer under the existing Postgres advisory lock, so the hierarchy check is serialized alongside the last-owner guard and is TOCTOU-safe. `ChangeMemberRole` and `RemoveMember` now take the caller's role (pulled from the JWT session), and return a new `ErrOwnerManagementForbidden` sentinel when a non-owner tries to mutate an owner. The two affected handlers map that sentinel to HTTP 403. A narrow `memberRoleSyncer` interface was introduced in the handlers package so the hierarchy path can be covered by unit tests without spinning up a real WorkOS client.

**Frontend.** `OrgMembersSettings` now computes a per-row `canManage` capability that mirrors the backend rule — admin viewers see owner rows as read-only (no role dropdown, no action menu), while owners retain full controls over other owners.

**Unrelated auth fix.** After the recent `ApiRequestError` refactor, the server's `error` field is exposed on the thrown error as `code` (and `error_description` as the Error `message`). `AuthProvider` and `QueryAuthSync` were still reading `err.error` / `err.error_description`, so the benign "unauthorized" branch never matched and logged-out users saw a generic error banner. Both now read the correct fields, with `QueryAuthSync` guarded by `instanceof ApiRequestError`.

## Migration

None required. The sync method signatures changed, but both methods are only called from the two affected handlers in this repo.
