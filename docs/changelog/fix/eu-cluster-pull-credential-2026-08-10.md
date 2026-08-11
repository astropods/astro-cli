## Summary

Tenant pods on additional clusters (e.g. the EU managed cluster) failed to pull images with a 401 from `astro-registry`. The registry's per-cluster pull authorization was already correct and tested — the bug was upstream: astro-server handed every cluster the same global pull credential, which is actually the primary cluster's. An additional cluster's tenants are never homed on `primary`, so the registry correctly refused the pull.

## Design

The cluster pull credential (CPC) now flows through the same per-cluster config chain already used for `agent_ingress_domain` / `pod_subnet_cidrs` / etc — a row on `public.clusters`, resolved via `clustercfg.Resolve` — rather than a new global env var:

- `clusters` gains a `pull_credential` column (plaintext CPC) alongside the existing `pull_key_hash` (its sha256, checked by the registry at `/token`).
- `clusterstore.Register` generates a CPC automatically at registration time and stores both columns in the same INSERT. `EnsurePullCredential` is a guarded, idempotent backfill for clusters registered before this column existed.
- The backfill runs from two places, no new admin action required: `UpdateCluster` calls it after every save (so re-saving a cluster's config through the existing edit flow fixes it), and astro-server runs a one-shot reconciliation pass at boot.
- `clustercfg.Resolve` now resolves `RegistryPullCredential` per cluster — the primary keeps its existing env-var-backed credential; an additional cluster reads its own row's credential and fails the deploy loudly (instead of silently substituting the wrong one) if pull-through is enabled but no credential has been issued yet.
- `deployer.go` now reads the resolved per-cluster credential instead of the single global config value — this line was the actual bug.

No changes to `astro-registry` (its cluster-scoped auth was already correct), the admin proto/API, the queen UI, or Terraform.

## Migration

None required — existing clusters are backfilled automatically (on next `UpdateCluster` call or server restart).
