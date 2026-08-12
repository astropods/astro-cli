# Queen cluster detail: surface all fields, add URL reachability checks

## Summary

The astro-queen cluster detail view rendered only 4 of the ~13 fields `RegisteredCluster`
actually carries — `updated_at` and the whole Langfuse/Loki/Prometheus/tenant-router
netpol config were collected by the edit form but never shown, so operators had no way
to see or verify that config without opening the edit dialog. Separately, "Check health"
only validated the K8s API endpoint; the optional observability/netpol URLs had no
health signal at all — a stale or unreachable Loki/Prometheus/tenant-router URL would go
unnoticed until it broke something in production.

## Design

**Missing fields.** `ClusterDetail` already computed `deployFields` (via
`clusterDeployFromCluster`) for the ingress-completeness check, but only 2 of its 9
fields reached JSX. Added `Updated` to the summary grid and a new "Langfuse / netpol"
card that mirrors the edit form's own field grouping (`ClusterDeployFieldset`).

**URL reachability.** Extended the existing `CheckClusterHealth` RPC rather than adding
a new action, so there's still one health action per cluster. `CheckClusterHealthResponse`
gained a `url_checks` field (`UrlReachability{label, url, reachable, error}`), populated
by concurrently TCP-dialing each non-empty Langfuse/Loki/Prometheus/tenant-router URL
with a 3s timeout. A plain TCP dial (not an HTTP GET) avoids false negatives from
endpoints that require auth and would otherwise 401/403. Results are transient — they
live only in the mutation response for the session, not persisted — and render as a
small reachable/unreachable badge next to the corresponding field after "Check health"
runs.

`astro-proto`'s admin API is hand-maintained (no buf codegen — see
`admin.pb.go`'s header), so the proto and the Go struct were updated in lockstep.

## Migration

None. Backend and frontend deploy together; no schema or config changes.
