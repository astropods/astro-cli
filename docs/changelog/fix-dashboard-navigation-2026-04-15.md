# Fix: dashboard org switcher ignores click on personal account when another account is starred

## Summary

Clicking your personal account in the dashboard org switcher had no effect when a non-personal account was set as the starred default. The view stayed on the starred org instead of switching to your personal account.

## Design

The dashboard computes the active account with a priority chain:

```
URL ?account= param → starred default (localStorage) → personal account
```

When switching to the personal account the switcher previously cleared the `?account=` param entirely, leaving the URL at `/dashboard` with no param. The starred org in `validStoredDefault` then won the fallback and the view never changed.

The fix is to always set `?account=<name>` explicitly on every switch, including when switching back to the personal account. The URL priority chain then picks up the explicit param and the starred default is correctly bypassed.

Two tests were added to `AgentDashboard.test.tsx`:
- switching from personal → org account
- switching from org → personal account when another account is starred (the regression case)

## Migration

No action required.
