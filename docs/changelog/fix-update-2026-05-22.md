## Summary

The "Upgrade to newest build" modal on deployment cards navigated using ephemeral router state (`autoConfigureNewBuild`) that was consumed by a component deleted in the agent detail page redesign. Confirming the upgrade had no effect.

## Design

`handleUpgradeConfirm` now navigates directly to `/{account}/agents/{deploymentId}/configure?build={latestBuildId}`. `AgentConfigure` already reads the `?build=` param and enters upgrade mode, so no changes were needed there. The `deploymentDetailHref` prop (previously required to compute the upgrade target) was removed — the path is now derived from `account` and `deploymentId` props that are always present.

## Migration

No action required.
