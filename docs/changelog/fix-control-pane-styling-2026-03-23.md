# fix-control-pane-styling — Configure Panel & Header Sticky Behaviour

## Summary

The deployed agent detail page had two layout issues: the agent detail header scrolled away instead of staying visible, and the configure panel scrolled away with the page, cutting off the footer buttons.

## Design

**Sticky agent detail header**: The outer wrapper of `ActiveDetailView` previously had `overflow: hidden`, which silently broke `position: sticky` by making the wrapper (not the document) the scroll container. Removing overflow from the wrapper restores sticky behaviour against the document scroll.

**Configure panel as sticky sidebar**: The layout shifts from a flex-column to a flex-row. The left column holds the agent header and tab content; the configure panel is a flex sibling with `position: sticky; top: 0; align-self: flex-start; height: 100vh`. This keeps the panel top permanently aligned with the Configure button as the user scrolls.

`overflowX: clip` is used on the panel wrapper instead of `overflow: hidden`. Both clip content during the open/close width animation, but `clip` does not create a scroll container — so the footer's `position: sticky; bottom: 0` references the viewport rather than the wrapper, guaranteeing the footer is always visible.

**Footer layout**: The Redeploy and Discard buttons are moved out of the scrollable form into a dedicated sticky footer at the shell level. They are laid out side-by-side (equal width, Discard left / Redeploy right). The form fields area uses `flex-1` to grow into available space.

## Migration

No changes required.
