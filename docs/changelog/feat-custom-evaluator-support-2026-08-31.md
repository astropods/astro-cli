# Custom evaluator support: persistence and document parsing

## Summary

Every agent currently runs the same hardcoded default evaluation set, with
no way for a builder to select different presets or define their own
evaluators. This lays the groundwork for per-agent evaluation sets: storage
for a builder-published set, and a parser that turns a builder's
`EVALUATION.yaml` into one. Nothing calls either yet, so no agent's active
evaluation set changes as a result of this change.

## Design

`eval_definitions` stores a builder's published evaluator set,
content-addressed and immutable. `agent_evaluations` is the mutable pointer
from an agent to its active definition; no row means the agent still uses
the default set. Both get store packages (`evaldefinitionstore`,
`evalagentstore`) following the project's usual store shape.

`evaldocument.Parse` validates a builder's YAML against the
`evaluation/v1` document contract (schema, evaluator count, unique keys,
preset references, prompt files) and computes a stable content-addressed
ref for the result. Custom evaluators can also carry an optional
`description`, matching what preset evaluators already expose.

## Migration

Apply `sql/astro-server/schema.sql` before deploying astro-server. Adds two
tables, both additive with no backfill.
