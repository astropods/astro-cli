# Cookie-based SSR dark mode

## Summary

Dark mode flashed on page load (especially mobile) because the server had no access to the user's theme preference in `localStorage`, so SSR always rendered light mode. React hydration then fought the inline script's DOM patch, causing a visible flicker.

## Design

The resolved theme is now mirrored to an `astro-theme` cookie (`light`/`dark`, never `auto`) so the server can render `<html class="dark">` directly in the SSR response. The root loader parses the cookie and `Layout` applies it via `useMatches()` with `suppressHydrationWarning` on `<html>`. A `ServerThemeContext` feeds the cookie value to `useResolvedTheme()` so downstream consumers (charts, starfield) also SSR with the correct theme.

Client-side, `setTheme()` and all listeners (cross-tab, system preference) write both `localStorage` and the cookie. The inline `<head>` script remains as a fallback, updated to `classList.toggle` and to write the cookie for migration/self-healing.

## Migration

No action required. Existing users get their cookie written on first page load after deploy.
