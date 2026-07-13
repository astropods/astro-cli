# Summary

The "returning user" marker cookie (`astro_returning`) tells the marketing nav to show "Log in" instead of "Get started" for people who have signed in before. It was written **client-side** (`AuthProvider`), which was the wrong layer for the production topology: the browser is on `astropods.com`, but Cloudflare rewrites the `Host` to `astropods.ai` before the origin, and the marketing/app routing is decided at the edge from the `astro_session` cookie. The client cookie also depended on the app bundle running and had to guess the right `Domain`.

# Design

Move the marker to the **server**, where the session cookie is already issued. `astro-server` (`handlers/auth.go`) now sets `astro_returning` alongside `astro_session` — in the OAuth callback (login) and on session refresh — via a shared `setReturningCookie` helper. It reuses the session cookie's `Domain` (`AUTH_COOKIE_DOMAIN`, `.astropods.com` in prod) and `Secure` setting, so it's shared across `*.astropods.com`, but is **not HttpOnly** (the marketing nav reads it in JS) and is deliberately never cleared on logout ("has ever logged in"), with a 1-year max-age.

This works because the server sets `Domain` explicitly from config, not from the (rewritten) `Host` — the same reason the session cookie already lands correctly on `astropods.com`. Setting it on refresh also back-fills existing sessions without requiring a re-login. The client-side writer in `AuthProvider` is removed.

# Migration

None — cookie behavior only; no API, schema, or config changes (`AUTH_COOKIE_DOMAIN` already set).
