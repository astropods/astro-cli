## Summary

GitHub auto-builds were generating deployment templates with the wrong agent image path, causing pods to fail to pull (`ImagePullBackOff`). The image was missing the `{env}-tenant-{accountID}/` ECR path segment.

## Design

**Root cause** — `GenerateDeploymentTemplate` had a fallback that constructed the agent image from `{registry}/{name}:{buildID}` when the spec had no image. GitHub builds registered the spec without setting the image, so the fallback produced a bare path instead of the full ECR tenant path.

**Fix** — The GitHub build worker now sets `agent.image` in the spec before registering, using the proxy registry format (`{proxyHost}/{accountID}/{name}:{buildID}`). This goes through the same `resolveImage` transformation as a CLI `ast push`, producing the correct `{registry}/{env}-tenant-{accountID}/{name}:{buildID}` ECR path.

The fallback in `GenerateDeploymentTemplate` is removed. If the spec has no agent image, template generation now fails with an explicit error rather than silently producing a wrong path.

## Migration

No action required. Existing deployments are unaffected. New GitHub auto-builds will generate correct ECR image paths going forward.
