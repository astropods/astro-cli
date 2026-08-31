# Account event stream: server foundation

## Summary

Pages that need to know when server state changed have had one option: ask
again on a timer. The agent detail page polled GitHub build status every five
seconds while a build ran, and every fifteen otherwise. Polling is slow when it
is quiet and wasteful when it is not, and it pushed every reader into defending
itself against data that arrived for the wrong subject at the wrong time.

This adds the server half of pushing instead: a durable, replayable,
account-scoped event stream. It has no producers and no consumers yet. The
client subscription and the first producer land separately, so this change can
be reviewed as infrastructure rather than as a feature.

## Design

The shape is the transactional outbox described in
[Transactionally Staged Job Drains in Postgres](https://brandur.org/job-drain),
whose author also wrote River, the job queue this codebase already runs.

**The row is the event.** `agent_events` takes one row per change, written by
the path that made the change, optionally inside that path's transaction so the
event and the state commit together. The row is what makes delivery recoverable;
everything else is an optimisation on top of it.

**One row per change, never one per recipient.** An agent's builds matter to the
publishing account and to every account running a deployment whose lineage
points at it. Writing a row per recipient would mean a popular blueprint with a
thousand downstream deployments writing a thousand rows for one build.
Recipients are resolved when rows are read, and the live path carries the
recipient list inside the notification, so storage tracks the change rate and
not the size of the audience.

**Events carry no state.** An event names the subject that changed. The client
invalidates and refetches through the query it already owns. That keeps one
authority on the server, so a missed event costs a late refresh rather than a
screen rendering state that arrived by a second route.

**Delivery is separate from durability.** PostgreSQL LISTEN/NOTIFY is the
wake-up. It is deliberately sent outside the transaction that wrote the row: a
transaction holding a pending notification takes an instance-wide lock through
its COMMIT so notifications appear in commit order, which would serialize every
writer behind it (see
[Postgres LISTEN/NOTIFY does not scale](https://www.recall.ai/blog/postgres-listen-notify-does-not-scale)).
Outside the transaction a notification can be lost, which is what replay covers.

**Replay closes the gap.** Clients reconnect with `Last-Event-ID` and receive
what they missed. The catch-up is capped at 200 events, and the cap is reported
in the `ready` frame so a client too far behind refetches rather than trusting a
partial replay. The heartbeat carries the newest event id, which is the only way
to notice a dropped notification while the connection stays open and nothing
further arrives.

**Fan-out is per replica.** Workers run in their own deployment and the API runs
several replicas, so an in-process hand-off would reach nobody. Each replica
subscribes to the channel and fans out to the SSE connections it holds. A
subscriber that cannot keep up is dropped rather than allowed to stall delivery
for everyone else.

The endpoint is `/api/v1/accounts/:account/events`, gated on account membership:
the stream names every agent in an account and the state of its builds. The
membership gate lives in the route chain, not the handler, and
`TestStreamAccountEventsRefusesANonMemberAtTheRoute` in
`handlers/events_test.go` asserts
the chain, since the handler alone cannot tell an authorized account from a
resolved one.

Retention is seven days, trimmed daily by a River periodic job. That window
bounds how far behind a disconnected client can be and still catch up, so it
buys latency after a long absence rather than correctness.

## Migration

Adds the `agent_events` table. Nothing writes to it yet, and no backfill: an
empty table means clients replay nothing and refetch, which is what they already
do.
