# Fix heart button hover flash on blueprint pages

## Summary

Clicking the heart button on a blueprint detail page caused the hover style to flash 2-3 times. The `HeartButton` and `ShareButton` components were defined inside the `BlueprintDetailBreadcrumb` body, so each parent re-render created new function references. React treated these as different component types and unmounted/remounted the DOM nodes, resetting the browser's `:hover` state each time.

## Design

Lifted `HeartButton` and `ShareButton` to module-level components. They now receive data via props instead of closing over parent state. React reconciles them in place across re-renders, preserving DOM identity and hover state.

## Migration

No migration required.
