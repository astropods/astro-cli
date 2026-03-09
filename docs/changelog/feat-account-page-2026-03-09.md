# Account settings page

## Summary

Users can now manage their account from a dedicated settings page at `/settings/account`. This includes editing display names, changing usernames with availability checking, and initiating account deletion. The session refresh flow was also improved to reflect profile changes immediately and reduce latency.

## Design

- **Settings layout** — A sidebar navigation shell at `/settings` with an `Outlet` for sub-pages. Currently has one section (Account); the layout is ready for additional sections like notifications or API keys.
- **Profile editing** — Display name field with unsaved-changes guards (react-router `useBlocker` + `beforeunload`). Saves via `PATCH /api/v1/me` which updates the user in WorkOS.
- **Username change** — Dialog with real-time availability checking (debounced), destructive confirmation checkbox, calls `PUT /api/v1/accounts/:account`.
- **Account deletion** — Dialog requiring typed username confirmation and checkbox acknowledgment. Server endpoint is stubbed (`DELETE /api/v1/accounts/:account` returns 501) until full resource teardown is implemented.
- **Session refresh** — `refreshSession` now fetches fresh user data from WorkOS concurrently with org membership sync, so profile changes appear immediately without re-login.
- **Shared abstractions** — Extracted `useDebouncedValue` hook, `AccountNameInput` component, and `DestructiveConfirmCheckbox` to eliminate duplication across onboarding, org creation, and settings dialogs.

## Migration

No migration required.
