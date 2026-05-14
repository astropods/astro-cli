# CEO Dashboard — Server: Account Summary Endpoint Patch

**Branch:** `sohum/ceodashbackendpatch`  
**Date:** 2026-05-12

---

## Summary

Extends the existing `GET /api/v1/accounts/:account/observability/summary` endpoint to return all data the Activity page needs — cost totals, model breakdowns, % change vs prior period, and active-agent count. Replaces the old flat `{total_traces, input_tokens, output_tokens, time_range}` shape with a structured response. Migrates the only current consumer (`DashboardStats.tsx`) to the new fields.

---

## Design

### New query params: `from` / `to`

Replaces the unused `start_time` / `end_time` params on this endpoint. When both are present the handler makes two parallel Langfuse calls via `errgroup` — current period and prior period of equal length — and returns a pre-computed `change` object. When absent ("All"), no prior-period call is made and `change` is omitted from the response.

### Pagination in `GetDailyMetrics`

`GetDailyMetrics` in `internal/langfuse/client.go` previously made a single request, silently truncating long date ranges. It now loops through all pages (`Meta.TotalPages`) and returns a flat `[]DailyMetric`. This is a signature change: return type is `([]DailyMetric, error)` instead of `(*DailyMetricsResponse, error)`.

### Active agents: `CountActiveAgentsDuringPeriod`

New method on `deploymentstore.Store` counts distinct `agent_name` values with a deployment live during `[from, to]`:

```sql
deployed_at <= $to AND (undeployed_at IS NULL OR undeployed_at >= $from)
```

For "All" mode (`from` / `to` absent) both params are set to `time.Now()`, yielding a count of currently-active agents.

### Response shape

```json
{
  "period": { "start": "…", "end": "…", "days": 7 },
  "totals": { "cost_usd": 12.50, "requests": 1400, "input_tokens": 800000, "output_tokens": 200000, "active_agents": 3 },
  "daily_avg": { "cost_usd": 1.79, "requests": 200, "tokens": 142857 },
  "change": { "cost_pct": 18.0, "requests_pct": 12.0, "tokens_pct": null },
  "cost_over_time": [{ "date": "2026-05-01", "models": [{ "model": "claude-sonnet-4", "cost_usd": 4.20 }] }],
  "cost_by_model": [{ "model": "claude-sonnet-4", "cost_usd": 8.40, "cost_pct": 67.2 }]
}
```

`change` is omitted entirely when `from`/`to` are absent. `cost_pct` / `requests_pct` / `tokens_pct` are `null` (not computed) when the prior-period value is 0.

### `DashboardStats.tsx` migration

`total_traces` → `totals.requests`, `input_tokens + output_tokens` → `totals.input_tokens + totals.output_tokens`. The MSW mock in `handlers.ts` and the `AgentDashboard.test.tsx` inline overrides are updated to the new shape.

---

## Migration

No action required by users. The old response shape (`total_traces`, `input_tokens`, `output_tokens`, `time_range`) is gone — `DashboardStats.tsx` is the only frontend consumer and is updated in this PR.
