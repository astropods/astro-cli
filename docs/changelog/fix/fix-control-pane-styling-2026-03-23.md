# Fix: Configure Panel & Header Sticky Behaviour

## Summary

The deployed agent detail page had two layout issues: the agent detail header scrolled away with the page instead of staying visible, and the configure panel was not anchored to the viewport — causing it to scroll away and its footer buttons to be cut off.

## Design

**Sticky agent detail header**
The outer wrapper of `ActiveDetailView` previously had `overflow: hidden` which silently broke `position: sticky` on the agent header by making the wrapper (not the document) the scroll container. Removing all overflow from the wrapper restores the browser's default sticky behaviour against the document scroll.

**Configure panel as sticky sidebar**
The configure panel moves from `position: absolute` (scrolls with the page) to `position: sticky; top: 0; align-self: flex-start; height: 100vh` inside a flex-row layout. The outer div becomes a flex row: a left column holds the agent header + tab content, and the panel is a flex sibling that sticks to the top of the viewport as the user scrolls — always aligned with the Configure button.

`overflowX: clip` is used on the panel wrapper instead of `overflow: hidden`. Both clip the content during the open/close width animation, but `clip` does not create a scroll container, which allows the footer's `position: sticky; bottom: 0` to reference the viewport rather than the wrapper. This guarantees the Redeploy/Discard footer is always visible regardless of scroll position or panel height.

**Footer layout**
The Redeploy and Discard buttons are extracted from inside the scrollable form into a dedicated sticky footer at the shell level. They are laid out side-by-side (`flex gap-2`, equal width) with Discard on the left and Redeploy on the right. The form fields area uses `flex-1` so it grows to fill available space when the panel is taller than the content.

## Migration

No changes required.
