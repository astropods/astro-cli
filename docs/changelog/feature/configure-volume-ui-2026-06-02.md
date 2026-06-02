## Summary

Polish pass on the Volume control in the Configure panel. The toggle was a custom hand-rolled checkbox with arbitrary text sizes (`text-[13px]`/`text-[11px]`) and radii (`rounded-[6px]`), and its dark-mode appearance had three rendering issues: an icon tile that fell back to a background matching the card surface, a primary fill that disappeared into the dark backdrop, and a primary-tinted border that read as nearly invisible.

## Design

Replaces the custom checkbox affordance with the shared `Switch` primitive, wraps the row in a `<label>` so the entire surface remains clickable without nesting interactive elements. Typography moves to semantic tokens (`text-heading-4`, `text-body-sm`) and radius aligns with the rest of the Configure panel's tiles.

Dark-mode handling uses the codebase's established two-mode pattern (see `Squircle.tsx`, `TimeRangeSelector.tsx`) — opacity-based primary fills don't perceptually swap across themes because the math blends against a different backdrop, so the selected state pairs each `bg-primary/*`/`border-primary/*` with a brighter `dark:` variant, and the icon shifts to `dark:text-indigo-300` for visibility on the tinted tile.

The container drops the `Card` chrome so the resting state is a transparent bordered row on the panel surface; selecting it lifts to a soft primary-tinted tile. This produces a stronger active/inactive delta and removes the previously-redundant card-on-card stacking.

## Migration

No action required. Props and behavior are unchanged.
