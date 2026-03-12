# River Queue Integration

Background job processing for astro-server, replacing ad-hoc goroutine workers with persistent, retryable, observable jobs via [River](https://riverqueue.com).

## Problem

astro-server runs 5 background workers as bare goroutines with `time.NewTicker` loops:

| Worker | Package | Interval | What it does |
|--------|---------|----------|--------------|
| OpenMeter heartbeat | `internal/openmeter` | 5 min | Emits metering events (compute usage, active deployments, active agents) |
| OpenMeter reconciler | `internal/openmeter` | once at startup | Backfills OpenMeter customers for accounts missing one |
| Namespace scanner | `internal/nsscan` | 10 min | Reconciles DB deployments ↔ K8s namespaces, maintains `namespace_ownership` |
| Drift checker | `internal/driftcheck` | 10 min | Compares desired deployment state against K8s cluster, logs drift |
| WorkOS events consumer | `internal/org` | 30 sec | Polls WorkOS Events API, processes membership/org/user changes |

All five share the same problems:

- **No retry.** If a tick panics or returns an error, it logs and waits for the next interval. Transient failures (network blip, DB lock timeout) are silently dropped.
- **No persistence.** Work state lives in memory. A server restart loses in-flight work and resets all cursors to "run immediately."
- **No observability.** No way to see which jobs are running, how long they take, or what failed — only scattered log lines.
- **No backpressure.** All workers start unconditionally on boot. No way to pause, throttle, or redistribute work across replicas.
- **No deduplication.** If two server replicas run, both execute the same periodic work (duplicate metering events, duplicate K8s API calls).

## Why River

River is a Postgres-based job queue for Go. It fits because:

- **Postgres-native.** Jobs live in the same database we already use. No new infrastructure (no Redis, no SQS). Transactional job insertion — enqueue a job in the same `tx` as a DB write, both commit or neither does.
- **Go generics.** Workers are typed: `river.Worker[HeartbeatArgs]`. Compile-time safety for job args, no `interface{}` casting.
- **LISTEN/NOTIFY.** Jobs are picked up immediately via Postgres `LISTEN/NOTIFY`, not polling. Sub-second latency for on-demand jobs without burning CPU on tight poll loops.
- **Unique jobs.** Built-in deduplication by args, period, or queue. Solves the multi-replica duplicate work problem.
- **Periodic jobs.** First-class `river.PeriodicJob` with cron-like scheduling. Replaces our `time.NewTicker` loops with cluster-singleton execution.
- **Retry with backoff.** Configurable per-worker max attempts and backoff. Dead jobs land in a `river_job` row with state `discarded` for inspection.
- **Observability.** `river_job` table is queryable — pending, running, completed, failed, discarded states. River UI available for debugging.

Alternatives considered: `pgx-job` (less mature), `Temporal` (too heavy), raw `pg_notify` (reimplements half of River).

## Architecture

### Dual connection pools

astro-server currently uses `database/sql` with `lib/pq`. River requires `pgx/v5` with `pgxpool`. Both will coexist:

```
┌─────────────────────────────────────────┐
│             astro-server                │
│                                         │
│  database/sql + lib/pq                  │
│  └─ existing stores, queries, Atlas     │
│                                         │
│  pgxpool.Pool                           │
│  └─ River client, river workers         │
│                                         │
│  Same DATABASE_URL ──────────────┐      │
└──────────────────────────────────┼──────┘
                                   ▼
                              ┌──────────┐
                              │ Postgres │
                              └──────────┘
```

Both pools connect to the same database. The `pgxpool` is only used by River internals and River workers. Existing stores continue using `database/sql` unchanged. If a River worker needs to query the DB, it can use either pool — but prefer the existing `*sql.DB` to avoid duplicating store logic.

Over time, stores may migrate to `pgx` directly, but that's a separate effort.

### Migration strategy

River needs its own tables (`river_job`, `river_leader`, `river_queue`, etc.). These are managed via **Bytebase**, not at server startup.

- River migration SQL is exported from `rivermigrate` and checked in at `apps/astro-server/migrations/001_river.sql` for reference and local dev.
- The SQL is deployed to production via Bytebase before the server starts — the server assumes River tables exist at boot.
- The `rivermigrate` package is a build dependency only (used for the export script), not called at runtime.
- Atlas's `atlas.sum` and migration directory are untouched — no conflict.

### Package layout

```
apps/astro-server/
├── internal/
│   ├── riverqueue/
│   │   ├── client.go        # New, Start, Stop — pgxpool + river.Client lifecycle
│   │   ├── workers.go       # river.Workers registry, all AddWorker calls
│   │   ├── heartbeat.go     # HeartbeatArgs + HeartbeatWorker
│   │   ├── reconciler.go    # ReconcilerArgs + ReconcilerWorker
│   │   ├── driftcheck.go    # DriftCheckArgs + DriftCheckWorker
│   │   ├── nsscan.go        # NsScanArgs + NsScanWorker
│   │   ├── workosevents.go  # WorkOSEventsArgs + WorkOSEventsWorker
│   │   └── periodic.go      # PeriodicJobs() — cron schedule definitions
```

Each worker file contains:
1. An `Args` struct implementing `river.JobArgs` (defines `Kind()` and queue)
2. A `Worker` struct implementing `river.Worker[Args]` (defines `Work(ctx, job)`)

The existing packages (`internal/openmeter`, `internal/nsscan`, etc.) keep their core logic. River workers call into them — they're adapters, not replacements.

## Worker migration plan

Phased rollout. Each phase converts one or more workers, validated independently.

### Phase 1: OpenMeter heartbeat

**Why first:** Fires every 5 minutes, stateless, easy to verify. No cursor or external state to manage. If it double-fires during migration, metering events are idempotent.

```go
type HeartbeatArgs struct{}
func (HeartbeatArgs) Kind() string { return "openmeter.heartbeat" }

// Periodic job — every 5 minutes, unique per period
```

The existing `Heartbeat.tick()` method becomes the body of `HeartbeatWorker.Work()`. Remove the `time.NewTicker` loop from `Heartbeat.Start()`.

### Phase 2: OpenMeter reconciler

**Why second:** Runs once at startup, already a batch processor. Convert to a job that's enqueued once on boot (with unique constraint so replicas don't duplicate).

