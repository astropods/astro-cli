# Deployment Detail Performance Spec

The deployment detail page (`/:account/agents/:deploymentId`) is slow to load because it calls `GET /api/v1/deployments?account={account}`, which fetches every deployment for the account and discards all but one. On the server, each deployment in the list triggers 5 sequential Kubernetes API calls, so a 10-deployment account makes 50 round-trips per page view. This spec covers three improvements in priority order: a single-deployment endpoint, parallelizing the list endpoint, and Redis caching. Each should be evaluated after shipping before proceeding. Section 3 (caching) should only be implemented if sections 1 and 2 are insufficient.

## 1. Single-deployment endpoint

### Problem

`DeployedAgentDetail` (`pages/DeployedAgentDetail.tsx:224`) calls `useDeployments(account)`, which hits `GET /api/v1/deployments?account={account}`, then filters the result to the single deployment matching the route param (`deployments.find(d => d.id === deploymentId)`). The list endpoint makes 5 K8s API calls per deployment in the account, so an account with 10 deployments triggers 50 K8s round-trips to render a single deployment's detail page. The cost scales with account size rather than being constant.

All 5 calls are needed, each maps to data the detail page displays:

| K8s call | Data used |
|----------|-----------|
| `CoreV1().Namespaces().Get()` | Checks if deployment is live; reads `manual_ingestions` from namespace annotations (shown in Configure panel) |
| `AppsV1().Deployments().List()` | Replica counts and readiness status shown in the Deployments tab |
| `AppsV1().StatefulSets().List()` | Workload rows for StatefulSet-based components (e.g. persistent knowledge stores) |
| `NetworkingV1().Ingresses().List()` | `external_urls`, primary URL shown in the Deployments tab |
| `CoreV1().Pods().List()` | Per-container ready status and log selection in `ActiveContainerAccordion` |

### New endpoint

```
GET /api/v1/deployments/:id
```

Query params: `account` (required, for membership verification, same pattern as existing endpoints).

Server logic:
1. Auth + membership check on `account`
2. `deployStore.GetDeploymentByID(id)` — single DB row
3. If namespace exists: run `listAstroDeployments()` for that namespace only (5 K8s calls, not 5N)
4. Apply `firstSeenAt`, audit timestamp, and avatar URL (same as list handler)
5. Return `{ "deployment": AgentDeployment }`

Handler signature mirrors `ListDeployments` — same dependencies, same middleware chain.

### Client change

New API method in `src/lib/api.ts`:

```ts
async getDeployment(account: string, id: string): Promise<{ deployment: AgentDeployment }>
```

New query hook in `src/api/queries/deployments.ts`:

```ts
useDeployment(account: string, id: string, enabled = true)
```

Query key: `deploymentKeys.detail(account, id)`, add to `src/api/queries/keys.ts`.

`DeployedAgentDetail` switches from `useDeployments(account)` to `useDeployment(account, deploymentId)`. `refetchInterval` logic (poll on transitional states) is preserved.

`useDeployments` remains unchanged, used by the list page and sidebar.

## 2. Parallelize ListDeployments

### Problem

`ListDeployments` (`handlers/deploy.go:812`) loops over `dbDeps` sequentially. Each iteration performs a namespace Get + `listAstroDeployments()`. For an account with N deployments the total wall time is `N x (k8s latency)` even though each namespace is independent.

### Approach

Convert the loop to a fan-out using `golang.org/x/sync/errgroup` (already in `go.mod`). Each deployment gets its own goroutine. Results are pre-allocated by index to avoid a mutex. K8s errors fall back to the DB-only entry and do not cancel other goroutines.

`firstSeenAt` queries run inside the same goroutine. The audit log and avatar resolution passes after the fan-out are already bulk queries and do not need parallelization.

```go
// before: sequential, total time = N x k8s latency
for _, dbDep := range dbDeps {
    result := fetchFromK8s(dbDep) // blocks until complete
    allDeployments = append(allDeployments, result)
}

// after: parallel, total time = 1 x k8s latency
results := make([]AgentDeployment, len(dbDeps))
g, gctx := errgroup.WithContext(ctx)
for i, dbDep := range dbDeps {
    i, dbDep := i, dbDep
    g.Go(func() error {
        results[i] = fetchFromK8s(gctx, dbDep) // runs concurrently
        return nil // errors fall back to DB entry, never propagate
    })
}
g.Wait()
```

## 3. Redis cache for K8s namespace state

### Problem

Even with a single-deployment endpoint, the 5 K8s API calls (`Namespaces.Get`, `Deployments.List`, `StatefulSets.List`, `Ingresses.List`, `Pods.List`) each have their own network round-trip. For users refreshing the detail page or polling during deployment, these calls repeat on every request with no reuse.

### Cache design

