## Summary

Fixes the review queue trace inspection flow so an already-open trace detail panel follows the queue selection. Users can open a trace once, then click through queue items without seeing stale panel content from the first trace.

Also fixes review queue pagination after load-more pages. The server now returns only page-aligned continuation offsets, preventing the UI from offering a follow-up load that the API rejects as an invalid offset.

## Design

The review queue still requires an explicit action to open the trace detail panel. Once the panel is open, the dataset page subscribes to the queue's active item and mirrors that trace into the existing panel. Explicit queue transitions update the panel directly: row selection, judgment success, and undo success all pass the newly selected queue item through the same trace-entry adapter.

When the active item is judged and no next queue item exists, the dataset page closes the trace detail panel instead of leaving the removed trace visible. If TanStack Query updates the cached queue data in the background and the selected item disappears, a guarded selection sync updates the panel only when the mirrored trace id actually changes underneath the user.

The sync is scoped to the dataset page and only runs while the panel is already visible, so selecting queue rows does not unexpectedly open the panel for users who are only reviewing inline. Opening a trace now also pins that queue item as the selected item, so cache reorders do not swap the panel away from the trace the user explicitly opened.

Review queue continuation now follows the upstream page model. When more pages are available, the next offset advances by the requested page size rather than by the number of traces returned on the current page. Partial pages and empty pages therefore cannot generate offsets like `57` for a `50` item page, and the API suppresses continuation when the upstream reports that the current page is the last page.

## Migration

No migration required.
