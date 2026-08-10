# Outbound domain coverage in Queen

## Summary

Brand icons are authored by hand, and until now nothing told us which ones were worth authoring next — the set grew from guesses about what agents call rather than from what they actually call. `queen <env> icons domains` answers that from real traffic: every destination agents reach, fleet-wide, with how often it is hit and how many deployments hit it.

## Design

`ListOutboundDomains` on AdminService runs two PromQL queries concurrently and joins them on `server_address`. Dropping the `k8s_namespace_name` and `service_name` filters that the per-deployment network endpoints pin is what makes it fleet-wide, though the cluster filter stays and spans every registered cluster rather than just the primary. The second number comes from a `count by (server_address, k8s_namespace_name, service_name)` — that pair is what Beyla emits per deployment — deduped into a set per domain in Go. The dedupe has to happen after the eTLD+1 fold: collapsing in PromQL would discard the deployment identity needed to count a vendor's multi-host caller once. Both numbers matter and neither alone is sufficient — volume alone promotes one chatty agent's private endpoint, breadth alone misses a single high-traffic vendor. The response carries no account or deployment identity, because neither informs whether an icon is worth drawing.

It is on-demand only: no job kind, no schedule. A fleet-wide aggregation over a month of high-cardinality series is expensive, and the consumer is a human deciding what to draw next.

Deciding which domains are *unresolved* happens in `packages/astro-brand-icons/scripts/unresolved.ts`, not on the server. The rules read that package's manifest — including the parent-domain walk that lets `myapp.vercel.app` resolve to Vercel — and a second implementation in Go is precisely how two alias tables drifted apart previously. The server does the extraction, the package does the resolution, and `queen icons domains --json` is the seam:

```
queen prod icons domains --json > /tmp/domains.json
bun run --cwd packages/astro-brand-icons unresolved /tmp/domains.json
```

Reducing a hostname to its domain now lives in `internal/peerdomain`, shared by the flows endpoint and this aggregation so the two cannot disagree about what a domain is. Relatedly, `promquery.Client` no longer pins a 10s `http.Client` timeout — that was a ceiling no caller could raise, and a month-wide aggregation cannot live within it. The 10s cap still applies to `Query`/`QueryRange`, so every existing caller is unchanged; a caller that genuinely needs longer opts in per-call via `QueryWithTimeout`, which can only tighten the caller's own deadline. Making the timeout implicit instead — honouring whatever deadline the context already carried — would have silently widened the per-request cap for the insights rollup and the messaging-count worker, which is why it is explicit.

## Migration

Nothing required. The RPC reports `FailedPrecondition` when `PROMETHEUS_URL` is unset, matching how the per-deployment network endpoints behave without metrics.
