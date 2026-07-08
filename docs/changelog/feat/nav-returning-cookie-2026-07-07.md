# Summary

The marketing site shows a "Get started" CTA to new visitors and "Log in" to returning ones, but it's a static site served from the shared `astropods.com` origin and can't read the app's HttpOnly session cookie. It needs a durable, client-readable signal that a user has logged in before.

# Design

On successful authentication, `AuthProvider` sets a non-HttpOnly `astro_returning=1` cookie (`Secure`, `SameSite=Lax`, `Path=/`, 1-year), mirroring the existing cookie writer in `use-active-account.tsx`. The marketing site's client JS reads this same-origin cookie to choose the CTA. It's set once and intentionally **not** cleared on logout — the semantic is "has ever logged in", not "currently logged in". Host-only today because the app and marketing site share the `astropods.com` host; a subdomain split would require `Domain=astropods.com`.

# Migration

None — additive client-side cookie, no API, DB, or config changes.
