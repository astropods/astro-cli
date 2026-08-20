# Fix: resolve the blueprint owner in the avatar backfill

## Summary

The blueprint avatar backfill warned on every run for the same deployments, and
those deployments never got an avatar:

```
Blueprint avatar backfill: blueprint avatar missing for deployment
deployment=xdq-9tt-40e account=peterdongo name=mindcraft
```

The deployment pass looked up the blueprint avatar under the deploying account.
A blueprint avatar is stored under the account that owns the blueprint, which is
a different account whenever someone deploys another account's public blueprint.
One popular public blueprint therefore produced one warning per account that
deployed it, on every 24h run, forever.

## Design

**Resolve the owner, not the deployer.** The deployment scan now joins accounts
on `COALESCE(d.source_account_id, d.account_id)`. That is the lineage rule the
deploy path and `deploymentstore` already use: `source_account_id` holds the
blueprint owner on a cross-account deploy and is NULL when the deployer owns the
blueprint.

`source_account_id` is `ON DELETE SET NULL`, so a deleted owner account collapses
the expression to the deployer's account instead of dropping the deployment from
the scan.

**Generate when there is nothing to copy.** A deployment outlives its blueprint,
so an archived or deleted blueprint leaves the copy with no source. The pass now
renders a placeholder from `owner/agent-name`, the same seed the blueprint pass
uses, so the deployment lands the image a copy would have produced. The job
converges: the next run sees the avatar and skips the deployment, rather than
re-reporting it.

Placeholders write through a new raw `WriteDeploymentAvatarJPEG`, mirroring the
blueprint pass. The upload path resizes and re-encodes untrusted input at a lower
JPEG quality, which would only degrade an image already generated at the output
size.

## Migration

None. Affected deployments pick up avatars on the next scheduled run.
