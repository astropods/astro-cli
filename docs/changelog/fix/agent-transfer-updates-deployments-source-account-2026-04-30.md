# Agent Transfer Repoints Cross-Account Deployments

## Summary

Transferring an agent between accounts (`POST /api/v1/agents/:account/:name/transfer`) rewrites the `agents` and `agent_versions` rows but leaves every existing deployment of that agent pointing `source_account_id` at the old publisher. The upgrade signal and one-click redeploy are both computed off `source_account_id`, so a stale value routes upgrade prompts to an account that no longer owns the blueprint — and that account could later publish a same-named build of its own.

The transfer endpoint has no UI surface (callable only via direct API with `agents:write` + dual-account membership) and the affected population is a handful of deployments across a handful of accounts. The fix lands now to keep the lineage invariant clean before PR4's FK makes inconsistent lineage a hard error.

## Security

A stale `source_account_id` is a lineage-spoofing surface: the publisher named by that column drives the in-product upgrade flow, so anyone with write access to that account can direct existing deployments at a build of their choosing. Two ways the lineage drifts in the current code:

1. **Source account is still alive after the transfer.** Transfer rewrites the `agents` row's `account_id`, taking the `name_reserved` ratchet with it, so source's `(account_id, name)` slot is now empty and accepts a fresh `Register`. The original owner can re-register the same name, push a build, and every cross-account deployment of the transferred agent sees that build as an upgrade because `source_account_id` still points at source.
2. **Source account name was reclaimed.** Source is deleted; `source_account_id` becomes `NULL` via `ON DELETE SET NULL`, and `resolveSourceAccountName` falls through to `deployment_spec_json.source.account` (a free-form name). Account names are reclaimable, so a new account taking that name resolves as the lineage publisher. PR5 in this plan closes this second path at the read layer; PR1 closes the first by ensuring `source_account_id` always points at the current publisher.

Both paths slip past the existing `prepareDeployment` validator because it asks "does this lineage own this build?" against the lineage the bug already rewrote — the new owner of the now-stale `source_account_id` legitimately owns whatever build they push.

Practical exposure today is bounded by current usage (single-digit cross-account transfers performed against the live API), and the data sweep below repairs them in place. We are fixing it before that surface grows and before the FK turns the inconsistency into a deploy-time error.

## Design

### Schema: `ON UPDATE CASCADE` on the agent FKs

