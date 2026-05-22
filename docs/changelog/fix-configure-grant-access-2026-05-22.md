# Fix: configure pane shows stale grants after redeploy

## Summary

After updating access grants in the configure pane and redeploying, the configure pane continued to show the old grants. The actual runtime access control was correct — only the UI display was stale.

## Design

The `PostDeploymentTemplate` handler maintains an in-memory `TemplateCache` (5-minute TTL) keyed by `account:sourceAccount:agent:build:deploymentID:revision`. On a cache hit it skips `generateTemplate` (an expensive registry fetch) and only runs `ShapeTemplate`. Auth grants are merged into the cached template at miss time via `mergeAuthorizationFromStore` — so a hit returns whatever grants were live when the entry was first populated, not the current state.

The fix makes `TemplateCache` shared between `PostDeploymentTemplate` and `DeployAgent`. After the deployment transaction commits successfully, `DeployAgent` calls `TemplateCache.DeleteByDeploymentID`, which scans the cache and evicts any entry whose key contains the deployment ID. The next configure-pane prefill is a cache miss and re-fetches live state from the authorization store.

`TemplateCache` is now constructed once in `main.go` and passed to both handlers. Tests that wired these handlers directly pass `nil` for the cache (which is already guarded against).

## Migration

No action required.
