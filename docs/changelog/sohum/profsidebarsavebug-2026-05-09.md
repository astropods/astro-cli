## Summary

Two bugs in the profile edit flow: saving caused all blueprints to briefly disappear, and clearing the display name field silently did nothing.

## Design

**Bug 1 — blueprints disappear on save**

Root cause chain:
1. `ProfileEditSidebar.handleSave()` called `refresh()` after saving
2. `refresh()` calls `api.refreshSession()` then `updateFromResponse(response, { isRefresh: true })`
3. The `isRefresh: true` flag increments `refreshVersion` in auth state
4. `QueryAuthSync` watches `refreshVersion` — on any increment it calls `queryClient.invalidateQueries()` with no args, nuking every query in the cache
5. All queries (blueprints, agents, etc.) refetched simultaneously; data disappeared during the window

`refresh()` exists for credential/token rotation. A profile edit doesn't rotate tokens so it never needed this. Fix: added `refreshUserData()` to `AuthProvider` — identical to `refresh()` but passes `{ isRefresh: false }` so `refreshVersion` never increments and `QueryAuthSync` never fires. `ProfileEditSidebar` and `ProfileSettings` now call `refreshUserData()` instead of `refresh()`.

**Bug 2 — clearing display name silently does nothing**

`handleSave()` had an `if (displayName.trim())` guard that skipped the `updateProfile` mutation when the field was empty, so clearing the name never got saved. Removed the guard so the mutation always fires.

Since an empty display name is invalid, both the sidebar and settings page now show an inline "Display name can't be empty" error and disable the Save button when the field is blank.

## Migration

No action required.
