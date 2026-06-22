# Fix blank/offset step in the blueprint create wizard

## Summary

Navigating the blueprint create wizard (e.g. Continue → Set up with GitHub →
Back → Continue, or reloading mid-flow) could leave the step carousel
misaligned: the active step rendered blank or shifted, with a sliver of the
adjacent step peeking in.

## Design

The wizard is a horizontal carousel — a 400%-wide flex track translated by
`translateX` inside a fixed-width viewport. The viewport used `overflow-hidden`,
which still establishes a scroll container. When focus moved to a button in an
off-screen slide (clicking Back/Continue, or focus restoration after reload),
the browser auto-scrolled the viewport horizontally to bring that focused
element into view — leaving `scrollLeft` non-zero and knocking the active slide
out of alignment even though the `translateX` was correct.

Switching the viewport to `overflow-clip` clips identically but creates **no**
scroll container, so focus can no longer scroll it; `scrollLeft` stays 0 and the
active slide always aligns to the viewport.

## Migration

None.
