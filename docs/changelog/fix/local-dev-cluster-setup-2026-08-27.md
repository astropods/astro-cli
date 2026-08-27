# Make local deploys work out of the box

## Summary

A fresh local setup could not deploy an agent, and one of the three defaults
responsible stopped the server from starting at all. Each fails differently and
none says what is wrong: the server rejects its own connection string, or the
deploy request returns success and the agent sits pending forever.

## Design

**The connection string required TLS the server does not offer.**
`.env.example` shipped `?sslmode=require`, and the per-developer dev Postgres
serves no TLS, so astro-server could not connect. It now ships
`?sslmode=disable`.

Dropping the parameter looks like the tidier fix and is wrong. astro-server
connects through `sql.Open("postgres", …)`, which is `lib/pq`, and `lib/pq`
reads an absent `sslmode` as `require`. libpq and pgx both default to `prefer`
and fall back to plaintext, so the same URL succeeds in `psql` and fails in the
server. Terraform's `dev-tenant` module outputs the string with no `sslmode`
for that reason, which is correct for `psql` and misleading here.

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
`all`; a working `DATABASE_URL` already ends in `sslmode=disable`, since no
other value connects. The node label applies itself the next time you run
`local-dev.sh`.
