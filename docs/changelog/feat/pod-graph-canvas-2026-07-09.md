# Pod graph: pannable/zoomable canvas

## Summary

The deployment pod graph laid its tiles out at a fixed scale in a clipped box.
That works for a handful of workloads, but a large deployment overflowed the
viewport with no way to reach the off-screen tiles — and on mobile the vertical
stack simply ran off both ends behind the surrounding page chrome. This makes
the graph a proper canvas: pan and zoom on desktop, a natively-scrolling list on
mobile, so an arbitrarily large graph stays navigable.

## Design

- **Transform-based world layer.** The tiles and edges render inside a single
  `translate/scale` layer (`view = { x, y, k }`). Pointer drag pans; wheel pans;
  ⌘/ctrl-wheel and trackpad pinch zoom toward the cursor; a subtle bottom-left
  control offers zoom in/out and fit. All of it lives in `usePanZoom`, which owns
  the view state and gesture handling; `PodGraph` stays a layout component.

- **Fit semantics.** Fit centers the content and scales it to frame everything,
  capped at `1` — a small graph sits at natural size, a large one scales down.
  The graph auto-fits once on first load, then leaves the view alone so runtime
  changes don't yank it around. A viewport resize snaps back to centered/fit
  (instant, forced) so a leftover pan/zoom can never strand the graph off-screen.

- **Pan is bounded.** The content's center is kept within the viewport, so it can
  be pushed at most halfway off any edge rather than scrolled fully out of sight.

- **Spring vs. instant.** Discrete actions (zoom buttons, fit) spring to the new
  view; drag/wheel/pinch apply instantly so they track the input 1:1. A single
  `animate` flag on the view state selects the transition.

- **Mobile is a scroll list, not a canvas.** Below the width breakpoint the graph
  is a normally-scrolling vertical column. Its scroll region is inset into the
  clear area between the top chrome and the bottom deployment panel — measured
  for the panel's real height — so tiles clip out of view under them instead of
  showing through, with a top fade mask matching the sibling agent-detail pages.

- **Stable while covered.** When a panel fully covers the graph (mobile pod
  detail, or expanded history), its layout inputs are frozen, so opening or
  expanding a panel doesn't reflow the hidden graph behind the animation.

## Migration

None. Internal component behavior only.
