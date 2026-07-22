# Deployment status reports what it's waiting on

## Summary

While a deployment sits in `deploying`, `GET /deployments/:id/status` returned a
single static string, "Waiting for workloads to become ready" — with no way to
tell *which* workload was blocking the transition to active. A deployment could
look fully healthy on the runtime endpoint (agent, sidecars, all pods Running
and Ready) yet stay in `deploying` indefinitely, with no visible explanation.

This happens because the two endpoints answer different questions from different
data. `/runtime` reports the observed cluster (its headline ready/replicas is the
agent workload alone, and its workload list contains only objects the informer
currently sees). `/status` reflects the `deployments.status` column, which the
deploy controller flips to `active` only once **every declared workload** is
observed *and* ready/complete. A workload that is declared but not currently
observed — e.g. an ingestion Job that was reaped or never created — is invisible
on `/runtime` but still holds the controller's readiness gate open, so status
stays `deploying` with no clue why.

## Design

`/status` now reconstructs the controller's readiness gate
(`aggregateDeploymentPhase`) in the handler from two cheap DB reads — the
declared workloads (`GetWorkloads`) versus the persisted per-workload status the
controller writes each sync (`GetWorkloadStatuses`) — and reports the blockers on
a new `waiting_on` array, populated only while `value === "deploying"`:

```json
{
  "value": "deploying",
  "reason": "provisioning",
  "details": "Waiting for sasbot-knowledge-ingest (not yet created), sasbot-agent (ContainerCreating)",
  "waiting_on": [
    { "workload": "sasbot-knowledge-ingest", "component": "ingestion", "phase": "missing" },
    { "workload": "sasbot-agent", "component": "agent", "phase": "progressing", "reason": "ContainerCreating" }
  ]
}
```

Each entry falls into one of two categories, matching the gate exactly:

- `phase: "missing"` — declared at apply time but not observed in the cluster.
  This is the only place such a workload surfaces; the runtime endpoint lists
  observed objects only.
- any other `phase` (`progressing`/`pending`) — observed but not yet ready,
  carried with its `reason`/`message`.

An empty `waiting_on` means the gate would pass and the controller's next sync
will flip the deployment to `active`.

Key decisions:

- **Reporting only, no cluster round-trip.** The handler reads the persisted
  status the controller already maintains — no per-request K8s calls — and the
  computation mirrors the controller's own gate so the two never disagree.
- **Best-effort enrichment.** A read error falls back to the previous static
  `details` string rather than failing the status request.
- **Scoped to `deploying`.** `pending`/`provisioning` keep their static message —
  before the worker hands off, no per-workload status exists, so `waiting_on`
  there would be all-`missing` noise.
- **Additive contract.** `waiting_on` is optional; existing consumers are
  unaffected. The client `DeploymentStatus` type gains a matching optional field.

## Migration

None. The new field is optional and only appears while a deployment is
`deploying`.
