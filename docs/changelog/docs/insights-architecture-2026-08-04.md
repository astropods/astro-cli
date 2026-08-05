# Durable Postgres rollup store for Insights

## Summary

Insights had no durable aggregate store. Every six hours a River worker re-aggregated 90 days of Langfuse data from scratch per account and stored the *rendered page* as JSON in Redis. Nothing accumulated: the work done at 06:00 was thrown away and redone at 12:00. Freshness was therefore bounded by recompute cost — the page's "results may take up to 6 hours" note was the upstream throttle leaking into the product — and a cold cache meant zeros plus a warning banner.

This adds a daily-grain fact table in Postgres as the system of record for usage aggregates. A completed day is rolled up once and never recomputed. The rollup-backed read path ships as `GET /api/v2/accounts/:account/insights` alongside the untouched v1 endpoint, with a client experiment to switch between them so the two can be compared on the same account before anything is retired.

Also documented: `docs/03-architecture/insights.md` (authoritative description of the shipped system) and `docs/01-spec/insights-rollup-spec.md` (this design).

## Design

### Grain

The design was settled by reading Langfuse's query engine at our deployed tag rather than inferring from our own call sites — one of which carried a stale comment that sent the first draft down the wrong path. Three findings shaped it:

- **Grouping by `tags` does not fan out.** Langfuse emits `arrayJoin` only for dimensions declared `explodeArray`; `tags` is a plain `string[]` without it, so grouping is by the whole array and sums are exact. One `traces`-view query grouped by `[tags, userId]` therefore yields cost, tokens and requests per `(deployment, actor)` — no per-deployment fan-out.
- **Model cannot join that grain.** `providedModelName` exists only on the `observations` view, whose cost does not reconcile with `traces` (already why the per-user chart reads `traces`).
- **Latency percentiles cannot be stored.** A `histogram` aggregation exists but compiles to ClickHouse's *adaptive* `histogram(bins)(x)`, whose boundaries derive from each query's own data, so bins cannot merge across days — the same defect as storing a scalar p95.

The result is two grains in one table, discriminated by a `grain` column:

```
grain = 'usage'  →  (account_id, day, source, deployment_id, actor_kind, actor_key)
grain = 'model'  →  (account_id, day, source, model)
```

`usage` is the measure grain and serves every surface; `model` exists only for the Models view, which stays on its current endpoint for now. Because the measure grain carries both dimensions, the `used_by` and `agents_used` chips fall out of the same rows as the totals, with no separate attribution concept.

Both grains describe the same spend, so summing across them double-counts. That is enforced structurally rather than by convention: `grain` leads the primary key, every store query builder takes a grain argument whose zero value is invalid, and CHECK constraints reject rows that populate the wrong dimensions for their grain.

`actor_key` holds the bare stable id with `actor_kind` as its namespace — members store the WorkOS user id, Slack actors the bare Slack user id. The Slack team is deliberately excluded because it comes from the directory, which is read-time enrichment. The payoff is that a member's id is the same key whether it arrived from a trace or from resolving a dev-tool `user.email`, so agent and dev-tool spend merge by plain aggregation instead of the 252-line fold.

### Roll-up

A daily River job rolls each account's completed days. Writes are a full replace per `(account, day, source, grain)` inside a transaction, which makes reruns and overlapping ticks converge without merge semantics. A three-day trailing re-roll absorbs late-arriving traces, and the watermark advances only after every day behind it commits — a failure holds it in place and is recorded, so a stall is visible rather than a silently stale cache entry.

Facts are folded by dimension tuple before insert, and that fold is load-bearing: because Langfuse groups by the whole tag array, one deployment legitimately arrives as several groups (`[deployment:x]` and `[deployment:x, env:prod]`). Summing them is correct, inserting both violates the primary key, and `ON CONFLICT` cannot help because Postgres refuses to let one statement touch the same row twice.

Steady-state upstream cost per account drops from `N + 4` Langfuse queries over a 90-day window four times daily to **two Langfuse queries plus two VictoriaMetrics queries per day**, flat in deployment count.

No tag filter is applied, so archived and deleted deployments are stored and the read path decides visibility. This is what lets usage history outlive the agent that produced it.

### Serving

The v2 handler reuses the existing view-model assembly rather than reimplementing it. The wire contract is frozen and that assembly is already correct and covered by the client's tests, so the only variable that changes is where the facts come from — which is exactly what the comparison is meant to test. A rewritten assembly would have confounded "are the rollups right?" with "is the new view model right?".

Consequently sort, filter and pagination still happen in Go on this path. The pushdown aggregates are already written (`cost_pct` via window function, `ORDER BY/LIMIT/OFFSET`, the system row pinned in SQL, non-admin per-developer visibility as a `WHERE` clause) and can be wired in once the facts are trusted, verified against an already-trusted v2 rather than against v1.

The agents table is `deployments LEFT JOIN facts UNION` orphaned facts. Both halves matter: a fact table has rows only where something happened, so the LEFT JOIN keeps deployed-but-idle agents on the page with their `not_instrumented` marker, and the UNION keeps spend visible after its deployment is deleted.

Identity decoration stays at read time, unchanged, so Slack display names and member profiles are never stale.

### Deliberate divergences from v1

Four differences are expected when comparing the two endpoints, and are not defects:

- v2 currently reads about one day low — the live overlay for today's partial day is not wired yet, so v2 serves complete days only.
- Change % is present in v2 when a dev-tool source is folded in; v1 discards it because it is computed from agent spend alone.
- The People spend chart folds dev-tool spend in v2 and does not in v1, so it finally agrees with the table beneath it.
- On accounts past the v1 read path's 100-deployment fan-out cap, v2's agent table is more complete. **v1 is the wrong side** — those accounts have had silently incomplete tables.

### Deferred decisions

Partitioning was dropped for now. The schema is applied by declarative `atlas schema apply`, which would read job-created rolling partitions as drift and plan to drop them; at the projected size (well under a MB/month for a large account) retention is a bounded `DELETE`. Converting later needs a migration either way, so nothing is foreclosed.

`latency_buckets` is forward-declared and left empty for a future ingest-side producer that can emit fixed boundaries. Until then p95 continues to come from the existing whole-period Langfuse query, unchanged.

## Migration

No user action is required, and nothing is user-visible by default.

The two new tables are additive and applied through the Atlas SDL like any other schema change, via the manually-dispatched prod migration workflow. **`/api/v2` returns errors until that migration is applied**; v1 is unaffected either way, and the roll-up workers no-op when their producer is not wired.

The new "Faster Insights" experiment under Settings → Experiments defaults to off. Enabling it points the Insights page at v2 for that browser only; disabling it returns to v1 with no state to unwind.
