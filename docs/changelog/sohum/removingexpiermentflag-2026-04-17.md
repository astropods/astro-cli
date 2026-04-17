## Summary

GitHub auto-build was behind an experiment flag that users had to manually enable in Settings. This removes the flag and makes the GitHub connection panel available to all blueprint owners by default.

## Design

- Removed `githubAutoBuild` from the `Experiments` interface and its default value.
- Removed the "GitHub Connection" toggle row from the Experiments settings page.
- Removed the `experiments.githubAutoBuild` guard in `BlueprintDetailSidebar` — the panel now renders for any user with edit access (`canEdit`), which is already gated to authenticated owners.
- Removed `setExperiment("githubAutoBuild", true)` side-effect calls in `GitHubConnectionPanel` and `NewBlueprint` (no longer needed).
- Removed the beaker/experimental badge from the GitHub sidebar section title.

No changes to the underlying auto-build logic, webhook handling, or API.

## Migration

No action required. Users who had the flag disabled will now see the GitHub panel automatically.
