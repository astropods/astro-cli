# Remove platform-provisioned knowledge stores

## Summary

Astro could provision a knowledge store itself: `POST .../knowledge` created a StatefulSet, PVC, Services, PodDisruptionBudget, and NetworkPolicies in a per-account `knlg0-{account-id}` namespace, and the platform then operated that database for the tenant's lifetime.

That path has been off by default since `KNOWLEDGE_ALLOW_MANAGED_CREATE` landed, leaving connect (bring-your-own database) as the only supported way to get a store. Keeping the provisioning code alive behind a flag meant carrying a second store lifecycle — a reconciler that advanced pods to ready and recreated credential Secrets, a compute-billing loop, K8s-backed logs/metrics/events endpoints, and a `mode` branch at every read — for a feature nobody could turn on. This removes it.

Astro is now purely a credential broker for knowledge: it holds encrypted connection details for a database the account already operates, and provisions no database infrastructure of its own.

## Design

**One kind of store.** Every store is external. The `knowledge_stores.mode` column still exists and is still written (always `'external'`) so the column's `NOT NULL` contract holds without a migration, but no code branches on it — `mode` is read back and reported in the API purely so legacy rows aren't mislabelled. `ModeManaged` and the `provisioning` status are gone.

The mode guards that gated behaviour are gone with them. Recheck previously rejected non-external stores up front; the endpoint lookup immediately after already rejects any store without a PrivateLink endpoint, which every legacy row is, so the guard was redundant.

**Credentials resolve one way.** `ResolveCredentials` previously had a K8s Secret fallback for provisioned stores running without KMS — the Secret held plaintext next to the StatefulSet. With no StatefulSet there is no Secret, so KMS envelope decryption is the only path and the `SecretReader` seam is deleted. This also drops the `k8s.ClusterClient` dependency from the knowledge handlers entirely.

**The reconciler does one thing.** `KnowledgeReconcileWorker` had three stages; two of them (advance provisioning pods, recreate missing credential Secrets) existed only for provisioned stores. It now polls PrivateLink endpoints and nothing else, and no longer needs a cluster registry or billing manager.

**Surface removed.** The create endpoint, plus the logs, log-stream, metrics, and events endpoints — all four of the latter read from the store's pod in the knowledge namespace, so they had no meaning for a database Astro doesn't run. `ast knowledge create` and `ast knowledge logs` go with them; `connect`, `list`, `status`, `credentials`, and `delete` are unchanged. In the UI the Logs tab, event timeline, and Storage/Public-access settings are gone.

**`knowledge_domain` removed from cluster config.** Its only consumer was the public-host assignment for provisioned stores, but it was still a *required* field — `clusterstore.Register/Update` rejected an empty value, and `clustercfg.Resolve` failed any deploy targeting a cluster row that lacked one. Operators were filling in a domain nothing read, to clear a check that could block deploys. It comes out of the whole chain: the `clusterfields` validator, `clusterstore`/`clustercfg`/`k8s.Registry`/`admingrpc` plumbing, the `KNOWLEDGE_DOMAIN` env var, the `clusters.knowledge_domain` column, queen's cluster form and completeness badge, and the Helm/Terraform values that fed it. The AdminService proto field numbers (17, 12, 11) are `reserved` rather than reused, so old and new binaries stay wire-compatible.

Note that `packages/astro-proto`'s Go code is hand-maintained, not generated — there is no `buf.gen.yaml`, despite the header comment saying to run `buf generate`. The structs were edited by hand to match the `.proto`.

**Knowledge billing removed.** `StartKnowledgeBilling` was only ever called when a provisioned store went ready, so the whole CU-hour loop became unreachable — and its one emitter, `emitKnowledgeCompute`, was already dormant (never called from `Tick`). That means no `knowledge_compute_usage` or `knowledge_storage_provisioned` event was being sent at all. The start/stop/reconcile/heal functions, the storage emitter, and the `knowledge_billing_state` table are all gone. Deployment compute metering is untouched — it is the only thing `Tick` ever emitted.

If `knowledge_compute_usage` or `knowledge_storage_provisioned` are still defined as billable metrics on the Metronome side, they can be retired there too; nothing emits them now.

**Deliberately left in place:**

- **Legacy rows and live resources.** No data migration runs for `knowledge_stores` and no StatefulSets are reaped. Any store still carrying `mode = 'managed'` keeps its row and lists as "Managed", but nothing manages it: delete removes only the DB row, and binding it to an agent fails at deploy time because it has no `HOST` credential. Operators clean up leftover `knlg0-*` namespaces out of band.

## Migration

No action for users of connected stores — connect, bindings, credentials, and PrivateLink are unchanged.

| Step | Action |
|---|---|
| Schema | Apply `sql/astro-server/schema.sql` **before** deploying astro-server. Drops `clusters.knowledge_domain` and the `knowledge_billing_state` table. The new binary touches neither, so applying first is safe; applying after also works, since the old binary tolerates both being present. |
| Deploy order | astro-infra (Helm) can go before or after — the chart simply stops setting `KNOWLEDGE_DOMAIN`, and the new server ignores it either way. |
| Env | `KNOWLEDGE_ALLOW_MANAGED_CREATE` and `KNOWLEDGE_DOMAIN` are no longer read and can be dropped from astro-server's environment. |
| Cluster config | Registering or editing a cluster in queen no longer asks for a knowledge domain, and clusters that were flagged **Incomplete** solely for a missing one now register and accept deploys. |
| API | Deployments that still set `KNOWLEDGE_ALLOW_MANAGED_CREATE=true` lose managed creation: `POST .../knowledge` now 404s rather than 403s. Call `POST .../knowledge/connect` instead. |
