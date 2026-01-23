# Engineering Assistant Agent

A knowledge assistant agent that answers engineering questions using RAG (Retrieval Augmented Generation) with self-hosted components and cloud integrations.

## Features

- **Self-hosted components:**
  - Sentence-transformers embedding model (all-MiniLM-L6-v2)
  - Qdrant vector database for documentation search
  - Redis for response caching

- **Cloud integrations:**
  - Anthropic Claude for LLM inference
  - GitHub for documentation ingestion
  - Tavily for web search

- **Custom tools:**
  - Document search function (Python)

- **Interfaces:**
  - Slack messaging
  - HTTP API

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Engineering Assistant                 │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │   Flask      │  │   Anthropic  │  │   Custom     │ │
│  │   Server     │  │   Claude     │  │   Tools      │ │
│  └──────────────┘  └──────────────┘  └──────────────┘ │
│         │                 │                  │          │
│         └─────────────────┴──────────────────┘          │
│                          │                               │
└──────────────────────────┼───────────────────────────────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
    ┌────▼────┐      ┌────▼────┐      ┌────▼────┐
    │ Qdrant  │      │  Redis  │      │ GitHub  │
    │ Vector  │      │  Cache  │      │   API   │
    │   DB    │      │         │      │         │
    └─────────┘      └─────────┘      └─────────┘
    (self-hosted)    (self-hosted)    (integration)
```

## Quick Start

### Local Development

1. **Copy environment file:**
   ```bash
   cp .env.example .env
   ```

2. **Add your credentials to `.env`:**
   ```bash
   ANTHROPIC_API_KEY=your-key-here
   GITHUB_TOKEN=your-token-here
   ```

3. **Start the agent with Astro CLI:**
   ```bash
   astro dev
   ```

   This will:
   - Spin up Qdrant, Redis containers locally
   - Build and run the agent container
   - Watch for code changes and hot-reload
   - Inject credentials from .env

4. **Test the agent:**
   ```bash
   curl -X POST http://localhost:8080/message \
     -H "Content-Type: application/json" \
     -d '{
       "content": "How do I use the Anthropic SDK?",
       "user_id": "test-user"
     }'
   ```

### Build and Publish

1. **Build all images:**
   ```bash
   astro build --tag v1.0.0
   ```

   Builds:
   - `engineering-assistant:v1.0.0` (agent)
   - `engineering-assistant-embedder:v1.0.0` (sentence-transformers)
   - `engineering-assistant-docs:v1.0.0` (Qdrant)
   - `engineering-assistant-cache:v1.0.0` (Redis)

2. **Publish to registry:**
   ```bash
   astro publish --registry ghcr.io/yourorg --tag v1.0.0
   ```

## Configuration

The agent is configured via `astro.yml` using the Astro spec format.

### Self-hosted Components

**Models:**
- `embedder`: Sentence-transformers model for embedding documents/queries

**Knowledge:**
- `docs`: Qdrant vector database storing embedded documentation
- `cache`: Redis for caching responses

**Tools:**
- `doc_search`: Custom Python function for searching documentation

### Integrations

**Models:**
- `primary_llm`: Anthropic Claude (requires API key)

**Tools:**
- `github`: GitHub API for doc ingestion (OAuth with repo:read scope)
- `web_search`: Tavily search API (requires API key)

### Injections

**docs_sync:**
- Source: GitHub repo (anthropics/anthropic-sdk-python)
- Trigger: Every 6 hours
- Pipeline: Extract → Chunk → Embed → Upsert to Qdrant

## API Reference

### POST /message

Process a message and generate a response.

**Request:**
```json
{
  "content": "How do I stream responses from Claude?",
  "user_id": "user123",
  "channel_id": "C123456"
}
```

**Response:**
```json
{
  "content": "To stream responses from Claude, use the streaming=True parameter...",
  "cached": false
}
```

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-18T10:30:00Z"
}
```

## Development

### Project Structure

```
engineering-assistant/
├── astro.yml           # Astro spec
├── Dockerfile          # Agent container build
├── requirements.txt    # Python dependencies
├── .env.example       # Environment template
├── src/
│   └── server.py      # Main agent server
└── tools/
    └── search.py      # Custom tool implementation
```

### Adding New Tools

1. Create tool function in `tools/`:
   ```python
   def my_tool(arg: str) -> dict:
       """Tool description"""
       return {"result": "..."}
   ```

2. Add to `astro.yml`:
   ```yaml
   tools:
     my_tool:
       type: function
       config:
         runtime: python
         handler: tools/my_tool.py
   ```

3. Agent can now use the tool

## Testing

Run tests locally:
```bash
# Unit tests
python -m pytest tests/

# Integration tests (requires running components)
astro dev &
python -m pytest tests/integration/
```

## Deployment

The agent is deployed via the Astro platform:

1. Push to registry: `astro publish`
2. Platform reads spec from OCI artifact
3. Provisions self-hosted components (Qdrant, Redis, embedding model)
4. Prompts user to authenticate integrations (Anthropic, GitHub, Tavily)
5. Deploys agent with injected credentials

## Troubleshooting

**Qdrant connection errors:**
- Check `QDRANT_HOST` and `QDRANT_PORT` in .env
- Verify Qdrant container is running: `docker ps | grep qdrant`

**Redis connection errors:**
- Check `REDIS_HOST` and `REDIS_PORT` in .env
- Verify Redis container is running: `docker ps | grep redis`

**Anthropic API errors:**
- Verify `ANTHROPIC_API_KEY` is set correctly
- Check API key has sufficient credits

## License

MIT
