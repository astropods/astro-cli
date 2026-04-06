## Summary

After a new-build redeploy, the "To chat, run: `ast playground …`" command in the Deployments tab either showed a placeholder or disappeared permanently, making the chat flow inaccessible.

## Design

**Root cause 1 — server picks the wrong build entry (`deploy.go`):** `enrichDeployment` groups K8s workloads by `agentKey:version` (build ID). During a rolling update, the namespace contains workloads for both the outgoing and incoming build. `GetDeployment` was calling `enrichDeployment(...)[0]`, which picks a random entry from a Go map. When it picked the old build's entry — whose ingress label had already been updated to the new version — `ExternalURLs` was empty, so the client received no URL.

Fix: scan the results for the entry whose `BuildID` matches `dbDep.BuildID` and use that instead of `[0]`.

**Root cause 2 — client stops polling too early (`deployments.ts`):** `refetchInterval` keeps polling when `hasContainerMismatch` is true (some container is not ready). But `hasContainerMismatch` returns false when `workloads` is empty — a transient state during the "between builds" window when old pods are gone and new ones haven't registered yet. With an empty workload list and status `"Running"`, polling stopped and the URL never appeared even after the new pods came up.

Fix: extract a `deploymentNeedsPolling` helper that also returns true when `status === "running"` but `workloads` is empty (`missingWorkloads`). Applied to both `useDeployment` and `useDeploymentSuspense`.

## Migration

No changes required.
