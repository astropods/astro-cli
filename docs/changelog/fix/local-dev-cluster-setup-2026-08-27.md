# Make local deploys work out of the box

## Summary

A fresh local setup could not deploy an agent. Two independent defaults broke
the path, and both failed silently: the deploy request returned success, the
UI showed the agent as pending, and nothing in the logs said why.

## Design

**The queue had no consumer.** `SERVER_MODE` picks which halves of astro-server
a process runs. `api` starts the HTTP and gRPC servers with an insert-only River
client; `worker` starts the job workers and the deployment controller; `all`
runs both. `.env.example` shipped `api`, so a deploy inserted a
`deployment.deploy` job that nothing ever claimed, and the deployment sat
`pending` forever. The config default is already `all` (`config.go`), so the
example file was overriding a working default with a broken one. It now ships
`all`, with a comment explaining why hosted environments differ.

**No node accepted the pods.** Agent pods select `workload-type: tenant` so they
land on the tenant node pool instead of the system pool that carries the cluster
fabric. Hosted clusters label their pools in Terraform. A local cluster has one
node and no pools, so the selector matched nothing: every agent pod stayed
`Pending` with "didn't match Pod's node affinity/selector", and the controller
marked the deployment `Unschedulable`. `local-dev.sh` now labels the local
node as the tenant pool, which is what it is. The step is idempotent, reads
`KUBE_CONTEXT` from the server's `.env` so it cannot touch a remote cluster by
accident, and warns rather than aborting when the context is unreachable.

The alternative was suppressing the selector in local mode, in astro-server. It
was rejected: the selector is set by five separate pod-spec builders, two of
which have no local-mode flag, and skipping it would make local pod specs
diverge from the ones that run in production.

## Migration

Existing local checkouts keep their own `apps/astro-server/.env`, which
`.env.example` does not touch. If yours has `SERVER_MODE=api`, change it to
`all`. The node label applies itself the next time you run `local-dev.sh`.
