# Fix faster load time

## Summary

The agents dashboard was making N+1 Langfuse requests on every load: one per
deployment card for the request count, one per card for the last-active
timestamp, plus a redundant account-level summary fetch caused by `staleTime:
0` defeating the SSR loader's cache priming.

## Design

**`staleTime` fix** — `useAccountObservabilitySummary` had `staleTime: 0`,
which caused TanStack Query to treat loader-primed data as immediately stale
and fire a background refetch on every mount. Raised to 5 minutes to match the
other activity query options.

**Bulk deployment summaries endpoint** — replaced the N parallel
`/deployments/:id/observability/summary` fan-out from the agent cards with a
single account-scoped endpoint:

```
GET /api/v1/accounts/:account/observability/deployment-summaries
```

The endpoint is registered on the `accountMember` router group so auth and
membership are handled by middleware. It fetches all active deployments for the
account, fans out Langfuse trace queries in parallel via errgroup, and returns
a map of `{ [deploymentId]: { total_traces, last_trace_at } }`. Per-deployment
failures log a warning and are omitted from the response rather than failing
the whole request. Context cancellation (client navigated away) returns
silently without logging.

The response shape is intentionally narrow — only the two fields the agent
cards consume — rather than reusing the full `ObservabilitySummaryResponse`.

**Client** — `useObservabilitySummaries` is rewritten from a `useQueries`
fan-out to a single `useQuery` against the new endpoint with a 5-minute
`staleTime`. The per-card `useObservabilitySummary` and `useObservabilityTraces`
hooks are removed entirely from `AgentCard`; `requests` and `lastActive` are
now passed as props from the parent. `DeployedAgentsSection` builds both maps
from the bulk response and passes values down, eliminating all per-card fetches.

The profile page's `AgentsTab` also consumes the same bulk endpoint, replacing
the previously hardcoded `requests={0}` and `lastActive="—"` values.

## Migration

No action required.
