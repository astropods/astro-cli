## Summary

The Configure tab's action footer only appeared once a user edited the form (or arrived via a rollback/upgrade link), so redeploying an agent with its existing configuration required making a throwaway change first. This makes the Redeploy action always available: users can re-apply the current configuration at any time, directly from the Configure tab.

## Design

The floating footer on the Configure tab is now persistently mounted instead of being conditionally rendered on `form.isDirty || hasOverride`. The button defaults to Redeploy and only swaps to Save for a name-only edit (`form.nameChanged && !form.deployChanged && !hasOverride`). Submitting in the clean state runs the same two-step deploy the upgrade/rollback flow already used (`form.deploy()` → deployment-template POST → `/api/v1/deploy`), re-applying the current spec — no server changes were needed since that path already supported redeploy-without-edits.

The footer message adapts to context (pending changes, name-only rename, rollback/upgrade, or clean "redeploy current configuration"), and the Discard button is hidden when there is nothing to discard. Because the footer no longer unmounts, the `AnimatePresence`/`exit` transitions were removed; the one-time mount animation is preserved.

## Migration

No action required. The Redeploy button is now always visible on a deployment's Configure tab.
