# Account-rooted authorization resource registration

## Summary

New organization Accounts, Blueprints, and Deployments now project into WorkOS as one Account-rooted resource tree. Astro records the desired resource state durably, completes WorkOS writes asynchronously, and exposes synchronization state in Queen without enforcing the new private-by-default policy yet.

## Design

`authorization_resource_sync` is the generic lifecycle ledger for all three resource types. An Account is registered beneath the WorkOS-required organization root; its Blueprints and Deployments are registered beneath that Account. Creation intent, Deployment renames and undeploys, and Blueprint archives update the ledger in the same Astro transaction. River jobs carry the full organization and resource key, apply the newest version to WorkOS, and retry failures. Job uniqueness uses resource type plus external ID so immediate and sweep reconciliation cannot run concurrently for one resource.

New Blueprints receive an immutable UUID for their WorkOS external ID and record their creator. After registration converges, creators receive `account-admin`, `blueprint-admin`, or `deployment-admin` as appropriate. A missing membership mirror delays only the creator assignment: successful resource registration and its WorkOS ID are persisted before the role retry.

Resource types own their permission and role definitions in WorkOS. Registration creates instances and scoped Admin assignments; it does not copy the Viewer, Writer, Maintainer, or Admin permission bundles onto each resource.

Historical Deployments keep using `deployment_fga_sync` until PR4 migrates them. New Deployments use only the generic ledger, and existing discovery and enforcement reads accept either ledger during this transition. Queen resolves Account, Blueprint, and Deployment sync state from the generic ledger and continues to show assignments from WorkOS.

## Migration

Apply the schema before deploying the binary. Apply the WorkOS model in `scripts/workos-fga/model.json` before testing; PR3 uses its Account, Blueprint, and Deployment types and their Admin roles. Variable, Audience, Insights, and Knowledge Store registration remains later work. No access enforcement changes in this PR.
