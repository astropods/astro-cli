# Astropods Agent Development Guide

To view the CLI help (installation, quick start, commands) in the terminal, run **`ast docs help`**.

## Quick Start

Build an agent with [Mastra](https://mastra.ai) and connect it to the Astropods messaging with a single call:

```typescript
import { Agent } from "@mastra/core/agent";
import { serve } from "@astropods/adapter-mastra";

const agent = new Agent({
  name: "My Agent",
  model: "anthropic/claude-sonnet-4-5",
  instructions: "You are a helpful assistant.",
});

serve(agent);
```

The adapter connects to the messaging service, registers the agent, and handles incoming messages. Run with `ast project start`.

You don't need to use Mastra, and can choose a different agent framework. In that case, implement the `AgentAdapter` interface from `@astropods/adapter-core` and call `serve(adapter)` explicitly, or use the `@astropods/messaging` SDK directly. The Mastra adapter is the reference implementation.

## Adding Tools

Mastra tools work out of the box. The adapter surfaces them in the playground and sends status updates as tools execute.

```typescript
import { Agent } from "@mastra/core/agent";
import { createTool } from "@mastra/core/tools";
import { z } from "zod";
import { serve } from "@astropods/adapter-mastra";

const weatherTool = createTool({
  id: "weather",
  description: "Get the current weather for a location",
  inputSchema: z.object({ location: z.string() }),
  outputSchema: z.object({ weather: z.string() }),
  execute: async ({ location }) => {
    const res = await fetch(`https://wttr.in/${location}?format=3`);
    return { weather: await res.text() };
  },
});

const agent = new Agent({
  name: "Weather Agent",
  model: "anthropic/claude-sonnet-4-5",
  instructions: "Use the weather tool to answer questions about weather.",
  tools: { weatherTool },
});

serve(agent);
```

## Environment Variables

Variables you need to run your agent, such as your API keys:

| Variable            | Description                         |
| ------------------- | ----------------------------------- |
| `ANTHROPIC_API_KEY` | Required for the examples (Claude). |

Run `ast project configure` to set them. `ast project start` injects them into the agent container.

The platform also auto-injects three variables into the agent container that you don't configure yourself:

| Variable                      | Purpose                                                                                        |
| ----------------------------- | ---------------------------------------------------------------------------------------------- |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP endpoint for agent telemetry (see below). Present in deployed environments only.    |
| `ASTRO_AGENT_NAME`            | Agent name; use as OpenTelemetry `service.name`                                                |
| `ASTRO_AGENT_BUILD`           | Build hash; use as OpenTelemetry `service.version`                                             |

> `ast project start` does not run a collector locally, so `OTEL_EXPORTER_OTLP_ENDPOINT` is absent during local dev. Guard your instrumentation setup on the env var so your agent boots cleanly in both environments.

## Instrumentation

Astropods agents send OpenTelemetry traces to `OTEL_EXPORTER_OTLP_ENDPOINT`, which the platform routes to a per-account observability backend. Tracing the LLM calls, tools, and agent steps lets you see token usage, costs, and tool behavior in one place.

Prefer an off-the-shelf instrumentation library for your stack — they emit richer spans (tool calls, agent steps, cost) than hand-written code and stay in sync with upstream SDK changes. When no library exists for your stack, emit manual spans using the [OpenTelemetry GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/) or [OpenInference conventions](https://github.com/Arize-ai/openinference).

### Mastra

The `template-ts-mastra` scaffold wires this up with `@mastra/observability` + `@mastra/otel-exporter`. Newly-scaffolded agents work out of the box.

### LangChain / LangGraph (Python)

Use Traceloop / OpenLLMetry and point it at the platform endpoint at startup:

```python
import os
from traceloop.sdk import Traceloop

endpoint = os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT")
if endpoint:
    Traceloop.init(
        app_name=os.environ.get("ASTRO_AGENT_NAME", "agent"),
        api_endpoint=endpoint,
    )
```

### Claude Agent SDK (`@anthropic-ai/claude-agent-sdk`)

The Agent SDK talks to the `claude` binary over IPC, so HTTP-level interceptors miss it. Use the dedicated OpenInference instrumentation, which patches `query()` and emits AGENT + TOOL spans with token counts and cost:

```bash
bun add @arizeai/openinference-instrumentation-claude-agent-sdk \
        @opentelemetry/sdk-trace-node @opentelemetry/exporter-trace-otlp-http \
        @opentelemetry/resources
