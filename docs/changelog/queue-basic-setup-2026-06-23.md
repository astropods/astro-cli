## Summary

Adds the frontend for building and reviewing eval datasets from production traces. The Evals tab now has two modes: a review queue for judging traces as dataset examples, and a dataset table for inspecting the examples already added.

## Design

The page uses a shared eval card shell so the Review queue and Dataset views have the same header treatment, dataset name placement, and Pretty/Raw content toggle. Eval data fetching lives in a dedicated query module with centralized query keys for the summary, dataset items, and review queue.

The Review queue is the default view. It loads traces from the backend queue endpoint, shows a selectable trace list, renders the selected trace with the existing trace content section, and lets reviewers submit Good, Bad, or Neutral verdicts. Successful judgments remove the trace from the local queue immediately and refresh the dataset summary and item queries, while leaving the queue cache stable so the selected item does not jump during refetch.

The initial preview path intentionally loads one backend queue page while trace flow and judgment behavior are validated. Follow-up pagination work should convert the review queue hook to an infinite query, consume the backend `next_offset` and `end_time` fields, and extend the visible queue when the local list runs low.

The Dataset view shows the reusable grade badge/sidebar, Good/Bad filter chips, and an expandable table of dataset items. Verdict filters are sent to the server so pagination remains consistent when a filter is active, and the table consumes the returned cursor for filtered "Show more" behavior.

Content rendering is shared through a parser that normalizes JSON, Markdown/text, empty values, and raw display. This keeps trace details and dataset rows visually consistent while still supporting full raw payload inspection.

## Migration

No user action is required. The Evals tab continues to depend on the eval dataset backend endpoints for summary data, dataset items, review queue traces, and judgment submission.
