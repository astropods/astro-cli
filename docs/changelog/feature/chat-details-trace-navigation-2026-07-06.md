## Summary

The chat details panel links users into the Monitor trace table, so Monitor needs stable URLs for opening a specific trace in context. This change makes trace detail state addressable and keeps the selected table row anchored in view.

## Design

Monitor now treats the `trace` query parameter as the source of truth for the trace detail panel. Selecting a row writes the trace ID into the URL, closing the panel removes it, and previous/next navigation updates it as users move through traces.

Trace rows now expose stable DOM anchors derived from trace IDs. When a selected trace is hidden behind the table expansion threshold, the table expands first and then scrolls the selected row into view.

## Migration

No user action is required. Existing Monitor links continue to open the tab normally; links that include `?trace=<id>` now open the trace detail panel directly.
