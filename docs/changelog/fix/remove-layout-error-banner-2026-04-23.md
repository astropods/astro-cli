# Remove Layout error banner

## Summary

The Layout component displayed a red inline error banner for auth errors and OAuth callback failures. This banner was visually disruptive and not the right UX pattern for surfacing errors — toast notifications are the intended approach. This change removes the banner and all supporting logic.

## Design

The Layout component previously maintained local state for OAuth callback errors (`callbackError`), a sessionStorage-based retry counter for stale CSRF state (`AUTH_RETRY_KEY`), three `useEffect` hooks (retry counter cleanup, error-param handling, auto-dismiss timer), and a conditional error `<div>`. All of this existed solely to power the inline banner.

With the banner removed, Layout is now a pure structural shell: it renders `ActiveAccountProvider`, `AppHeader`, and the router `Outlet` with no side effects or local state. The `error` field on `AuthState` is preserved for future use (e.g. toast-based error display).

## Migration

No migration required.
