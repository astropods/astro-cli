# PATCH judgment: clear reasons on verdict change

## Summary

Changing a dataset judgment's verdict left its selected judgment criteria (reasons) behind. Reasons
are scoped to a verdict — a `good` reason stores `dimension_value = 1`, a `bad` reason `-1` — so a
`good → bad` flip previously kept stale `+1` rows. Reasons only cascade on a judgment-row delete, never
on a verdict update, so the PATCH path had no way to drop them.

## Design

PATCH stays verdict-only (`{ verdict }`). When the verdict actually changes, the handler now clears the
judgment's reasons and records the previous set so any later failure restores the judgment to its exact
prior state:

- A single store method, `SetVerdictAndReasons`, updates the verdict and (when the verdict changes)
  replaces the reasons in one transaction, returning the previous verdict and the reasons it replaced.
  Because it reads the previous verdict inside the same transaction, the no-op decision and the reason
  clear are atomic and race-free.
- The same method reverses the change on rollback: the compensating closure calls it with the previous
  verdict and the returned reasons, so a failed Langfuse item write or count update restores verdict and
  reasons together — also atomically.
- On a verdict change the Langfuse item is upserted with an empty `judgment_criteria` array. A PATCH that
  resends the current verdict is a no-op: it neither clears reasons nor re-upserts the item, so it cannot
  leave the stored reasons and the Langfuse metadata out of sync.

Criteria are re-submitted through a separate criteria endpoint (out of scope here).

## Migration

None. Existing dataset items without `judgment_criteria` metadata remain valid — the field is treated as
optional on read.
