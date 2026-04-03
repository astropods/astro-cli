## Summary

UI polish pass on the Deployments tab and configure/rollback panel, plus two correctness fixes for the rollback flow.

## Design

**Deployments tab visual polish**
- Current deployment row: left accent bar is now status-colored (teal for active, yellow for deploying/restarting, coral for pausing/failed) via `inset box-shadow` instead of a static `border-l-primary`, eliminating the double-border caused by the card's own border.
- History sub-rows: revision number (`#1`, `#2`…) moves to the left of the type badge with a fixed `w-8` span so the badge left edge aligns with service name text in the card above.
- History header stats reformat from `[N] configs · [N] builds` (floated far right) to `Configs [N] · Builds [N]` at 12px from the "HISTORY" label, matching the Services label pattern.
- "Config change" badge color changes from amber to blue, reserving warm colors for error/rollback states.
- "Services" label is no longer forced uppercase.

**Rollback configure panel**
- Context banner height changed to `py-[13.5px]` so its bottom border aligns with the Monitor/Deployments tab bar border.
- `ConfigurePanelLoaded` is keyed on `revisionOverride` so the form fully remounts (resetting all `useState`) when switching to a historical revision — fixes stale form data on rollback.

**Bug fixes**
- `useDeployAgent` now invalidates `deploymentKeys.detail(deployment_id)` on success, so the agent name in the header updates immediately after redeploy without a page refresh.
- `GetPrefilledDeploymentTemplate` now resolves the requested revision *before* calling `generateTemplate`, using the revision's own `build_id` instead of the current deployment's `build_id`. Previously rollback would generate a template from the wrong build and only overlay old config settings on top of it.
- When a revision is requested, `display_name` is now restored from `storedSpec.Target.DisplayName` so the old agent name pre-populates the rollback form.

## Migration

No migration required.
