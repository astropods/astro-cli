# Traces as a first-class agent tab

## Summary

Move trace inspection out of Monitor into a dedicated Traces tab so operational metrics and request-level debugging have distinct workspaces.

## Issues addressed

- Fixes [#1607](https://github.com/astropods/astro/issues/1607) by applying trace search and user filtering to the complete selected time window before pagination, so older traces do not need to be loaded into the browser before they can be found.
- Fixes [#1679](https://github.com/astropods/astro/issues/1679) by giving the agent-detail identity, navigation, and runtime actions non-overlapping responsive regions so every control remains clickable through the desktop-to-mobile transition.

## Design

The Traces tab owns trace time ranges, loading, deep links, table exploration, and the existing detail panel. The existing table remains the foundation, retaining its global name, ID, and user search; server-aware trace counts; and incremental loading. The default newest-first view fetches 100 traces initially, then exposes explicit Show more and Show less controls for paging and collapsing the table. A searchable user column filter and click-to-order Date, Latency, and Cost headers extend that foundation. The user filter comes from a grouped server query, so its identities and counts cover the complete selected window independently of loaded table pages. The list omits trace status because Langfuse does not provide a trustworthy trace-level value without loading observations.

Search, user filters, and ordering are part of the traces API contract. Timestamp ordering and exact user filters remain natively paginated. Missing-user and raw name/ID/user searches use Langfuse's structured filters; enriched identity search resolves the bounded user facet set and queries only matching user IDs. Langfuse does not support latency/cost ordering, so those criteria sort at most 1,000 filtered candidates and the response explicitly marks partial results when the cap is reached. Criteria results are cached for five minutes, and shared loads outlive an individual viewer request within a fixed timeout. The list contract contains only table metadata; input/output remain arbitrary detail payloads and are loaded from the trace detail endpoint. The client only owns control state and renders server-provided rows. Previous/next detail navigation uses that same ordered result. Trace links resolve directly to this tab, including redirects from legacy Monitor trace query links and `#traces` bookmarks.

Trace details retain the existing panel content and use a local fixed-width shell on larger screens. The panel can expand over the page and automatically switches to an overlay below the responsive threshold; resizing is intentionally left for a future shared-panel refactor.

The shared agent-detail top bar uses a three-region responsive grid for identity, tab navigation, and runtime actions. Each region has an explicit shrink boundary, preventing long agent names from covering adjacent controls. Before the full tab strip can crowd the runtime actions, navigation switches to the design-system Select control. Below the mobile threshold the hidden runtime actions leave a two-column identity/navigation layout, while deployment actions remain available from the identity menu.

Agent cards now open Deployments by default, matching the builder workflow and the existing default deployment route.

Evaluation concepts are intentionally excluded: the trace table has no verdict column, judgment state, dataset integration, or eval-specific detail view.

## Migration

No migration is required. Existing generated trace links now point to the dedicated Traces tab.
