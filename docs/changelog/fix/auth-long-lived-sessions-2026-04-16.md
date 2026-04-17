# Long-lived auth sessions and login error recovery

## Summary

Users were intermittently getting kicked to login after idle periods, and encountering a dead-end "State parameter mismatch" error when returning through the OAuth callback. This change fixes both issues by switching to industry-standard long-lived sessions and adding auto-recovery for stale login state.

## Design

### Root cause: hardcoded 1-hour session expiry

`CreateSession()` was called with a hardcoded `expiresIn: 3600` (1 hour) in three places — initial login, org switch, and token refresh — even though `SessionMaxAge` is configured to 30 days. This forced constant client-side refresh cycles and made idle sessions expire quickly.

The fix replaces the hardcoded value with `int(h.cfg.Auth.SessionMaxAge.Seconds())` so sessions are born with the full configured lifetime (default 30 days). Each refresh slides the window forward another 30 days. `CreateSession` already caps expiry at `SessionMaxAge`, so the cap logic remains as a guard.

### Root cause: concurrent refresh token race

When a tab regained focus after idle, two things fired simultaneously:
1. The visibility-change handler called `refreshIfNeeded()` → hit `/auth/refresh`
2. TanStack Query's `refetchOnWindowFocus` triggered 401s → `QueryAuthSync` called `checkAuth()` → hit `/me`

Both requests tried to use the same refresh token. WorkOS does refresh token rotation — the second request got a stale token and failed, logging the user out.

**Fix:** `checkAuth()` is now deduplicated via a shared in-flight promise ref. Concurrent callers receive the same promise instead of each making a separate `/me` request. The proactive refresh machinery (`isTokenExpiringSoon`, `refreshIfNeeded`, `isRefreshing`) was removed entirely — with 30-day sessions, the visibility/focus handler simply calls `checkAuth()`, which hits `/me` where the server handles refresh transparently.

### Root cause: 5-minute CSRF cookie + no error recovery

The `auth_state` cookie (OAuth CSRF protection) expired after 5 minutes. Users who paused on the login screen (MFA setup, reading terms, distraction) would see "State parameter mismatch" with no recovery path.

**Fix:** Cookie TTL extended to 15 minutes. Additionally, the client now auto-retries the login flow once on `invalid_state` (tracked via `sessionStorage` counter to prevent infinite loops), clearing the counter on successful auth.

### Security model

These changes do not weaken any security boundary:

- **Session cookie security is unchanged:** `HttpOnly`, `Secure`, `SameSite`, AES-256-GCM encryption via PBKDF2-derived key. No secrets are exposed to JavaScript.
- **The short session expiry was not a security control.** If an attacker obtained the session cookie, they also obtained the refresh token (encrypted inside the same cookie) and could extend the session indefinitely. The 1-hour expiry only punished legitimate users.
- **Actual security boundaries remain:** cookie theft prevention (HTTPS + HttpOnly + SameSite), server-side session revocation via WorkOS `RevokeSession`, and CSRF protection via SameSite cookies.
- **CSRF state cookie extended to 15 minutes, not removed.** The state parameter validation is unchanged — only the window is wider.
- **Stale permissions are mitigated** by `checkAuth()` on every visibility/focus change, which re-validates the session and refreshes user data (role, permissions, accounts) from the server.
- **This is the standard approach** used by GitHub, Google, Slack, and Linear — long-lived server-side sessions with strong cookie security and server-side revocation.

### Client-side simplification

The `AuthProvider` is now simpler:
- **Removed:** `isTokenExpiringSoon`, `isRefreshing` ref, `refreshIfNeeded` callback — all artifacts of the 1-hour refresh cycle
- **Kept:** `refresh()` function — still used by 5 consumer components for explicit refreshes after user actions (profile updates, org changes, etc.)
- **Added:** `checkAuthPromiseRef` for deduplication, simple `checkAuth()` call on visibility/focus

## Migration

No action required. Session and cookie max-age defaults are already 30 days in config. Existing sessions will continue to work — they'll just stop expiring after 1 hour.
