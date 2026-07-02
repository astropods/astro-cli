# Eval judgment criteria — data layer and summary counts

## Summary

Groundwork for judgment reasons on the v2 eval dataset flow (spec:
`docs/01-spec/eval-dataset-v2-judgment-reasons-spec.md`). Reviewers will soon be able to explain *why* a
trace is `good`/`bad` by selecting fixed criteria dimensions. This change adds storage, server-side
dimension validation, and dataset summary counts for those dimensions. Judgment write behavior is
unchanged; criteria writes will land with the endpoint-specific follow-up PRs.

## Design

- **New table `eval_dataset_judgment_reasons`** stores one row per selected criterion for a judgment,
  keyed by `(eval_dataset_id, trace_id, dimension_key)`. It cascades from `eval_dataset_judgments` on
  delete, and `dimension_value` is constrained to `[-1, 1]` (human selections use `-1`/`1`; the range
  leaves room for future non-human producers). `dimension_key` is deliberately *not* constrained in the
  database — the server validates it, so new criteria ship without a schema change.
- **Server-owned enum.** `judgmentstore.CriterionDimension` (accuracy, completeness,
  instruction_following, scope_clarity, tone) is the validation contract. The server stores keys only.
  An ordered `CriterionDimensions` slice backs zero-filled count derivation.
- **Dataset summary counts.** `judgmentstore.CriterionCounts` groups stored criterion rows by
  `dimension_key`, treating positive values as good and negative values as bad. The deployment dataset
  summary response includes `criteria_counts`, zero-filled for every server-known dimension so callers
  receive a stable shape even before any criteria have been recorded.

## Migration

Additive schema change only — a new empty table. No action required.
