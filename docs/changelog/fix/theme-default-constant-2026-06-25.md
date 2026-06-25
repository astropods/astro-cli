## Summary

The default-theme change (now `dark`) hardcoded the fallback value across several call sites in `theme.ts`, and missed three spots in `root.tsx` that still fell back to `light`. As a result, a first load with no theme cookie or `localStorage` value would render light for the initial paint despite dark being the intended default.

## Design

Introduces a single `DEFAULT_THEME` constant in `lib/theme.ts` as the source of truth for the resolved default. Every default fallback — `ServerThemeContext`, `parseCookieTheme`, the SSR system-theme fallback, and `loadTheme` — now references it instead of a literal. `root.tsx` imports the same constant for its loader fallback, the `ServerThemeContext` provider value, and the inline no-FOUC script (interpolated into the script string), so server, blocking script, and client all agree on one value.

## Migration

No action required.
