# Faster deployment-by-namespace and agent-version reads

## Summary

Two read paths did far more work than their callers needed.

`deployments.namespace` had no index. Every by-namespace lookup was a full
sequential scan of `deployments` followed by a sort, to return one row. The
deployment controller runs that lookup on every reconcile, the observation
evaluator runs it per breaching namespace, and the pod Alerts tab runs it per
request. On a 150k-row table the scan reads 4,085 buffers and recruits two
parallel workers to return a single row (verify with
`EXPLAIN (ANALYZE, BUFFERS) SELECT ... FROM deployments WHERE namespace = $1
ORDER BY deployed_at DESC LIMIT 1`).

`agentindex.Index.Get` loaded every published build of an agent, each with its
full `spec_json`, `readme`, and `agent_card_json`, and unmarshalled every spec.
Five of its six callers never read `Versions`: they want visibility, a name, or
avatar colors. Two of them discard the result entirely and use it as an
existence check. For an agent with 200 builds that is roughly 5.8 MB shipped
from Postgres where 2.9 KB was wanted.

`ValidateLineage` had the same shape in miniature. It asked whether one
`(account, agent, build)` row exists by loading that row's spec, readme, and
card, then unmarshalling the spec and throwing it away. It runs on the deploy
path and during template prefill.

## Design

**One index serves both namespace lookups.**

```sql
CREATE INDEX idx_deployments_namespace_latest ON public.deployments(namespace, deployed_at DESC);
```

The trailing `deployed_at DESC` is the point: both queries are
`WHERE namespace = $1 ORDER BY deployed_at DESC LIMIT 1`, so the index returns
rows already in the requested order and the executor stops at the first one.
Ordering on `namespace` alone would still leave a sort node. The plan drops from
a parallel sequential scan plus sort to a 4-buffer index scan, and
`GetDeploymentByNamespace` gets the same treatment for free by filtering
`status <> 'undeployed'` while walking the index in order.

A namespace is derived from the deployment id (`astro-<compact-id>-0`), so it is
effectively unique. That makes the index maximally selective and makes the old
seq scan maximally wasteful.

`TestNamespaceIndexShape` in `deploymentstore` guards this: it reads
`pg_indexes` and fails if the index is gone or no longer leads with
`(namespace, deployed_at DESC)`, which is the property the
`ORDER BY ... LIMIT 1` depends on. It deliberately does not assert the plan.
Which plan wins depends on the rows present and on the statistics at that
moment, and `deployments` is written by tests in other packages running
concurrently, so a plan assertion there fails for reasons that have nothing to
do with the index.

**Reads on `agent_versions` now cost what the caller asked for.**

`Index.Get` returns the `agents` row with `Versions` empty.
`Index.GetWithVersions` is the explicit full read, used only by
`GET /agents/:account/:name`, which renders build history. Splitting them this
way makes the cheap call the default and the expensive one visible at the call
site. The deploy path, the deployment-template path, and the transfer
existence and collision checks all take the cheap one.

`ValidateLineage` is now its own query rather than a wrapper over `GetVersion`:

```sql
SELECT 1 FROM agent_versions
WHERE account_id = $1 AND name = $2 AND build_id = $3
LIMIT 1
```

That is an index-only scan on `agent_versions_pkey`, reads no payload column,
and unmarshals nothing. Its error strings are unchanged (`build not found: …`
for a missing row, `failed to query version: …` for a DB failure), and it still
does not filter on `agents.archived_at`, so a version published before its agent
was archived stays redeployable.

## Migration

Apply `sql/astro-server/schema.sql` before deploying astro-server. Adds one
index on `deployments`, no other schema change and no backfill. The index build
takes an `ACCESS EXCLUSIVE` lock for its duration; at current `deployments` size
that is brief, so run `CREATE INDEX CONCURRENTLY` by hand first only if write
downtime on that table is unacceptable.

No API contract changes. `GET /agents/:account/:name` returns the same body as
before.
