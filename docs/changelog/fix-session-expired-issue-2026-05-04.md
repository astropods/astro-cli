---
## Summary

When a WorkOS session expired while the local session cookie was still valid, navigating to org settings would fail with a generic "something went wrong, try refreshing" error instead of redirecting the user to log in. This happened because the local cookie check passed but the org-switch endpoint was the only place that actually round-tripped to WorkOS, revealing the dead session too late.

## Design

The fix operates at two layers:

**Server** (`handlers/auth.go`): `SwitchOrg` now inspects the WorkOS error for `invalid_grant` / "Session has already ended" and returns 401 + `session_expired` instead of the generic 400 + `switch_failed`. A narrow `orgTokenRefresher` interface was introduced so the WorkOS call in `SwitchOrg` can be stubbed in tests without mocking the entire WorkOS client.

**Client** (`AuthProvider.tsx`): `switchOrg` catches errors before they reach callers. A 401 or `session_expired` code triggers `window.location.replace(api.getLoginUrl(currentPath))`, sending the user to login with the current page as the redirect target so they land back after authenticating. Any other error is re-thrown so `OrgSettingsLayout` can still show its existing error state.

## Migration

No action required.
