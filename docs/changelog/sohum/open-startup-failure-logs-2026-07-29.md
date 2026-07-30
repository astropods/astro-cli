## Summary

Failed agent startups left users on the deployment graph, requiring them to identify the failed workload and investigate it manually. Startup failures now open the relevant workload's diagnostics automatically.

## Design

The deployment status response remains the source of truth for the startup failure state and preferred workload. Once it reports an error, the deployment page uses the same live workload status calculation as the pod tiles, then matches the server's failed workload list by workload name or component. Only workloads that resolve to an error state qualify; if no reported workload matches, the first errored pod is used as a fallback. The panel opens on General so container state and startup diagnostics remain visible regardless of whether logs exist; Logs remain available for manual investigation. Probing and paused workloads retain those higher-priority states and do not auto-open, and no panel opens when no pods are errored.

Automatic opening is guarded per deployment build and failure. If a pod panel is already open when the failure surfaces, the current selection and tab are preserved and that failure is treated as handled. Each workload panel owns its tab state and starts on General, while later user navigation remains in control. Closing the panel or changing tabs remains a user-controlled choice; the same failure does not repeatedly reopen the panel. A later successful transition resets the guard so a new startup failure can surface diagnostics again.

## Migration

No action required.
