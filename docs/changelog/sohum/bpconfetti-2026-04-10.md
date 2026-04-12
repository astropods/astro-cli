# Blueprint Confetti — Panel 3 Celebration

## Summary

When a blueprint is successfully created and the flow reaches panel 3 (Review), a confetti burst fires to celebrate the moment.

## Design

`LiveRevealConfetti` was extended to accept an optional `containerRef` prop. When provided, the canvas sizes itself to the container element's dimensions rather than the window — keeping particles clipped within the panel. When omitted, the existing full-screen behavior (used by `LiveRevealOverlay` on deployment) is preserved.

In `NewBlueprint.tsx`, a `reviewPanelRef` is attached to the panel 3 content area, which also gains `relative` and `overflow-hidden` to contain the canvas. `LiveRevealConfetti` mounts when the step renders and fires once.

## Migration

No action required.
