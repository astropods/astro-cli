## Summary

Env vars in the deployment detail view shuffled on every poll because the server resolves ConfigMap and Secret keys by iterating Go maps (non-deterministic order), and the client displayed them as-received. A deployment with a crashed container triggers polling every 3 seconds, so users saw the list continuously reorder.

## Design

Env vars are now sorted alphabetically by key on the client after mapping:

```ts
vars: (c.env ?? []).map((e) => ({ key: e.name, ... })).sort((a, b) => a.key.localeCompare(b.key))
```

This happens in the `serviceRows` memo in `DeploymentsTab`, before vars reach `EnvVarsPanel`. The sort is stable and locale-aware, so the display is consistent regardless of what order the server returns keys in.

The continuous polling itself is expected: `hasContainerMismatch` returns true whenever any container is not ready (e.g. a pod in CrashLoopBackOff), which triggers a 3-second refetch interval until the deployment stabilizes.

## Migration

Nothing required.
