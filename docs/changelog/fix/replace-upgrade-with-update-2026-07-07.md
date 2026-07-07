# fix: use "Update" instead of "Upgrade" for build-update actions

## Summary

The web client used "Upgrade" in a few places for moving a deployed agent to a newer build. "Upgrade" reads like a paid upsell, while the rest of the product (the deployed agent card) already says "Update". This aligns the terminology so the same action is named the same way everywhere. Closes #1552.

## Design

Replaced the user-facing "Upgrade" copy with "Update" in the three surfaces where it appeared: the deployment-history new-build nudge, the blueprint sidebar per-instance action and its tooltip, and the configure-page build-update banner and its footer message. Unit and Playwright tests that assert on that visible text were updated to match.

Scope was kept to user-facing copy. Internal identifiers (for example the `UpgradeNudge` component and the `isUpgrade` flag) and test descriptions were left unchanged, and no validation or behavior changed.

## Migration

None. This is a copy-only change.
