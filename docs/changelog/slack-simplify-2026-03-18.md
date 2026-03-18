# Consolidate Slack adapter variables into SLACK_CONFIG

## Summary

Replaces three individual Slack deployment variables (`SLACK_ACTIONABLE_REACTIONS`, `SLACK_ALLOWED_CHANNEL_IDS`, `SLACK_ALLOWED_USER_IDS`) with a single `SLACK_CONFIG` JSON variable. The server now silently strips unknown submitted variables instead of rejecting them, preventing opaque deploy failures when agent specs rename or consolidate variables between builds. The client separates real submission variables from UI-only display fields so virtual keys never leak into the deploy payload.

## Design

### Server: EnforceEditable strips unknown variables

`EnforceEditable` in `deployment_editable.go` previously returned validation errors for submitted variables not present in the template. It now deletes them from the submitted map instead. This is a general-purpose fix: any future variable rename or consolidation will degrade gracefully rather than blocking deploys.

### Server: Template generation

`GenerateDeploymentTemplate` emits a single `SLACK_CONFIG` variable containing the JSON-serialized `SlackAdapterConfig` (reactions, channel IDs, user IDs). The three individual variables and their `wireInterfaceEnvironment` refs are removed.

### Client: Variable/display field separation

`useDeployForm.ts` splits the former `allAdapterFieldDefs` into two structures:
- `adapterVariableDefs` — real template variables sent to the server (excludes virtual Slack keys and `SLACK_CONFIG` itself).
- `adapterDisplayFields` — all fields rendered in the UI, including the three virtual Slack config inputs.

`fulfillTemplate` iterates `adapterVariableDefs` when building the submission payload, so virtual keys are never injected. `DeployFormFields` and other UI consumers use `adapterDisplayFields`.

### Client: SLACK_CONFIG serialization

`slackConfig.ts` handles the serialize/deserialize round-trip between the `SLACK_CONFIG` JSON string and the three virtual form fields. `extractInitialValues.ts` deserializes `SLACK_CONFIG` into virtual fields on form load.

### E2E test infrastructure

- Mock backend templates updated to use `SLACK_CONFIG` instead of individual variables.
- Playwright ports moved from 8787/4317 to 48787/44317 to avoid collisions with dev servers and OTel collectors.
- `install-slack-template.spec.ts` renamed to `slack-adapter.spec.ts` and organized into `test.describe("deploy page")` and `test.describe("configure page")` blocks.

### Test coverage

- **Unit**: `deployment_editable_test.go` — three tests for the strip behavior (extra vars stripped, mixed known/unknown, all unknown).
- **Handler integration**: `deploy_test.go` — submits legacy vars alongside `SLACK_CONFIG`, verifies only template-defined vars are persisted via sqlmock expectations.
- **K8s E2E**: `k8s_test.go` — strips stale vars via `EnforceEditable`, applies to a real cluster, verifies only `SLACK_CONFIG` appears on the messaging sidecar.
- **Frontend E2E**: `slack-adapter.spec.ts` — 9 tests covering fresh deploy, optional field omission, spec-default round-trip, bulk import, configure/redeploy, overlapping targets, and server error UX.

## Migration

No action required for agents whose deployed Slack config matches their `astropods.yml` defaults — the first redeploy will work seamlessly. Agents where users customized Slack config values at deploy time (beyond what's in the spec) will see those customizations reset to spec defaults on the next configure page load; users must re-enter them before redeploying.