```go
type ReconcilerArgs struct{}
func (ReconcilerArgs) Kind() string { return "openmeter.reconciler" }
// Unique: ByPeriod(24h) — at most once per day across replicas
```

### Phase 3: Drift checker

Convert the 10-minute ticker to a periodic River job. `DriftCheckWorker.Work()` calls `Checker.Check()` and logs the report. Same logic, just scheduled by River.

### Phase 4: Namespace scanner

Similar to drift checker — periodic job every 10 minutes. The scan hooks mechanism stays; the worker calls `Scanner.Scan()` and runs hooks on the result.

### Phase 5: WorkOS events consumer

Most complex — has cursor state, error tracking, and stuck detection. The `poll()` method becomes the worker body. Key difference: River provides retry with backoff, so the manual `stuck_event_id` / `stuck_since` tracking can be simplified (failed jobs are retried automatically).

Cursor management (`workos_event_cursor` table) remains unchanged — it tracks API pagination position, which is orthogonal to job retry.

### Phase 6: Async API jobs (future)

Once the infrastructure is in place, new features can enqueue one-off jobs from API handlers:
- Deployment teardown (K8s resource cleanup)
- Image garbage collection
- Webhook delivery
- Async build triggers

These aren't existing workers — they're new capabilities unlocked by having a job queue.

## Wiring & lifecycle

In `main.go`, the `runWorker` function currently starts all goroutines. It becomes:

```go
func runWorker(cfg, db, ...) (context.CancelFunc, error) {
    pool, err := pgxpool.New(ctx, cfg.DatabaseURL)

    rq, err := riverqueue.New(pool, riverqueue.Config{
        DB:           db,           // existing *sql.DB for store access
        K8sClient:    k8sClient,
        AccountStore: accountStore,
        OMClient:     omClient,
        // ... other deps workers need
    })

    rq.Start(ctx)  // runs rivermigrate, then river.Client.Start()
    return rq.Stop, nil
}
```

Shutdown: `rq.Stop()` calls `river.Client.Stop()` which gracefully drains in-flight jobs (with a configurable timeout), then closes the pgxpool.

### Dependency injection

River workers need access to existing stores and clients. The `riverqueue.Config` struct holds all shared dependencies. Individual workers receive them at construction time (when registered in `workers.go`), not at job execution time.

## Dependencies

New Go module dependencies:

```
github.com/riverqueue/river          # core client + worker types
github.com/riverqueue/river/riverdriver/riverpgxv5  # pgx v5 driver
github.com/riverqueue/river/rivermigrate            # schema migration
github.com/jackc/pgx/v5                             # pgx pool (River's required driver)
```

`pgx/v5` is the only significant new dependency — it's the standard Go Postgres driver and will likely replace `lib/pq` project-wide eventually.
