# Server pull-through — pass tenant references through, inject the pull secret

## Summary

Second half of registry pull-through (see `docs/01-spec/registry-pull-through-spec.md`). The server now (a) **passes tenant image references through unchanged** — the pushed `registry.<domain>/{account}/{image}` is already the pull URL — and (b) injects the **image-pull credential** into each tenant namespace, so pods pull through `astro-registry` (which fronts ECR) rather than authenticating to ECR directly. All name→repo mapping lives in the registry (separate change), so the control plane does no image parsing or rewriting.

## Design

- **No image rewriting.** The deployment template returns tenant references (those on the proxy registry host) unchanged; the old ECR-URL rewrite, the `{env}-tenant-` prefix, and `ECRNamespace` are removed from the server. Public images (ECR pull-through cache) and third-party images are still handled. The applier's `ImageResolver` is a passthrough.
- **Pull-secret injection.** On every apply, before pods are created, the applier writes a `dockerconfigjson` Secret (`astro-registry-pull`) for the proxy host into the tenant namespace from `REGISTRY_PULL_CREDENTIAL` (the CPC, delivered via External Secrets) and links it to the namespace `default` ServiceAccount's `imagePullSecrets`. All tenant pods use the default SA, so they pick it up automatically — no pod-spec changes. No-op when the credential/proxy host are unset (local dev).
- **Unconditional.** No runtime flag; rollback during the proving phase is redeploying the prior server build.

## Deploy order

Deploy **after** the registry accepts the CPC and resolves namespaces (`PRIMARY_PULL_KEY_HASH` + namespace resolution live) and `REGISTRY_PULL_CREDENTIAL` is wired to astro-server (astro-infra). Otherwise pods hit `ImagePullBackOff`. Roll out on the primary cluster first and bake before registering additional clusters.

## Migration

No user action; developer push and the WorkOS flow are unchanged.
