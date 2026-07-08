# Summary

The scheduled production smoke test asserted a nav "Login" link, but the marketing site now renders "Get started" (→ /signup) for new visitors and "Sign in" (→ /login) only when the app's `astro_returning` cookie is set. The cookie-less smoke browser always sees "Get started", so the old assertion fails on `main`.

# Design

Update the homepage smoke test to assert the new-visitor default CTA: a "Get started" link pointing at `/signup`. The returning-user "Sign in" state depends on a client cookie the smoke run doesn't have, so it isn't exercised here.

# Migration

None — test-only change.
