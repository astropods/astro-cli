# Slack channel and user allowlists

## Summary

Adds optional `allowed_channel_ids` and `allowed_user_ids` fields to the Slack adapter config, letting agents restrict which Slack channels and users can interact with them. Also fixes a resolver bug where empty non-secret variable references leaked as unresolved `${variables.*}` tokens into container env.

## Design

### Spec and schema

`SlackAdapterConfig` in `astro-spec` gains two new `[]string` fields: `allowed_channel_ids` and `allowed_user_ids`. Both are optional and default to empty (allow all). The JSON schema in `DevInterfaces.JSONSchema()` registers matching array properties so `astropods.yml` validation covers them.

```yaml
dev:
  interfaces:
    messaging:
      adapters: [slack]
      slack:
        allowed_channel_ids: [C123, C999]
        allowed_user_ids: [U123, U999]
```

### Template generation

`GenerateDeploymentTemplate` reads defaults from the spec's `SlackAdapterConfig` via two new helpers (`slackAllowedChannelIDsDefault`, `slackAllowedUserIDsDefault`) and emits `SLACK_ALLOWED_CHANNEL_IDS` and `SLACK_ALLOWED_USER_IDS` as comma-separated, optional, non-secret variables targeting `interface.slack`. `wireInterfaceEnvironment` wires `${variables.*}` refs into `interfaces.environment` so the messaging sidecar receives them.

### CLI compose path

The CLI's compose builder already serializes the full `SlackAdapterConfig` as `SLACK_CONFIG` JSON. The new fields flow through automatically via the struct's JSON tags.

### Client deploy form

`ADAPTER_CONFIG.slack` in `useDeployForm.ts` adds two new optional text inputs (Allowed Channel IDs, Allowed User IDs) with comma-separated placeholders. Mock backend fixtures and e2e specs cover fresh deploy, import, omit-optional, and redeploy flows.

### Resolver fix

`resolveValue` in `spec_resolver.go` previously skipped replacement when `replacement == ""`, causing empty non-secret `${variables.*}` refs to leak as literal strings into container env. The fix introduces a `shouldReplace` flag: non-secret variables always resolve (even to `""`), while empty secret variables remain unresolved intentionally to preserve stripped-spec semantics.

## Migration

No action required. Existing agents without the new fields continue to allow all channels and users. To restrict access, add the fields to `astropods.yml` or fill them in the deploy UI.
