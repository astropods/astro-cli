# Eval-dataset doc updates

## Summary

Updates the eval-dataset docs to match the judgment-flow removal in
`chore-cleanup` and to fix a few pre-existing inaccuracies.

## Design

- `docs/03-architecture/traces-to-eval-dataset.md` now describes only the
  live evaluator flow, with no judgment-flow history.
- `docs/README.md`'s eval-dataset area-map row points at the current design
  spec and lists which older specs are out of date.
- `eval-dataset-v2-spec.md`, `eval-dataset-v2-judgment-reasons-spec.md`, and
  `eval-dataset-v2-judge-signal-spec.md` each get a short "out of date, see X"
  banner.
- `eval-dataset-evaluation-spec.md` gets accuracy fixes: its proposed
  `/datasets/:id/...` routes now each carry the real shipped
  `/deployments/:id/dataset/...` path, and its persistence tables now match
  the actual schema.

## Migration

None. Documentation only.
