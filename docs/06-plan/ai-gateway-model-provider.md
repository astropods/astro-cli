# AI Gateway as a Model Provider — deploy-time model selection

## Summary

Make the Astro AI Gateway a first-class **model provider** in the `astropods.yml` `models:` block, and add **deploy-time model selection**. Today the Gateway is a lone boolean `agent.astro_ai_gateway: true` that injects `ASTRO_GATEWAY_URL` + `ASTRO_GATEWAY_API_KEY`; the agent hard-codes the model. This change lets an agent declare Gateway model **options** and lets the deployer **pick one** at deploy time, injected via the existing `MODEL_<NAME>` env convention.

- Declare options: `models.<name>.provider: gateway` with a `models: [...]` list.
- At deploy, the astro-client UI (and server) present a dropdown; the user picks one.
- The choice is injected as `MODEL_<NAME>`; endpoint/auth come from the stable, shared `ASTRO_GATEWAY_URL` + `ASTRO_GATEWAY_API_KEY`.
- `agent.astro_ai_gateway` is **deprecated** (kept working) in favor of `provider: gateway`.

The model list is a menu of **deploy-time options**, NOT a hard allow-list. v1 leaves the Bifrost virtual-key grant permissive (`allowed_models: ["*"]`); no Bifrost client change.

## Spec shape

```yaml
models:
  default:
    provider: gateway
    models: [claude-sonnet-4-6, gpt-4o, claude-haiku-4-5]   # selectable options
```

- `gateway` is a reserved, special model provider — neither a cloud-credential provider nor a container provider. It is not added to the builtin provider registry; a helper `IsGatewayModelProvider(name)` recognizes it.
- Model semantics: `IsProviderMode()` true, `DeploysContainer()` false (no sidecar, no `MODEL_<NAME>_HOST/PORT/URL` connection wiring).
- A gateway entry with an empty `models` list is enable-only: stable `ASTRO_GATEWAY_*` injected, no selector, agent hard-codes the model.

## Env injection

- **Stable, shared** — whenever *any* model uses `provider: gateway`: `ASTRO_GATEWAY_URL` + `ASTRO_GATEWAY_API_KEY`. These never vary.
- **Varying, per entry** — the deploy-time selected model id is injected as `MODEL_<SANITIZE(name)>`, the same prefix container-mode models use for `MODEL_<NAME>_HOST/PORT/URL`. Multiple gateway entries fall out naturally: each is its own selector and its own `MODEL_<NAME>` (e.g. `MODEL_WORKHORSE`, `MODEL_FAST`).

Agent code reads the endpoint from `ASTRO_GATEWAY_URL` and the model id from `MODEL_<NAME>` (e.g. `MODEL_DEFAULT`).

## Deploy-time selection (reuses existing machinery)

"Declare options → pick one at deploy → inject the choice as an env var" is exactly a `display-as: select` Input with `options`, which already flows end-to-end:

- Select dropdown UI — `apps/astro-client/src/components/deploy/VariableField.tsx:155`
- Value collection / payload — `TemplateRequest.variables` (`apps/astro-client/src/lib/api.ts:440`), deploy (`api.ts:2394`)
- Promotion to a UI Variable — `collectVariablesFromInputs` (`apps/astro-server/internal/deployment/template.go:1242`)
- Env injection (§8.4) — `packages/astro-spec/envresolver.go:108`
- Persistence + redeploy prefill — `deployment_build_env` (`apps/astro-server/internal/deploymentstore/normalized.go:744`; read back `GetDeploymentVariables:865`)

For each gateway model entry, the server **generates** a select Variable — `spec.Variable{ Name: "MODEL_"+SanitizeEnvName(name), DisplayAs: "select", Options: entry.models, Default: first, Optional: true, Targets: [agent] }` — which surfaces the dropdown in the deploy UI with no client change. (Model-level inputs are not UI-promoted today, so this is generated as an agent-targeted Variable per `template.go:1242-1279`.) Selection is optional and defaults to the first option.

## Gateway enablement + deprecation

