# Infrastructure Usage Endpoints

## Summary

The existing `/usage` endpoint only returns current-period totals scoped to quota enforcement via the OpenMeter entitlements API. There was no way to query raw compute consumption per deployment over an arbitrary time range — making it impossible to build cost visibility features that show users what their deployments are actually consuming.

## Design

Two new endpoints query the OpenMeter `compute` meter directly, bypassing the entitlements layer:

```
GET /api/v1/accounts/:account/usage/infrastructure?from=&to=
GET /api/v1/agents/:account/:name/usage/infrastructure?from=&to=
```

Both return the same shape:

```json
{
  "account_id": "...",
  "from": "2026-05-01T00:00:00Z",
  "to": "2026-05-20T23:59:59Z",
  "usage": {
    "deployment_compute": 206.70
  }
}
```

- **Account endpoint** — sums all `compute_usage` events for the account over the time range (no `groupBy`).
- **Agent endpoint** — takes an agent name (`:name`) and returns the total compute across all deployments of that agent, scoped to the account. Groups by `agent_name` and sums all result rows so every deployment contributes to the total. Returns 404 if the agent does not exist.

- `from`/`to` are optional RFC3339 params; both default to start of current month through now.
- The `usage` wrapper is intentionally extensible — `knowledge_compute` and `knowledge_storage` can be added as sibling keys in a follow-on without changing the response shape.

A new `QueryMeter` method was added to the OpenMeter client (`internal/openmeter/client.go`) to support direct meter queries with subject, time range, groupBy, and filterGroupBy parameters.

## Migration

No action required. The existing `/usage` endpoint is unchanged.
