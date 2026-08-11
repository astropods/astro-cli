## Summary

Deployment template generation used to rewrite bare Docker Hub image references (e.g. `redis:7-alpine`) to an explicit ECR pull-through cache path (`{ecrHost}/dockerhub/library/redis:7-alpine`) so tenant pods pulled through ECR instead of Docker Hub directly. That routing is now handled by infra (containerd registry mirror config redirecting Docker Hub pulls to the ECR pull-through cache), so the control-plane rewrite is redundant and has been removed.

## Design

- `resolveImage` — the function that performed the rewrite — is deleted; it had degenerated into an identity function once the rewrite logic was removed, called from 6 sites for no behavioral benefit.
- `resolveBuiltImage` now returns the image (explicit or synthesized from the `{agent}-{kind}-{name}` convention for `container.build` components) directly, instead of piping it through `resolveImage`.
- The two other former call sites (the observability collector image and the messaging sidecar image) now use their string values directly.
- Tenant images (hosted on the tenant's proxy registry) and third-party images (explicit registry host) were already passed through unchanged, so this only changes bare Docker Hub references — they now appear in the deployment spec exactly as authored in `astropods.yml`.

## Migration

None required. Deployment specs generated after this change carry unrewritten Docker Hub image references; infra's registry mirror config is responsible for routing those pulls through the ECR pull-through cache.
