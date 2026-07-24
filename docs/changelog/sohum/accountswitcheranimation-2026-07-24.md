## Summary

Account-filtered pages now reveal updated results with the same subtle fade and rise used when navigating between agent-detail tabs. This makes account changes feel continuous without animating persistent page controls.

## Design

A shared content-reveal primitive owns the existing 250ms entrance motion and respects the user's reduced-motion preference. Aggregate resource lists reveal once when their initial non-empty results settle, then replay only when a new account selection produces non-empty results, including transitions to or from All accounts. Empty destinations and later background result-count changes remain static, which prevents cached data or polling updates from reanimating the whole result set.

Insights keeps the previous account view steady while its `keepPreviousData` response is a placeholder, then replays the reveal exactly once when the selected account's response settles. Search input, pagination, and toolbars remain stable throughout these transitions.

Agent-detail views use the same primitive and motion contract, keeping the source interaction and filtered-page behavior aligned.

## Migration

No action required.
