# Remove Ollama / self-hosted model providers

## Summary

Ollama is no longer supported. It was the only built-in *model* provider that deployed a sidecar container (a "self-hosted model provider"); every other built-in model provider (`anthropic`, `openai`, `google`, `gemini`, `cohere`) is cloud and injects credentials only. Removing Ollama therefore retires the entire "provider-mode model container" path across the spec, server, and CLI. This is the first step in reworking the `models:` spec around the Astro AI Gateway.

## Design

The models surface now has exactly two shapes:

- **Provider mode** — `provider: <cloud>` (or a custom provider). Cloud providers inject `{PROVIDER}_API_KEY` credentials; no container is deployed.
- **Container mode** — `container: {image, port, gpu, ...}`. The user supplies a self-hosted inference server. The platform deploys it as a Deployment and wires `MODEL_<NAME>_HOST/PORT/URL` into the agent environment.

Consequences of Ollama being the sole self-hosted model provider:

- `Model.DeploysContainer()` is now true only for container mode. `Model.ResolvedContainer()` returns the user's container config, or a zero value for provider mode.
- The provider registry's model-container machinery is gone: the `ollama` entry, the `BuiltinProvider` `GPU`/`NodeSelector`/`Tolerations` fields, the `Toleration` type, `GetModelProvider`, and `ConnectionAddress.BaseURL`. Model providers no longer inject `{ENV_PREFIX}_HOST/PORT/URL/BASE_URL/MODEL`.
- Server: model deployment collapses to container mode. The persistent-model StatefulSet path (with hardcoded `ollama list`/`ollama pull` readiness and GPU auto-enable from the registry) is removed; all model sidecars are Deployments. Container-mode GPU (`container.gpu:`) is unchanged.
- CLI: the native-Ollama dev subsystem (host detection, model pull, RAM checks), the compose `NativeOllama` build option, and the `ast add` Ollama model-picker screen are removed. Knowledge providers (qdrant/redis/postgres/mysql/neo4j) and their self-hosted container machinery are untouched — they still deploy StatefulSets and share the generic provider fields and k8s GPU infra.

## Migration

Specs using `provider: ollama` (or any self-hosted model provider) are no longer valid as a managed provider. To run a self-hosted model, declare it in **container mode**:

```yaml
models:
  llm:
    container:
      image: my-inference-server:latest
      port: 8000
```

The agent then reads `MODEL_LLM_HOST` / `MODEL_LLM_PORT` / `MODEL_LLM_URL`. Cloud model providers and the Astro AI Gateway (`agent.astro_ai_gateway: true`) are unaffected.
