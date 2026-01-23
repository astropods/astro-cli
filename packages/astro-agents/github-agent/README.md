# GitHub Agent Example

This is an example agent that can fetch and analyze GitHub repositories using the Astro framework.

## Overview

This agent demonstrates how to:
- Create an HTTP-based agent (no messaging platform required)
- Use astro-workflows for GitHub integration
- Package and deploy an agent using `astro.yml`

## Prerequisites

- Anthropic API Key (`ANTHROPIC_API_KEY`)

## Configuration

The agent is configured via `astro.yml`:
- **Runtime**: Builds from Dockerfile in this directory
- **Interface**: Uses `http` for direct API access
- **Inference**: Listens on port 8080

## Building

From this directory:

```bash
astro build
```

This will:
1. Build the agent container image from this directory (self-contained)
2. Package as an OCI artifact

Note: Since this uses `http` interface (not `astro-messaging`), no sidecar or Redis is required.

The agent builds from its own directory - no monorepo dependencies needed.

## Running Locally

1. Install dependencies:

```bash
bun install
```

2. Set up environment variables:

```bash
export ANTHROPIC_API_KEY=your-api-key
```

3. Run the agent:

```bash
bun run dev
```

Or with Docker:

```bash
docker build -t github-agent .
docker run -p 8080:8080 -e ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY github-agent
```

## Usage

Send a POST request to `/chat`:

```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Can you fetch the README from facebook/react?"}'
```

Check health:

```bash
curl http://localhost:8080/health
```

## Project Structure

```
github-agent/
├── astro.yml           # Agent configuration
├── Dockerfile          # Container image definition
├── README.md           # This file
└── src/
    └── index.ts        # GitHub agent server implementation
```

## How It Works

1. The agent listens for HTTP POST requests on `/chat`
2. Messages are processed using Claude (Anthropic API)
3. The agent can fetch GitHub README files using the workflow tool
4. Responses are returned as JSON

## Customization

To customize this agent:
1. Edit `src/index.ts` to change agent behavior
2. Add more tools from astro-workflows
3. Update `astro.yml` to modify configuration
4. Rebuild with `astro build`
