# Transparent session refresh for expired tokens

## Summary

Sessions that expire while a user is away now refresh transparently instead of forcing a full re-login. Previously, `UnsealSession` rejected expired sessions outright, so any request with a stale cookie received an immediate auth error. This was especially annoying for users who leave a tab open overnight.

## Design

**Server (`astro-server`):**
- `UnsealSession` no longer checks expiry. It returns the decrypted session regardless of its `ExpiresAt`, allowing callers to decide how to handle it.
- The auth middleware still calls `IsSessionValid` after unsealing, so all protected API routes continue to reject expired sessions.
- `/auth/me` checks `IsSessionValid` and, if the session is expired, attempts a transparent refresh via the stored WorkOS refresh token before returning an error.
- `/auth/refresh` intentionally accepts expired sessions — that's its purpose.
- Default `SessionMaxAge` and `CookieMaxAge` increased from 1d/7d to 30d/30d to reduce login frequency.

**Client (`astro-client`):**
- `AuthProvider.checkAuth` no longer flashes a loading state when re-checking an already-authenticated session (e.g. after a 401 triggers a retry). Only the initial unauthenticated check shows loading.
- `refreshVersion` now only increments on explicit refresh calls, not on the initial auth check, preventing unnecessary downstream refetches.
- Mount-time auth check deduplicated — reuses `checkAuth()` instead of an inline copy with identical logic.
- `AuthGuard` renamed to `WaitlistGuard` to clarify its temporary purpose.
- `Onboarding` page adds its own auth guard since it can't use `ProtectedRoute` without creating a redirect loop.

## Migration

No migration required. Session cookie format is unchanged; existing cookies will continue to work. The longer max-age means users with existing cookies will simply stay logged in longer.
