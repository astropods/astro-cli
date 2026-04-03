# Redis Caching for K8s Namespace State

## Summary

Step 3 of the deployment detail performance improvements. Even with a single-deployment endpoint (step 1) and parallel K8s fetches (step 2), each request to `ListDeployments` still makes multiple K8s API round-trips per deployment on every fetch. This adds a Redis-backed cache that eliminates all K8s and per-deployment DB calls on warm requests for the dashboard list.

## Design

### Cache package

New package `internal/k8scache` defines a `Cache` interface backed by raw JSON bytes (to avoid circular imports with the `handlers` package) with two implementations:

- `RedisCache` — enabled when `REDIS_URL` is set; 15-second TTL
- `NoopCache` — always misses; used when Redis is not configured

A single key prefix scopes cache keys to this application, avoiding collisions on shared Redis instances:

- `astro:k8s:list:{namespace}` — lightweight K8s data (`Deployments` + `StatefulSets`) used by `ListDeployments`

TTL is specified per `Set` call rather than hardcoded in the cache layer. `k8scache` exports a `ListTTL` constant (15 seconds) as the default.

`k8scache.InvalidateNamespace` clears the list key for a namespace and is the single invalidation call used everywhere.

### Read path

The cache check lives inside `enrichDeployment`, before any DB or K8s calls. On a cache hit:

- No namespace `Get`
- No K8s list calls
- No `GetDeploymentFirstEventAt` DB query

Only the Redis `GET` and in-memory DB field merge run. `CreatedAt` on cache hits uses `dbDep.DeployedAt` (avoiding the per-deployment DB round-trip); the first-event timestamp is only fetched on cache misses.

On a miss the full K8s path runs, the raw K8s result is stored in Redis, then DB fields are merged before returning.

`ListDeployments` benefits — the dashboard is fast on repeat loads within the 15-second window. `GetDeployment` does not use the cache.

### Write path (invalidation)

The list cache key is cleared immediately after each successful K8s mutation:

| Mutation | Location |
|----------|----------|
| Deploy / rollback apply | `DeployWorker.Work()` after `deployer.Apply()` |
| Wake up apply | `WakeUpWorker.Work()` after `deployer.Apply()` |
| Scale to zero | `StopDeployment` handler after `k8s.StopNamespaceWorkloads()` |
| Namespace deletion | `UndeployWorker.Work()` after `deployer.Teardown()` |

### Config

`REDIS_URL` is the only new environment variable. When unset the server behaves exactly as before.

## Migration

No migration required. Set `REDIS_URL` to enable caching; leave it unset to keep the existing behavior.
