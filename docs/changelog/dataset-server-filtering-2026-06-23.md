## Summary

Adds the backend support needed for the eval dataset table to render graded summary state and filter judged dataset items by verdict.

## Design

The dataset summary keeps its counts in Astro and computes the letter grade server-side using the A-F scale. The response exposes `next_grade` and `next_grade_progress` so clients can show progress within the current grade band without duplicating grade math.

Dataset items remain sourced from Langfuse. Langfuse does not currently support filtering dataset items by metadata, so `GET /dataset/items` accepts `verdict=good|bad` and scans Langfuse pages until it can return the requested filtered slice. This keeps the frontend table filter honest: when a user filters to Good or Bad, "Show more" loads the next matching rows instead of loading an unfiltered Langfuse page and hoping it contains more matching items.

Filtered pagination uses an opaque `next_cursor` so follow-up requests resume scanning where the previous response stopped instead of refetching every prior Langfuse page. The cursor is a server-owned token that carries the cursor format version, Langfuse dataset name, requested verdict, page size, raw Langfuse page, raw index within that page, and the number of matching rows already emitted. Clients only pass it back as `cursor`; they do not inspect or construct it.

Filtered totals come from Astro's local good/bad counters in the normal in-sync case. If the scan exhausts early because Langfuse has fewer reachable matching items than the local counter says, the response clamps `total_items` and `total_pages` to the reachable matched count so the client does not render unreachable pages. The scan also has a hard page bound based on dataset size plus slack to avoid unbounded upstream requests if Langfuse pagination metadata is malformed.

Unfiltered dataset item listing continues to use Langfuse page pagination.

## Migration

No user action is required. Existing eval datasets keep their local counts; the new response fields are derived from those counts at read time.
