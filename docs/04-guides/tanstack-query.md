# TanStack Query — Data Integration Guide

## Architecture

All server data flows through TanStack Query. The layers are:

1. **`lib/api.ts`** — `ApiClient` handles HTTP (fetch, auth cookies, error parsing). No caching logic.
2. **`api/queries/*.ts`** — Domain-specific hooks (`useAgents`, `useDeployments`, etc.) wrap `useQuery`/`useMutation` and handle caching, invalidation, and enabled-gating.
3. **Components** — Call the hooks. Never call `api.*` directly for reads.

## Query Keys

All keys live in `api/queries/keys.ts`. Every `useQuery` and `invalidateQueries` call must reference a key factory — never inline string arrays.

```ts
import { agentKeys } from '@/api/queries/keys';
// Good
useQuery({ queryKey: agentKeys.detail(name), ... });
// Bad
useQuery({ queryKey: ['agents', name], ... });
```

Keys are hierarchical. Invalidating a prefix busts all children:
- `invalidateQueries({ queryKey: agentKeys.all })` clears the list AND all detail/config caches.
- `invalidateQueries({ queryKey: deploymentKeys.all })` clears the deployment list and all logs.

## Adding a New Query

1. Add the key factory entry in `keys.ts`.
2. Add a hook in the appropriate domain file (`agents.ts`, `deployments.ts`, or create a new one).
3. Re-export from `api/queries/index.ts`.
4. Use `enabled` to gate on required params so the query doesn't fire prematurely.

## Mutations and Invalidation

Mutations should invalidate the queries they affect in `onSuccess`:

```ts
useMutation({
  mutationFn: api.deployAgent.bind(api),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: deploymentKeys.all });
  },
});
```

Rules:
- Invalidate the **narrowest scope** that covers what changed. After deploying, invalidate deployments — not agents.
- Use `onSuccess` for invalidation, not `onSettled`, so failed mutations don't trigger unnecessary refetches.
- Add optimistic updates only when the UX demands it (e.g., toggling a switch). For most CRUD, letting the invalidation refetch is simpler and safer.

## Defaults

Global defaults are in `lib/queryClient.ts`:
- **staleTime: 60s** — data is fresh for 1 minute. Override per-query for volatile data (e.g., logs use `staleTime: 0`).
- **gcTime: 5 min** — inactive cache entries are garbage collected after 5 minutes.
- **retry** — no retry on 4xx errors; up to 2 retries on 5xx/network errors. Mutations never retry.
- **refetchOnWindowFocus: true** — stale queries refetch when the user tabs back.

Override at the hook level when a query has different needs. Do not change the global defaults without discussion.

## Error Handling

The `ApiClient` throws `ApiError` objects on non-2xx responses. TanStack Query surfaces these via `error` on the hook return value. Handle errors in the component:

```tsx
const { data, error, isLoading } = useAgents();
if (error) return <ErrorState error={error} />;
```

For mutations, use the `onError` callback or the returned `error` from `useMutation` — not try/catch around `mutateAsync`.

## What Not to Do

- **Don't call `api.*` directly in components for reads.** Always go through a query hook so caching works.
- **Don't create one-off `QueryClient` instances.** Use the singleton from `lib/queryClient.ts`.
- **Don't duplicate types.** Query hooks return the types already defined in `lib/api.ts`.
- **Don't use `queryClient.fetchQuery` in components.** That's for prefetching in loaders/server code. In components, use `useQuery`.
- **Don't store server data in React state.** If you find yourself doing `const [agents, setAgents] = useState(...)` with data from the server, use a query hook instead.
