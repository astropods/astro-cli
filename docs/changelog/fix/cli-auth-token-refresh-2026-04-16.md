# Fix CLI token refresh reliability

## Summary

CLI users were intermittently getting logged out with "token expired and refresh failed" errors, requiring `ast login` to re-authenticate. Three separate issues contributed to this.

## Design

### Org-scoped token fetch burns the refresh token

`GetOrgScopedAccessToken` calls `RefreshAccessTokenForOrg` to get an org-scoped JWT but discards the response's refresh token. WorkOS uses refresh token rotation — each use invalidates the old token and returns a new one. This means after any org-scoped push, the stored refresh token is stale and the next regular `refreshToken()` call fails.

**Fix:** After the org-scoped token fetch, if WorkOS returned a new refresh token, persist it back to the profile. The org-scoped access token is still ephemeral (not saved), but the refresh token stays current.

### Zero ExpiresAt skips refresh entirely

`shouldRefresh` returned `false` when `ExpiresAt` was zero (e.g., corrupted storage, migrated credentials from an older CLI version). This meant the CLI would keep using the access token until the server rejected it — but since there's no 401 retry, the command would just fail.

**Fix:** Treat zero `ExpiresAt` as "unknown, refresh to be safe" by returning `true` from `shouldRefresh`. The refresh call will get a proper token with a known expiry.

### No 401 retry on push registration

If a token expired during a long-running push (build + push can take minutes), the final registration request would get a 401 and fail with a terminal error. The CLI had no mechanism to refresh and retry.

**Fix:** Added `RefreshAndUpdateHeader` helper to the auth package. The push registration flow now catches 401 responses, refreshes the token, and retries once before failing. This is scoped to the registration request only — the Docker push uses a separate registry token.

## Migration

No action required. Existing credentials will work — the zero-expiry fix means stale credentials are automatically refreshed on next use rather than failing.
