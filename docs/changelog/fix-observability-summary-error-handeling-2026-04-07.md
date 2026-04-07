## Summary

`GET /api/v1/accounts/:account/observability/summary` was returning HTTP 503 when
Langfuse had not been provisioned for the account. This is a normal state for accounts
that have never deployed an agent, so a 503 is incorrect — the endpoint should return
an empty result.

## Design

Langfuse credentials are provisioned lazily at first deploy time via `deployer.Apply`.
Until that point, `langfuseStore.Get` returns `nil, nil` (no row, no error). The
account summary handler was treating this the same as a service failure and returning
503. The fix returns a zero-value 200 response instead:

```json
{ "total_traces": 0, "input_tokens": 0, "output_tokens": 0, "time_range": { ... } }
```

The deployment-level summary handler (`resolveLangfuseContext`) was not affected — it
only reaches this path after a deployment exists, which implies provisioning has run.

## Migration

No action required.
