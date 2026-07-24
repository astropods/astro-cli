# Summary

The event-driven deployment controller is the sole writer of the deployment
read-model (`deployment_workload_status` + `deployment_runtime_status`, feeding
both `/status` and `/runtime`). It ran unconditionally on the assumption that
`astro-worker` stays `replicas: 1` — the moment the worker scaled out, every
replica would start its own informers and race on the per-deployment full-replace
writes. This change removes that assumption by putting the controller behind a
Postgres advisory-lock leader election, so the worker can scale without
corrupting the read-model.

# Design

A new `internal/leaderelection` package elects a single leader across replicas
using a session-level Postgres advisory lock — a natural fit because Postgres is
already the single coordination point the read-model design hinges on.

The lock primitive is `github.com/allisson/go-pglock`, which owns the
dedicated-connection pinning and the `pg_advisory_lock` SQL (session-level
advisory locks live on the backend that took them, so the connection must be
pinned). The package adds the orchestration on top:

- **Non-blocking acquire.** Followers poll `pg_try_advisory_lock` on a retry
  interval rather than blocking a connection in `pg_advisory_lock`.
- **Crash failover for free.** If the leader process dies, its backend connection
  drops and Postgres releases the lock automatically — no lease renewal, no
  fencing token, no stale-lock timeout to tune. A standby acquires on its next
  retry tick.
- **Step-down.** While leading, a heartbeat re-probes the lock (a no-op that
  stacks within our own session, then releases the extra hold); any error means
  the connection died, so the leader cancels `leaderCtx`, unwinding the
  controller before a second replica takes over.
- **Explicit release.** On graceful step-down the lock is released with
  `pg_advisory_unlock` (not connection close, which only returns the pinned conn
  to the pool without ending the session) so a standby takes over immediately.

`Run(ctx, db, cfg, onElected)` invokes `onElected(leaderCtx)` only while this
replica holds the lock; the worker runs the controller there. At `replicas: 1`
the sole replica is always leader, so behavior is unchanged.

**Cross-replica reconcile nudge.** When the DeployWorker applies a deployment it
nudges the controller to reconcile immediately, so a no-change redeploy (which
produces no informer event) doesn't wait for the periodic resync. But the
DeployWorker can run on any replica while only the leader drains the controller
queue — so the nudge is routed through Postgres `LISTEN/NOTIFY` (a thin
`internal/pgnotify` wrapper over `lib/pq`'s auto-reconnecting `Listener`): the
worker `NOTIFY`s the namespace, and the leader's listener — started under
`leaderCtx`, so its lifetime tracks leadership — feeds it into the queue. The
nudge is best-effort: a notification dropped during a listener reconnect falls
back to the resync, so this is a latency optimization, not a durable path.

**Mechanism choice.** A K8s `Lease` via client-go `tools/leaderelection` was the
alternative, but it needs `coordination.k8s.io` RBAC and a home cluster; the
advisory lock reuses infrastructure we already have. River (already a dependency)
does Postgres leader election internally but exposes no public hook in v0.31, so
its elector could not be reused.

# Migration

None. No schema change (advisory locks are keyed, not stored) and no RBAC; the
only addition is the `go-pglock` dependency. Existing single-replica deployments
behave identically. Scaling `astro-worker` past one replica is now safe: extra
replicas idle as hot standbys.
