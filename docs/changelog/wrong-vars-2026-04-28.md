# Seed slack `anyone` grant at deploy time

## Summary

Slack-enabled deployments were ending up with an empty `anyone_adapters` claim in the signed deploy token, leaving the slack channel unreachable: every request hit the per-call authorize endpoint with no matching grant and was denied. Observed in messaging-container logs as `"anyone_adapters":null` even when `slack` was in `interfaces.adapters`.

## Design

The fresh-deploy seed (`seedFreshAuthGrants`) only fires in the template-prefill path and only when *both* the web and slack grant blocks are empty. Two real paths bypass it:

- **CLI-direct deploy** — the CLI doesn't go through `PostDeploymentTemplate`, so nothing seeds defaults.
- **Web-grants-set, slack-grants-missing template** — once the user (or their `astropods.yml`) populates web grants, the seed bails out and never touches slack.

Either way the deploy proceeds, the grants table gets persisted from the submitted spec (no slack rows), and the signed token's `anyone_adapters` claim ends up empty. The container's fast-path skips slack and the per-request authorize call denies — slack is dead on arrival.

Add `ensureSlackAnyoneGrant` as a deploy-time invariant in `prepareDeployment`:

- If `slack` is in `interfaces.adapters` and `interfaces.auth.slack.grants` is empty, seed `[{anyone: true}]`.
- Existing slack grants (e.g. an explicit `account_id` scope) are preserved.
- Web grants are not touched.

It runs on both `submittedSpec` (which feeds `buildAuthorizationGrants` → DB) and `resolveResult.Spec` (which feeds the stored revision spec → `Applier.ApplyDeploymentSpec` → token claim) so the persisted grant and the signed token agree. `validateAuthorizationSpec` runs after the seed; `anyone` under slack is already accepted.

## Migration

None. Existing deployments are unaffected until they redeploy. On the next deploy of a slack-enabled agent without an explicit slack grant, an `anyone` grant is seeded automatically — matching the same default the template path already produces for fresh deploys.
