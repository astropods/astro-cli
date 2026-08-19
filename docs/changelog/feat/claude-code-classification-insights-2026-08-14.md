# Claude Code prompt classification

## Summary

Insights shows how much Claude Code costs but not what it was used for. This adds a pipeline that labels each prompt by **purpose** (work / personal / ambiguous) and **topic** (15 categories), and stores daily aggregates keyed to the spend Insights already reports.

Two of the three Foundry work-classifier heads are consumed. The **task** axis is deferred: it is the only per-turn head, it needs preceding-turn context that cannot be assembled without session grouping, and Claude Code does not currently send a session id.

Clicking the Claude Code row in the Insights agents table opens a source detail page showing that breakdown, and naming the developers behind it — a category spike is only actionable once you can see who caused it.

## Design

**Prompts come from Langfuse, not Postgres.** Claude Code traces are tagged `claude-code` and carry the prompt on `trace.input`. Reading them required a new capability: every existing trace query applied a `deployment:<id>` tag filter, and dev-tool traces have no deployment, so `GetDevtoolTraces` scopes by source tag instead. An unscoped read is refused outright rather than silently returning every trace in the account's shared project.

**Classification is per trace.** The purpose and topic heads were trained at conversation level, but their 256-token cap means they effectively saw the opening turns of a conversation, and no session id exists to group by. A single interaction is close enough to that distribution; `unit_kind` is retained in the schema so session-level rows can land later without a migration.

**The pipeline is a River worker on its own queue**, deliberately separate from the insights roll-up: it depends on an external inference service, and a Foundry outage must lag labels without stalling spend reporting. Worker concurrency is capped at 2 because the classifier heads share a CPU-inference pool that degrades past roughly eight in-flight requests.

**Batching is not an optimisation, it is the design.** A single-item request costs ~480ms against ~9ms per item batched. Prompts already labelled at the current model version are filtered out in Postgres before anything reaches the network, so re-running a day costs one SELECT.

**The backfill walks backward.** Two cursors bound a `[backfilled_from, classified_through]` window; the forward edge advances daily while history fills in behind it, so the most recent day — the one anyone looks at — completes first. A forward walk would spend hours on ancient days before the page showed anything.

**Cost is partitioned, not recomputed.** Each developer's daily spend, taken from the same figure Insights reports, is split across the labels their prompts carried. Segments therefore always sum to the number on the page that links to them, rather than arriving from a second pipeline that would drift. The tradeoff is that every prompt in a developer's day is treated as equally expensive.

**The backfill is sized for retraining, not for the page.** Labelled prompts are a training corpus feeding continuous retraining of the classifier heads in the background, so the pass reaches back 400 days — as far as the ingest key — while the widest range the page offers is 90. Depth is bounded by available telemetry rather than by what any UI renders.

That is also what settles the cost source. Reading spend from `insights_usage_daily` would make segments sum to the page total by construction, but the roll-up only holds 90 days, so everything older would price at zero. Langfuse is the only source covering the full window.

**The read path is one query and four windows.** A request fetches the widest range once and slices it per range in Go, so the range selector never refetches.

**A wide axis folds rather than truncates.** Topic has 15 labels, more than a stacked bar or a palette reads. The top eight by cost are kept and the tail collapses into the axis's own remainder label, marked `aggregated` so the legend can say so. The chart and the table apply the same fold, so their segments always agree, and the folded labels still sum to the axis total.

**Coverage is reported, not inferred from emptiness.** Prompts with no labels, labels with no cost, and no usage at all are three different stories with three different actions — content collection is off in a console Astro does not control, the model is unpriced in Langfuse, or nobody used the tool. The payload distinguishes them rather than rendering all three as an empty chart.

**Both axes render together, and the charts are the filter.** Purpose and topic answer different questions about the same prompts, so a toggle made comparing them a memory exercise. A donut arc or legend row selects a category; a bar selects a day. On a time series the x position is what a reader is pointing at, so a stacked segment cannot also mean a category — that ambiguity is why the two dimensions live on different charts. Unselected segments dim rather than disappear, because a category has to be read against the rest and removing the others would rescale the axis mid-read.

A day narrows only the named breakdown, so the chart keeps the range and the spike stays visible while the table explains it. It is a scoped query on its own cache key, so drilling in and back out is served from cache.

**The page names who, not just what.** `insights_classification_daily` is keyed on the actor and the charts already fetch the widest window, so the per-developer breakdown is a second fold of rows in hand rather than a new query. Each label reports the share of that person's *own* prompts beside the count:

```
        personal   of their own
busy         10          10%
quiet         8          80%
```

Count ranks `busy` first every time; the share surfaces the outlier. Axis columns can also be switched to measure a named label, so ranking by percent personal is a sort rather than a hunt — someone at 45% personal otherwise renders as "Work" and disappears. Person totals come from a single axis rather than summing them, since every prompt is labelled on each axis.

**Visibility splits by surface, not by page.** Charts stay account-wide for everyone; the named breakdown shows admins everyone and members only themselves. The aggregates legitimately need every actor's rows, so that restriction is applied in Go rather than SQL. A viewer whose dev-tool address is not linked to their account matches no row, which is reported as its own state — "we cannot tell which prompts are yours" and "you have no prompts" call for different actions.

**Colour is per axis, keyed to a label's declared slot.** A shared palette could not colour the axes independently, since each axis's first label occupies slot 0. Keying to position in a response would repaint the chart whenever the fold reordered it, which happens on every range change.

## Migration

Apply `sql/astro-server/schema.sql` — three new tables: `trace_classifications` (per-trace labels, primary key doubling as the idempotency guarantee), `insights_classification_daily` (daily aggregates), and `classification_state` (the two-cursor watermark).

Two new environment variables, `FOUNDRY_INFERENCE_URL` and `WORK_CLASSIFIER_VERSION`. Both must be set together — startup fails otherwise — and leaving both unset disables classification entirely, with the workers becoming no-ops. The version is a stamp on stored labels, not something the serving API exposes, so it must track `work_classifier_versions` in the astro-infra Terraform pin. Drift is silent: a retrained model served under a stale stamp is never re-classified.

## Open items

- **Dev-tool addresses are not linked to members.** Attribution joins `account_member_emails`, which mirrors one WorkOS address per user, while Claude Code reports whatever address the developer's CLI is signed in as. Where they differ every actor stays unidentified, so the breakdown shows bare addresses and the member visibility rule matches nothing — only the admin view is real today. The schema already anticipates the fix: `source` distinguishes WorkOS-synced emails from directly added ones, direct-add is unimplemented, and the attribution lookup applies no source filter, so a row of any source resolves it with no producer change.
- **Public documentation.** The customer-facing page covers spend but not classification — what the categories are, or that prompts are categorised at all.
- **Retention.** `trace_classifications` has no TTL or purge sweep; rows are removed only by account cascade. Because the table doubles as the retraining corpus, a TTL trades corpus size against storage rather than just trimming a cache. Storing `purpose` per developer was considered and accepted; how long those rows live was not.
