# Configure page detects resource-config changes

## Summary

On the deploy configure page, editing only advanced provisioning fields (CPU, memory, volume mount, storage size) did not surface the "Save & Redeploy" action bar. The form still sent the new values to the server on an unrelated save, but users had no signal that their resource change required a redeploy — making the change easy to miss or appear ignored.

## Design

Redeploy detection is driven by `useChangeTracking`, which compares an `initial` against a `current` `TrackedFormState` and reports a `requiresRedeploy` flag based on per-field categories (`"cosmetic"` vs `"redeploy"`). The four advanced provisioning fields lived in `useDeployForm`'s state and were threaded into the deployment request, but were never added to `TrackedFormState` — so a change to them simply didn't exist as far as the action bar was concerned.

The fix extends the tracked state with the four resource fields, all categorized as `"redeploy"`:

```ts
agentCpu, agentMemory, agentVolumeMount, agentStorageSize
```

To compare against the deployment's existing values, `DeployFormInitialValues` now carries the same four fields. The seeding effect that reads `templateResponse.provisioning.agent` populates both the live state (as before) and the initial-values snapshot, and `applyValues` resets them so `reset()` keeps working. `ConfigureDeployment` wires the new fields into both `trackedState` and `initialTrackedState`.

Categorizing all four as `"redeploy"` matches reality: changing CPU/memory rolls the pod, and changing volume mount or storage size triggers PVC reconciliation. None of these are cosmetic.

## Migration

None — UI-only change.
