# Prove a redelivered webhook collapses and a suspension stops the workloads

## Summary

Two billing behaviours were asserted only against fakes. One is the dedupe that
stops a redelivered webhook applying twice. The other is the call that scales an
account's workloads to zero, which is the step that makes a suspension mean
anything.

## Webhook dedupe

Providers redeliver. Stripe repeats an event until it sees a 2xx, and a
redelivered gating event applied twice is a suspension applied twice.

The defence is River's unique-by-args index on the event ID. Every test of it
asserts the `InsertOpts` struct the code returns. A struct assertion cannot tell
whether the index exists in the schema, whether River consults it on insert, or
which fields it covers. No test had ever inserted two jobs and watched one
disappear.

### Design

The cases run against the real River tables in the integration Postgres, through
the same `InsertStripeWebhook` and `InsertMetronomeWebhook` the handlers call.

**The count comes from the job table, not from River's answer.** Each case reads
`river.river_job` for the event ID. Asking River whether it skipped a duplicate
would trust the component under test to report on itself, and a unique index
that silently stopped applying would still answer correctly.

**The redelivery changes a sibling field.** Only `EventID` carries
`river:"unique"`, so a second delivery whose hosted invoice URL differs must
still collapse. Inserting two identical payloads would pass whether the tag
scoped the index to one field or to all of them.

Four cases: a Stripe redelivery collapses, two distinct events both enqueue, a
Metronome redelivery collapses through the shared `webhookInsertOpts`, and an
event with no ID enqueues twice. That last one is the intended behavior. An
unidentified event cannot be deduped, and dropping it would discard a real event
on the theory that it matched something.

Both collapse cases were confirmed to fail with the unique options removed: two
jobs stored instead of one, three instead of one.

## Suspension against a real cluster

The status machine, the dunning timer, and the 402 all have tests, and all of
them stop at the point where `BillingSuspendWorker` calls
`StopNamespaceWorkloads`. That call had never run against a cluster. A
suspension that leaves the agents running bills the account for the period it
was supposed to have stopped.

Four cases under the `k8s` tag: every managed Deployment and StatefulSet reaches
zero, a re-apply restores the original replica counts, a second stop is a no-op,
and CronJobs are suspended.

**Both workload kinds, and the third that has no replicas.** The agent is a
Deployment and persistent knowledge is a StatefulSet, so a stop that reaches one
leaves half the footprint running. A CronJob has no replica count at all, and
suspending it is a separate branch that a replica-only reading of the function
misses. That branch is where a scheduled ingestion would keep consuming after
the agent stopped.

**Resume asserts the stop first.** Resume re-applies the spec rather than
remembering a replica count, so the case would pass on a stop that never
stopped anything. It checks zero before re-applying.

Test names are chosen so their first 20 characters differ, because `uniqueNS`
truncates there. Three names that agreed on that prefix would share one
namespace, serialize on each other's teardown, and leave the group broken when
one cleanup failed.

## Migration

None. The cases join the existing `e2e` package. CI already runs it under both
`-tags integration` and `-tags k8s`.
