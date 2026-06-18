## Summary

The Insights agents and people tables opened with only five rows visible, which forced most accounts into "Show more" immediately. The default server-requested window is now 25 rows so the first paint matches what operators typically want to scan.

## Design

The client and server defaults stay aligned: `DEFAULT_TABLE_LIMIT` drives the initial `agents_limit` / `people_limit` query params, loader priming, and reset behavior on search, sort, and collapse. The server’s `defaultInsightsTableLimit` matches so callers that omit limits still get a bounded 25-row slice.

`TopSpendersTable` server pagination now accepts `defaultVisibleRows`, `pageSize`, and `showLessLabel` from Insights instead of hardcoding a five-row baseline. That keeps the collapse affordance ("Show top 25") in sync with the fetch limit — without it, expanding to 25 rows would incorrectly offer "Show less" on first load.

"Show more" still grows the limit by 10 rows per click (25 → 35 → 45). The table footer also stays visible when every row is loaded but the user is still expanded past the default window, so collapse remains available after the last page fetch.

## Migration

No action required.