```

```typescript
import { Resource } from '@opentelemetry/resources';
import { NodeTracerProvider, BatchSpanProcessor } from '@opentelemetry/sdk-trace-node';
import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { ClaudeAgentSDKInstrumentation } from '@arizeai/openinference-instrumentation-claude-agent-sdk';
import * as ClaudeAgentSDK from '@anthropic-ai/claude-agent-sdk';

const endpoint = process.env.OTEL_EXPORTER_OTLP_ENDPOINT;
if (endpoint) {
  const provider = new NodeTracerProvider({
    resource: new Resource({
      'service.name': process.env.ASTRO_AGENT_NAME ?? 'agent',
      'service.version': process.env.ASTRO_AGENT_BUILD ?? 'dev',
    }),
  });
  provider.addSpanProcessor(new BatchSpanProcessor(
    new OTLPTraceExporter({ url: `${endpoint}/v1/traces` }),
  ));
  provider.register();
  new ClaudeAgentSDKInstrumentation().manuallyInstrument(ClaudeAgentSDK);

  // Flush buffered spans on shutdown. Adding any listener replaces Node's
  // default terminate behavior, so we must call process.exit ourselves.
  const shutdown = async () => {
    try { await provider.shutdown(); } catch {}
    process.exit(0);
  };
  process.once('SIGTERM', shutdown);
  process.once('SIGINT', shutdown);
}
```

The SDK is ESM-only, so `manuallyInstrument` is required (the auto-registration path won't apply). See the [OpenInference docs](https://github.com/Arize-ai/openinference/tree/main/js/packages/openinference-instrumentation-claude-agent-sdk) for the full setup including how to re-export the instrumented `query`.

### Raw Anthropic / OpenAI SDKs

The standard `@anthropic-ai/sdk` and `openai` packages make direct HTTP calls and work with HTTP-level auto-instrumentation. Use OpenInference (`@arizeai/openinference-instrumentation-{anthropic,openai}`) or Traceloop (`@traceloop/instrumentation-{anthropic,openai}`).

## Project Structure

```
├── agent/index.ts      # Main agent entry point
├── astropods.yml       # Spec (schema: https://astropods.com/schema/package.json)
├── Dockerfile          # Agent container
├── package.json

```

You can optionally add ingestion pipelines. Add an `ingestion/` directory with `Dockerfile` and `index.ts`, then declare each pipeline in `astropods.yml`:

```
├── agent/index.ts
├── ingestion/
│   ├── Dockerfile      # Ingestion container build
│   └── index.ts        # Ingestion entry point

├── astropods.yml       # Add ingestion entries with container.build and trigger
├── Dockerfile
├── package.json
└── .env
```

Example `astropods.yml` ingestion entry:

```yaml
ingestion:
  sync:
    container:
      build:
        context: .
        dockerfile: ingestion/Dockerfile
    trigger:
      type: startup   # or schedule, manual, webhook
```

## Knowledge Stores

Knowledge entries define sidecar databases for your agent. Use built-in providers for zero-config setup, or bring your own container image.

### Built-in providers

```yaml
knowledge:
  vectors:
    provider: qdrant

  cache:
    provider: redis

  db:
    provider: postgres
```

Available providers: `qdrant` (vector search), `redis` (key-value), `postgres` (relational + pgvector), `neo4j` (graph). Provider-mode entries always get a persistent volume — the provider defines its own data path. To get an ephemeral store, use a custom container without a `volume`.

Each provider injects connection env vars into the agent: `{PROVIDER}_HOST`, `{PROVIDER}_PORT`, `{PROVIDER}_URL` (e.g. `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_URL`).

**Connecting from your agent:** the platform generates a random password and a platform-managed user for each postgres deployment, then injects all five credentials into the agent container via secrets: `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`. Always read all five from env vars — never hardcode user or password:

```typescript
const pool = new Pool({
  host: process.env.POSTGRES_HOST ?? 'localhost',
  port: parseInt(process.env.POSTGRES_PORT ?? '5432'),
  database: process.env.POSTGRES_DB ?? 'postgres',
  user: process.env.POSTGRES_USER ?? 'postgres',
  password: process.env.POSTGRES_PASSWORD ?? 'postgres',
});
```

> **Note:** `POSTGRES_URL` is also injected but may not work reliably with all client libraries. Prefer the individual env vars above.

### Custom containers

Use any Docker image as a knowledge store:

```yaml
knowledge:
  db:
    container:
      image: pgvector/pgvector:pg17
      port: 5432
      volume: /var/lib/postgresql/data
    inputs:
      - name: POSTGRES_DB
        datatype: string
        default: my_db
        description: Database name
      - name: POSTGRES_USER
        datatype: string
        default: postgres
      - name: POSTGRES_PASSWORD
        datatype: string
        secret: true
        description: Database superuser password
