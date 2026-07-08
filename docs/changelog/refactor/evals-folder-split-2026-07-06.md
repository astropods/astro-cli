# Refactor evals folder into feature subfolders

## Summary

The agent-detail `evals/` folder had grown into a flat directory mixing two
distinct features — the **review queue** (label traces one at a time) and the
**dataset** (table of graded items) — with a 1200-line `ReviewQueueView.tsx`
holding the container, list, detail pane, verdict controls, and a dozen helpers
in one module. This made the surface hard to navigate and the main component hard
to reason about. This change reorganizes the folder by feature and decomposes the
monolith into focused files, with no behavior change.

## Design

The folder now separates by feature, with files used by both views kept at the
root:

```
evals/
  (shared)       EvalTabCard, judgment-criteria, useJudgmentCriteriaSelection,
                 review-queue-motion, CriterionLabels
  review-queue/  ReviewQueueView, ReviewQueueList, ReviewQueueDetail,
                 ReviewQueueVerdictControls, review-queue-utils,
                 JudgmentCriteriaPanel, QuickUndoToast
  dataset/       DatasetView, DatasetTable, DatasetItemRow, DatasetGradeSidebar,
                 DatasetGrade, DatasetFilterChips, DatasetRowActionsMenu
```

`ReviewQueueView` is now a container that owns only queue state, mutation wiring,
and layout. Its former internals split by responsibility:

- **`ReviewQueueList`** — sidebar list, rows, load-more, sentiment dots.
- **`ReviewQueueDetail`** — detail pane, empty state, trace link, position nav.
- **`ReviewQueueVerdictControls`** — verdict buttons and keyboard shortcuts.
- **`review-queue-utils`** — pure helpers (baseline status, trace-entry mapping,
  adjacency, page-index lookup, id truncation).

Moves used `git mv` to preserve history. Only import paths changed for consumers:
`AgentDataset.tsx` and two Storybook stories point at the new subfolder paths.

## Migration

None. Pure structural refactor — no API, prop, or behavior changes.
