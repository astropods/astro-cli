## Summary

Blueprint name availability check incorrectly blocked reuse of archived blueprint names. The server allows creating a blueprint with an archived name (it unarchives and resets it), but the client-side check treated any existing blueprint — archived or not — as a conflict.

## Design

The root cause was two-fold: the server's `GET /api/v1/agents/:account/:name` handler did not serialize `archived_at` in its response, and the client `Blueprint` type didn't include the field.

Fix: expose `archived_at` from the handler's `AgentResponse`, add it to the client `Blueprint` type, and update the availability check:

```ts
const nameIsTaken = !!existingBlueprint && !existingBlueprint.archived_at;
```

This keeps the check as a single O(1) point-lookup against `useBlueprint` rather than fetching the full account blueprint list.

## Migration

Nothing required.
