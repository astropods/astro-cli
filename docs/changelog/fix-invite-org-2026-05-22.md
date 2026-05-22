# Fix: org invite acceptance now works without logout

## Summary

When an existing user accepted a WorkOS org invite, they were redirected back to Astro with an `invalid_state` error and remained logged into their previous account. Accepting the invite while already logged in left the new org invisible until the user logged out and back in.

The root cause was that WorkOS's hosted invite UI (`authkit.app/invite?invitation_token=...`) handles the auth flow entirely on its own side. When it redirected to `/auth/callback`, no `auth_state` cookie existed (Astro's `Login()` never ran to create one), so the CSRF state check failed.

## Design

The fix routes invite acceptance through Astro's `/auth/login` endpoint by passing the WorkOS `invitation_token` as a query parameter. `Login()` forwards the token to WorkOS's authorization URL, which handles invite acceptance as part of the normal OAuth flow. This means state is generated and stored in a cookie before WorkOS is involved, so `/auth/callback` receives the matching state and processes the session normally — including `SyncMembershipsForUser` running with an already-active membership.

`GetAuthorizationURL` in `WorkOSClient` was refactored to accept an `AuthorizationURLOpts` struct (replacing the variadic `ScreenHint` param) so both `ScreenHint` and `InvitationToken` can be passed together. The WorkOS Go SDK v6 does not expose `InvitationToken` on `GetAuthorizationURLOpts`, so the parameter is appended manually to the built URL — the WorkOS authorization endpoint accepts it as a standard query param.

The WorkOS dashboard **User invitation URL** must be set to `https://astropod.ai/auth/login` (no query params). WorkOS appends `?invitation_token=<token>` automatically — including the placeholder in the configured URL causes WorkOS to URL-encode it literally and append a second malformed parameter, resulting in an "Invalid invitation" error.

## Migration

Set **User invitation URL** in the WorkOS dashboard to `https://astropod.ai/auth/login`. No code changes or deployments required beyond what is in this PR.
