# Give local dev a cluster config

## Summary

Local dev has no cluster. Boot sync reads `CLUSTER_CONFIG_PATH` to learn each
cluster's ingress domains and observability URLs, but `buildRegistryConfig`
returned before it read the file whenever `K8S_CLIENT_MODE=local`. The env vars
that used to carry those values (`INGRESS_DOMAIN`, `INGESTION_INGRESS_DOMAIN`,
`AGENT_INGRESS_PUBLIC_DOMAIN`, `POD_SUBNET_CIDRS`, `LOKI_URL`, `PROMETHEUS_URL`,
and the rest) no longer reach `config.Load`, so nothing filled the gap.

`clustercfg.Resolve` keeps a fallback for the case where no default cluster is
configured. That fallback reads exactly those config fields, so it returned
zeros: a locally deployed agent got no ingress domain, `promquery.NewClient`
got an empty URL and returned nil, and the metrics panels had no backend.

## Design

A cluster's configuration comes from one place, its config entry, and local dev
now has an entry like every other cluster. `scripts/dev.sh` generates it, since
the cluster id, kube context, and backend URLs differ per developer and nothing
usable can be checked in. The file lands in a gitignored `.cluster-config.json`
and `dev.sh` exports `CLUSTER_CONFIG_PATH` and `DEFAULT_CLUSTER_ID` before
starting the server. Existing `.env` keys feed the generated entry, so
`LOKI_URL` and `PROMETHEUS_URL` work again by the route the developer already
expects.

`buildRegistryConfig` loads, resolves, and syncs in both modes. Only the EKS
bootstrap fields stay EKS-only: a local cluster builds its client from
kubeconfig, so its entry's EKS coordinates are inert and its CA is empty, which
`UpsertFromConfig` already permits for the default cluster.

That inertness is also why `Registry.Get` now returns the primary for the
default cluster's own id. Deployments record the primary's id rather than NULL,
so a local deploy resolves its client by id, and building one from the row
would dial placeholder EKS coordinates instead of Docker Desktop.

A local server whose cluster config is missing, unreadable, or names a
`DEFAULT_CLUSTER_ID` no entry matches now exits instead of starting. Those
values have no other source, so the alternative is a server that comes up
looking healthy and hands out agents with no URL. Managed environments keep the
existing warn-and-degrade path, where an operator is watching and the API is
worth serving without Kubernetes.

## Migration

None for deployed environments; they already mount a cluster config. Local dev
picks the change up on the next `moon run astro-server:dev`. Developers who set
ingress or subnet variables in `.env` should keep them: `dev.sh` reads them into
the generated entry.
