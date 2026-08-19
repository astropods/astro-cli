# Claude Code Classification Insights

## Summary

Claude Code prompts land in each account's Langfuse project but have no product surface — the [prompt-collection spec](devtool-prompt-collection-spec.md) was explicitly collection-only, feeding the offline `astro-models` corpus. Meanwhile Insights shows Claude Code *spend* (from VictoriaMetrics) with no indication of what the spend was for.

This feature classifies those prompts and surfaces the result: clicking the Claude Code row in the Insights agents table opens a source detail page showing what the account uses Claude Code for.

Two axes ship: **purpose** (work / personal / ambiguous) and **topic** (15 categories). The **task** axis is deferred — it is the only per-turn head, it needs preceding-turn context we cannot assemble without session grouping, and the model needs more work.

## Classification service

Three ModernBERT heads served by KServe in the Foundry cluster, one InferenceService per axis, provisioned in `astro-infra` (`terraform/environments/foundry/serving_workloads.tf`). Only two are consumed here.

```
POST https://inference.foundry.astroids.ai/v1/models/work-classifier-{purpose,topic}:predict
```

Measured characteristics that drive the design:

| Property | Value | Consequence |
|---|---|---|
| Batch form | `{"instances":[{"text":...},...]}` | Required; a bare array of strings throws a tokenizer error |
| Single item | ~480 ms | Never classify one at a time |
| Batch of 256 | ~9 ms/item | ~50x throughput; batch is the only viable mode |
| Concurrency | degrades past ~8 in-flight | Small worker pool; cap the queue low |
| Determinism | bit-identical scores | Results are cacheable/idempotent by content |
| Truncation | 512 tokens server-side | Long prompts clip; irrelevant at our input sizes |
| Output | top-1 label + score only | No distribution, no `top_k`; runner-up views impossible |
| Version | not exposed over HTTP | Must mirror the Terraform pin as server config |

No auth (IP-allowlisted per `astro-infra` decision 0007), no scale-to-zero, no model-list endpoint. `GET /v1/models/{name}` returns `{"name":...,"ready":true}` and serves as the health probe.

