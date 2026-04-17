## Summary

Log search previously filtered out non-matching rows, which made it hard to understand matching lines in context. Search now highlights matches inline while keeping all rows visible, following the same model as browser find (Cmd+F).

## Design

Search is decoupled from the level filter hook (`useLogFiltering` no longer takes a `search` parameter — it only manages error/warning filters). The `LogViewer` component holds the search term, defers it via `useDeferredValue` to keep typing responsive, and applies two visual effects per row:

- **Matching rows**: each occurrence of the search term in the message is wrapped in a `<mark>` with a yellow background (`bg-yellow-300/60`), preserving the original text color.

The log list was also virtualized in a prior commit, so highlight computation only runs on the visible window of rows — not the full dataset.

## Migration

No action required.
