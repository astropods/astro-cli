# Replace judgment criteria (PUT) + criteria in Langfuse metadata

## Summary

Reviewers can set the criteria (reasons) for an already-judged trace without changing its verdict, and
those criteria now appear on the Langfuse dataset item. Previously criteria lived only in Astro's DB and
the item metadata always held an empty array.

## Design

New `PUT /api/v1/deployments/:id/dataset/judgments/:trace_id/criteria`, body
`{ "criteria": [{ "dimension_key": "accuracy", "value": 1 }, ...] }`:

- Values are passed through and stored as given (not derived from the verdict); any value in `[-1, 1]` is
  accepted, so a future LLM judge can write partial scores. Empty array clears.
- `ReplaceReasons` replaces the reason rows in one transaction (verdict read `FOR UPDATE`), returning the
  previous set for rollback; it shares its `DELETE … RETURNING` + `INSERT` body with `SetVerdictAndReasons`.
- `upsertJudgmentDatasetItem` now serializes the real `judgment_criteria`, which also fixes the PATCH
  count-failure path that previously dropped criteria from the item.
- Errors: bad/duplicate key or out-of-range value → 400, missing judgment → 404, `unknown` verdict → 409.

## Migration

None. `judgment_criteria` populates as judgments are re-saved through the new endpoint.
