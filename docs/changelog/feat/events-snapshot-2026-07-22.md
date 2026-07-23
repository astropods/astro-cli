# Deployment events are controller-observed, not read live per request

## Summary

`GET /deployments/:id/events` listed Kubernetes events from the cluster on every
request (with a short cache), so each viewer's poll could hit the K8s API. It
also returned noisy, badly-ordered data: a `List(Limit: 200)` is not time-sorted,
so busy namespaces returned an arbitrary page dominated by old events from
previous deploys, and ordering relied on the deprecated `LastTimestamp` (zero on
modern events), burying actively-firing events.

## Design

Events now follow the same observe→persist→read model as runtime and status. The
deployment controller, on each sync, lists the namespace's events once, scopes
them to the deployment's current objects, humanizes them, sorts newest-first, and
stores them on the existing runtime snapshot (`deployment_runtime_status`) as an
`events` array. The endpoint deserializes that document — no per-request K8s call,
no cache.

Key points:

- **No new table** — events ride the runtime snapshot document (one row per
  deployment), written in the same `persistRuntimeSnapshot` path, best-effort so a
  failed events List never blocks the runtime view.
- **Correct freshness** — sync runs on any watched-object change (scheduling,
  image-pull, crash, restart all mutate a Pod) with a 2-minute resync floor, so
  events appear near-real-time with a bounded ceiling.
- **Correct ordering** — last-seen resolves `Series.LastObservedTime` (repeats)
  before `LastTimestamp`/`EventTime`/creation, so an actively-firing event sorts
  newest.
- **Less noise** — events are scoped to the deployment's current workloads/pods,
  dropping events from prior deploys and deleted pods.
- **Plain language** — reason→copy humanization moved server-side into the
  controller; the API carries the humanized title/guidance/severity.

## Migration

None. The response shape is unchanged; the source moved from live K8s to the
persisted snapshot.
