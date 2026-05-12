## Summary

Knowledge store creation endpoints were not checking entitlements before allowing a new store to be created. Users could exceed their `knowledge_stores` and `knowledge_storage` quota limits without receiving a 402 response, even though those entitlement features were already defined and enforced for other resources (agents, members, deployments).

## Design

Both creation routes now go through `ent.Wrap()` before the handler runs, consistent with the pattern used for agents and members:

- `POST /knowledge` — wrapped with `"knowledge_stores"` and `"knowledge_storage"` (managed stores provision storage, so both limits apply)
- `POST /knowledge/connect` — wrapped with `"knowledge_stores"` only (external/BYOD stores don't consume provisioned storage)

PrivateLink endpoint creation inside `ConnectKnowledgeStore` additionally performs an inline `entCheck.Check("knowledge_endpoints")` before the endpoint record is created. This mirrors the `DeployAgent` pattern for conditional checks that depend on request body fields.

When a limit is exceeded and enforcement is enabled, the server returns 402 with the standard `ENTITLEMENT_LIMIT_REACHED` body. The OpenAPI spec for both routes now documents the 402 response.

`LimitResponse` was also fixed to align with the rest of the server's error shape: `error` is now a short label (`"Limit reached"`) and `details` is the full human-readable message string. Previously `details` was a nested object, which caused the frontend to display `[object Object]` instead of the actual message.

## Migration

No action required.
