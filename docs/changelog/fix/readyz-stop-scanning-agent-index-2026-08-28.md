# Readiness stops scanning the agent index

## Summary

The readiness probe queried the database on every check, and the query it ran
fanned out one round trip per registered agent. As database latency rose, the
probe crossed its one-second budget, all three astro-server replicas failed
readiness inside the same period, and the Service was left with no endpoints.
Requests to `/auth/login` and `/auth/callback` returned 503 while the pods
themselves were running and healthy.

Measured on a live replica during the incident: `/livez` returned in under a
millisecond while `/readyz` took 0.81s, 3.00s, and 3.70s on the same pod. The
whole difference was the query.

## Design

`Readyz` no longer reaches the database. Readiness answers "should this replica
receive traffic", and every replica shares one database instance, so gating on
it converts a slow database into zero endpoints. Startup already refuses to run
without the database (`main.go` calls `db.Ping` and exits on failure), so a live
process is a connected one. That covers the case readiness was reaching for.

`Healthz` keeps a database signal, because it exists to report detail and
Kubernetes does not consult it. It now calls a new `agentindex.PingContext`
under a two-second timeout instead of `List`.

`List` is deleted. Both of its callers were probes, so removing them left a
production method that only a test exercised. `e2e/archive_test.go` loses its
cross-account assertion and keeps the account-scoped and public-listing ones,
which cover the same archive semantics on surfaces that ship
(`ListForAccount`, `ListPublicAgents`).

The fan-out is worth naming, because it is what made the probe fragile rather
than merely wasteful. `List` selects every non-archived agent, then issues one
`agent_versions` query per agent, loading `spec_json`, `readme`, and
`agent_card_json` for every version and unmarshalling each spec into a
`map[string]any`. Both callers discarded the result. On a 118-agent dataset that
is 119 sequential round trips and roughly 900 kB per probe, every ten seconds,
per replica. The cost scales with the agent table, so the probe multiplied
database latency by the agent count: a 1ms database gave a 119ms probe, and a
30ms database gave 3.7s.

## Migration

None. The probe endpoints and their response codes are unchanged.

Readiness no longer reflects database health. A new `internal/dbhealth` monitor
takes that signal instead: it pings on a 15 second interval and logs
`database: unreachable` and `database: reachable again` on transitions only.
Alert on those log lines rather than on pod readiness. `/healthz` still reports
the same state on demand.

Worth noting for whoever picks up the alerting: readiness was not reaching
operators anyway. `DeploymentReplicaMismatch` is the rule that turns NotReady
into a page, and it has been `Normal (NoData)` since 2026-06-10.
`PodOOMKilled` and `PodCrashLooping` were also silent through this. Those are
separate from this change and still need fixing.
