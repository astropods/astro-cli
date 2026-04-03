## Summary

Reverts layout changes introduced in prior commits that modified the root `Layout` component.

## Design

Removes the `ErrorBoundary` wrapper and inner scroll `div` that were wrapping `<Outlet />`, and restores `min-h-screen` (from `h-screen`) on the root container. The layout is back to its pre-change state.

## Migration

No action required.
