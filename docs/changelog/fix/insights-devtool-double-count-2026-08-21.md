# Stop counting dev-tool spend twice in Insights

## Summary

Insights counted Claude Code spend twice: once branded as "Claude Code" and once
as "Unattributed usage". A day with $1.1K of Claude Code usage reported $2.2K,
and every surface built on the fact table inherited the error, including the stat
cards, both charts, `cost_pct`, and the People rows.

Two producers read the same traces. Claude Code shares the account's Langfuse
project with agent traces, tagged `claude-code` and carrying no `deployment:`
tag. `RollUpDevtoolsDay` filters the traces view to that tag and writes
`source='claude-code'`. `RollUpAgentsDay` queried the same view with no filter at
all, so those traces also landed under `source='agents'` with an empty
`deployment_id`, which the read path renders as the unattributed row. Both rows
carried the full amount.

## Design

The two producers now partition the traces view instead of overlapping it. A
trace tagged with a registered dev-tool source belongs to that source, so the
agent usage grain skips it. The rule is the exact complement of the dev-tool
grain's inclusion rule, which is what keeps account totals whole while removing
the overlap.

The skip happens on the returned rows rather than as a Langfuse filter. The
`tags` dimension already returns whole tag arrays, so the producer can match them
directly, and the result does not depend on how a Langfuse version implements a
negative array filter.

The unattributed row keeps its meaning: agent traces that reported no deployment.
It no longer absorbs local dev-tool usage that already has a branded row.

The model grain still reads the observations view unfiltered. Nothing reads that
grain yet, so it double counts nothing today.

## Migration

Deployments need no action, but history does not correct itself in full. Each
tick re-rolls the trailing 3 days, so the last 3 days repair themselves within a
day of the deploy. Older days keep the inflated rows until the account is
re-rolled. To repair the full 90-day window, delete the account's
`insights_rollup_state` row, which drops the watermark and forces a cold
backfill.
