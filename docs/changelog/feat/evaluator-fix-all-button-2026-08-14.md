## Summary

The evaluators panel on astro-queen's Deployments page (#1989) only fixed
drifted deployments one at a time. With more than a handful drifted, an
operator had to click "Fix" once per row.

## Design

astro-queen only — no backend change. The drift list for an evaluator gets a
"Fix all" button that calls the existing `FixDeploymentDrift` RPC once per
currently-listed row, sequentially, showing progress (`Fixing… n/total`). A
row that fails doesn't stop the run; the next `ListEvaluatorDrift` refresh
still shows it as `drifted`/`fix_failed` with its real detail, since each
call re-checks and persists the actual post-fix state server-side.

## Migration

None.
