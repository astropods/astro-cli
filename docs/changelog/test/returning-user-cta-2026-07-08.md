# Summary

The returning-user nav CTA spans two systems — the app sets a durable `astro_returning` cookie on login, and the marketing nav reads it to show "Sign in" instead of "Get started". Nothing exercised that handshake end-to-end against a real login.

# Design

Add an authenticated smoke test (in the `auth` project, which reuses the real-login session state) that:
1. Asserts the login persisted the `astro_returning` cookie on the shared `astropods.com` origin.
2. Loads `/home` — the always-marketing landing page, excluded from the apex CloudFront session-cookie check so a logged-in browser reaches the marketing nav there (the apex `/` would serve the app).
3. Asserts the nav CTA reads "Sign in" (→ /login), the returning-user state.

# Migration

None — test-only change.