While wiring up integration coverage we discovered the existing `Transfer` flow had been broken end-to-end since the FKs on `agent_versions(account_id, name) → agents(account_id, name)` and `agent_hearts(account_id, agent_name) → agents(account_id, name)` were declared. The constraints are checked immediately, so the very first statement (`UPDATE agents SET account_id = $target ...`) orphans every child row for the duration of that statement and the FK aborts the transaction. No existing test ran `Transfer` against a real Postgres — the unit tests use `sqlmock` (which doesn't enforce FKs) and there was no e2e coverage — so the failure was latent until PR1's new integration test surfaced it.

Both FKs are now declared `ON UPDATE CASCADE ON DELETE CASCADE`. Re-keying the `agents` row cascades the move to `agent_versions` and `agent_hearts` atomically inside the statement, so there is never a window where children dangle. This also closes the symmetric `agent_hearts` orphaning path that Transfer never wrote to (and would have hit at commit time the moment any user hearted a transferred agent).

### Transactional fix on the write path

`agentindex.Transfer` runs three statements inside a single transaction:

1. `UPDATE agents SET account_id = $target …` — the cascade above moves `agent_versions` and `agent_hearts` along with it.
2. `UPDATE agent_versions SET updated_at = $now WHERE account_id = $target AND name = $name` — audit-bumps the version timestamps. Cascade only touches FK columns, so this statement explicitly records the transfer event. The `WHERE` filters on the *target* account because the rows have already moved.
3. The lineage fix this PR is named for:

```sql
UPDATE deployments SET source_account_id = $target
WHERE source_account_id = $source AND agent_name = $name
```

The deployments FK is single-column on `source_account_id → accounts.id`, so it does not ride the agents cascade — this third statement is what actually moves lineage. Folding it into the same `tx` is what makes the fix safe: a partial transfer (agent moved, deployments not yet repointed) is exactly the inconsistent state that produces the bug, so all three writes commit or roll back together. The statement intentionally does not enforce a non-zero `RowsAffected` — agents with no deployed instances at the time of transfer are normal and must still commit.

The WHERE clause is keyed on `(source_account_id, agent_name)`, not on `agent_name` alone, so unrelated agents with the same name on a different source are not touched, and a same-account deploy whose `source_account_id` already matched the source is correctly migrated alongside cross-account ones.

### One-shot data sweep

The transactional fix protects every future transfer; the small set of rows from transfers performed before this fix is repaired in place by a new `RebindStaleSourceAccountIDs` method on `deploymentstore.Store`. Single set-based UPDATE, runs at server startup right after the existing `BackfillSourceAccountIDs` pass:

```sql
WITH candidates AS (
  SELECT av.name, av.build_id, av.account_id,
         COUNT(*) OVER (PARTITION BY av.name, av.build_id) AS n
  FROM agent_versions av
)
UPDATE deployments d
SET source_account_id = c.account_id
FROM candidates c
WHERE c.name = d.agent_name
  AND c.build_id = d.build_id
  AND c.n = 1
  AND d.source_account_id IS NOT NULL
  AND d.source_account_id <> c.account_id
RETURNING d.id;
```

Detection is self-addressing: each candidate row knows its `agent_name` and `build_id`, the `agent_versions` row for that pair already moved to the new owner during `Transfer`, and the PK on `(account_id, name, build_id)` makes the candidate unique unless an unrelated account independently registered the same name and build hash. The `c.n = 1` predicate is the safety bound for that vanishingly rare collision — ambiguous rows are left for human triage rather than silently rebound. Rows whose `(name, build_id)` no longer exists in `agent_versions` (publisher actually deleted the build) are also left alone; those are PR4's FK and PR5's `resolveSourceAccountName` to handle, not this PR.

The two passes run in series inside the existing 5-minute startup goroutine. `BackfillSourceAccountIDs` failure no longer short-circuits the rebind, since the two touch disjoint row sets (NULL vs non-NULL `source_account_id`). The `<>` predicate excludes already-correct rows, so once every row's lineage matches `agent_versions` subsequent boots are a no-op.

### Tests

Three layers, kept separate so the fast path runs without infrastructure. A new `astro-server:test-integration` Moon target runs the Postgres-only tier locally via `scripts/e2e.sh integration` (mirrors the CI `test-go-integration` job), separate from the K8s `astro-server:e2e` target so engineers don't pay the kind/vcluster cost when they only care about lineage:

- `agentindex` sqlmock unit tests pin the SQL ordering and argument shape of `Transfer`, including a rollback case that proves the new statement participates in the existing transaction.
- A new `e2e/transfer_test.go` (`-tags integration`) exercises the cross-table effect against a real Postgres: it seeds three deployments — a cross-account deploy, a same-account deploy, and an unrelated agent on the same source — runs `Transfer`, and asserts the first two repoint while the third is untouched. It also covers the no-deployments path so the empty-set commit is locked in. **This is the test that surfaced the FK ordering bug**; without `ON UPDATE CASCADE` on the agent FKs it failed with `pq: update or delete on table "agents" violates foreign key constraint "agent_versions_account_id_name_fkey"`.
- A new `e2e/rebind_stale_source_account_id_test.go` covers the data sweep against every branch its WHERE clause encodes: rebinds the unique-candidate case, leaves the ambiguous (`n > 1`), unknown-build, already-correct, and `NULL`-source cases alone, runs idempotently, and repairs multiple rows in one pass (which a future row-by-row "fix" would fail).

## Migration

No operator action required. Atlas applies the FK option change inline on the next `atlas schema apply` (a `DROP CONSTRAINT` + `ADD CONSTRAINT` pair, transactional in Postgres). The startup sweep runs in the same goroutine as the existing `source_account_id` backfill, runs as a no-op on subsequent boots once all rows are consistent, and emits a `source_account_id stale rebind complete` log line with the count of repaired rows on the first boot after deployment.
