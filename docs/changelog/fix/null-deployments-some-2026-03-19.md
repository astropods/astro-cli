## Summary

Deploying an agent from the UI threw "Cannot read properties of null (reading 'some')" even though the deployment itself succeeded. The error appeared in the mutation's optimistic cache update path, not in the actual deployment flow.

## Design

In `useDeployAgent`'s `onSuccess` handler, the code reads the cached `DeploymentsListResponse` and calls `.some()` on `deployments` to decide whether to patch the cache optimistically or invalidate the query. The optional chaining was only guarding `prev` but not `prev.deployments`:

```ts
// before — crashes if deployments is null
prev?.deployments.some(...)

// after — safe in all cases
prev?.deployments?.some(...)
```

If the cache entry exists but `deployments` is null (e.g. on a first-ever deploy before the list query has resolved), the unguarded `.some()` throws. Adding `?.` on `deployments` makes both guards consistent.

## Migration

No action required.
