# Fix: blueprint setup screen cannot scroll

## Summary
On the "Setup your agent blueprint" screen (the configuration step shown after
selecting a repo), content taller than the viewport was unreachable. Both the
page wrapper and the centered content column were `flex flex-1` with
`overflow-hidden`, which clamped height to the viewport and clipped anything
below the fold without offering a scrollbar.

## Design
The page is a flex column whose single content child holds the heading,
progress bar, and the carousel card. The outer wrapper now scrolls vertically
(`overflow-y-auto`) instead of clipping, and the inner content column drops its
`overflow-hidden` so it sizes to its content and lets the wrapper scroll. The
short (non-overflowing) case is unchanged: `flex-1` still fills the viewport and
the content stays centered.

## Migration
None.