```

- `volume` — where to mount persistent data inside the container. Setting `volume` makes the entry persistent (a PVC is provisioned on deploy and a named volume in local dev). Omit `volume` for an ephemeral store.
- `inputs` — values injected into the container at runtime. Set via `ast project configure` or defaults. Use `secret: true` for passwords and keys (requires `ast project configure` before starting).

Custom containers inject `KNOWLEDGE_{UPPER(name)}_HOST` and `KNOWLEDGE_{UPPER(name)}_PORT` into the agent (e.g. `KNOWLEDGE_DB_HOST`, `KNOWLEDGE_DB_PORT`).

## Frontend Agents

Some agents serve their own web UI rather than (or in addition to) the messaging protocol. Declare this in `astropods.yml` and ensure your container handles port 80.

### `astropods.yml`

```yaml
spec: blueprint/v1
name: my-frontend-agent

agent:
  build:
    context: .
    dockerfile: Dockerfile
  interfaces:
    frontend: true    # agent serves its own UI
    messaging: false  # no messaging sidecar

dev:
  interfaces:
    frontend:
      port: 80        # port the container listens on locally (default: 80)
```

**Critical:** `interfaces` MUST be nested under `agent:`. Placing it at the top level of the spec is silently ignored — the frontend will not be routed.

In production the platform always routes to **port 80**. Your container must `EXPOSE 80` and start on that port. Use `dev.interfaces.frontend.port` only when your local dev server runs on a different port.

### Production container requirements

The production container filesystem is **read-only**. Any file or directory your process writes to at startup must be pre-created during the Docker build:

```dockerfile
# Pre-create writable paths before the read-only filesystem is applied
RUN touch ./backend/.env && mkdir -p ./backend/.langgraph_api
```

### Running multiple processes (e.g. Next.js + LangGraph backend)

When your agent packages a frontend and a backend in the same container, use a `start.sh` script to launch both:

```bash
#!/bin/bash
set -e

# Start backend in the background
(cd /app/backend && node_modules/.bin/langgraphjs dev --no-browser --host 0.0.0.0) &

# Start frontend in the foreground (keeps the container alive)
cd /app/frontend && exec node node_modules/.bin/next start
```

```dockerfile
ENV PORT=80
ENV NODE_ENV=production
EXPOSE 80

# Pre-create any files the backend writes at startup
RUN touch ./backend/.env && mkdir -p ./backend/.langgraph_api

COPY start.sh ./
RUN chmod +x start.sh
CMD ["./start.sh"]
```

Key points:
- The frontend must be the foreground process (`exec`) so the container exits when it does.
- Bind the backend to `0.0.0.0` (not `localhost`) so the frontend can reach it on IPv4.
- Server-side code reads `process.env` at runtime so the platform can inject env vars. Next.js `NEXT_PUBLIC_` vars are baked into the client bundle at build time — set them before `npm run build` in the Dockerfile.

## Packages

| Package                     | Purpose                                                              |
| --------------------------- | -------------------------------------------------------------------- |
| `@mastra/core`              | LLM agent with tools and memory                                      |
| `@astropods/adapter-mastra` | Connects Mastra agent to Astropods messaging                         |
| `@astropods/messaging`      | Messaging SDK for direct gRPC connection (when not using an adapter) |

Install: `bun add @mastra/core @astropods/adapter-mastra`

## Development

```bash
ast project start   # Start agent and messaging
ast project logs    # Tail logs
```

Open the playground at http://localhost:3100 to chat with your agent.
