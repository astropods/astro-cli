# Knowledge store UI improvements

## Summary

A round of polish to the Knowledge Stores list and detail pages, plus a fix to surface real server error messages in store creation and connection flows.

## Design

### Error message surfacing

API errors were thrown as plain objects, causing `instanceof Error` checks in mutation error handlers to always fall through to hardcoded fallbacks ("Failed to connect store"). `ApiRequestError extends Error` wraps the API error payload into a proper Error subclass, so `mutation.error.message` now returns the server's `error_description`. The existing `.status` property is preserved for 401/404/409 checks elsewhere.

Error display in `NewKnowledgeStore` and `ConnectKnowledgeStoreDialog` was updated from plain `<p className="text-destructive">` to `<ErrorPanel variant="inline">`, matching the inline error pattern used on the Deploy page.

### Knowledge stores table

The table now uses the shared `Table` component with a `rounded-sm` bordered container, a `bg-muted` header row, `text-body` sizing with `leading-5` line-height, and `text-muted-foreground` header text. The Mode column uses the `Tag` component (`blue` for Managed, `default` for External). The actions menu uses a vertical kebab icon with a trash icon for delete. The status badge no longer renders an indicator dot.

### Knowledge store detail page

The tab system is restored with Overview and Logs tabs. Cards use `bg-white` backgrounds; the page background is `bg-surface` to match the global nav. The Logs tab uses `useKnowledgeLogs` (polling) with `LogViewer` and a time range selector.

### Copy

"Could not" replaced with "Couldn't" in user-facing error strings across knowledge store, deployment settings, and deployment history views.

### Learn more button

The Learn more button on the Knowledge Stores list links to the external docs and opens in a new tab.

## Migration

No changes required.