New package `apps/astro-server/internal/k8scache`.

Cache key: `k8s:ns:{namespace}`, one entry per namespace, which maps 1:1 to a deployment since each deployment has its own namespace.
TTL: 15 seconds. Deployment state changes (pod readiness, replica counts) are visible within one poll cycle.
Value: JSON-serialized `AgentDeployment`, the K8s-sourced fields populated by `listAstroDeployments()`. This is the data that is expensive to fetch; DB-owned fields (`id`, `name`, `display_name`, `avatar_url`, `created_at`, `updated_at`) are not cached and are merged in at read time.

Example cached value for key `k8s:ns:dep-abc123`:

```json
{
  "status": "running",
  "replicas": 2,
  "ready": 2,
  "components": ["agent", "collector"],
  "external_urls": [
    { "name": "agent", "url": "https://my-agent.astropod.ai", "type": "agent" }
  ],
  "workloads": [
    {
      "name": "my-agent-deployment",
      "kind": "Deployment",
      "component": "agent",
      "age": "3d",
      "containers": [
        { "name": "agent", "state": "running", "ready": true, "restart_count": 0 },
        { "name": "collector", "state": "running", "ready": true, "restart_count": 0 }
      ],
      "urls": [{ "name": "agent", "url": "https://my-agent.astropod.ai", "type": "agent" }]
    }
  ],
  "manual_ingestions": []
}
```

Interface:

```go
type Cache interface {
    Get(ctx context.Context, namespace string) ([]AgentDeployment, bool)
    Set(ctx context.Context, namespace string, deps []AgentDeployment) error
    Invalidate(ctx context.Context, namespace string) error
}
```

Two implementations:
- `RedisCache` — uses `github.com/redis/go-redis/v9`, enabled when `REDIS_URL` is set
- `NoopCache` — always misses, used when Redis is not configured (no behavior change in that case)

### Invalidation

Call `cache.Invalidate(namespace)` immediately after any successful K8s mutation in:
- `DeployAgent` — after spec apply
- `StopDeployment` — after scale to zero
- `WakeUpDeployment` — after scale up
- `RollbackDeployment` — after rollback apply
- `UndeployAgent` (river worker) — after namespace deletion

### Integration points

`GetDeployment` and `ListDeployments` both call `listAstroDeployments()` via the same code path. Inject the cache at the handler level: check cache before calling `listAstroDeployments()`, store result on miss.

`listAstroDeployments` signature does not change, cache wrapping happens in the callers.

### Config

Add to `apps/astro-server/internal/config/config.go`:

```go
RedisURL string // REDIS_URL — enables K8s state caching when set
```

## 4. Files to modify/create

| File | Change |
|------|--------|
| `apps/astro-server/internal/config/config.go` | Add `RedisURL` |
| `apps/astro-server/internal/k8scache/cache.go` | New: `Cache` interface, `RedisCache`, `NoopCache` |
| `apps/astro-server/handlers/deploy.go` | Add `GetDeployment` handler; inject cache in `ListDeployments` and `GetDeployment`; parallelize `ListDeployments` loop |
| `apps/astro-server/main.go` | Register `GET /api/v1/deployments/:id`; wire `k8scache` from config; call `cache.Invalidate` after mutations |
| `apps/astro-server/go.mod` | Add `github.com/redis/go-redis/v9` |
| `apps/astro-client/src/lib/api.ts` | Add `getDeployment` method |
| `apps/astro-client/src/api/queries/keys.ts` | Add `deploymentKeys.detail` factory |
| `apps/astro-client/src/api/queries/deployments.ts` | Add `useDeployment` hook |
| `apps/astro-client/src/pages/DeployedAgentDetail.tsx` | Switch from `useDeployments` to `useDeployment` |

## 5. Key decisions

**Single-deployment endpoint over client-side filtering:** the list endpoint will always over-fetch for detail-page use cases regardless of client caching because TanStack Query deduplicates by query key. `useDeployments(account)` and `useDeployment(account, id)` have different keys, so the detail page either uses the cached list (fast but stale) or fetches all again. A dedicated endpoint is the only way to make the detail page consistently fast on first load.

**15s TTL:** short enough that users see fresh pod status within one polling cycle. Long enough to absorb repeated refreshes and background polls. The invalidation-on-mutation path ensures deploy/stop/rollback actions show updated state immediately.

**Noop cache fallback:** Redis is optional. When `REDIS_URL` is unset the server behaves exactly as today. No operational requirement is added for environments without Redis.

**No K8s informers:** informers (SharedInformerFactory + Watch) would eliminate K8s API latency entirely but require a long-lived watch connection, changes to the k8s client abstraction, and careful handling of cache sync at startup. Redis caching with a 15s TTL achieves the same user-visible outcome with far less complexity.
