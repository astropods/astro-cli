# Migrate all background workers to River queue

## Summary

astro-server ran five background workers as bare goroutines with `time.NewTicker` loops. These had no retry, no persistence across restarts, no observability, and no deduplication across replicas. This change migrates all five to River periodic jobs, building on the existing River infrastructure (Phase 1 heartbeat was already done).

## Design

Each worker follows the same adapter pattern: a thin River worker struct calls into the existing package's core logic (`Scan()`, `Check()`, `Poll()`, `Run()`). The original packages are unchanged — River workers are adapters, not replacements.

**Workers migrated:**

| Worker | Kind | Schedule | Queue | Uniqueness |
|--------|------|----------|-------|------------|
| OpenMeter reconciler | `openmeter.reconciler` | 24h, RunOnStart | default | ByPeriod 24h |
| Drift checker | `driftcheck` | 10m, RunOnStart | default | ByPeriod 10m |
| Namespace scanner | `nsscan` | 10m, RunOnStart | default | ByPeriod 10m |
| WorkOS events | `workos.events` | 30s, RunOnStart | `workos` (MaxWorkers: 1) | ByQueue |

The WorkOS events consumer gets a dedicated single-worker queue to prevent cursor racing — only one poll runs at a time across all replicas.

**Key changes to support this:**

- `riverqueue.Config` now carries all worker dependencies (AccountStore, K8sClient, OrgClient, WorkOSAPIKey)
- `periodicJobs()` accepts `Config` to conditionally register WorkOS jobs only when the API key is set
- `LogReport`/`LogResult`/`Poll` exported on driftcheck, nsscan, and org packages so River workers can call them
- `runWorker()` in main.go is now just K8s client init + River queue startup
- Removed the completed namespace migration hook (`migrate_hook.go`)

## Migration

No action required. River tables must already exist (managed via Bytebase). The server assumes they're present at boot. Worker behavior is identical — same intervals, same logic, same dependencies — just scheduled by River instead of raw goroutines.
