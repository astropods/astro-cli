## Summary

Redesigns the global navigation with a compact scope switcher, centered nav links with count indicators, and updated profile/org management placement.

## Design

**Scope switcher** — compact pill inline after the logo; username only (no display name), no border, `rounded-sm`. No default fill; `hover:bg-stone-200`. Org avatars resolved via `UserAvatar` (CDN handle lookup) instead of a generic building icon.

**Nav links** — centered using absolute positioning. Active state uses medium font weight. Agent and blueprint counts shown as `Tag` components next to each link; hidden when count is zero. Blueprints listed before Agents.

**Create Organization** — moved from the profile dropdown into the scope switcher dropdown as the last item, separated by a divider. Organizations list removed from the profile dropdown entirely.

## Migration

No migration required.
