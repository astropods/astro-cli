# Multi-Model Support for Ollama

**Date:** 2026-03-07
**Spec Version:** v1.0 → v1.1

## Summary

Previously each model entry supported a single model via `model: <string>`. A single ollama instance can serve many models, but users had to create separate entries for each one — duplicating provider config, GPU settings, and resource definitions.

The new `models` field accepts an array of model identifiers, allowing one ollama entry to pull and serve multiple models from a single instance.

## Design

### Spec Field

```yaml
models:
  llm:
    provider: ollama
    models: [llama3.2, mistral, deepseek-r1]
```

`models` and the deprecated `model` are mutually exclusive. The parser rejects specs that set both. A `ResolvedModels()` method on the `Model` type normalizes both fields into a `[]string`, providing a single code path for all consumers.

### Environment Variable Injection

`{EnvPrefix}_MODEL` (e.g. `OLLAMA_MODEL`) is injected as a comma-separated list. A single model produces the same value as before — no breaking change for agents that read this var.

```
OLLAMA_MODEL=llama3.2,mistral,deepseek-r1
```

### Model Pulling

The platform pulls models sequentially at container startup. Each model gets its own `ollama pull` command chained with `&&`:

```sh
ollama pull llama3.2 && ollama pull mistral && ollama pull deepseek-r1
```

This applies to both the server-side PostStart lifecycle hook (k8s) and the CLI's local dev compose entrypoint.

### Healthchecks

Readiness probes verify all models are available before marking the container as ready. Each model gets a `grep -q` check against `ollama list`, joined with `&&`:

```sh
ollama list | grep -q 'llama3.2' && ollama list | grep -q 'mistral'
```

This replaces the previous single-model grep. When no models are specified, the healthcheck falls back to the provider's default health path.

### Backward Compatibility

The deprecated `model` field is still parsed and handled transparently. `ResolvedModels()` wraps it as a single-element slice, so all downstream code works without branching. The JSON schema accepts both fields. The parser emits a validation error only if both are set simultaneously.

### Scaffold & TUI

The CLI scaffold template and the `astro add model` TUI now emit `models: [...]` instead of `model: ...`. The scaffold config itself keeps a single `Model string` since the creation wizard selects one model at a time.

## Migration

No migration required. Existing specs using `model` continue to work. To adopt:

```yaml
# before
model: llama3.2

# after
models: [llama3.2]
```