- Derive "uses gateway" = `agent.AIGateway == true` OR any model has `provider: gateway`. Set `DeploymentAgent.AIGateway` from this derived value in `template.go:333`, so the existing mint/injection path (`deployer.go:156`, `spec_applier.go:105`, provisioner, validator) works unchanged — no new deployment-spec field for enablement.
- Deprecate `Container.AIGateway`: keep parsing; update the jsonschema description to "Deprecated: use a `provider: gateway` model"; emit a deprecation notice from `ParseSpec` / CLI validate. `astro_ai_gateway: true` together with a `provider: gateway` model is an error (redundant/ambiguous).
- `ASTRO_GATEWAY_URL` / `ASTRO_GATEWAY_API_KEY` injection is unchanged, now triggered by derived enablement.

## Bifrost (no change in v1)

`AllowedModels: ["*"]`, `bedrock`, `$20/mo` remain hardcoded (`apps/astro-server/internal/aigateway/client.go:78,125`). The options list is selection-only. Constraining `bifrostProviderConfig.AllowedModels` to the declared options is a documented future enhancement (extend `KeyRequest` + the `deployer.go:164` call site).

## Changes by layer

1. **`packages/astro-spec`** — `provider.go` (`IsGatewayModelProvider`, reserved name); `spec.go` (deprecate `AIGateway` doc; gateway-aware `DeploysContainer`/`ResolvedContainer` = no container); `parser.go` (gateway validation, add missing `Default ∈ Options` check for `select`, boolean/gateway conflict, deprecation notice); regenerate `astropods.schema.json`; tests.
2. **`apps/astro-server`** — `internal/deployment/template.go` (derive AIGateway enablement from gateway models; generate the select Variable; exclude gateway entries from `ds.Models`); selected-model injection rides the generated Variable → §8.4 → `spec_applier.go`; `validator.go` gate fires on derived enablement. Template + e2e tests.
3. **`apps/astro-client`** — works via the generated select Variable (existing dropdown). Optional: label/group the model selector in `VariableFields.tsx`. Verify the deploy form renders it.
4. **`apps/astro-cli` + scaffold** — `ast create --model gateway` and scaffold templates emit `models: { default: { provider: gateway, models: [...] } }` instead of `astro_ai_gateway: true`; agent templates read `process.env.MODEL_DEFAULT` (i.e. `MODEL_<entry name>`) + `ASTRO_GATEWAY_URL`. Keep the boolean working with a deprecation warning.
5. **Docs + changelog** — this doc; `docs-public/.../astropods-package-spec.mdx` + the `ai-gateway` page (document `provider: gateway`, options, deploy-time selection, `MODEL_<NAME>` + shared `ASTRO_GATEWAY_*`, boolean deprecation + migration); new changelog file.

## Verification

1. `moon run astro-spec:test astro-spec:typecheck` — schema regen clean; gateway validation + `Default ∈ Options` covered.
2. `moon run astro-server:test astro-server:vet` — template generates the select Variable; enablement derived; gateway entries excluded from `ds.Models`; env asserts `MODEL_<NAME>=<selected>` plus shared `ASTRO_GATEWAY_URL`/`_API_KEY`.
3. Deploy flow: `PostDeploymentTemplate` on a `provider: gateway` spec returns a Variable with `displayAs: select` + options; deploying a chosen value persists a `user_var` row and injects `MODEL_<NAME>`; redeploy prefills the prior choice.
4. astro-client: run the deploy form against such a template; confirm the model dropdown renders and the choice round-trips.
5. Backward-compat: a spec still using `astro_ai_gateway: true` deploys unchanged (URL+KEY injected, no selector) and emits a deprecation warning.

## Migration

Replace the boolean with a Gateway model entry:

```yaml
# before
agent:
  astro_ai_gateway: true

# after
models:
  default:
    provider: gateway
    models: [claude-sonnet-4-6]
```

Agent code moves from a hard-coded model to reading `MODEL_DEFAULT` (endpoint stays `ASTRO_GATEWAY_URL`). The boolean continues to work during deprecation.
