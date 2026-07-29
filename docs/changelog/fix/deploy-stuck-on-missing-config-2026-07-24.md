# Fix: deployments no longer hang when a pod references a missing env source

## Summary

A deployment whose pod referenced a ConfigMap/Secret that did not exist would
sit in `deploying` indefinitely and block all redeploys for that agent until the
30-minute stale-deployment watchdog eventually failed it. The trigger seen in
production was a malformed secret input (a stray trailing character) that left an
agent pod wedged on `configmap "<agent>-<build>-config" not found`. The
deployment status never moved off `deploying`, so the user could not redeploy a
fix — they had to wait out the watchdog.

## Design

Two independent gaps combined to produce the hang; both are closed.

**Controller did not treat missing-config as terminal.** The deploy controller
classifies a pod as permanently failed based on its container waiting reason
(`permanentWaitReasons`). The kubelet reports `CreateContainerConfigError` when a
container references a ConfigMap/Secret that does not exist — but only the
distinct `CreateContainerError` was in the set. Missing-config pods were
therefore treated as transient and never failed the deployment. Added
`CreateContainerConfigError` to the terminal set, so such a pod fails after the
normal 90s grace and the deployment transitions to `failed` immediately, making
it redeployable instead of waiting ~30 minutes.

**Applier created workloads over a failed env source.** `ApplyDeploymentSpec`
applied the agent's shared Secret and ConfigMap, and on error merely recorded it
in `result.Errors` while continuing to build the StatefulSet whose `envFrom`
still referenced them — guaranteeing a wedged pod. The applier now aborts with an
error before creating any workload when an agent env-source apply fails. The
deploy worker already maps a non-nil apply error to `failed`, so the deployment
fails fast with the specific resource error instead of producing a dangling
reference.

Note: a `%` (or other special character) in a secret value is valid in K8s
Secret/ConfigMap data and is stored verbatim — confirmed by a value round-trip
test. The fixes address the failure *handling*, so any cause of a missing env
source (rejected apply, malformed input) now surfaces fast and recoverably rather
than hanging.

## Migration

None. Existing stuck deployments still clear via the watchdog; new occurrences
fail fast and can be redeployed immediately.
