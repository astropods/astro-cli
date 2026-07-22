# Fix OnDelete StatefulSets stuck "deploying" forever

## Summary

A deployment whose StatefulSet uses the OnDelete update strategy (spec
`update.strategy: recreate`, e.g. the knowledge cache) could stay in `deploying`
indefinitely — across redeploys — even with its pod healthy and 1/1 ready.

The deployment controller derives per-workload health from live K8s state and
only lets a deployment go `active` once every declared workload is `ready`. For a
StatefulSet, readiness required the rollout to be settled: `CurrentRevision ==
UpdateRevision` and `UpdatedReplicas >= desired`. OnDelete never recreates pods
on a spec change — the pod keeps running on the old revision until it is manually
deleted — so those pointers lag permanently. The workload was therefore reported
`progressing` forever, and the deployment never left `deploying`.

## Design

`deriveStatefulSetHealth` now treats an OnDelete StatefulSet as `ready` on the
ready count alone (`ReadyReplicas >= desired`, generation observed): a ready pod
on the serving revision is OnDelete's steady state, not a rollout in progress.
RollingUpdate keeps its stricter revision/updated-replica check, so a genuine
rollout still reads as `progressing` until the new pod is up.

## Migration

None. Affected deployments self-heal on the next controller sync — the already-
ready StatefulSet flips to `ready`, the readiness gate passes, and the deployment
transitions to `active` without a redeploy.
