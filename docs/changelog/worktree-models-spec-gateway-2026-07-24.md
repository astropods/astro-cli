# AI Gateway as a model provider (deploy-time model selection)

## Summary

The Astro AI Gateway becomes a first-class model provider in the `models:` block, with deploy-time model selection. This replaces the deprecated `agent.astro_ai_gateway: true` boolean.

```yaml
models:
  default:
    provider: gateway
    models: [claude-sonnet-4-6, gpt-4o]   # selectable options
```

## Design

- **`gateway` is a reserved model provider** — not in the builtin registry, deploys no container, needs no user credentials.
- **Deploy-time selection.** The `models` list is a menu of options (not an allow-list). The server generates a `display-as: select` deployment variable per gateway entry (options = the list, default = first); the deploy form renders the existing dropdown — no new UI. The choice persists per deployment and prefills on redeploy.
- **Env injection.** Whenever any model uses `provider: gateway`, the stable shared `ASTRO_GATEWAY_URL` + `ASTRO_GATEWAY_API_KEY` are injected. The selected model id follows the existing model env convention: `MODEL_<NAME>` (e.g. `MODEL_DEFAULT`).
- **Enablement + deprecation.** "Uses gateway" is derived (`AstroSpec.UsesGateway()`) from a `provider: gateway` model or the deprecated boolean, and drives the existing virtual-key mint / env injection / admission gate unchanged. The boolean still works (enable-only, no selector), is mutually exclusive with a gateway model, and `ast validate` warns on it. The Bifrost grant is unchanged in this pass — the options list drives selection, not gateway-side enforcement.

## Migration

Move gateway agents from the boolean to a gateway model, and read the chosen model from `MODEL_<NAME>`:

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

Agent code reads the endpoint from `ASTRO_GATEWAY_URL` and the model id from `MODEL_DEFAULT`. The boolean continues to work during deprecation.
