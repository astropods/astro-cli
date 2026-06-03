# CLI: preserve active account on re-login

## Summary

`ast account switch` already persisted the active org in credentials, but `ast login` rebuilt the profile from scratch and dropped `current_account`, so every re-authentication landed on the personal account. Users working in an org had to switch again after token refresh or explicit re-login.

## Design

**Reuse existing profile fields.** No new storage keys. Before saving the post-OAuth profile, login reads the prior `default` profile’s `current_account` and `previous_account`.

**Restore when still valid.** After `GET /api/v1/me` repopulates accounts, login copies the prior selection only if that account name is still in the membership list. `previous_account` is restored too so `ast account switch -` keeps working across re-login.

**Transient fetch failures.** If `/api/v1/me` fails on re-login, login keeps the stored account list and active selection from the previous session (only when that selection exists in the stored list) instead of treating the org as removed or prompting for a new username. The “no longer available” note only runs when a non-empty account list was fetched and the prior org is missing from it (e.g. removed from the org). An empty successful `/api/v1/me` on re-login fails with an error instead of restoring stale membership.

**Explicit override and fallback.** `ast login --account <name>` skips restoration and uses `SetCurrentAccount` as before. If the prior account is gone, login falls back to personal and prints a one-line note.

**Display.** Success output resolves the active account via `GetCurrentAccount()` so the printed account matches what subsequent commands use.

## Migration

No action required. Re-run `ast login` to pick up the behavior. Use `ast login --account <name>` when you want a different account than the last active one.

Bump `apps/astro-cli/VERSION` to **0.14.2** for release.
