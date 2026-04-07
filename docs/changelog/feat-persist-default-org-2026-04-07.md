# Persist default org in dashboard

## Summary

Users with multiple accounts (personal + orgs) always landed on their personal account when navigating to the dashboard, with no way to make an org the default view. This adds the ability to pin any account as the default.

## Design

A star button sits next to the OrgSwitcher in the dashboard header. Clicking it saves the current account name to `localStorage` under the key `astro:default-account`. On load, if no `?account=` query param is present in the URL, the stored default is applied before falling back to the personal account.

The stored default is validated against the user's current account list on each load — if the account is no longer accessible (e.g., left an org), it is silently ignored and falls back to the personal account.

Star states:
- Filled: the current account is the active default
- Outline: clicking will set it as the default
- Disabled: personal account with no stored override (already the natural fallback)

Clicking a filled star on an org clears the stored default, restoring personal account as the fallback.

## Migration

No action required. Users who have not set a default will continue to see their personal account on the dashboard.

## Note

The default account preference is currently stored in `localStorage`, which means it is device-specific and lost if the user clears browser data. This is a temporary solution — the preference will be moved to the database in a future change so it persists across devices and sessions.
