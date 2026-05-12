# Restore avatar-color grid stroke in dark mode on Blueprint detail

## Summary

The subtle SVG grid behind the avatar/title on Blueprint detail pages stopped
picking up the blueprint's avatar color in dark mode, and most recently
ended up rendered as a much brighter flat-white pattern than intended. The
grid is meant to softly echo the blueprint's identity color in both themes.

## Design

The original implementation (Apr 22) drew the dark-mode grid with
`stroke={avatar_colors?.vibrant_light ?? var(--color-teal-700)}`,
`strokeWidth="0.5"`, `strokeOpacity="0.2"` — a faint colored grid matching
the light-mode treatment.

Two later commits regressed it:

- The extraction into `GradientGridWash` swapped the dark stroke from the
  avatar color to a hardcoded `white` at `strokeOpacity="0.12"`. Subtle, but
  no longer color-aware.
- A later pass bumped the dark grid to `strokeWidth="0.75"` and
  `strokeOpacity="0.4"`, making the white pattern noticeably brighter.

The fix restores the dark-mode pattern stroke to use `gridColor`
(`avatar_colors?.vibrant_light`, falling back to `var(--color-teal-700)`)
with `strokeWidth="0.5"` and `strokeOpacity="0.2"`, matching the original
behavior. Light-mode grid is unchanged.

## Migration

None required.
