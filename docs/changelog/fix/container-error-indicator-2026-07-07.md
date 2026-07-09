# feat: flag containers that have errors in their logs

## Summary

On the deployments tab a container could be logging errors while still showing a healthy status, and the only way to notice was to open its logs. This adds an error indicator to the container card when its logs contain errors, so a problematic container stands out at a glance. Closes #1434.

## Design

Each live, long-running pod card probes its containers for a recent error-level log, reusing the existing `useLastErrorLog` query (a single tail line at `level=error`). Detection lives in a shared `use-container-log-errors` module so the tile and the detail panel read the same cached lookups. One probe component is rendered per container so the hook count stays stable across renders.

The card shows a small severity icon (error uses the error icon in red; a warning variant uses the warning icon in amber) placed after the age, with an instant tooltip. Because unhealthy pods already surface the last error message, the tile indicator is limited to the non-unhealthy case so there is no duplicate signal.

Clicking a pod that has errors opens its detail panel straight to the Logs tab and surfaces the last error as a banner at the top of the panel, so the user lands on something actionable instead of an empty General tab.

## Migration

None. This is additive.
