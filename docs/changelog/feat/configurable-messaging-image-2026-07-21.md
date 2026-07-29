## Summary

The messaging sidecar image was hardcoded to `astropods/messaging:latest` at deployment-template generation, identical across every environment. Because preview and prod share one account-level ECR pull-through cache (`dockerhub/astropods/messaging:latest`), both track the same mutable upstream tag: prod could silently pick up a new `latest` on any pod cycle, and there was no way to pin or update prod independently of preview.

This makes the messaging image configurable per environment and adds a controlled way to pin prod to an immutable digest — so prod only changes when someone deliberately promotes, while preview keeps tracking `latest`. It requires no infra (Terraform/Helm) changes; the pin lives in a repo-tracked config file.

## Design

**Config knob.** `GenerateDeploymentTemplate` reads the messaging image from `TemplateInput.MessagingImage`, sourced from a new `MESSAGING_IMAGE` env var via `DeploymentConfig.MessagingImage`. Empty falls back to `defaultMessagingImage` (`astropods/messaging:latest`), the single source of truth in the deployment package, so behavior is unchanged unless the var is set. The value is a bare Docker Hub ref (tag or digest); it flows through the existing `resolveImage` rewrite to the same `{ecrHost}/dockerhub/...` pull-through path, and digest refs (`...@sha256:...`) survive intact. The `admingrpc` template-repair path constructs `TemplateInput` without deployment config, so leaving the field empty there keeps it on the default automatically.

**Per-environment wiring, no infra change.** `MESSAGING_IMAGE` is not in the astro-server chart's explicit `env:` block; it rides the existing `envFrom: configMapRef` that imports the `astro-server-config` ConfigMap. `deploy-prod.yml` builds that ConfigMap from `config/astro-server/prod.env`. So the pin is a single line in `config/astro-server/prod.env`; `preview.env` omits the key and therefore tracks `latest`. This PR pins prod to the current `astropods/messaging:latest` digest (`@sha256:a8aacdd8...`) — the same image content prod runs today, now addressed immutably so it no longer drifts.

**Promotion automation.** `promote-messaging-prod.yml` (manual `workflow_dispatch`) resolves the immutable manifest digest of the chosen `astropods/messaging` tag (default `:latest`) via `docker buildx imagetools inspect`, rewrites the `MESSAGING_IMAGE` line in `prod.env`, and opens a PR (per repo git policy; modeled on `bump-cli-on-chat-change.yml`). Merging then running **Deploy (Prod)** with astro-server selected rolls the server and applies the ConfigMap.

Because the image is baked into each deployment spec at generation time, a new pin applies to deployments generated afterward; existing agents pick it up on their next redeploy — the same propagation model as before.

The collector sidecar follows the identical `resolveImage` pattern and could be made configurable and pinned the same way; left out here to keep scope on messaging.

## Migration

No action required. Prod is pinned to the digest of the current `latest`, so it runs the same image bytes it does today; preview still tracks `latest`. To move prod to a newer image later, run **Promote messaging to prod**, merge the PR it opens, and deploy.
