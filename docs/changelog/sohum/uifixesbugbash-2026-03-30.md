# UI Fixes — Bug Bash

## Summary

Fixes an outer viewport scroll on the deployed agent detail page (Monitor / Deployments tabs). At 100% zoom the page was adding a document-level scrollbar because the layout's `min-h-screen` container doesn't give a definite height to the flex chain, so the inner scroll containers couldn't properly bound their content.

## Design

`ActiveDetailView` manages its own internal scroll via a `flex: 1; min-height: 0; overflow-y: auto` content viewport. The outer `Layout` uses `min-h-screen` (needed for other pages that rely on document scroll), so the fix is applied at the component level: while `ActiveDetailView` is mounted, `document.documentElement.style.overflow` is set to `hidden`, preventing the browser from adding an outer scrollbar. It is restored on unmount so navigating away returns normal document scroll behavior.

## Migration

No action required.
