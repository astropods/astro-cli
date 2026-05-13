## Summary

Adds a smoke test to verify the Discord community invite link on the marketing homepage is valid and not expired, following the Slack → Discord swap in astro-ai-website PR #38.

## Design

New test in `tests/smoke/public.spec.ts` under the "External links" describe block. It locates the Discord link by role, extracts the invite code from the `href`, then hits `https://discord.com/api/v9/invites/{code}` directly — a 200 response confirms the invite is active; anything else fails the test.

## Migration

No action required.