Model version is pinned in `astro-infra` at `terraform.tfvars` (`work_classifier_versions`, currently `27d22b8f54bc-ganesha` for all axes). Since the API exposes no version, astro-server carries its own config value that must be updated in lockstep. Drift is silent — see [Open items](#open-items).

## Input granularity

`fine_tune.py` trains purpose and topic at conversation level (`conv_text()` joins all turns) with `MAXLEN = 256`. Task is per-turn with an 8-turn context window.

We classify **per trace**. One Claude Code trace is one `claude_code.interaction` — a single user prompt plus its full response — and `trace.input` carries the prompt verbatim.

This is a deliberate mismatch with training, accepted for two reasons: the 256-token cap means the heads effectively saw only the opening one to three prompts of a conversation anyway, so a single substantive prompt is near-distribution; and Claude Code is not currently sending a session id, so conversation grouping does not exist to classify against. Expect softer accuracy on terse, ambiguous prompts.

`unit_kind` is retained in the schema so session-level rows can land later without migration.

## Data flow

Prompts are read from Langfuse, not Postgres. Claude Code traces are tagged `claude-code` and carry `trace.input` (prompt), `trace.output` (response), `userId` (email, since the July identity mapping), and a null `sessionId`.

**A new Langfuse read path is required.** Every existing trace query in `internal/langfuse/client.go` applies `traceFilter.deploymentID`, and Claude Code traces have no deployment — there is currently no way to list them from astro-server at all. The new query filters by tag and requests `fields=core,io`.

A River worker on a dedicated queue processes one (account, day) at a time, mirroring `insights_rollup.go`:

1. Fetch `claude-code` traces for the day
2. Skip traces already classified at the current model version
3. Batch remaining prompts (~256) to each axis
4. Write results
5. Advance the watermark

Worker count stays low (2) so concurrent account backfills cannot stampede the ISVCs.

Hourly cadence. Labels do not need to be fresher than the daily aggregates they feed.

## Storage

Two tables. Classification results are stored per trace; aggregates are precomputed for the charts.

`trace_classifications` — one row per (account, trace, axis). The primary key is the idempotency guarantee: a trace is never classified twice, and re-runs are safe upserts. `model_version` sits outside the key so a retrain overwrites rather than accumulating history, and stale rows are found with `WHERE model_version <> $current`.

`insights_classification_daily` — per (account, day, source, axis, label, actor), carrying trace counts and attributed cost.

This is a separate table rather than a new grain on `insights_usage_daily`, whose `grain` CHECK and `insights_usage_daily_shape_check` are already carrying significant load and whose dimensions (model, deployment_id) do not apply here.

No Redis cache. The results table already prevents duplicate work, which dominates any content-addressed hit rate — most prompts are unique, and the deterministic-hash cache should be added only if measured hit rate justifies it.

## Cost attribution

Claude Code spend is read from Langfuse, the same source as deployed-agent spend.

Cost per label is computed by partitioning that figure, per user per day, by that user's label shares:

```
cost(user, day, label) = spend(user, day) × share_of_that_user's_prompts_labelled
```

`fetchDevtoolUsage` produces the per-user spend and Langfuse traces carry the same email, so segments always sum to the number the main Insights page shows — consistency by construction rather than by two pipelines agreeing.

Limitation: every prompt in a user's day is treated as equally expensive. Given observed cache-token ratios (18,611 cache-read against 73 output on one interaction), per-prompt cost varies enormously, so segment splits are approximate while totals are exact.

A day whose spend lookup fails is not written at all. `ReplaceDayAggregates` is a full replace and a backfilled day is never revisited, so writing on a failed lookup would zero that day's cost permanently.

Exact per-prompt cost is available — Claude Code's `claude_code.api_request` log event carries `cost_usd` and all four token counts, and astro-otel currently drops it via `synthesizeSpan`'s default case. Wiring it would allow token-weighted shares. Deferred; see [Open items](#open-items).

## Backfill depth

Classification backfills 400 days, as far back as the ingest key. `insights_usage_daily` holds 90, matching the widest range Insights offers.

**The page is not the reason the backfill is deep.** Labelled prompts are a training corpus: they feed continuous retraining of the work-classifier heads in the background, whether or not any range renders them. Depth is therefore bounded by what telemetry exists, not by what the UI can show, and days 91–400 are the point of the exercise rather than a side effect of it.

`trace_classifications` stores the label, score, model version and trace id; the prompt text stays in Langfuse. The corpus is the join of the two, assembled at training time — which is also why the per-trace table exists at all rather than only the daily aggregates the page reads.

That depth is what fixes the cost source. Reading spend from `insights_usage_daily` would make segments sum to the page total by construction and would drop the ordering dependency between the two producers, but it would zero cost for every day past 90. Langfuse is the only source that reaches the whole window. Inside 90 days both paths call the same `fetchDevtoolUsage` with the same bounds, so they agree unless traces land between the two runs; the roll-up re-rolls a trailing 3 days while classification re-runs today and yesterday, so a trace arriving 3 days late updates one and not the other.

## API

`GET /api/v1/accounts/:account/insights/sources/:source`

Returns the same four-range envelope as the Insights payload so the range selector stays refetch-free. One fetch at the widest range is sliced per range in Go; a request is a single query against `insights_classification_daily`.

`?day=YYYY-MM-DD` narrows the per-developer breakdown to one UTC day without touching the charts, so a spike stays on screen while the table answers who caused it. It is a separate query key on the client, so drilling in and back out is served from cache. A malformed value is dropped rather than guessed at, since silently widening the drill back to the range would misreport who was involved.

`Coverage` is a first-class part of the payload: the classified window, whether the backfill finished, whether any content exists, and whether spend is priced. These are distinct failure modes with different answers — a source with prompts and no labels means content collection is off in a console Astro does not control, and one with labels and no cost means the model is unpriced in Langfuse. Collapsing them into emptiness sends the reader after the wrong thing.

The Claude Code agents-table row links to `/insights/sources/claude-code?account=<name>`. Absolute and account-stamped by necessity: the table only routes an href beginning with `/` client-side, and anything else becomes a plain anchor that reloads the page and drops the query string — taking the account scope with it.

## Per-developer breakdown

The charts answer *what* the tool is used for; they cannot answer *who*, which is the question a spike actually raises. The aggregate table carries `actor_kind`/`actor_key`, so the named breakdown is a second fold of rows the charts already fetched — no extra query.

Each label carries the share of that person's **own** prompts alongside the count. Ranking by count only re-finds the heaviest users; the share is what separates unusual from busy, and it is the column a governance question sorts on. Axis columns default to a person's top category but can be switched to measure a named label, because "who is most personal" is invisible when the column only ever reports each person's largest category — someone at 45% personal renders as "Work".

Totals come from one axis rather than summing them: every prompt carries a label on each axis, so adding across axes double-counts it.

**Visibility.** Admins see everyone; a member sees only themselves. The charts stay account-wide either way — they are aggregates over the whole account and legitimately need every actor's rows, which is why the restriction is applied in Go rather than in SQL. An earlier version gated the whole page on the admin check and rendered nothing at all for a member, which is the failure mode this split exists to avoid.

A restricted viewer whose dev-tool address is not linked to their account matches no row. That is reported as its own state, not as an empty table: "we cannot tell which prompts are yours" and "you have no prompts" call for different actions. Until direct-add lands (see [Open items](#open-items)) this is the common case, and the member half of the visibility model is inert.

## UI

New route `insights/sources/:source` (the existing `insights` route is flat, not nested). Page follows the Insights conventions: TanStack Query hook using `INSIGHTS_QUERY_OPTS`, a key factory entry in `observabilityKeys`, a loader with `usePrimeQueryCache` for SSR hydration, filters through `usePersistentSearchParams`, and the same `AccountScopeFilter` so a scope arriving on the URL survives.

Both axes render together rather than behind a toggle: they answer different questions about the same prompts, and switching made comparing them a memory exercise.

**Selection.** Clicking a donut arc or legend row selects a category; clicking a bar selects that day. On a time series the x position is what a reader is pointing at, so a stacked segment cannot also mean a category — that ambiguity is why the two live on different charts. Unselected segments dim rather than disappear, since a category has to be read against the rest and removing the others would rescale the axis mid-read. Selection is component state, not a URL param: it is a lens on a payload already loaded, and it clears on a range change where the clicked segment may not survive the fold.

A person's row expands to their full distribution on both axes. The expanded panel is deliberately read-only — filtering from inside it re-sorted the table underneath the open row, so the row jumped and read as a different panel opening by itself.

**Colour.** One palette per axis, keyed to a label's slot in that axis's declared space rather than its position in a response, because the tail fold reorders that list per range. A shared palette could not colour the axes independently, since each axis's first label is slot 0. Saturated theme shades only: the neutral families read as disabled rather than as a category, and the pale tints are hard to tell apart.

Topic has 15 labels — more than any palette or stacked bar reads cleanly. The server keeps the top eight by cost and folds the tail into the axis's own remainder label, marked `aggregated`. The fold happens once, server-side, so the chart and the table cannot disagree about which segments exist.

## Coverage and empty states

Prompt collection is opt-in via `OTEL_LOG_USER_PROMPTS`, which lives in each enterprise's Anthropic admin console and cannot be read or pushed by Astro. Accounts with telemetry but no content flag have full spend charts and zero classifications.

"Content collection not enabled" is therefore a first-class empty state, distinct from "no usage". The server cannot detect the flag directly and infers it from the absence of trace input over a window.

`OTEL_REDACT_ATTRIBUTES` on astro-otel strips prompt bodies if ever enabled. The worker must treat empty input as permanently unclassifiable rather than retrying.

## Privacy

The `purpose` axis labels prompts work / personal. Rendered per named developer in an account-admin view, that is an employee-monitoring surface built on content collected under terms written for offline model training.

**Decided: `purpose` is stored per developer**, in the same actor key space as the other axes. Per-user attribution was considered and accepted rather than defaulted into.

The per-key email exclusions on `otel_ingest_tokens` are the opt-out and work without change here: excluded users' content never reaches Langfuse, so there is no prompt text to classify and no row is written for them.

## Open items

- **Model version drift.** The Foundry API exposes no version; astro-server's config value must be updated whenever `work_classifier_versions` changes in `astro-infra`. Needs a runbook note, or a version endpoint on the serving side.
- **Session grouping.** `OTEL_METRICS_INCLUDE_SESSION_ID` (misleadingly named — it gates traces and logs too) is `false` in the managed-settings block, which is why `sessionId` is null. Flipping it would enable conversation-level classification, but `session.id` lands on metric *datapoint* attributes, which astro-otel does not strip (it only clears resource attributes) — so flipping it without a datapoint-level strip pipes unbounded cardinality into VictoriaMetrics.
- **Exact cost attribution.** Wiring `claude_code.api_request` into `synthesizeSpan` would give per-trace cost and enable token-weighted segment splits.
- **Task axis.** Deferred pending model work and session grouping.
- **No auth on the serving side.** Prompt text is POSTed unauthenticated (IP-allowlisted per astro-infra decision 0007), and requests carry no account identifier, so a Foundry-side incident cannot be scoped to affected accounts. `Client.predict` is the single chokepoint if that changes.
- **Dev-tool addresses are not linked to members.** Attribution joins `account_member_emails`, which mirrors one WorkOS address per user; Claude Code reports whatever address the developer's CLI is authenticated as. Where they differ every actor stays `unidentified`, so the named breakdown shows bare addresses and the member visibility rule matches nothing. The schema already anticipates the fix — `source` distinguishes WorkOS-synced emails from ones added directly, and direct-add is unimplemented. `EmailsForAccount` has no source filter, so a row with any source resolves attribution with no producer change.
- **No retention policy.** `trace_classifications` has no TTL or purge sweep; rows are removed only by the account foreign-key cascade. A TTL is not simply a storage-cost decision here — the table is a training corpus, so expiring rows shrinks the retraining set. Whatever policy lands has to be set against corpus value, not against table size.
