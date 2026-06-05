# Rename `agent.ai_gateway` spec field to `agent.astro_ai_gateway`

## Summary

The spec opt-in for the AI Gateway is renamed from `ai_gateway` to
`astro_ai_gateway` in `astropods.yml`. The new name makes the field
unambiguous in a spec that may grow other gateway-shaped concepts and
makes it obvious at a glance that this is the Astro-managed gateway,
not a generic "ai gateway" toggle.

## Design

One field, one rename. The YAML/JSON tag on `AstroSpec.Agent.AIGateway`
and `DeploymentAgent.AIGateway` changes; the Go struct field name
(`AIGateway`) is unchanged, so no call sites move.

```yaml
agent:
  image: my-agent:latest
  astro_ai_gateway: true
```

What stays the same — these were intentionally **not** renamed:

- DB tables (`account_ai_gateway`, `deployment_ai_gateway`,
  `account_ai_gateway_dev_keys`) — would require migrations
- Env vars (`AI_GATEWAY_URL`, `AI_GATEWAY_MASTER_KEY`) — would require
  helm/terraform changes
- Go package `internal/aigateway/`
- Terraform files (`ai_gateway.tf`, `ai_gateway_waf.tf`) and the
  `ai_gateway` Postgres role

Scope was limited to the user-facing spec surface: the field name in
`astropods.yml`, the JSON schema, validator error paths, and the
comments / test description strings that reference it.

## Migration

**Agent authors.** Rename the field in `astropods.yml`:

```diff
 agent:
   image: my-agent:latest
-  ai_gateway: true
+  astro_ai_gateway: true
```

No backward-compatibility shim — the old key is no longer recognized
by the spec parser. The validator error path is now
`agent.astro_ai_gateway`.

**Operators.** Nothing to do; env vars, DB tables, and infra are
unchanged.
