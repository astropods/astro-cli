# Spec-driven Slack config and reaction filtering

## Summary

Promotes Slack adapter configuration from hardcoded values to a generic, spec-driven system. Adds UI support for configuring actionable emoji reactions at deploy time. Suppresses infrastructure error noise in Slack channels. Includes local dev fixes for Docker Desktop image resolution.

## Changes

### Slack configuration refactoring

**`SlackConfig` split** — Messaging module's `config.go` splits the monolithic `SlackConfig` into `SlackCredentials` (secrets: bot token, app token) and `SlackAdapterConfig` (public config: actionable reactions). The sidecar now receives adapter config via a `SLACK_CONFIG` JSON env var, keeping the credential flow unchanged.

**Spec promotion** — `actionable_reactions` is declared under `interfaces.messaging.slack` in `astropods.yml`. The CLI's compose builder serializes the full `SlackAdapterConfig` into `SLACK_CONFIG` for `ast dev`, and the server's deployment template emits `SLACK_ACTIONABLE_REACTIONS` as a comma-separated env var for production K8s deployments.

**Server-owned variable** — `SLACK_ACTIONABLE_REACTIONS` is registered as a server-owned variable in the deployment spec, preventing consumers from accidentally removing it during redeploys.

### UI changes

**Reactions text field** — The deploy form presents `actionable_reactions` as a simple comma-separated text input under the Slack toggle, rather than exposing the full generic config structure. Default values from the agent's `astropods.yml` pre-fill the field.

**Credentials vs config distinction** — `useDeployForm.ts` now explicitly separates adapter secret credentials (`SLACK_BOT_TOKEN`, `SLACK_APP_TOKEN`) from public adapter config (`SLACK_ACTIONABLE_REACTIONS`), making the conceptual boundary clear in client code.

### Error suppression

**`ErrNoAgentStream` sentinel** — Replaces scattered `fmt.Errorf` calls in the gRPC server. The Slack adapter's `sendErrorMessage` checks `errors.Is` against this sentinel and suppresses it to logs only, keeping infrastructure noise out of Slack channels. User-facing errors still post normally.

### Local dev fixes

**Image resolution** — `resolveImage` in `template.go` skips the ECR dockerhub pull-through cache prefix when `Environment == "local"`, so images like `qdrant/qdrant:latest` resolve correctly on Docker Desktop.

**Pull policy** — `imagePullPolicyForMode` returns `IfNotPresent` for local mode, allowing locally-built images to be used while still pulling third-party images on first use.

### Tests

- `TestResolveImage_LocalEnvironmentSkipsDockerHubRewrite`, `_StillResolvesTenantImages`, `_ProdEnvironmentStillRewritesDockerHub`
- `TestImagePullPolicyForMode`
- Fixed `deployableSpec` test fixture to include `SLACK_ACTIONABLE_REACTIONS`

## Migration

Agents that want emoji reactions must add the structured Slack config:

```yaml
dev:
  interfaces:
    messaging:
      adapters: [slack, web]
      slack:
        actionable_reactions: [ticket]
```

Agents using the legacy `interfaces: [slack, web]` format are unaffected — they simply won't receive any reaction events.
