# Direct WorkOS resource lifecycle

## Summary

Organization resources now follow one direct WorkOS authorization lifecycle without a second Astro sync ledger. Account is the effective parent for supported product resources; Organization remains only the WorkOS-required root.

## Design

One shared lifecycle contract wraps WorkOS create, read, update, and delete calls. Account creation registers the Account and its singleton Insights resource. Existing creation paths register Blueprints, Deployments, Variables, and Knowledge Stores beneath that Account. Product rename and removal paths update or delete the corresponding resource. The Audience creation API currently landing separately uses the same contract.

Astro remains the product source of truth. A WorkOS failure is logged without rolling back the successful local create; PR4 compares Astro with WorkOS and backfills any missing resources. Blueprints receive an immutable UUID because their current database key includes the mutable name.

Deployment now uses the same direct lifecycle as every other resource. Its sync store, River worker, retry sweep, and creator assignment are removed. FGA discovery is account-scoped when the existing experiment is enabled and no longer depends on a Deployment ledger row.

## Migration

Apply the nullable `agents.uid` column before deploying the server. Configure the Account-rooted resource types in WorkOS first. Preview and production keep `FGA_ENFORCEMENT_ENABLED=false` while registration and backfill populate WorkOS; shadow comparisons remain non-blocking. Keep the existing `deployment_fga_sync` table through the rolling deployment; it is unused by the new binary and can be dropped in the final cleanup.
