# `ast create --model gateway`

## Summary

Scaffolding a new agent previously assumed a bring-your-own-key model: the
generated project declared an `anthropic`/`openai` provider and the next-steps
banner told the user to run `configure` and paste in an API key. That is
friction for anyone who just wants to start building against the Astro AI
Gateway, which provides managed model access with no provider account.

This adds `gateway` as an opt-in value for the existing `--model` flag on
`ast create`. Choosing it scaffolds an agent already wired to the gateway —
no provider key required.

## Design

`gateway` is a new branch in the same flag the CLI already validates and
tab-completes; the default (anthropic BYOK) is unchanged, so existing behavior
is untouched.

When selected, the choice flips a single `AIGateway` flag on the scaffold
config, which then drives every template through the existing conditional
rendering path:

- **Spec** — emits `agent.astro_ai_gateway: true` and omits the provider
  `models:` block, so no provider credential is expected at deploy time.
- **Agent source** — both templates target the gateway's OpenAI-compatible
  API by reading the injected `ASTRO_GATEWAY_URL` / `ASTRO_GATEWAY_API_KEY`
  env vars. Mastra gets an `@ai-sdk/openai` provider (pinned `^2.0.0`, since
  v1 emits an AI SDK v4 model that Mastra's `stream()` rejects); LangChain
  points `ChatOpenAI` at the gateway base URL.
- **Next steps** — the post-create banner replaces "set your API keys" with
  `ast login`, since the gateway is account-scoped rather than key-scoped.

Because the gateway env vars are injected by the deployer rather than derived
from a provider in the spec, they are not part of the spec's auto-env set; the
scaffold surfaces them explicitly so the generated source comments still list
what the container receives.

The `--model ollama[/<model>]` shortcut is dropped from `ast create` — its
hard-coded model list went stale and the curated-list/tab-completion UX never
worked well. Ollama remains a fully supported provider everywhere else
(`ast add`, `ast dev` native mode, compose); scaffold without `--model` and
add it with `ast add`. The scaffold engine keeps its generic self-hosted
model-provider support unchanged.

## Migration

None for the gateway feature. The default model choice is unchanged and
`gateway` is purely additive. Existing projects can opt in by adding
`astro_ai_gateway: true` to `astropods.yml` (see the [AI Gateway
guide](/ai-gateway)). `ast create --model ollama[/...]` is no longer
accepted — use `ast add` to attach an Ollama model after scaffolding.
