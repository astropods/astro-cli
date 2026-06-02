## Summary

Operators debugging preview/prod agents had no CLI equivalent for what the `/agents` dashboard shows (activity sparkline, last-seen heartbeat, trace totals). `ast agent trace` only talked to Langfuse live for the Monitor trace list, and it failed to decode list responses when Langfuse returned object-shaped `input`/`output` (`failed to decode response: json: cannot unmarshal object into Go struct field … output of type string`).

## Design

`ast agent trace` now has three explicit modes, each aligned with a different UI surface and backend path:

| Mode | Flag | API | Same as UI |
|------|------|-----|------------|
| Trace list (default) | — | `GET …/deployments/{id}/observability/traces` | Agents → deployment → **Monitor** (paginated trace table) |
| Trace detail | `--trace-id` | `GET …/observability/traces/{traceId}` | Monitor → trace **Overview** (I/O, observations, scores) |
| Activity summary | `--summary` | `GET …/accounts/{account}/observability/deployment-summaries` | **`/agents` card** (sparkline + last active) |

**Why `--summary` is separate.** The agents page does not load Langfuse on every request. `ObsSummaryRefreshWorker` pulls Langfuse metrics on a ~10m cadence and writes per-deployment entries to Redis (`obssummary`). The bulk handler only reads that cache. That is why a flat EU card can still show a heartbeat while `agent trace` (live traces) returns 502 when Langfuse/ClickHouse is unhealthy — different failure domains. `--summary` exists so CLI checks match “what does the card show?” without implying traces are queryable right now.

**What `--summary` renders (human output).** After resolving `--name` / `--id` to a deployment, the CLI prints the cached entry for that deployment id:

- **Total traces** — count in the summary window (card request count).
- **Last active** — relative time plus raw `last_trace_at` RFC3339 timestamp.
- **Requests / Tokens (30d)** — one Unicode block per day (oldest → newest), height scaled to the series peak, plus a stats line (`total`, `peak/day`, `active days`). Large token totals use compact `k`/`M` suffixes. This mirrors the card sparkline shape without dumping 30 comma-separated integers.

If the deployment is missing from `summaries` (new deploy, worker not run yet, or refresh failed), the CLI reports that no cached summary exists yet (~10m refresh), same as the UI hiding the sparkline.

**`--json`.** Machine-readable `{ deployment_id, display_name, summary }` with full `request_series` / `token_series` arrays for scripting; no sparkline encoding in JSON.

**Decode fix.** List `traceEntry` uses `json.RawMessage` for `input` and `output`, consistent with trace detail and Langfuse’s variable payload shapes, so Monitor-style listing works when fields are objects rather than strings.

`--summary` cannot be combined with `--trace-id` (summary snapshot vs single-trace drill-down).

Public Fern docs (`docs-public/fern/docs/pages/cli-reference.mdx`, `managing-agents.mdx`) and `docs/02-cli/cli-command-tree.md` document the new flag. `apps/astro-cli/CLAUDE.md` now requires Fern updates alongside CLI changes.

## Migration

No changes required. Rebuild or `ast upgrade` after release to pick up the CLI.

Bump `apps/astro-cli/VERSION` to **0.14.1** for release.
