# River: domain queues and consistent job-kind naming

## Summary

Almost every River job landed in the shared `default` pool, so a backlog in one
class of work (backfills, reconciles) competed for the same workers as billing,
metering, and insights. Job-kind names had also drifted into three styles —
dotted (`billing.suspend`), bare snake_case (`account_purge`), and bare words
(`deploy`, `github_build`) — with split namespaces for one concern
(`metrics.message_count_sync` vs `metering.heartbeat`).

## Design

**Queues by domain.** Each job now routes to a queue matched to its workload, so
one class of work can't starve another. Sizes are per-queue worker caps:

| Queue | Workers | Jobs |
|-------|--------:|------|
| `deploy` | 5 | `deployment.*` |
| `build` | 3 | `build.github` |
| `billing` | 3 | `billing.*` |
| `metering` | 3 | `metering.*` |
| `insights` | 3 | `insights.*`, `obs.summary_refresh` |
| `maintenance` | 5 | avatar/provider backfills, `knowledge.reconcile`, `account.purge`, `privatelink.*`, `workos.member_email_reconcile` |
| `default` | 10 | fallback for anything unrouted |

Queue names are centralized in `queues.go`; routing is set on each job's
`InsertOpts().Queue`, which River applies to periodic and manually-triggered
inserts alike.

**Consistent kind naming.** All kinds are now `domain.action` dotted form
(e.g. `account_purge`→`account.purge`, `github_build`→`build.github`,
`metrics.message_count_sync`→`metering.message_count_sync`,
`deploy`→`deployment.deploy`). The three call sites that referenced kinds by raw
string — the cluster-migration SQL, the build-cancel SQL, and the Queen
migrations page — were updated in lockstep.

## Migration

Straight rename, no compatibility shims. River matches workers to jobs by the
`kind` string, so a job already enqueued under an old kind (or in the renamed
`github_build` queue) won't be picked up. The affected jobs are overwhelmingly
periodic and idempotent — they re-enqueue under the new names on their next tick.
The only user-triggered exceptions (`deployment.deploy`, `build.github`) would at
most need a one-time re-trigger if in flight during deploy. No schema changes.
