# spec-update: Multi-Model Support for Ollama

**Date:** 2026-03-07
**Spec Version:** v1.0 → v1.1

## Summary

Added support for specifying multiple models on a single ollama provider entry. Previously, each model entry could declare at most one model via `model: <string>`. The new `models: [<string>]` field accepts an array, allowing the platform to pull and serve multiple models from a single ollama instance.

The `model` field is deprecated but remains functional for backward compatibility.

## Changes

### Spec (`docs-public/fern/docs/pages/astropods-package-spec.mdx`)

- Bumped spec version to v1.1, added changelog table.
- Added `models` (string array) field to the Models table (Section 4.1).
- Marked `model` as deprecated.
- Updated env var injection (Section 8.2): `{EnvPrefix}_MODEL` is now a comma-separated list.
- Added validation rule 10a: `models` and `model` are mutually exclusive.
- Updated Appendix A.1 and Appendix C example.

### `packages/astro-spec`

- **spec.go** — Added `Models []string` field to `Model` struct. Added `ResolvedModels()` helper that returns `Models` if set, falls back to wrapping deprecated `Model` as a single-element slice. Updated `ResolvedContainer()` to inject comma-separated `_MODEL` env var.
- **parser.go** — Added mutual exclusivity validation for `models` and `model`.
- **envresolver.go** — Updated `_MODEL` env key injection to use `ResolvedModels()` with comma-join.
- **astropods.schema.json** — Added `models` array property to the Model schema definition.

### `apps/astro-server`

- **deployment/template.go** — `buildDeploymentModel()` uses `ResolvedModels()` for the deployment model name, agent env injection, and healthcheck. Healthcheck builds a compound grep (`&&`-joined) to verify all models are pulled.
- **k8s/spec_applier.go** — Healthcheck and PostStart hook split the comma-separated model string and build compound commands: one `grep -q` per model for readiness, one `ollama pull` per model for the pull hook.

### `apps/astro-cli`

- **compose/builder.go** — Healthcheck uses compound grep for all resolved models. Entrypoint pulls each model sequentially. Env var injection uses comma-joined value.
- **cmd/explain.go** — Prints comma-joined model list in the explain output.
- **cmd/repair.go** — Takes first element from `ResolvedModels()` for scaffold config (scaffold expects a single model).
- **tui/add/screens.go** — `buildEntry()` emits `models: [...]` instead of `model: ...` for ollama entries.
- **scaffold/templates/template-ts/astropods.yml** — Template emits `models: [{{.Model}}]`.

## Migration

No migration required. Existing specs using `model: <string>` continue to work via `ResolvedModels()`. Implementations will emit a deprecation warning when `model` is used. To adopt the new field, replace:

```yaml
models:
  llm:
    provider: ollama
    model: llama3.2
```

With:

```yaml
models:
  llm:
    provider: ollama
    models: [llama3.2]
```
