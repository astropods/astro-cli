# fix/deploy-blueprint-from-correct-account — 2026-05-08

## Summary

Deploying a public blueprint owned by a different account looked up vault variables under the **blueprint owner's** account instead of the deploying user's account. The vault picker, validation of `{{vars.*}}` references, and the inline load-error message were all keyed off the wrong account, which surfaced as empty/forbidden variables on the deploy form.

## Design

- **`useDeployForm`** seeds form state from the interactive template response when it loads. That seed previously included `targetAccount: extracted.targetAccount`, where `extracted.targetAccount` is the blueprint's URL/source account. For a fresh public-blueprint deploy this overwrote the user's personal-account default and re-keyed `useAccountVariables(targetAccount)` to the blueprint owner.
- The fix drops the `extracted.targetAccount` fallback in the seeding step. `targetAccount` is now only seeded when an explicit `initialValues.targetAccount` is provided.
- Account locking for flows that must pin to the source account is already handled by `allowedTargetAccounts`:
  - `AgentConfigure` (redeploy) pins `[deployment.account]`.
  - `DeployBlueprint` pins `[blueprint.account]` for **private** blueprints.
- For public blueprint deploys, `allowedTargetAccounts` is unset, so the existing `rawTargetAccount = _targetAccount || personalAccount?.name` derivation correctly defaults the picker — and the vault query — to the deploying user's personal account.

## Migration

None.
