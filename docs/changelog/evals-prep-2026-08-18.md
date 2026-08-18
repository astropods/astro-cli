# Eval dataset prep cleanup

## Summary

The eval-dataset evaluation rollout adds roughly six endpoints, five tables, and a UI
cutover across the next several PRs. This change clears the ground they land on: dead
grade code, a 1429-line handler file that every later step would rewrite a slice of, a
duplicated auth preamble, and Langfuse item identity locked inside private handler
helpers. No behavior changes.

## Design

Five steps, one commit each:

1. Delete the dataset grade helpers and the `grade`, `next_grade`,
   `next_grade_progress`, `cases_to_next_grade`, `good_count`, and `bad_count` fields
   from the summary response. Nothing read them. `item_count` stays, so the underlying
   counter columns stay too until dataset items replace them.
2. Add `resolveDeploymentAccess`, which authenticates the caller, loads the deployment
   named by `:id`, and checks account membership. `resolveLangfuseContext` and the
   network handlers' `resolveDeploymentContext` now build on it, and the dataset
   handlers that hand-rolled the same block call it directly.
3. Move the Langfuse dataset item identity and compensation into `evaldataset` as
   `ItemID`, `UpsertItem`, and `DeleteItem`, with metadata supplied by the caller.
   Verdict-specific logic stays in the judgment handlers, so `evaldataset` gains no
   dependency on the legacy judgment types. `loadDatasetEnsured` folds in the
   legacy-dataset-name heal that admission also needs.
4. Rename the review-queue cursor's filter fields to `Filter`, `LocalTime`, and
   `LocalTrace`. The JSON tags stay as they are, so cursors already in flight keep
   decoding.
5. Split `handlers/dataset.go` into summary, review-queue, items, and judgments files,
   with the test file split to match. The judgment handlers and their tests now sit in
   files that legacy cleanup can delete outright.

`ItemID` reproduces the previous hash exactly, including the zero-byte separator between
dataset name and trace ID. A changed hash would orphan every existing Langfuse item, so a
golden-value test pins it.

## Migration

None.
