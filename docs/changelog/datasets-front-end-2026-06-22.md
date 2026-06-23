## Summary

Replaces the stub Evals tab with a dataset view — bordered card with filter chips, a grade sidebar, and an expandable items table.

## Design

The Evals page on `/dataset` renders a single bordered card: dataset name + Good/Bad filter chips + Pretty/Raw toggle in the header; baseline grade sidebar on the left (letter, headline, "X% to {next_grade}" bar, composition); items table on the right (verdict pill, 2-line input/output, judged-by). Rows expand inline to show the full input + expected output side-by-side, with a tooltip on "Expected output" and a Good/Bad example label.

The table uses TanStack Query and the shared table primitives. Each verdict filter has its own query key and consumes the server-provided `next_cursor`, so "Show more" loads more rows for the active filter instead of filtering only the already-loaded page.

The Queue mode toggle, review queue, judgment submission, and reasoning chips land in the follow-up PR.

## Migration

No user action is required. The Evals tab continues to live behind the `evals` experiment flag. This frontend expects the dataset summary and filtered item pagination fields introduced by the companion server PR.
