# Auth system cleanup: login/signup routes, deep linking, and waitlist removal

## Summary

The authentication system carried dead waitlist code and used a fragile inline `<ProtectedRoute>` wrapper pattern that could crash when hooks ran before the auth check. This overhaul removes the waitlist, introduces proper `/login` and `/signup` routes, adds post-login deep linking, and moves route protection to the route configuration level.

## Design

### Route-level auth via `ProtectedLayout`

Auth protection is now declared in `routes.ts` using a layout route (`ProtectedLayout.tsx`) that checks authentication before any child page mounts. This replaces the old pattern of wrapping each page's JSX in `<ProtectedRoute>`, which was fragile — if a page called hooks before the wrapper (as `DeployBlueprint.tsx` did), they'd execute for unauthenticated users and crash.

```
layout("components/Layout.tsx", [
  // Public: blueprints, profiles, onboarding, login/signup, 404
  // Protected (auth checked once at layout level):
  layout("components/ProtectedLayout.tsx", [
    dashboard, settings, deploy, admin, configure, ...
  ])
])
```

All 8 pages that previously used `<ProtectedRoute>` had it removed. The `ProtectedRoute` component and its tests were deleted.

### `/login` and `/signup` thin redirect routes

These are client-side routes that immediately redirect to the server's `/auth/login` endpoint (which redirects to WorkOS AuthKit). They are not intermediary pages — WorkOS hosts the actual login/signup UI.

- `/signup` passes `screen_hint=sign-up` so WorkOS opens on the sign-up screen
- Both accept a `?redirect=` param for post-login deep linking
- Both use `window.location.replace()` to avoid back-button loops
- The AppHeader "Log in" and "Sign up" buttons now link to these routes

### Post-login deep linking

Previously, after login users always landed on `/`. Now the full return path is preserved:

- **Client**: `ProtectedLayout` redirects to `/login?redirect=/original/path`
- **Client**: `Login.tsx` passes the redirect param to `api.getLoginUrl(redirect)`
- **Server**: Login handler reads `redirect` query param, resolves relative paths against `FRONTEND_URL`, validates the origin, and stores the full URL in an `auth_redirect` cookie
- **Server**: Callback handler checks `auth_redirect` cookie first, then falls back to `auth_origin` cookie, then `FRONTEND_URL`

The redirect URL is origin-validated to prevent open redirects, stored in an httpOnly cookie to prevent tampering during the OAuth flow, and has a 5-minute TTL.

### WorkOS `screen_hint` support

`GetAuthorizationURL` now accepts an optional `screen_hint` variadic parameter. When `/signup` is visited, the server passes `usermanagement.SignUp` to WorkOS, which opens AuthKit directly on the sign-up screen.

### Waitlist removal

Removed all waitlist code from `astro-server`: handler, store package, route registration, OpenAPI response type, reserved name, and database table definition. Updated public docs to replace waitlist references with signup links.

## Migration

No migration required. The waitlist table should be dropped from production databases when convenient (it was verified empty).
