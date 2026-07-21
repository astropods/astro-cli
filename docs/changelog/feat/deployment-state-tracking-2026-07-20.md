# Remove legacy deployment machinery (reconcile worker, KEDA, orphan recovery, drift)

## Summary

Clears out legacy deployment machinery ahead of the planned event-driven
deployment controller (see `docs/plans/deployment-state-tracking.md`). Nothing
here changes live behavior — each piece was either not executing or only
reachable through a path that read schema objects already dropped. Removed:

1. **The periodic `ReconcileWorker`** — pod-failure escalation, stale-job
   recovery, drift reporting, and orphaned-namespace recovery. Its periodic
   trigger was commented out and it was enqueued nowhere else, so none of its
   logic ran.
2. **KEDA scale-to-zero** — KEDA is no longer used. The backend never created
   KEDA resources; it only *reacted* to external autoscaling (via the reconciler
   above) through a `scaled_down` deployment status and a `scaled_namespaces`
   table.
3. **Orphaned-namespace recovery** — with the reconciler gone it had no driver.
   The `namespace_ownership` table (already unreferenced) and
   `RecoverOrphanedDeployment` are removed.
4. **Drift detection** — the periodic driver was the reconciler; only an
   admin-on-demand path remained, and it read a `deployment_resolved_keys` table
   already dropped from the schema. Removed end to end (server + proto + queen).

## Design

**What is preserved.** Manual pause is untouched: the `stopped` status and the
full wake-up/resume path (`WakeUpWorker`, the `WakeUpDeployment` HTTP + gRPC
endpoints, and the queen/client Pause/Resume controls) remain, now keyed off
`stopped` alone. `GetDeploymentsInStatus` is retained — the future controller
reuses it. Quota still does not count paused deployments (unchanged; it just no
longer references `scaled_down`).

**KEDA / `scaled_down`.** Removed the `StatusScaledDown` constant, the
`scaled_namespaces` table and its `Mark`/`Is`/`ClearScaledDown` store methods,
the cluster-migration cleanup branch, and every `scaled_down` branch across the
status/quota/migration paths and the admin, queen, and client UIs (status
filters, colour maps, the KEDA "Scaled Down" banner).

**Drift.** Removed `BuildDriftReport`, the store `DriftReport`/`ResolvedKeys`
types and their methods, the admin gRPC `RefreshDriftReport` /
`BackfillResolvedKeys` RPCs, and the queen drift tab/filter/badge and their
routes, hooks, and types. The AdminService `.proto` never declared these RPCs
and the `.pb.go` files are hand-maintained (no `buf.gen.yaml`), so the generated
stubs were edited directly — no regeneration.

**Fallout cleanup.** With drift gone, `SaveNormalizedSpec`'s
`resolved *ResolvedEnv` parameter was dead (it only fed the removed
`resolved_keys` write) and is dropped from the signature and all call sites.
That made `RepairNormalizedSpec`'s env-resolution block and its `liveSecretData`
parameter dead — removed, which also drops a per-repair live K8s Secret read in
the admin RPC. Orphaned config (`NormalizedSpecConfig.ManagedSecrets`) and stale
comments referencing the dropped `deployment_variables`/`deployment_resolved_keys`
tables were cleaned up.

## Migration

Operational only — no config or public API changes:

- Drop the now-unused `public.scaled_namespaces` and `public.namespace_ownership`
  tables (removed from the checked-in schema; the live drop is a Bytebase
  migration).
- Migrate any historical deployment rows still in `scaled_down` to `stopped`.

`deployment_resolved_keys` and the `drift_report` / `drift_checked_at` columns
were already absent from the schema, so they need no migration.
