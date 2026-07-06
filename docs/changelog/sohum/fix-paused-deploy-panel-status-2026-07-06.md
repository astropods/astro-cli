## Summary

Paused deployment surfaces now show consistent paused status during and after pause. Previously, the graph respected the deployment pause state only after the deployment record caught up, so a pause could briefly empty runtime containers while the record still looked running and workload tiles/panels flashed as starting.

## Design

The deployments page continues to compute paused state from the deployment record and now passes that state to the workload details panel. The panel applies the same status precedence as workload tiles: deployment-level paused state overrides per-workload runtime status.

The pause mutation now seeds paused detail, list, and status caches immediately after the server acknowledges the stop. Runtime reads still refetch, but they no longer create a temporary running-plus-empty-workloads state in the UI.

## Migration

No user action required.
