# Filter the agents list by status

## Summary

The agents dashboard had no way to filter by status, so finding the stopped or errored agents in a long list meant scanning every card. The toolbar now offers an All / Active / Stopped / Error control.

## Design

The dashboard already loads the complete multi-account deployment catalog and does its search and sort on the client (`useAgentFilters`), so status filtering lives in the same place rather than as a separate server round-trip. `useAgentFilters` gains a `statusFilter` and applies it alongside the existing text filter, matching against the loose UI status string each summary already carries (`dbStatusToUIStatus` on the server):

- `active` matches `Running`
- `stopped` matches `Stopped`
- `error` matches `error` (failed, suspended, and undeployed all surface as error)

The toolbar renders a segmented All / Active / Stopped / Error control that drives that state. A filter that returns nothing shows the existing "No agents match your filters." empty state with the toolbar intact, and Clear resets the status filter along with search and account filters. The active filter is persisted to local storage (via the shared page-filter store) and restored on the next visit, so a user returning to the dashboard sees the filter they last set.

## Migration

None. The default (All) behavior is unchanged.
