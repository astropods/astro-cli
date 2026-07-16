## Summary

Deploy and Configure previously chose a blueprint build implicitly. Users could update to the latest build or roll back to a prior deployment, but could not deliberately deploy an arbitrary published build that had never been deployed before.

This change addresses [issue #1536](https://github.com/orgs/astropods/projects/1/views/2?pane=issue&itemId=208262330&issue=astropods%7Castro%7C1536).

## Design

Deploy and Configure now share a blueprint-version picker backed by the blueprint's published versions. GitHub-backed builds show their commit title and abbreviated SHA, while other builds fall back to version and build metadata. Explicit Latest and Current markers identify those states, and the existing Agents-page Update available badge calls out a newer build. Users can pick any readable, published blueprint build—older or newer than the currently deployed version—even if that build has never been deployed before. Selecting a version loads that exact build through the existing deployment-template flow; the page's existing Deploy or Redeploy action applies it.

Configure keeps an unsubmitted build choice local to the active tab. Direct build links are consumed as one-time handoffs, so refreshing the page or returning to Configure defaults the picker to the agent's currently deployed build.

Switching builds keeps the existing Deploy or Configure form mounted while the next deployment template resolves. The picker shows progress and the existing fields remain visible but temporarily inactive, then update together once the new build's values are ready. If the requested build cannot load, the page identifies the stale fields inline and offers a return to its default build.

Redeploy shows progress for the complete template-finalization and scheduling request. Configure navigates to the Deployments tab only after the server accepts the revision; validation and scheduling failures remain inline so the user's form values and specific error details stay available.

The agent detail layout still carries the server-provided `latest_build_id` into Configure when the detail response omits it. That field remains authoritative for Latest/update state and preserves cross-account private-blueprint suppression. Readable blueprint versions provide the selectable history and commit context; the deployed build is synthesized as a Current option if it is no longer present in that history.

## Migration

No user action is required. Existing deployment and rollback links continue to work, and users can now choose any readable published build before deploying or redeploying.
