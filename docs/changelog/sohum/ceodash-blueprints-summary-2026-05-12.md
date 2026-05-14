# CEO Dashboard — Server: Blueprints Summary Endpoint

**Branch:** `sohum/ceodash-blueprints-summary`  
**Date:** 2026-05-12

---

## Summary

Adds `GET /api/v1/accounts/:account/observability/blueprints-summary` — a new endpoint that fans out to Langfuse across all account deployments and returns per-blueprint aggregated observability: cost, requests, tokens, P95 latency, and top model. This is PR 2 of the CEO Dashboard spec; it has no frontend changes and no dependency on PR 1 beyond living on the same branch.

---

## Design

### Fan-out with bounded concurrency

The handler fetches all visible deployments for the account, then fans out with `errgroup.SetLimit(10)` under a 30s `context.WithTimeout`. Each goroutine calls `fetchDeploymentMetrics`, which makes two sequential Langfuse calls per deployment: `GetDailyMetrics` (cost, tokens, requests, model breakdown) and `GetMetrics` traces view (P95 latency). Errors per deployment are swallowed — missing data is treated as zero so every blueprint row still appears.

### Aggregation by agent_name

`buildBlueprintsSummary` is a pure function (no I/O) that groups `deploymentMetrics` results by `agent_name`. Per group it sums cost/tokens/requests, takes the max P95 across deployments (conservative approximation), and picks `top_model` as the model with highest cumulative cost. Results are sorted by `cost_usd` descending.

### Zero-value guards

When `requests == 0`, `cost_per_request` and `tok_per_request` are set to `0` explicitly, preventing Go float64 division-by-zero from producing `+Inf` (which `encoding/json` serializes as `null`, breaking the schema).

### Response shape

```json
{
  "blueprints": [
    {
      "agent_name": "code-reviewer",
      "requests": 420,
      "cost_usd": 8.40,
      "cost_per_request": 0.02,
      "input_tokens": 210000,
      "output_tokens": 84000,
      "tok_per_request": 700,
      "p95_latency_ms": 1850,
      "top_model": "claude-sonnet-4"
    }
  ],
  "period": { "start": "2026-05-01T00:00:00Z", "end": "2026-05-08T00:00:00Z", "days": 7 }
}
```

When Langfuse is not configured for the account, the endpoint returns `200` with `blueprints: []` and an empty period — same pattern as the account summary endpoint.

---

## Migration

No action required. This is a net-new endpoint with no existing consumers.
