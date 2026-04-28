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
    persistent: true    # data survives restarts

  cache:
    provider: redis

  db:
    provider: postgres
    persistent: true
```

Available providers: `qdrant` (vector search), `redis` (key-value), `postgres` (relational + vector via pgvector), `neo4j` (graph).

Each provider injects connection env vars into the agent: `{PROVIDER}_HOST`, `{PROVIDER}_PORT`, `{PROVIDER}_URL`.

### Custom containers

Use any Docker image as a knowledge store:

```yaml
knowledge:
  db:
    container:
      image: pgvector/pgvector:pg17
      port: 5432
      volume: /var/lib/postgresql/data
    persistent: true
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

- `volume` — where to mount persistent data inside the container. Required with `persistent: true`.
- `inputs` — values injected into the container at runtime. Set via `ast project configure` or defaults. Use `secret: true` for passwords and keys.

Custom containers inject `KNOWLEDGE_{UPPER(name)}_HOST` and `KNOWLEDGE_{UPPER(name)}_PORT` into the agent (e.g. `KNOWLEDGE_DB_HOST`, `KNOWLEDGE_DB_PORT`).

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
