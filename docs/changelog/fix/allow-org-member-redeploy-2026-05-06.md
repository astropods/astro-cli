# Configure-page redeploy uses the deployment's account, not the viewer's personal account

## Summary

Hitting **Save & Redeploy** from the configure tab of a deployment owned by an organization rejected with `404 source agent not found` whenever the source blueprint was private. Initial deploys from the blueprint page worked because that page already pinned `target.account` for private blueprints; the configure page did not, and the form silently fell back to the viewer's personal account name. The server then saw `source.account = <org>` (private) with `target.account = <personal>`, treated it as a cross-account deploy, and `canDeploySourceAgent` 404'd it.

## Design

Two complementary fixes — one root-cause, one semantic guard — so the bug class is hard to reintroduce.

### Root cause: dropped `targetAccount` in the seeding path

`useDeployForm` initializes `_targetAccount` from `iv?.targetAccount ?? ""` and the derived `rawTargetAccount` falls back to `personalAccount?.name` when state is empty. The post-template seeding effect builds a `merged` view that includes `targetAccount: extracted.targetAccount` (the URL/owning account), but `applyValues(merged)` only fanned out `deployName`, variables, adapters, schedules, and webAuthEnabled — it never called `setTargetAccount`. Result: `_targetAccount` stayed `""` and the form's effective target stayed pinned to `personalAccount.name` for the lifetime of the page.

`applyValues` now also seeds `_targetAccount` (`if (v.targetAccount !== undefined) setTargetAccount(v.targetAccount)`), so any caller passing `iv.targetAccount` *or* relying on the template-derived `extracted.targetAccount` gets it applied to state, not just to `initialValues` (which only powers change detection).

### Semantic guard: redeploy is account-locked

`AgentConfigure.tsx` now passes `allowedTargetAccounts: [account]` into `useDeployForm`. The configure page hides the account picker and the server's in-place update path keys off `deployment_id` (it does not mutate `account_id`), so the form should never even consider another target. With `allowedTargetAccounts` set, `useDeployForm`'s existing fallback (`selectableAccounts[0]?.name ?? ""`) keeps `targetAccount` pinned to the URL account regardless of how `_targetAccount` is initialized — making the seeding bug above non-fatal even if a future regression reintroduces it.

### Why this only bit organization deployments

For deployments living in the viewer's personal account, `personalAccount.name` happened to equal the URL account, so the dropped seed was invisible. For organization deployments, the two diverged and `target.account` shipped as the personal account name. The server's `prepareDeployment` then ran `canDeploySourceAgent(orgAcct, personalAcct, agent)` where the IDs differ; private blueprints fail that check and the response is 404 `source agent not found`. Public blueprints would *not* have surfaced a 404 here — they'd have been accepted and would have written the wrong `target.account` into `deployment_spec_json`, a quieter form of the same bug.

## Tests

A new regression in `AgentConfigure.test.tsx` simulates a viewer who is a member of both a personal (`mattcolozzo`) and an organization (`astropods`) account, viewing the configure page for a deployment in the org. The test asserts that the `POST /api/v1/deploy` payload carries `source.account = astropods` and `target.account = astropods`. It fails on `main` with `expected 'mattcolozzo' to be 'astropods'` and passes after the fix.

## Migration

No migration required. No API or schema changes.
