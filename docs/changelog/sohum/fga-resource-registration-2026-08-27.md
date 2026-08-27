# Direct WorkOS resource registration

## Summary

New organization resources now register directly in the Account-rooted WorkOS authorization tree. This PR only creates resource records; it does not add authorization enforcement, a synchronization ledger, or a new background worker.

## Design

One shared registrar wraps the WorkOS SDK and treats an existing external ID as success. Account creation registers the Account and its singleton Insights resource. Existing creation paths register Blueprints, Deployments, Variables, and Knowledge Stores beneath that Account. The Audience resource contract is included for the Audience creation API currently landing separately.

Astro remains the product source of truth. A WorkOS failure is logged without rolling back the successful local create; PR4 compares Astro with WorkOS and backfills any missing resources. Blueprints receive an immutable UUID because their current database key includes the mutable name. Existing Deployment lifecycle synchronization remains unchanged.

## Migration

Apply the nullable `agents.uid` column before deploying the server. Configure the Account-rooted resource types in WorkOS first. No access behavior changes in this PR.
