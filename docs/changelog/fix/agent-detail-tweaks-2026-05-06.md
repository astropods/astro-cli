# Agent detail cosmetic tweaks

## Summary

Cosmetic polish pass across the agent detail pages: updated color treatments for deployment states, upgrade banners, the starfield background, and the monitor time-range picker.

## Design

- Deployment tile transitional states (deploying, restarting, pausing, resuming) now use yellow instead of teal.
- Upgrade nudge in the deployment panel redesigned: non-clickable banner with an indigo treatment, positioned above the active deployment, with a proper Button CTA.
- Rollback/upgrade banners on the configure page updated from purple to indigo with light/dark-aware text and backgrounds.
- Light-mode starfield gradient reworked to a blue-to-pink-to-amber oklch gradient for better saturation.
- Monitor time-range pill switcher active state uses primary color in light mode for better visibility.
- Replaced leftover `stone-*` color references in configure page components with `slate-*`.

## Migration

No migration required.
