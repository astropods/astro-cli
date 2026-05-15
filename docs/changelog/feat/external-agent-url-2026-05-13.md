# External agent URL injection

## Summary

Agents that serve a frontend often need to know their own public URL — to
generate OAuth callback URLs, signed links, webhooks pointing back at
themselves, etc. Before this change the only platform-injected URL was
`ASTRO_AGENT_URL`, which is the cluster-internal service DNS
(`http://<svc>.<ns>.svc.cluster.local:8080`). Agents had to guess or be
configured with their external URL out-of-band, creating a circular
dependency: you don't know the host until after the deployment is created
with an ingress.

Closes #616.

## Design

When an agent declares an exposed endpoint (`expose.enabled: true` on any
endpoint — what `frontend: true` desugars to) **and** the platform can
resolve a public host for it, the resolver now injects:

```
ASTRO_EXTERNAL_AGENT_URL=https://<host>
```

The host comes from the same selection used to build the ingress:

1. `endpoints.<name>.expose.domain` from the spec (custom domain), else
2. `GenerateIngressHost(agent, namespace, ingressDomain)` (platform default).

When neither is available — local dev without an `INGRESS_DOMAIN`, no
exposed endpoint, etc. — the variable is simply omitted; nothing fabricated.
The existing `ASTRO_AGENT_URL` / `ASTRO_AGENT_HOST` keep their cluster-
internal meaning so sidecars and east-west callers don't get pushed through
the ALB.

Plumbed through `ResolveContext.ExternalAgentHost` (the env-map resolver)
and `ResolveOptions.ExternalAgentHost` (the build-env row resolver) so all
three call sites — the K8s applier, the `deployment_build_env` writer in
the deployer, and the drift-detection paths in `RepairNormalizedSpec` and
`BackfillResolvedKeys` — produce the same key set.

The K8s applier extracts the host once via a small `resolveAgentIngressHost`
helper and reuses it for both env injection and the actual ingress build,
so the URL we inject is byte-identical to the host the ingress will use.

## Migration

None. Existing deployments without an exposed endpoint are unaffected. New
deployments with `frontend: true` (or any exposed endpoint) automatically
gain the new variable on next apply.
