# feat: search the trace list

## Summary

The traces table on the Monitor tab could only be filtered by status, so there was no way to find a specific trace by keyword, and the list was fetched with a low cap. This adds a search box and raises the fetch limit so many more traces are searchable. Closes #1530.

## Design

- A search input (reusing the shared `FilterInput`) sits on the left of the filter bar, with the status filter and the trace count grouped on the right. It matches the query case-insensitively against each trace's span name, trace id, user id, the displayed user name (`display_name`/`username`), and status label, and it combines with the status filter. The span name and the displayed user name are searchable even though the raw id is what the row stores, so a search by what is actually on screen returns results.
- The Monitor page now requests up to 500 traces (was 100), so search covers a much larger window. The existing deep-link hydration still handles a trace that falls outside even that window.

Server-side pagination beyond the fetched window is a natural follow-up; this change filters client-side over the loaded set, which covers the reported need (search plus a higher ceiling than 100).

## Migration

None. This is additive.
