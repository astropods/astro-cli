# Human-readable workload errors on the deployment card

## Summary

When a workload failed to start (e.g. an image that can't be pulled), the
deployment card showed the tile as "Error" but no reason: the card only surfaced
errors scraped from container *logs*, and a container stuck in ImagePullBackOff
never runs, so it produces no logs. The failure reason lived on the runtime
container status but was ignored, and where it did surface elsewhere it leaked
raw Kubernetes codes ("ImagePullBackOff") that mean nothing to users.

## Design

**Plain language, server-side.** Raw K8s reason codes are translated to
explanations before leaving the server (e.g. ImagePullBackOff → "Couldn't pull
the container image", CrashLoopBackOff → "The container keeps crashing on
startup", OOMKilled → "The container ran out of memory"). Unknown codes collapse
to empty rather than leaking the code. This applies to both the runtime container
status (`message`) and the status endpoint's per-workload lists.

**Error on the card.** The deployment tile now prefers the container's own
failure explanation over a log line, so an image-pull failure shows its reason
directly on the card instead of nothing.

**Status endpoint context.** `GET /status` reports the workloads keeping a
deployment out of `active` — `waiting_on` while deploying, `failed_on` on error —
as `WorkloadIssue` items with a humanized `message`, and names them in `details`
(e.g. "Deployment failed: sasbot-agent (Couldn't pull the container image)").

## Migration

None. New fields are optional; raw `reason` codes are no longer sent on runtime
container status.
