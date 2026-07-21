# Dev-tool usage in Insights

## Summary

Local AI coding-tool usage (Claude Code today; Codex/Cursor next) now appears on the account **Insights** page as distinct sources alongside deployed-agent spend, rather than a separate view. A **Sources** filter (default all on) includes or excludes each source across the spend chart, stat cards, agents table, and — per developer — the People view. One place to see total AI spend with each dev tool broken out.

## Design

**Source-generic.** Everything keys on the `astro.source` label, so Claude Code is just the first entry in an adapter registry, not a special case. A source adapter maps a tool's OTLP metric names (cost, tokens) and a brand-icon key; adding a tool is one entry.

**One server-side pipeline.** Dev-tool usage is read from VictoriaMetrics and merged into the Insights view model **on the server, before sort/pagination/percentage** — the same pipeline that already produces the agent chart, stat cards, and tables. Sources become chart series and stat-card contributions per range, a synthetic `system` row per source in the agents table, and a roll-up into each developer's People row. Because the merge happens over the full data set (not a paginated page), sorting, search, `cost_pct`, and pagination are correct by construction, and there is no client-side re-derivation.

**People roll-up.** The server maps each developer's telemetry email to an account member via the local member-email mirror (`member_emails` — a single indexed lookup, no per-request WorkOS calls) and merges their dev-tool spend into that member's row; a developer with no matching Astro identity appears as their own email-labeled row. Each tool a developer used shows as an "agent used" chip (brand logo + name tooltip + an "External" tag). A person's total = agent + dev-tool spend.

**Per-developer visibility is gated.** The dev-tool breakdown in the People view is admin-only: account admins (the `org:admin` permission, or the sole member of a personal account) see every developer's spend; other members see only their own row folded in. Aggregate spend — the chart, stat cards, and the Claude Code agents-table row — stays visible to everyone. The gate lives in the server fold (the raw per-developer data never reaches a non-admin), reuses the existing local permission check (no extra WorkOS call), and is forward-compatible with a later reporting-hierarchy gate that widens "your own" to "you + your reports."

**Sources filter = query param.** The filter drives a `hide_sources` query parameter (source keys and/or `agents`); the server folds in only the enabled sources. Toggling refetches — consistent with how the tables already refetch on sort/search/paging — and `keepPreviousData` avoids flicker. The client renders exactly what the server returns; branding (chart colors, logos) is resolved client-side from the source registry.

**Astro brand mark.** Astro is a first-class brand icon: source SVGs added to the `astro-brand-icons` package (same CDN pipeline as every other logo) and registered in the agent-card integration registry, so our own mark resolves through `getIntegrationIconUrl` like any integration.

**Graceful-empty.** No metrics backend, a query error, or no usage → the dev-tool block is simply absent and Insights renders unchanged; it never 5xxes on the metrics path.

## Limitations / follow-ups

- **Tables are account-wide**, so dev-tool contributions to the tables reflect the widest computed window (90d); the chart and stat cards are range-scoped as before.
- **Per-developer visibility** is admin-gated (members see only their own dev-tool spend); widening it to a reporting hierarchy ("see your team") via WorkOS FGA is tracked separately.
- **Prod VM naming** (dots-preserved vs `usePrometheusNaming`) is unconfirmed; graceful-empty is the safety net until it matches.

## Migration

None. Additive fields on the existing Insights endpoint. A source's metrics light up once its ingest forwards to VictoriaMetrics and astro-server's `PROMETHEUS_URL` points at that VM; until then the source is absent and Insights is unchanged.
