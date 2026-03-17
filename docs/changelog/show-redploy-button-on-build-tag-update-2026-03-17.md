# Show redeploy prompt and build-drift badges when a newer agent build exists

## Summary

After running `ast push`, users had no way to know a newer build was available for an already-deployed agent without manually comparing build IDs. This change surfaces build-version drift across the client UI and provides a one-click redeploy path from the configure page.

## Design

A `BuildUpdateBadge` micro-component compares the deployment's current `build_id` against the latest published version from the agent registry and renders a badge showing the version jump (e.g. `New build abc123 → def456`).

- **My Agents cards**: each card fetches account-level agent data (with 15s foreground-only polling) and shows the badge when the deployment is behind the latest build.
- **Deployment detail header**: the same badge appears next to the status indicator so users see drift from the detail view too. A neutral muted badge always shows the current `build_id` below the deploy date for at-a-glance version context.
- **Configure page floating action bar**: when a newer build exists but no config edits have been made, a build-only `Redeploy` button appears with a stacked badge (`New build available` / `old → new`). Editing config fields switches the CTA to `Save & Redeploy` as before.
- **Post-redeploy optimistic update**: the deploy mutation optimistically patches the deployment's `build_id` in the TanStack Query cache using the `DeployResponse`, so badges clear immediately on navigation without waiting for the server to finish reconciling. Deployment list invalidation is deferred to avoid overwriting the optimistic state with stale data.
- **Polling efficiency**: `refetchIntervalInBackground` is explicitly `false` so polling pauses when the tab is not focused.

E2E tests (`build-update-badge.spec.ts`) cover: badge on the agents list card, badge on the detail header, negative case for up-to-date deployments, current-build-id badge presence, build-only redeploy submission, badge clearing after successful redeploy (optimistic cache), action bar transition from build-only to dirty "Save & Redeploy" and back via Cancel, and failed-redeploy error resilience. The mock backend is stateful — deploy responses update the in-memory deployment list, and a `/test/reset` endpoint restores initial state between tests. Playwright is configured with `workers: 1` so all tests run sequentially against the shared mock.

## Migration

No migration required. The feature is purely additive — existing deployments and workflows are unaffected.
