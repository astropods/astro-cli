# Map Claude Code token usage into Langfuse

## Summary

Claude Code generations arrive in Langfuse with empty usage and no cost. Every trace shows `usageDetails: {}` and `totalCost: null`, so the only view of dev-tool spend is the VictoriaMetrics path that Insights reads.

The tokens were never missing. Claude Code puts them on the `claude_code.llm_request` span under bare attribute names:

```
input_tokens, output_tokens, cache_read_tokens, cache_creation_tokens
```

Langfuse reads either the `gen_ai.usage.*` semantic convention or its own `langfuse.observation.usage_details`, and Claude Code emits neither — it only sets `gen_ai.request.model`, `gen_ai.system`, and the response fields. astro-otel forwards trace spans untouched apart from identity and tags, so the counts reached Langfuse and were ignored.

This is a translation gap, not a collection gap.

## Design

astro-otel gains `mapLangfuseUsage`, applied to span attributes in the traces path alongside the existing `tagClaudeCode` and `mapLangfuseIdentity`. Spans carrying no token attributes pass through untouched, so only `llm_request` generations are affected and no empty usage object is attached to tool or interaction spans.

Target key names come from Langfuse's own `default-model-prices.json`, which prices `cache_creation_input_tokens` and `cache_read_input_tokens` alongside `input` and `output`. Matching those exactly is what makes the counts priceable rather than merely recorded.

Cache keys are not optional. A single measured interaction carried 21,388 cache-creation and 18,611 cache-read tokens against 2 input and 73 output — cache traffic is the overwhelming majority of Claude Code's volume, and mapping only input/output would understate cost by orders of magnitude while looking plausible.

**Cost is deliberately not forwarded.** Claude Code computes a `cost_usd` client-side and reports it on the `claude_code.api_request` log event, which astro-otel currently drops. Sending tokens and letting Langfuse price them keeps one costing method across dev-tool and agent traffic, rather than mixing an exact figure for one source with a derived figure for the other. It also avoids a second pricing path to keep current.

This rides the **traces** signal, which the managed-settings block enables unconditionally — unlike the logs signal, which is gated behind the prompt-collection opt-in. No settings change is required and it applies to every instrumented account.

astro-otel also gains `godotenv.Load()`, matching astro-server and astro-registry, so a local `.env` is read at startup. A documented `.env.example` accompanies it; a repo-level `!.env.example` negation keeps new templates trackable, since the existing `.env.*` rule would otherwise silently exclude them.

## Migration

None required. Behaviour is additive: spans without token attributes are unchanged, and the new attribute is ignored by anything that does not read it.

Cost only materialises once the account's Langfuse instance has a model definition matching the reported model. Two known gaps sit outside this change:

- Langfuse's bundled price list has no `claude-opus-5` entry, so that model records tokens and prices at zero until a definition is added.
- Model definitions are applied by the Langfuse **worker** from a shipped constant, not by database migrations. An instance whose worker is unhealthy keeps whatever list it was seeded with at database creation and never updates.

Both surface identically — tokens present, cost zero — which is worth knowing before treating a zero as absence of usage.

---

# Source Claude Code Insights from Langfuse

## Summary

With tokens mapped, Langfuse can price Claude Code generations, so Insights no longer needs a second telemetry pipeline for dev-tool spend. Both read paths — the v1 page and the v2 daily roll-up — move from VictoriaMetrics PromQL to the same Langfuse metrics query the agent path already uses.

Agent spend and dev-tool spend were sourced differently for no reason other than history. This collapses them.

## Design

One query per source replaces four PromQL queries per source per range: the traces view, dimensioned by `tags` and `userId` with a day time-dimension, so each row is a (day, tag-set, developer) cell. Dev-tool traces carry no deployment, so the source tag is the only scope.

Cells are kept unfolded and sliced per range, so the widest window is fetched once and every narrower range is derived from it rather than re-queried.

Three pieces of machinery disappear because they existed only to work around `increase()`. `devtoolTodayCost`, `applyTodayBucket` and the window-total-versus-daily-sum split were all compensating for PromQL dropping the current partial day; a day-granularity time dimension simply returns it.

**Request counts become real.** The dev-tool agent row hardcoded `Requests: 0` because no request metric was emitted, which also forced `cost_per_request` and `tok_per_request` to zero. Langfuse's traces view counts them.

**Zero cost is now a distinguishable state.** Langfuse derives cost from a model definition, so an unpriced model produces a real zero that looks exactly like no usage. `CostUnavailable` is set when tokens are present but cost is zero, logged on both read paths and exposed on the API so a client can render it as a fault rather than as `$0`.

## Migration

**Historical Claude Code spend is not carried over.** Langfuse holds no usage for generations that predate the token mapping, so dev-tool history in Insights starts from deploy. VictoriaMetrics retains the old data; nothing reads it.

Cost depends on the account's Langfuse instance having a model definition matching the reported model. Two known gaps make that fail closed rather than loudly:

- Langfuse's bundled price list has no `claude-opus-5` entry.
- Model definitions are applied by the Langfuse worker from a shipped constant, not by database migrations, so an instance whose worker is unhealthy never updates its list.

In both cases tokens and request counts are still correct and `CostUnavailable` is set. Deploying before those are resolved shows Claude Code usage with no spend attached.

VictoriaMetrics is untouched and still receives dev-tool metrics; Prometheus continues to serve pod metrics, network timeseries, message-count metering, and the observation engine.
