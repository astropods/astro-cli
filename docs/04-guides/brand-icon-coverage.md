# Brand icon coverage

Brand icons in `packages/astro-brand-icons` are authored by hand. This is how you find out which ones to author next, from what agents actually call rather than from guesswork.

## Pulling the list

```
queen prod icons domains --json > /tmp/domains.json
bun run --cwd packages/astro-brand-icons unresolved /tmp/domains.json
```

The first command aggregates every outbound destination across the fleet and reports two numbers per registrable domain: total requests, and how many distinct deployments called it. The second filters that against the shipped manifest and ranks what has no icon, breadth first — a domain many deployments hit is worth more than one chatty agent's private endpoint.

Flags on `icons domains`:

| Flag | Default | Notes |
|---|---|---|
| `--days` | 30 | Lookback window. Capped at 30 — the metrics store's retention, so a longer window would report coverage it cannot have. |
| `--limit` | 200 | Top N domains by request count. Capped at 2000. Ranking happens before the script's noise filter, so bare IPs can occupy slots. |
| `--json` | off | Required for the filter script. Table output is for reading. |

Resolution deliberately lives in the TypeScript script, not the server: the rules read that package's manifest, and a second implementation in Go is how two alias tables drifted apart before. The server extracts domains; the package decides coverage.

## Requirements and failure modes

The RPC needs `PROMETHEUS_URL` set on astro-server and returns `FailedPrecondition` when it isn't. It is on-demand only — there is no job kind and no schedule, because a fleet-wide aggregation over a month of high-cardinality series is expensive and the consumer is a human deciding what to draw.

Expect it to take a while. The CLI allows five minutes and the server caps each query at four, but the metrics backend has its own ceiling that is lower — VictoriaMetrics defaults `-search.maxQueryDuration` to 30s and the managed chart does not override it, so in practice that is the real limit.

If the query fails, the server logs `Outbound domain edge query done` with an `edge_series` count. That query returns one series per (host, deployment) pair and is the only part of the request without a cardinality ceiling — `--limit` cannot bound it, because ranking happens after hostnames are folded onto their domain. Narrow `--days` first.

Coverage is bounded by whatever `PROMETHEUS_URL` points at. The cluster matcher spans every registered cluster, but if a cluster's metrics live in a store the server does not query, its agents contribute nothing and nothing says so.

## Adding the icons

Each icon is a light/dark SVG pair under `packages/astro-brand-icons/sources/` plus an entry in `icons.json`. Use authoritative logos in the vendor's brand colours — not monocolour traces of them — and check the result with `moon run astro-brand-icons:dev`, which renders the whole set against both backgrounds.
